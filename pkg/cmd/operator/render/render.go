package render

import (
	"github.com/spf13/cobra"
	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
)

func NewRenderCommand(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render manifests",
		Long:  "Render manifests",
	}

	cmd.AddCommand(NewTargetCommand(streams))

	return cmd
}
