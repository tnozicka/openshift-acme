package util

import (
	"flag"
	"fmt"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	FlagLogLevelKey = "loglevel"
)

func UsageError(cmd *cobra.Command, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s\nSee '%s -h' for help and examples.", msg, cmd.CommandPath())
}

// func BindViperNames(v *viper.Viper, fs *flag.FlagSet, viperName string, cobraName string) {
// 	// errors here are mistakes in the code and cobra will panic in similar conditions; let's not handle it differently here right now
//
// 	flag := fs.Lookup(cobraName)
// 	if flag == nil {
// 		panic(fmt.Sprintf("Viper can't bind flag: %s", cobraName))
// 	}
//
// 	err := v.BindPFlag(viperName, flag)
// 	if err != nil {
// 		panic(err)
// 	}
// }

// func BindViper(v *viper.Viper, fs *flag.FlagSet, name string) {
// 	BindViperNames(v, fs, name, name)
// }

func MirrorViperForGLog(cmd *cobra.Command, v *viper.Viper) error {
	if v.IsSet(FlagLogLevelKey) {
		// The flag itself needs to be set for glog to recognize it.
		// Makes sure loglevel can be set by environment variable as well.
		err := cmd.PersistentFlags().Set(FlagLogLevelKey, v.GetString(FlagLogLevelKey))
		if err != nil {
			return fmt.Errorf("failed to set %q flag: %v", FlagLogLevelKey, err)
		}
	}

	return nil
}

func InstallGLog(cmd *cobra.Command, defaultLogLevel int32) error {
	from := flag.CommandLine
	if flag := from.Lookup("v"); flag != nil {
		level := flag.Value.(*glog.Level)
		levelPtr := (*int32)(level)
		cmd.PersistentFlags().Int32Var(levelPtr, FlagLogLevelKey, defaultLogLevel, "Set the level of log output (0-10)")
		if cmd.PersistentFlags().Lookup("v") == nil {
			cmd.PersistentFlags().Int32Var(levelPtr, "v", defaultLogLevel, "Set the level of log output (0-10)")
		}
		cmd.PersistentFlags().Lookup("v").Hidden = true
	}
	err := flag.Set("logtostderr", "true")
	if err != nil {
		return fmt.Errorf("failed to set logtostderr flag: %v", err)
	}
	// Make glog happy
	err = flag.CommandLine.Parse([]string{})
	if err != nil {
		return fmt.Errorf("failed to parse command line: %v", err)
	}

	return nil
}
