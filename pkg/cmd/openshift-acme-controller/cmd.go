package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	routev1 "github.com/openshift/api/route/v1"
	routeclientset "github.com/openshift/client-go/route/clientset/versioned"
	routescheme "github.com/openshift/client-go/route/clientset/versioned/scheme"
	routeinformersv1 "github.com/openshift/client-go/route/informers/externalversions/route/v1"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	kvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/labels"
	kvalidationutil "k8s.io/apimachinery/pkg/util/validation"
	kcoreinformersv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	kcorelistersv1 "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/tnozicka/openshift-acme/pkg/acme/challengeexposers"
	acmeclientbuilder "github.com/tnozicka/openshift-acme/pkg/acme/client/builder"
	cmdutil "github.com/tnozicka/openshift-acme/pkg/cmd/util"
	routecontroller "github.com/tnozicka/openshift-acme/pkg/controllers/route"
	"github.com/tnozicka/openshift-acme/pkg/signals"
)

const (
	DefaultLoglevel                  = 4
	Flag_LogLevel_Key                = "loglevel"
	Flag_Kubeconfig_Key              = "kubeconfig"
	Flag_Acmeurl_Key                 = "acmeurl"
	Flag_SelfNamespace_Key           = "selfnamespace"
	Flag_ExposerIP                   = "exposer-ip"
	Flag_ExposerPort                 = "exposer-port"
	Flag_ExposerListenIP             = "exposer-listen-ip"
	Flag_Namespace_Key               = "namespace"
	Flag_AccountName_Key             = "account-name"
	Flag_DefaultRouteTermination_Key = "default-route-termination"
	Flag_Labels                      = "labels"
	SelfLabels_Path                  = "/dapi/labels"
	ResyncPeriod                     = 10 * time.Minute
	Workers                          = 10
)

func NewOpenShiftAcmeCommand(in io.Reader, out, err io.Writer) *cobra.Command {
	v := viper.New()
	v.SetEnvPrefix("openshift_acme")
	v.AutomaticEnv()
	replacer := strings.NewReplacer("-", "_")
	v.SetEnvKeyReplacer(replacer)

	// Parent command to which all subcommands are added.
	rootCmd := &cobra.Command{
		Use:   "openshift-acme",
		Short: "openshift-acme is a controller for Kubernetes (and OpenShift) which will obtain SSL certificates from ACME provider (like \"Let's Encrypt\")",
		Long:  "openshift-acme is a controller for Kubernetes (and OpenShift) which will obtain SSL certificates from ACME provider (like \"Let's Encrypt\")\n\nFind more information at https://github.com/tnozicka/openshift-acme",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return cmdutil.UsageError(cmd, "Unexpected args: %v", args)
			}

			return RunServer(v, cmd, out)
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// We have to bind Viper in Run because there is only one instance to avoid collisions
			v.BindPFlags(cmd.PersistentFlags())

			if v.IsSet(Flag_LogLevel_Key) {
				// The flag itself needs to be set for glog to recognize it.
				// Makes sure loglevel can be set by environment variable as well.
				cmd.PersistentFlags().Set(Flag_LogLevel_Key, v.GetString(Flag_LogLevel_Key))
			}

			return nil
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	rootCmd.PersistentFlags().StringP(Flag_Kubeconfig_Key, "", "", "Absolute path to the kubeconfig file")
	rootCmd.PersistentFlags().StringP(Flag_Acmeurl_Key, "", "https://acme-staging.api.letsencrypt.org/directory", "ACME URL like https://acme-v01.api.letsencrypt.org/directory")
	rootCmd.PersistentFlags().StringP(Flag_Namespace_Key, "n", "", "Restricts controller to namespace. If not specified controller watches all namespaces.")
	rootCmd.PersistentFlags().StringP(Flag_AccountName_Key, "", "acme-account", "Name of the Secret holding ACME account.")
	rootCmd.PersistentFlags().StringP(Flag_ExposerIP, "", "", "IP address on which this controller can be reached inside the cluster.")
	rootCmd.PersistentFlags().Int32P(Flag_ExposerPort, "", 5000, "Port for http-01 server")
	rootCmd.PersistentFlags().StringP(Flag_ExposerListenIP, "", "0.0.0.0", "Listen address for http-01 server")
	rootCmd.PersistentFlags().StringP(Flag_SelfNamespace_Key, "", "", "Namespace where this controller and associated objects are deployed to. Defaults to current namespace if this program is running inside of the cluster.")
	rootCmd.PersistentFlags().StringP(Flag_DefaultRouteTermination_Key, "", string(routev1.InsecureEdgeTerminationPolicyRedirect), "Default TLS termination of the route.")
	rootCmd.PersistentFlags().StringP(Flag_Labels, "", "", "Comma seperated list of labels and values that will be applied to challenge URL")

	from := flag.CommandLine
	if flag := from.Lookup("v"); flag != nil {
		level := flag.Value.(*glog.Level)
		levelPtr := (*int32)(level)
		rootCmd.PersistentFlags().Int32Var(levelPtr, Flag_LogLevel_Key, DefaultLoglevel, "Set the level of log output (0-10)")
		if rootCmd.PersistentFlags().Lookup("v") == nil {
			rootCmd.PersistentFlags().Int32Var(levelPtr, "v", DefaultLoglevel, "Set the level of log output (0-10)")
		}
		rootCmd.PersistentFlags().Lookup("v").Hidden = true
	}
	flag.Set("logtostderr", "true")
	// Make glog happy
	flag.CommandLine.Parse([]string{})

	return rootCmd
}

