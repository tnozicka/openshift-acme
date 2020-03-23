package openshift_acme_controller

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	kvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog"

	routeclientset "github.com/openshift/client-go/route/clientset/versioned"

	"github.com/tnozicka/openshift-acme/pkg/api"
	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	cmdutil "github.com/tnozicka/openshift-acme/pkg/cmd/util"
	acmeissuer "github.com/tnozicka/openshift-acme/pkg/controller/issuer/acme"
	routecontroller "github.com/tnozicka/openshift-acme/pkg/controller/route"
	kubeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/kube"
	routeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/route"
	"github.com/tnozicka/openshift-acme/pkg/signals"
)

type Options struct {
	genericclioptions.IOStreams

	Annotation                  string
	Workers                     int
	Kubeconfig                  string
	ControllerNamespace         string
	LeaderelectionLeaseDuration time.Duration
	LeaderelectionRenewDeadline time.Duration
	LeaderelectionRetryPeriod   time.Duration
	CertOrderBackoffInitial     time.Duration
	CertOrderBackoffMax         time.Duration
	Namespaces                  []string
	AcmeOrderTimeout            time.Duration

	ExposerImage string

	restConfig  *restclient.Config
	kubeClient  kubernetes.Interface
	routeClient routeclientset.Interface
}

func NewOptions(streams genericclioptions.IOStreams) *Options {
	return &Options{
		IOStreams:  streams,
		Workers:    10,
		Kubeconfig: "",

		LeaderelectionLeaseDuration: 60 * time.Second,
		LeaderelectionRenewDeadline: 35 * time.Second,
		LeaderelectionRetryPeriod:   10 * time.Second,
		CertOrderBackoffInitial:     5 * time.Minute,
		CertOrderBackoffMax:         24 * time.Hour,

		Annotation:       api.DefaultTlsAcmeAnnotation,
		AcmeOrderTimeout: 15 * time.Minute,

		ExposerImage: "",

		Namespaces: []string{metav1.NamespaceAll},
	}
}

