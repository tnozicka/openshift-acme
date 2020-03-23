package openshift_acme

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/klog"

	kvalidationutil "k8s.io/apimachinery/pkg/util/validation"

	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	cmdutil "github.com/tnozicka/openshift-acme/pkg/cmd/util"
	"github.com/tnozicka/openshift-acme/pkg/httpserver"
	"github.com/tnozicka/openshift-acme/pkg/signals"
)

type Options struct {
	genericclioptions.IOStreams

	ResponseFile string
	Port         uint16
	ListenIP     string
}

func NewExposerOptions(streams genericclioptions.IOStreams) *Options {
	return &Options{
		IOStreams:    streams,
		ResponseFile: "",
		Port:         5000,
		ListenIP:     "0.0.0.0",
	}
}

func NewOpenShiftAcmeExposerCommand(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewExposerOptions(streams)

	// Parent command to which all subcommands are added.
	rootCmd := &cobra.Command{
		Use:   "openshift-acme-exposer",
		Short: "openshift-acme-exposer is a simple http server for exposing ACME token for validation.",
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

			err = o.Run(cmd, streams.ErrOut)
			if err != nil {
				return err
			}

			return nil
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := cmdutil.ReadFlagsFromEnv("OPENSHIFT_ACME_EXPOSER_", cmd)
			if err != nil {
				return err
			}

			return nil
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	rootCmd.PersistentFlags().StringVarP(&o.ResponseFile, "response-file", "f", o.ResponseFile, "File containing data to expose using format `URI Response`.")
	rootCmd.PersistentFlags().Uint16VarP(&o.Port, "port", "p", o.Port, "Port for http-01 server")
	rootCmd.PersistentFlags().StringVarP(&o.ListenIP, "listen-ip", "l", o.ListenIP, "Listen address for http-01 server")

	cmdutil.InstallKlog(rootCmd)

	return rootCmd
}

func (o *Options) Validate() error {
	if o.ResponseFile == "" {
		return fmt.Errorf("no response-file specified")
	}

	errs := kvalidationutil.IsValidPortNum(int(o.Port))
	if len(errs) > 0 {
		return fmt.Errorf("invalid port %v: %s", o.Port, strings.Join(errs, ", "))
	}

	errs = kvalidationutil.IsValidIP(o.ListenIP)
	if len(errs) > 0 {
		return fmt.Errorf("invalid listen IP %q: %s", o.ListenIP, strings.Join(errs, ", "))
	}

	return nil
}

func (o *Options) Complete() error {
	return nil
}

func (o *Options) Run(cmd *cobra.Command, out io.Writer) error {
	stopCh := signals.StopChannel()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-stopCh
		cancel()
	}()

	klog.Infof("loglevel is set to %q", cmdutil.GetLoglevel())

	bytes, err := ioutil.ReadFile(o.ResponseFile)
	if err != nil {
		return err
	}

	server := httpserver.NewServer(fmt.Sprintf("%s:%d", o.ListenIP, o.Port), nil)

	err = server.ParseData(bytes)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()

		err = server.Run()
		if err != nil {
			errCh <- err
			return
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		<-ctx.Done()

		// Second SIGINT results in exit(1) so it can be forcefully terminated that way for now
		err := server.Shutdown(context.TODO())
		if err != nil {
			errCh <- err
			return
		}
		return
	}()

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	return errors.NewAggregate(errs)
}
