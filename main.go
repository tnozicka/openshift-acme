package main

import (
	"os"

	"github.com/tnozicka/openshift-acme/cmd"
)

func main() {
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
