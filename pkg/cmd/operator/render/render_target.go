package render

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	"github.com/tnozicka/openshift-acme/pkg/controller/operator/assets"
	"github.com/tnozicka/openshift-acme/pkg/controller/operator/assets/target_v100"
	"github.com/tnozicka/openshift-acme/pkg/signals"
	apierrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"
)

type TargetTemplateData struct {
	assets.Data
}

type TargetOptions struct {
	genericclioptions.IOStreams

	OutputDir string

	assets.Data
}

func NewTargetOptions(streams genericclioptions.IOStreams) *TargetOptions {
	return &TargetOptions{
		IOStreams: streams,
		Data: assets.Data{
			ClusterWide: true,
		},
	}
}

func NewTargetCommand(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewTargetOptions(streams)

	cmd := &cobra.Command{
		Use:   "target",
		Short: "Render target manifests",
		Long:  "Render target manifests",
		RunE: func(cmd *cobra.Command, args []string) error {
			defer klog.Flush()

			err := o.Validate()
			if err != nil {
				return err
			}

			err = o.Complete()
			if err != nil {
				return err
			}

			err = o.Run(streams)
			if err != nil {
				return err
			}

			return nil
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.Flags().StringVarP(&o.OutputDir, "output-dir", "", o.OutputDir, "Directory for writing the manifests.")
	cmd.Flags().StringVarP(&o.Image, "image", "", o.Image, "Target image.")
	cmd.Flags().StringVarP(&o.TargetNamespace, "namespace", "", o.TargetNamespace, "Controller namespace.")
	cmd.Flags().BoolVarP(&o.ClusterWide, "cluster-wide", "", o.ClusterWide, "ClusterWide determines if this should be cluster wide deployment or localized to specific namespaces.")
	cmd.Flags().StringArrayVarP(&o.AdditionalNamespaces, "additional-namespace", "", o.AdditionalNamespaces, "Additional namespaces the controller should manage.")

	return cmd
}

func (o *TargetOptions) Validate() error {
	var errs []error

	if len(o.Image) == 0 {
		errs = append(errs, errors.New("image can't be empty"))
	}

	if len(o.TargetNamespace) == 0 {
		errs = append(errs, errors.New("controller namespace can't be empty"))
	}

	if len(o.OutputDir) == 0 {
		errs = append(errs, errors.New("output-dir can't be empty"))
	}

	if o.ClusterWide && len(o.AdditionalNamespaces) != 0 {
		errs = append(errs, errors.New("you can't specify additional namespaces for cluster wide deployment"))
	}

	return apierrors.NewAggregate(errs)
}

func (o *TargetOptions) Complete() error {
	return nil
}

func (o *TargetOptions) Run(streams genericclioptions.IOStreams) error {
	stopCh := signals.StopChannel()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-stopCh
		cancel()
	}()

	templateData := TargetTemplateData{
		Data: o.Data,
	}

	templateDir := "target_v1.0.0"
	templates, err := target_v100.AssetDir(templateDir)
	if err != nil {
		panic(err)
	}
	for _, templatePath := range templates {
		templateContent := string(target_v100.MustAsset(filepath.Join(templateDir, templatePath)))
		t, err := template.New(templatePath).Parse(templateContent)
		if err != nil {
			panic(err)
		}
		var buffer bytes.Buffer
		err = t.Execute(&buffer, templateData)
		if err != nil {
			panic(err)
		}

		templateFileName := filepath.Base(templatePath)
		fileName := strings.TrimSuffix(templateFileName, filepath.Ext(templateFileName))
		outputFile := filepath.Join(o.OutputDir, fileName)
		err = ioutil.WriteFile(outputFile, buffer.Bytes(), 0644)
		if err != nil {
			return err
		}
	}

	return nil
}