func NewOpenshiftAcmeControllerCommand(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewOptions(streams)

	// Parent command to which all subcommands are added.
	rootCmd := &cobra.Command{
		Use:   "openshift-acme-controller",
		Short: "openshift-acme-controller is a controller for Kubernetes (and OpenShift) which will obtain SSL certificates from ACME provider (like \"Let's Encrypt\")",
		Long:  "openshift-acme-controller is a controller for Kubernetes (and OpenShift) which will obtain SSL certificates from ACME provider (like \"Let's Encrypt\")\n\nFind more information at https://github.com/tnozicka/openshift-acme",
		RunE: func(cmd *cobra.Command, args []string) error {
			defer klog.Flush()

			err := o.Validate()
			if err != nil {
				return err
			}

			err = o.Complete()
			if err != nil {
				return err
			}

			err = o.Run(cmd, streams)
			if err != nil {
				return err
			}

			return nil
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := cmdutil.ReadFlagsFromEnv("OPENSHIFT_ACME_CONTROLLER_", cmd)
			if err != nil {
				return err
			}

			return nil
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	rootCmd.PersistentFlags().StringVarP(&o.Annotation, "annotation", "", o.Annotation, "The annotation marking Routes this controller should manage.")
	rootCmd.PersistentFlags().IntVarP(&o.Workers, "workers", "", o.Workers, "Number of workers to run")
	rootCmd.PersistentFlags().StringVarP(&o.Kubeconfig, "kubeconfig", "", o.Kubeconfig, "Path to the kubeconfig file")
	rootCmd.PersistentFlags().StringVarP(&o.ControllerNamespace, "controller-namespace", "", o.ControllerNamespace, "Namespace where the controller is running. Autodetected if run inside a cluster.")
	rootCmd.PersistentFlags().StringArrayVarP(&o.Namespaces, "namespace", "n", o.Namespaces, "Restricts controller to namespace(s). If not specified controller watches all namespaces.")

	rootCmd.PersistentFlags().DurationVar(&o.LeaderelectionLeaseDuration, "leaderelection-lease-duration", o.LeaderelectionLeaseDuration, "LeaseDuration is the duration that non-leader candidates will wait to force acquire leadership.")
	rootCmd.PersistentFlags().DurationVar(&o.LeaderelectionRenewDeadline, "leaderelection-renew-deadline", o.LeaderelectionRenewDeadline, "RenewDeadline is the duration that the acting master will retry refreshing leadership before giving up.")
	rootCmd.PersistentFlags().DurationVar(&o.LeaderelectionRetryPeriod, "leaderelection-retry-period", o.LeaderelectionRetryPeriod, "RetryPeriod is the duration the LeaderElector clients should wait between tries of actions.")

	rootCmd.PersistentFlags().DurationVar(&o.CertOrderBackoffInitial, "cert-order-backoff-initial", o.CertOrderBackoffInitial, "Initial value for the exponential backoff guarding retrying failed orders.")
	rootCmd.PersistentFlags().DurationVar(&o.CertOrderBackoffMax, "cert-order-backoff-max", o.CertOrderBackoffMax, "The upper limit for for the exponential backoff guarding retrying failed orders.")

	rootCmd.PersistentFlags().StringVarP(&o.ExposerImage, "exposer-image", "", o.ExposerImage, "Image to use for exposing tokens for http based validation. (In standard configuration this contains openshift-acme-exposer binary, but the API is generic.)")

	cmdutil.InstallKlog(rootCmd)

	return rootCmd
}

func (o *Options) Validate() error {
	var errs []error

	for _, namespace := range o.Namespaces {
		if namespace == metav1.NamespaceAll {
			continue
		}
		errStrings := kvalidation.ValidateNamespaceName(namespace, false)
		if len(errStrings) > 0 {
			errs = append(errs, fmt.Errorf("invalid namespace %q: %s", namespace, strings.Join(errStrings, ", ")))
		}
	}
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}

	if len(o.ExposerImage) == 0 {
		// Default to env if present
		ei, ok := os.LookupEnv("OPENSHIFT_ACME_EXPOSER_IMAGE")
		if !ok {
			return fmt.Errorf("exposer image not specified")
		}

		if len(ei) == 0 {
			return fmt.Errorf("OPENSHIFT_ACME_EXPOSER_IMAGE contains empty string")
		}

		o.ExposerImage = ei
	}

	// TODO

	return nil
}

func (o *Options) Complete() error {
	var err error

	if len(o.Kubeconfig) != 0 {
		klog.V(1).Infof("Using kubeconfig %q.", o.Kubeconfig)
		o.restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: o.Kubeconfig}, &clientcmd.ConfigOverrides{}).ClientConfig()
		if err != nil {
			return fmt.Errorf("can't create config from kubeConfigPath %q: %w", o.Kubeconfig, err)
		}
	} else {
		klog.V(1).Infof("No kubeconfig specified, using InClusterConfig.")
		o.restConfig, err = restclient.InClusterConfig()
		if err != nil {
			return fmt.Errorf("can't create InClusterConfig: %w", err)
		}
	}

	if len(o.ControllerNamespace) == 0 {
		// Autodetect if running inside a cluster
		bytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			return fmt.Errorf("can't autodetect controller namespace: %w", err)
		}
		o.ControllerNamespace = string(bytes)
	}

	o.kubeClient, err = kubernetes.NewForConfig(o.restConfig)
	if err != nil {
		return fmt.Errorf("can't build kubernetes clientset: %w", err)
	}

	o.routeClient, err = routeclientset.NewForConfig(o.restConfig)
	if err != nil {
		return fmt.Errorf("can't build route clientset: %w", err)
	}

	if len(o.Namespaces) == 0 {
		// empty namespace will lead to creating cluster wide informers
		o.Namespaces = []string{metav1.NamespaceAll}
	} else {
		// We must watch our own namespace for global issuers
		o.Namespaces = append(o.Namespaces, o.ControllerNamespace)

		seen := map[string]struct{}{}
		var uniqueNamespaces []string
		for _, ns := range o.Namespaces {
			_, ok := seen[ns]
			if !ok {
				uniqueNamespaces = append(uniqueNamespaces, ns)
				seen[ns] = struct{}{}
			}
		}
		o.Namespaces = uniqueNamespaces
	}
	klog.V(1).Infof("Managing namespaces: %#v", o.Namespaces)

	return nil
}

