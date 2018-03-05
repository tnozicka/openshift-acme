package cmd

import (
	"fmt"
	"os"

	"github.com/tnozicka/openshift-acme/pkg/cmd"
)

func Run() error {
	command := cmd.NewOpenShiftAcmeCommand(os.Stdin, os.Stdout, os.Stderr)
	err := command.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	}
	return err
}