func getClientConfig(kubeConfigPath string) *restclient.Config {
	if kubeConfigPath == "" {
		glog.Infof("No kubeconfig specified, using InClusterConfig.")
		config, err := restclient.InClusterConfig()
		if err != nil {
			glog.Fatalf("Failed to create InClusterConfig: %v", err)
		}
		return config
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath}, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		glog.Fatalf("Failed to create config from kubeConfigPath (%s): %v", kubeConfigPath, err)
	}
	return config
}

func RunServer(v *viper.Viper, cmd *cobra.Command, out io.Writer) error {
	// Register OpenShift groups to kubernetes Scheme
	routescheme.AddToScheme(scheme.Scheme)

	stopCh := signals.StopChannel()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-stopCh
		cancel()
	}()

	acmeUrl := v.GetString(Flag_Acmeurl_Key)
	glog.Infof("ACME server url is %q", acmeUrl)

	// Better to read flag value than viper here to make sure the value is what glog uses.
	loglevel, err := cmd.PersistentFlags().GetInt32(Flag_LogLevel_Key)
	if err != nil {
		return err
	}
	glog.Infof("ACME server loglevel == %d", loglevel)

	config := getClientConfig(v.GetString(Flag_Kubeconfig_Key))

	kubeClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to build kubernetes clientset: %v", err)
	}

	routeClientset, err := routeclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to build route clientset: %v", err)
	}

	namespace := v.GetString(Flag_Namespace_Key)
	if namespace == "" {
		glog.Info("Watching all namespaces.")
	} else {
		errs := kvalidation.ValidateNamespaceName(namespace, false)
		if len(errs) > 0 {
			return fmt.Errorf("flag %q has invalid value: %s", Flag_Namespace_Key, strings.Join(errs, ", "))
		}
		glog.Infof("Watching only namespace %q.", namespace)
	}

	accountName := v.GetString(Flag_AccountName_Key)
	if accountName == "" {
		return fmt.Errorf("flag %q can't be empty string", Flag_AccountName_Key)
	}
	errs := kvalidation.NameIsDNSSubdomain(accountName, false)
	if len(errs) > 0 {
		return fmt.Errorf("flag %q has invalid value: %s", Flag_AccountName_Key, strings.Join(errs, ", "))
	}

	selfNamespace := v.GetString(Flag_SelfNamespace_Key)
	if selfNamespace == "" {
		glog.V(4).Infof("%q is unspecified, trying inCluster", Flag_SelfNamespace_Key)
		selfServiceNamespaceBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			return fmt.Errorf("failed to detect selfServiceNamespace: %v", err)
		}
		selfNamespace = (string)(selfServiceNamespaceBytes)
	} else {
		errs := kvalidation.NameIsDNSSubdomain(selfNamespace, false)
		if len(errs) > 0 {
			return fmt.Errorf("flag %q has invalid value: %s", Flag_SelfNamespace_Key, strings.Join(errs, ", "))
		}
	}

	var selfSelector map[string]string
	_, err = os.Stat(SelfLabels_Path)
	if err == nil {
		labelsBytes, err := ioutil.ReadFile(SelfLabels_Path)
		if err != nil {
			return fmt.Errorf("failed to read self labels file %q: %v", SelfLabels_Path, err)
		}

		labelsSet, err := labels.ConvertSelectorToLabelsMap(strings.Replace(strings.Replace(string(labelsBytes), "\n", ",", -1), "\"", "", -1))
		if err != nil {
			return fmt.Errorf("failed to parse labels in self labels file %q: %v", SelfLabels_Path, err)
		}

		selfSelector = map[string]string(labelsSet)
		glog.Infof("Setup self selector %#v", selfSelector)
	}

	exposerIP := v.GetString(Flag_ExposerIP)
	if exposerIP == "" {
		return fmt.Errorf("%q can't be empty string", Flag_ExposerIP)
	} else {
		errs := kvalidationutil.IsValidIP(exposerIP)
		if len(errs) > 0 {
			return fmt.Errorf("flag %q has invalid value: %s", Flag_ExposerIP, strings.Join(errs, ", "))
		}
	}

	exposerPort := v.GetInt(Flag_ExposerPort)
	errs = kvalidationutil.IsValidPortNum(exposerPort)
	if len(errs) > 0 {
		return fmt.Errorf("flag %q has invalid value: %s", Flag_ExposerPort, strings.Join(errs, ", "))
	}

	exposerListenIP := v.GetString(Flag_ExposerListenIP)
	if exposerListenIP == "" {
		return fmt.Errorf("%q can't be empty string", Flag_ExposerListenIP)
	} else {
		errs := kvalidationutil.IsValidIP(exposerListenIP)
		if len(errs) > 0 {
			return fmt.Errorf("flag %q has invalid value: %s", Flag_ExposerListenIP, strings.Join(errs, ", "))
		}
	}

	defaultRouteTermination := routev1.InsecureEdgeTerminationPolicyType(v.GetString(Flag_DefaultRouteTermination_Key))
	switch defaultRouteTermination {
	case routev1.InsecureEdgeTerminationPolicyRedirect,
		routev1.InsecureEdgeTerminationPolicyAllow,
		routev1.InsecureEdgeTerminationPolicyNone:
	default:
		return fmt.Errorf("flag %q has invalid value: %q", Flag_DefaultRouteTermination_Key, defaultRouteTermination)
	}

	labelsSelector := v.GetString(Flag_Labels)
	labelsSet, err := labels.ConvertSelectorToLabelsMap(labelsSelector)
	if err != nil {
		return fmt.Errorf("failed to parse labels in flag %q: %v", SelfLabels_Path, err)
	}

	routeInformer := routeinformersv1.NewRouteInformer(routeClientset, namespace, ResyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	glog.Infof("Starting Route informer")
	go routeInformer.Run(stopCh)

	secretInformer := kcoreinformersv1.NewSecretInformer(kubeClientset, namespace, ResyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	glog.Infof("Starting Secret informer")
	go secretInformer.Run(stopCh)

	listenAddr := fmt.Sprintf("%s:%d", exposerListenIP, exposerPort)
	glog.Infof("Exposer listen address is %q", listenAddr)
	http01, err := challengeexposers.NewHttp01(ctx, listenAddr)
	if err != nil {
		return err
	}

	exposers := map[string]challengeexposers.Interface{
		"http-01": http01,
	}

	// Wait secretInformer to sync so we can create acmeClientFactory
	if !cache.WaitForCacheSync(stopCh, secretInformer.HasSynced) {
		return fmt.Errorf("timed out waiting for secretInformer caches to sync")
	}
	secretLister := kcorelistersv1.NewSecretLister(secretInformer.GetIndexer())
	acmeClientFactory := acmeclientbuilder.NewSharedClientFactory(acmeUrl, accountName, selfNamespace, kubeClientset, secretLister)

	rc := routecontroller.NewRouteController(acmeClientFactory, exposers, routeClientset, kubeClientset, routeInformer, secretInformer, exposerIP, int32(exposerPort), selfNamespace, selfSelector, defaultRouteTermination, labelsSet)
	go rc.Run(Workers, stopCh)

	<-stopCh

	// TODO: We should wait for controllers to stop

	glog.Flush()

	return nil
}
