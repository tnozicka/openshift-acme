package openshift_acme

import (
	"fmt"
	"io"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	kvalidationutil "k8s.io/apimachinery/pkg/util/validation"

	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	cmdutil "github.com/tnozicka/openshift-acme/pkg/cmd/util"
)

type ExposerOptions struct {
	genericclioptions.IOStreams
	Loglevel int32

	Data     *string
	Port     uint16
	ListenIP string
}

func NewExposerOptions(streams genericclioptions.IOStreams) *ExposerOptions {
	return &ExposerOptions{
		IOStreams: streams,
		Loglevel:  2,
		Data:      nil,
		Port:      5000,
		ListenIP:  "0.0.0.0",
	}
}

func NewOpenShiftAcmeExposerCommand(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewExposerOptions(streams)

	v := viper.New()
	v.SetEnvPrefix("openshift_acme_exposer")
	v.AutomaticEnv()
	replacer := strings.NewReplacer("-", "_")
	v.SetEnvKeyReplacer(replacer)

	// Parent command to which all subcommands are added.
	rootCmd := &cobra.Command{
		Use:   "openshift-acme-exposer",
		Short: "openshift-acme-exposer is a simple http server for exposing ACME token for validation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			defer glog.Flush()

			err := o.Complete()
			if err != nil {
				return err
			}

			err = o.Validate()
			if err != nil {
				return err
			}

			err = o.Run(v, cmd, streams.ErrOut)
			if err != nil {
				return err
			}

			return nil
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// We have to bind Viper here because there is only one instance to avoid collisions
			err := v.BindPFlags(cmd.PersistentFlags())
			if err != nil {
				return fmt.Errorf("failed to bind Viper: %v", err)
			}

			err = cmdutil.MirrorViperForGLog(cmd, v)
			if err != nil {
				return fmt.Errorf("failed to install glog: %v", err)
			}

			return nil
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	rootCmd.PersistentFlags().StringP("data", "d", *o.Data, "Data to expose for every request on this server.")
	rootCmd.PersistentFlags().Uint16P("port", "", o.Port, "Port for http-01 server")
	rootCmd.PersistentFlags().StringP("listen-ip", "", o.ListenIP, "Listen address for http-01 server")

	err := cmdutil.InstallGLog(rootCmd, o.Loglevel)
	if err != nil {
		panic(fmt.Errorf("failed to install glog: %v", err))
	}

	return rootCmd
}

func (o *ExposerOptions) Complete() error {
	return nil
}

func (o *ExposerOptions) Validate() error {
	if o.Data == nil {
		return fmt.Errorf("no data specified for the exposer")
	}

	errs := kvalidationutil.IsValidPortNum(int(o.Port))
	if len(errs) > 0 {
		return fmt.Errorf("Port has invalid value: %s", strings.Join(errs, ", "))
	}

	return nil
}

func (o *ExposerOptions) Run(v *viper.Viper, cmd *cobra.Command, out io.Writer) error {
	glog.Infof("Running with loglevel == %d", o.Loglevel)

	return nil

	/*
		stopCh := signals.StopChannel()
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-stopCh
			cancel()
		}()

		exposerPort := v.GetInt(Flag_ExposerPort)
		errs := kvalidationutil.IsValidPortNum(exposerPort)
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

		data := v.GetString(Flag_Data_Key)
		if data == "" {
			glog.Warning("Exposing empty data.")
		}

		handleAll := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, data)
			// o not log the content here as pod logs can be collected and the token is secret
			glog.V(2).Infof("Responded for path %q", r.RequestURI)
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/", handleAll)
		listenAddr := fmt.Sprintf("%s:%d", exposerListenIP, exposerPort)
		server := &http.Server{
			//Addr:    addr,
			Handler: mux,
		}

		listener, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return err
		}

		glog.Infof("Listening on http://%s/", listener.Addr().String())

		go func() {
			<-ctx.Done()
			glog.Info("Stopping http server...")
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			server.Shutdown(ctx)
		}()

		return server.Serve(listener)
	*/
}
