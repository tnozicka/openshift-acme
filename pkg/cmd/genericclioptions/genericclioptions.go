package genericclioptions

import (
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/spf13/cobra"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

// IOStreams is a structure containing all standard streams.
type IOStreams struct {
	// In think, os.Stdin
	In io.Reader
	// Out think, os.Stdout
	Out io.Writer
	// ErrOut think, os.Stderr
	ErrOut io.Writer
}

type InClusterReflection struct {
	Namespace string
}

func (o *InClusterReflection) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&o.Namespace, "namespace", "", o.Namespace, "Namespace where the controller is running. Auto-detected if run inside a cluster.")
}

func (o *InClusterReflection) Validate() error {
	return nil
}

func (o *InClusterReflection) Complete() error {
	if len(o.Namespace) == 0 {
		// Autodetect if running inside a cluster
		bytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			return fmt.Errorf("can't autodetect controller namespace: %w", err)
		}

		o.Namespace = string(bytes)
	}

	return nil
}

type ClientConfig struct {
	Kubeconfig string
	RestConfig *restclient.Config
}

func (cc *ClientConfig) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&cc.Kubeconfig, "kubeconfig", "", cc.Kubeconfig, "Path to the kubeconfig file")
}

func (c *ClientConfig) Validate() error {
	return nil
}

func (c *ClientConfig) Complete() error {
	var err error

	if len(c.Kubeconfig) != 0 {
		klog.V(1).Infof("Using kubeconfig %q.", c.Kubeconfig)
		c.RestConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: c.Kubeconfig}, &clientcmd.ConfigOverrides{}).ClientConfig()
		if err != nil {
			return fmt.Errorf("can't create config from kubeConfigPath %q: %w", c.Kubeconfig, err)
		}
	} else {
		klog.V(1).Infof("No kubeconfig specified, using InClusterConfig.")
		c.RestConfig, err = restclient.InClusterConfig()
		if err != nil {
			return fmt.Errorf("can't create InClusterConfig: %w", err)
		}
	}

	return nil
}

type LeaderElection struct {
	LeaderelectionLeaseDuration time.Duration
	LeaderelectionRenewDeadline time.Duration
	LeaderelectionRetryPeriod   time.Duration
}

func NewLeaderElection() LeaderElection {
	return LeaderElection{
		LeaderelectionLeaseDuration: 60 * time.Second,
		LeaderelectionRenewDeadline: 35 * time.Second,
		LeaderelectionRetryPeriod:   10 * time.Second,
	}
}

func (le *LeaderElection) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().DurationVar(&le.LeaderelectionLeaseDuration, "leaderelection-lease-duration", le.LeaderelectionLeaseDuration, "LeaseDuration is the duration that non-leader candidates will wait to force acquire leadership.")
	cmd.PersistentFlags().DurationVar(&le.LeaderelectionRenewDeadline, "leaderelection-renew-deadline", le.LeaderelectionRenewDeadline, "RenewDeadline is the duration that the acting master will retry refreshing leadership before giving up.")
	cmd.PersistentFlags().DurationVar(&le.LeaderelectionRetryPeriod, "leaderelection-retry-period", le.LeaderelectionRetryPeriod, "RetryPeriod is the duration the LeaderElector clients should wait between tries of actions.")
}

func (n *LeaderElection) Validate() error {
	return nil
}

func (n *LeaderElection) Complete() error {
	return nil
}
