package cmd

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-playground/log"
	"github.com/go-playground/log/handlers/console"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tnozicka/openshift-acme/pkg/acme"
	"github.com/tnozicka/openshift-acme/pkg/acme/challengeexposers"
	cmdutil "github.com/tnozicka/openshift-acme/pkg/cmd/util"
	acme_controller "github.com/tnozicka/openshift-acme/pkg/openshift/controllers/acme"
	route_controller "github.com/tnozicka/openshift-acme/pkg/openshift/controllers/route"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	Flag_LogLevel_Key             = "loglevel"
	Flag_Kubeconfig_Key           = "kubeconfig"
	Flag_Masterurl_Key            = "masterurl"
	Flag_Listen_Key               = "listen"
	Flag_Acmeurl_Key              = "acmeurl"
	Flag_Selfservicename_Key      = "selfservicename"
	Flag_Selfservicenamespace_Key = "selfservicenamespace"
	Flag_Watchnamespace_Key       = "watch-namespace"
)

func loglevelToLevels(level int) []log.Level {
	if level >= len(log.AllLevels) {
		level = len(log.AllLevels)
	}

	r := []log.Level{}
	r = append(r, log.AllLevels[len(log.AllLevels)-level:]...)
	return r
}

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
			cmdutil.BindViper(v, cmd.PersistentFlags(), Flag_LogLevel_Key)
			cmdutil.BindViper(v, cmd.PersistentFlags(), Flag_Kubeconfig_Key)
			cmdutil.BindViper(v, cmd.PersistentFlags(), Flag_Masterurl_Key)
			cmdutil.BindViper(v, cmd.PersistentFlags(), Flag_Listen_Key)
			cmdutil.BindViper(v, cmd.PersistentFlags(), Flag_Acmeurl_Key)
			cmdutil.BindViper(v, cmd.PersistentFlags(), Flag_Selfservicename_Key)
			cmdutil.BindViper(v, cmd.PersistentFlags(), Flag_Selfservicenamespace_Key)
			cmdutil.BindViper(v, cmd.PersistentFlags(), Flag_Watchnamespace_Key)

			// Setup logger
			loglevel := v.GetInt(Flag_LogLevel_Key)
			cLog := console.New()
			log.RegisterHandler(cLog, loglevelToLevels(loglevel)...)

			return nil
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	rootCmd.PersistentFlags().Int8P(Flag_LogLevel_Key, "", 8, "Set loglevel")
	rootCmd.PersistentFlags().StringP(Flag_Kubeconfig_Key, "", "", "Absolute path to the kubeconfig file")
	rootCmd.PersistentFlags().StringP(Flag_Masterurl_Key, "", "", "Kubernetes master URL")
	rootCmd.PersistentFlags().StringP(Flag_Listen_Key, "", "0.0.0.0:5000", "Listen address for http-01 server")
	rootCmd.PersistentFlags().StringP(Flag_Acmeurl_Key, "", "https://acme-staging.api.letsencrypt.org/directory", "ACME URL like https://acme-v01.api.letsencrypt.org/directory")
	rootCmd.PersistentFlags().StringP(Flag_Selfservicename_Key, "", "acme-controller", "Name of the service pointing to a pod with this program.")
	rootCmd.PersistentFlags().StringSliceP(Flag_Watchnamespace_Key, "w", []string{""}, "Restrics controller to namespace. If not specified controller watches for routes accross namespaces.")
	rootCmd.PersistentFlags().StringP(Flag_Selfservicenamespace_Key, "", "", "Namespace of the service pointing to a pod with this program. Defaults to current namespace this program is running inside; if run outside of the cluster defaults to 'default' namespace")

	return rootCmd
}

func RunServer(v *viper.Viper, cmd *cobra.Command, out io.Writer) error {
	defer log.Trace("Controller finished").End()
	log.Info("Starting controller")

	// Setup signal handling
	signalChannel := make(chan os.Signal, 10)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGABRT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	acmeUrl := v.GetString(Flag_Acmeurl_Key)
	log.Infof("ACME server url is '%s'", acmeUrl)

	kubeConfigPath := v.GetString(Flag_Kubeconfig_Key)
	masterUrl := v.GetString(Flag_Masterurl_Key)
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeConfigPath)
	if err != nil {
		log.Fatal(err)
	}
	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	watchNamespaces := v.GetStringSlice(Flag_Watchnamespace_Key)
	// spf13/cobra (sadly) treats []string{""} as []string{} => we need to fix it!
	if len(watchNamespaces) == 0 {
		watchNamespaces = []string{""}
	}
	log.Debugf("namespaces: %#v", watchNamespaces)

	ac := acme_controller.NewAcmeController(ctx, clientset.CoreV1(), acmeUrl, watchNamespaces)
	log.Info("AcmeController bootstraping DB")
	bootstrapTrace := log.Trace("AcmeController bootstraping DB finished")
	if err := ac.BootstrapDB(true, true); err != nil {
		log.Errorf("Unable to bootstrap certificate database: '%+v'", err)
	}
	bootstrapTrace.End()

	log.Info("AcmeController initializing")
	ac.Start()
	defer ac.Wait()
	defer cancel()
	log.Info("AcmeController started")

	listenAddr := v.GetString(Flag_Listen_Key)
	http01, err := challengeexposers.NewHttp01(ctx, listenAddr, log.Logger)
	if err != nil {
		log.Fatal(err)
	}
	challengeExposers := map[string]acme.ChallengeExposer{
		"http-01": http01,
	}

	selfServiceNamespace := v.GetString(Flag_Selfservicenamespace_Key)
	if selfServiceNamespace == "" {
		namespace, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			selfServiceNamespace = "default"
			log.Warnf("Unable to autodetect service namespace. Defaulting to namespace '%s'. Error: %s", selfServiceNamespace, err)
		} else {
			selfServiceNamespace = string(namespace)
		}
	}
	selfService := route_controller.ServiceID{
		Name:      v.GetString(Flag_Selfservicename_Key),
		Namespace: selfServiceNamespace,
	}
	rc, err := route_controller.NewRouteController(ctx, clientset.CoreV1(), ac, challengeExposers, selfService, watchNamespaces)
	if err != nil {
		log.Errorf("Couln't initialize RouteController: '%s'", err)
		return err
	}
	log.Info("RouteController initializing")
	rc.Start()
	defer rc.Wait()
	defer cancel()
	log.Info("RouteController started")

	acDone := make(chan struct{}, 1)
	go func() {
		ac.Wait()
		acDone <- struct{}{}
	}()

	rcDone := make(chan struct{}, 1)
	go func() {
		rc.Wait()
		rcDone <- struct{}{}
	}()

	select {
	case <-acDone:
		return errors.New("AcmeController ended unexpectedly!")
	case <-rcDone:
		return errors.New("RouteController ended unexpectedly!")
	case s := <-signalChannel:
		log.Infof("Cancelling due to signal '%s'", s)
		return nil
	}
}
