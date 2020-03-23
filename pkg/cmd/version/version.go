package version

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	apimachineryversion "k8s.io/apimachinery/pkg/version"
)

func NewVersionCommand(fullName string, versionInfo apimachineryversion.Info, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Display version",
		Long:  "Display version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(out, "%s %v\n", fullName, versionInfo)
		},
	}

	return cmd
}
