package render

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tnozicka/openshift-acme/pkg/cmd/genericclioptions"
	"github.com/tnozicka/openshift-acme/pkg/controller/operator/assets"
	apierrors "k8s.io/apimachinery/pkg/util/errors"
)

var (
	controllerImage = "quay.io/tnozicka/openshift-acme:controller"
	exposerImage    = "quay.io/tnozicka/openshift-acme:exposer"
	targetNamespace = "acme-controller"
)

func TestTargetOptionsValidate(t *testing.T) {
	tt := []struct {
		name        string
		options     TargetOptions
		expectedErr error
	}{
		{
			name: "valid config",
			options: TargetOptions{
				Data: assets.Data{
					AdditionalNamespaces: nil,
					TargetNamespace:      targetNamespace,
					ControllerImage:      controllerImage,
					ExposerImage:         exposerImage,
					ClusterWide:          true,
				},
			},
			expectedErr: nil,
		},
		{
			name: "required fields",
			options: TargetOptions{
				Data: assets.Data{
					AdditionalNamespaces: nil,
					TargetNamespace:      "",
					ControllerImage:      "",
					ExposerImage:         "",
					ClusterWide:          true,
				},
			},
			expectedErr: apierrors.NewAggregate([]error{
				errors.New("controller image can't be empty"),
				errors.New("exposer image can't be empty"),
				errors.New("controller namespace can't be empty"),
			}),
		},
		{
			name: "AdditionalNamespaces conflict with ClusterWide",
			options: TargetOptions{
				Data: assets.Data{
					AdditionalNamespaces: []string{"test"},
					TargetNamespace:      targetNamespace,
					ControllerImage:      controllerImage,
					ExposerImage:         exposerImage,
					ClusterWide:          true,
				},
			},
			expectedErr: apierrors.NewAggregate([]error{
				errors.New("you can't specify additional namespaces for cluster wide deployment"),
			}),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tc.options.OutputDir = tmpDir

			err := tc.options.Validate()
			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("expected error %v, got %v", tc.expectedErr, err)
			}
		})
	}

}
func TestTargetOptionsRun(t *testing.T) {
	commonFiles := []string{
		"deployment.yaml",
		"issuer-letsencrypt-live.yaml",
		"issuer-letsencrypt-staging.yaml",
		"namespace.yaml",
		"role.yaml",
		"rolebinding.yaml",
		"serviceaccount.yaml",
		"pdb.yaml",
	}
	tt := []struct {
		name          string
		options       TargetOptions
		expectedFiles []string
	}{
		{
			name: "cluster wide",
			options: TargetOptions{
				Data: assets.Data{
					AdditionalNamespaces: nil,
					TargetNamespace:      targetNamespace,
					ControllerImage:      controllerImage,
					ExposerImage:         exposerImage,
					ClusterWide:          true,
				},
			},
			expectedFiles: commonFiles,
		},
		{
			name: "single namespace",
			options: TargetOptions{
				Data: assets.Data{
					AdditionalNamespaces: nil,
					TargetNamespace:      targetNamespace,
					ControllerImage:      controllerImage,
					ExposerImage:         exposerImage,
					ClusterWide:          false,
				},
			},
			expectedFiles: commonFiles,
		},
		{
			name: "specific namespaces",
			options: TargetOptions{
				Data: assets.Data{
					AdditionalNamespaces: []string{"test"},
					TargetNamespace:      targetNamespace,
					ControllerImage:      controllerImage,
					ExposerImage:         exposerImage,
					ClusterWide:          false,
				},
			},
			expectedFiles: commonFiles,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tc.options.OutputDir = tmpDir

			err := tc.options.Validate()
			if err != nil {
				t.Fatal(err)
			}

			err = tc.options.Complete()
			if err != nil {
				t.Fatal(err)
			}

			err = tc.options.Run(genericclioptions.IOStreams{
				In:     os.Stdin,
				Out:    os.Stdout,
				ErrOut: os.Stderr,
			})
			if err != nil {
				t.Fatal(err)
			}

			var files []string
			err = filepath.Walk(tc.options.OutputDir, func(path string, info os.FileInfo, err error) error {
				r, err := filepath.Rel(tc.options.OutputDir, path)
				if err != nil {
					return err
				}

				if !info.IsDir() {
					files = append(files, r)
				}

				return nil
			})
			if err != nil {
				t.Error(err)
			}

			sort.Strings(files)
			sort.Strings(tc.expectedFiles)

			if !reflect.DeepEqual(files, tc.expectedFiles) {
				t.Errorf("expected and rendered files differ: %s", cmp.Diff(tc.expectedFiles, files))
			}
		})
	}
}
