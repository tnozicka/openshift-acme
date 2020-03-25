package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	routescheme "github.com/openshift/client-go/route/clientset/versioned/scheme"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"

	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	cmd "github.com/tnozicka/openshift-acme/pkg/cmd/openshift-acme-exposer"
)

func init() {
	klog.InitFlags(flag.CommandLine)
	err := flag.Set("logtostderr", "true")
	if err != nil {
		panic(err)
	}
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	// Register OpenShift groups to kubernetes Scheme
	utilruntime.Must(routescheme.AddToScheme(scheme.Scheme))

	command := cmd.NewOpenShiftAcmeExposerCommand(genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	})
	err := command.Execute()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
