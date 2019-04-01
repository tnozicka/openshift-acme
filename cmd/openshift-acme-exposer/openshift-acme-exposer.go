package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	cmd "github.com/tnozicka/openshift-acme/pkg/cmd/openshift-acme-exposer"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	command := cmd.NewOpenShiftAcmeExposerCommand(genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	})
	err := command.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
