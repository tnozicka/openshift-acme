package openshift_acme_operator

import (
	"context"
	"flag"
	"fmt"
	"sync"

	routeclientset "github.com/openshift/client-go/route/clientset/versioned"
	"github.com/spf13/cobra"
	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	cmdutil "github.com/tnozicka/openshift-acme/pkg/cmd/util"
	"github.com/tnozicka/openshift-acme/pkg/controller/operator/targetconfigcontroller"
	"github.com/tnozicka/openshift-acme/pkg/leaderelection"
	kubeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/kube"
	"github.com/tnozicka/openshift-acme/pkg/signals"
	"github.com/tnozicka/openshift-acme/pkg/version"
	apierrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

type Options struct {
	genericclioptions.IOStreams
	genericclioptions.LeaderElection
	genericclioptions.ClientConfig
	genericclioptions.InClusterReflection

	OperandNamespace string
	OperandImage     string

	kubeClient  kubernetes.Interface
	routeClient routeclientset.Interface
}

func NewOptions(streams genericclioptions.IOStreams) *Options {
	return &Options{
		IOStreams:      streams,
		LeaderElection: genericclioptions.NewLeaderElection(),

		OperandNamespace: "",
		OperandImage:     "",
	}
}

func NewOpenshiftAcmeOperatorCommand(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewOptions(streams)

	cmd := &cobra.Command{
		Use:   "openshift-acme-operator",
		Short: "openshift-acme-operator is a controller to manage deployment and lifecycle of openshift-acme cotnrolelr",
		Long:  "openshift-acme-operator is a controller to manage deployment and lifecycle of openshift-acme cotnrolelr",
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

	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	o.LeaderElection.AddFlags(cmd)
	o.ClientConfig.AddFlags(cmd)
	o.InClusterReflection.AddFlags(cmd)

	cmd.PersistentFlags().StringVarP(&o.OperandNamespace, "operand-namespace", "", o.OperandNamespace, "Namespace for deploying the controller.")
	cmd.PersistentFlags().StringVarP(&o.OperandImage, "operand-image", "", o.OperandImage, "Controller image.")

	cmdutil.InstallKlog(cmd)

	return cmd
}

func (o *Options) Validate() error {
	var errs []error

	errs = append(errs, o.ClientConfig.Validate())
	errs = append(errs, o.LeaderElection.Validate())
	errs = append(errs, o.InClusterReflection.Validate())

	return apierrors.NewAggregate(errs)
}

func (o *Options) Complete() error {
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

	return nil
}

func (o *Options) Run(cmd *cobra.Command, streams genericclioptions.IOStreams) error {
	klog.Infof("%s version %s", cmd.Name(), version.Get())
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
		cmd.Name(),
		cmd.Name()+"-locks",
		o.Namespace,
		o.kubeClient,
		o.LeaderelectionLeaseDuration,
		o.LeaderelectionRenewDeadline,
		o.LeaderelectionRetryPeriod,
		func(ctx context.Context) error {
			return o.run(ctx, cmd, streams)
		},
	)
}

func (o *Options) run(ctx context.Context, cmd *cobra.Command, streams genericclioptions.IOStreams) error {
	kubeInformersForNamespaces := kubeinformers.NewKubeInformersForNamespaces(
		o.kubeClient,
		[]string{
			o.OperandNamespace,
		},
	)

	tcc := targetconfigcontroller.NewTargetConfigController(o.OperandImage, o.kubeClient, kubeInformersForNamespaces)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tcc.Run(ctx)
	}()

	<-ctx.Done()

	return nil
}
