package operator

import (
	"github.com/spf13/cobra"
	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	"github.com/tnozicka/openshift-acme/pkg/cmd/operator/render"
	cmdutil "github.com/tnozicka/openshift-acme/pkg/cmd/util"
)

func NewOperatorCommand(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "openshift-acme-operator",
		Short: "openshift-acme-operator is a controller to manage deployment and lifecycle of openshift-acme controller",
		Long:  "openshift-acme-operator is a controller to manage deployment and lifecycle of openshift-acme controller",

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.ReadFlagsFromEnv("OPENSHIFT_ACME_CONTROLLER_", cmd)
		},
	}

	cmd.AddCommand(NewRunCommand(streams))
	cmd.AddCommand(render.NewRenderCommand(streams))

	cmdutil.InstallKlog(cmd)

	return cmd
}
