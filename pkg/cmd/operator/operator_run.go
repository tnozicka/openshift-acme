package operator

import (
	"context"
	"fmt"
	"sync"
	"time"

	routeclientset "github.com/openshift/client-go/route/clientset/versioned"
	"github.com/spf13/cobra"
	operatorclientset "github.com/tnozicka/openshift-acme/pkg/client/acme/clientset/versioned"
	operatorinformers "github.com/tnozicka/openshift-acme/pkg/client/acme/informers/externalversions"
	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	cmdutil "github.com/tnozicka/openshift-acme/pkg/cmd/util"
	"github.com/tnozicka/openshift-acme/pkg/controller/operator/targetcontroller"
	"github.com/tnozicka/openshift-acme/pkg/leaderelection"
	kubeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/kube"
	"github.com/tnozicka/openshift-acme/pkg/signals"
	"github.com/tnozicka/openshift-acme/pkg/version"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

type RunOptions struct {
	genericclioptions.LeaderElection
	genericclioptions.ClientConfig
	genericclioptions.InClusterReflection

	kubeClient     kubernetes.Interface
	routeClient    routeclientset.Interface
	operatorClient operatorclientset.Interface

	OperandNamespace string
	OperandImage     string
}

func NewRunOptions(streams genericclioptions.IOStreams) *RunOptions {
	return &RunOptions{
		LeaderElection:   genericclioptions.NewLeaderElection(),
		OperandNamespace: "",
		OperandImage:     "",
	}
}

func NewRunCommand(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewRunOptions(streams)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the controller.",
		Long:  "Run the controller.",
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

			err = o.Run(streams, cmd.Name())
			if err != nil {
				return err
			}

			return nil
		},

		SilenceErrors: true,
		SilenceUsage:  true,
	}

	o.LeaderElection.AddFlags(cmd)
	o.ClientConfig.AddFlags(cmd)
	o.InClusterReflection.AddFlags(cmd)

	cmd.Flags().StringVarP(&o.OperandNamespace, "operand-namespace", "", o.OperandNamespace, "Namespace for deploying the controller.")
	cmd.Flags().StringVarP(&o.OperandImage, "operand-image", "", o.OperandImage, "Controller image.")

	return cmd
}

func (o *RunOptions) Validate() error {
	var errs []error

	errs = append(errs, o.ClientConfig.Validate())
	errs = append(errs, o.LeaderElection.Validate())
	errs = append(errs, o.InClusterReflection.Validate())

	if len(o.OperandNamespace) == 0 {
		return fmt.Errorf("operand namespace not specified")
	}

	return apierrors.NewAggregate(errs)
}

func (o *RunOptions) Complete() error {
	err := o.InClusterReflection.Complete()
	if err != nil {
		return err
	}

	err = o.ClientConfig.Complete()
	if err != nil {
		return err
	}

	o.kubeClient, err = kubernetes.NewForConfig(o.RestConfig)
	if err != nil {
		return fmt.Errorf("can't build kubernetes clientset: %w", err)
	}

	o.routeClient, err = routeclientset.NewForConfig(o.RestConfig)
	if err != nil {
		return fmt.Errorf("can't build route clientset: %w", err)
	}

	o.operatorClient, err = operatorclientset.NewForConfig(o.RestConfig)
	if err != nil {
		return fmt.Errorf("can't build operator clientset: %w", err)
	}

	return nil
}

func (o *RunOptions) Run(streams genericclioptions.IOStreams, commandName string) error {
	klog.Infof("%s version %s", commandName, version.Get())
	klog.Infof("loglevel is set to %q", cmdutil.GetLoglevel())

	stopCh := signals.StopChannel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-stopCh
		cancel()
	}()

	return leaderelection.Run(
		ctx,
		commandName,
		commandName+"-locks",
		o.Namespace,
		o.kubeClient,
		o.LeaderelectionLeaseDuration,
		o.LeaderelectionRenewDeadline,
		o.LeaderelectionRetryPeriod,
		func(ctx context.Context) error {
			return o.run(ctx, streams)
		},
	)
}

func (o *RunOptions) run(ctx context.Context, streams genericclioptions.IOStreams) error {
	kubeInformersForNamespaces := kubeinformers.NewKubeInformersForNamespaces(
		o.kubeClient,
		[]string{
			o.OperandNamespace,
			corev1.NamespaceAll,
		},
	)

	operatorInformers := operatorinformers.NewSharedInformerFactory(o.operatorClient, 10*time.Minute)

	tcc := targetcontroller.NewTargetController(
		o.OperandNamespace,
		o.OperandImage,
		o.kubeClient,
		o.operatorClient.AcmeV1(),
		operatorInformers.Acme().V1().ACMEControllers(),
		kubeInformersForNamespaces,
	)

	var wg sync.WaitGroup
	defer wg.Wait()

	kubeInformersForNamespaces.Start(ctx.Done())

	wg.Add(1)
	go func() {
		defer wg.Done()
		operatorInformers.Start(ctx.Done())
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tcc.Run(ctx)
	}()

	<-ctx.Done()

	return nil
}