func (o *Options) Run(cmd *cobra.Command, streams genericclioptions.IOStreams) error {
	var leWg sync.WaitGroup
	var wg sync.WaitGroup
	leCtx, leCancel := context.WithCancel(context.Background())
	defer func() {
		klog.Info("Waiting for controllers to finish...")
		wg.Wait()

		// Leader election doesn't end gracefully yet, flush just in case
		klog.Flush()

		klog.Info("Waiting for leaded election loop to finish...")
		leCancel()
		leWg.Wait()

		klog.Flush()
	}()

	stopCh := signals.StopChannel()
	ctx, cancel := context.WithCancel(leCtx)
	go func() {
		<-stopCh
		cancel()
	}()

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	// add a uniquifier so that two processes on the same host don't accidentally both become active
	id := hostname + "_" + string(uuid.NewUUID())
	klog.V(4).Infof("Leaderelection ID is %q", id)

	// we use the Lease lock type since edits to Leases are less common
	// and fewer objects in the cluster watch "all Leases".
	lock := &resourcelock.ConfigMapLock{
		ConfigMapMeta: metav1.ObjectMeta{
			Name:      "acme-controller-locks",
			Namespace: o.ControllerNamespace,
		},
		Client: o.kubeClient.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	leChan := make(chan struct{})
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   o.LeaderelectionLeaseDuration,
		RenewDeadline:   o.LeaderelectionRenewDeadline,
		RetryPeriod:     o.LeaderelectionRetryPeriod,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				close(leChan)
			},
			OnStoppedLeading: func() {
				select {
				case <-leCtx.Done():
					// Graceful termination, control loops are already stopped
					klog.Info("leaderelection lock released")

					// fail safe
					time.AfterFunc(3*time.Second, func() {
						klog.Fatalf("Failed to exit in time after releasing leaderelection lock")
					})

				default:
					// Leader election lost
					klog.Fatalf("leaderelection lost")
				}
			},
		},
		Name: "openshift-acme",
	})
	if err != nil {
		return fmt.Errorf("leaderelection failed: %v", err)
	}

	leWg.Add(1)
	go func() {
		defer leWg.Done()
		le.Run(leCtx)
	}()

	select {
	case <-leChan:
		klog.Infof("Acquired leaderelection")
	case <-stopCh:
		klog.Info("Interrupted before leaderelection")
		return nil
	}

	klog.Infof("loglevel is set to %q", cmdutil.GetLoglevel())

	kubeInformersForNamespaces := kubeinformers.NewKubeInformersForNamespaces(o.kubeClient, o.Namespaces)
	routeInformersForNamespaces := routeinformers.NewRouteInformersForNamespaces(o.routeClient, o.Namespaces)

	ac := acmeissuer.NewAccountController(o.kubeClient, kubeInformersForNamespaces)

	rc := routecontroller.NewRouteController(o.Annotation, o.CertOrderBackoffInitial, o.CertOrderBackoffMax, o.ExposerImage, o.ControllerNamespace, o.kubeClient, kubeInformersForNamespaces, o.routeClient, routeInformersForNamespaces)

	kubeInformersForNamespaces.Start(stopCh)
	routeInformersForNamespaces.Start(stopCh)

	wg.Add(1)
	go func() {
		defer wg.Done()
		rc.Run(ctx, o.Workers)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ac.Run(ctx, o.Workers)
	}()

	<-ctx.Done()

	return nil
}
