package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/tnozicka/openshift-acme/cmd"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}

	os.Exit(0)
}
