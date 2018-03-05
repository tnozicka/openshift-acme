package util

import (
	"fmt"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func UsageError(cmd *cobra.Command, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s\nSee '%s -h' for help and examples.", msg, cmd.CommandPath())
}

func BindViperNames(v *viper.Viper, fs *flag.FlagSet, viperName string, cobraName string) {
	// errors here are mistakes in the code and cobra will panic in similar conditions; let's not handle it differently here right now

	flag := fs.Lookup(cobraName)
	if flag == nil {
		panic(fmt.Sprintf("Viper can't bind flag: %s", cobraName))
	}

	err := v.BindPFlag(viperName, flag)
	if err != nil {
		panic(err)
	}
}

func BindViper(v *viper.Viper, fs *flag.FlagSet, name string) {
	BindViperNames(v, fs, name, name)
}
