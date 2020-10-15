// Code generated for package target_v100 by go-bindata DO NOT EDIT. (@generated)
// sources:
// bindata/operator/target_v1.0.0/deployment.yaml.tmpl
// bindata/operator/target_v1.0.0/issuer-letsencrypt-live.yaml.tmpl
// bindata/operator/target_v1.0.0/issuer-letsencrypt-staging.yaml.tmpl
// bindata/operator/target_v1.0.0/namespace.yaml.tmpl
// bindata/operator/target_v1.0.0/pdb.yaml.tmpl
// bindata/operator/target_v1.0.0/role.yaml.tmpl
// bindata/operator/target_v1.0.0/rolebinding.yaml.tmpl
// bindata/operator/target_v1.0.0/serviceaccount.yaml.tmpl
package target_v100

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

// Name return file name
func (fi bindataFileInfo) Name() string {
	return fi.name
}

// Size return file size
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}

// Mode return file mode
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}

// Mode return file modify time
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}

// IsDir return file whether a directory
func (fi bindataFileInfo) IsDir() bool {
	return fi.mode&os.ModeDir != 0
}

// Sys return file is sys mode
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _target_v100DeploymentYamlTmpl = []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: openshift-acme
  labels:
    app: openshift-acme
spec:
  selector:
    matchLabels:
      app: openshift-acme
  replicas: 2
  strategy:
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: openshift-acme
    spec:
      serviceAccountName: openshift-acme
      containers:
      - name: openshift-acme
        image: {{ .ControllerImage }}
        imagePullPolicy: Always
        args:
        - --exposer-image={{ .ExposerImage }}
        - --loglevel=4
{{- if not .ClusterWide }}
        - --namespace=$(CURRENT_NAMESPACE)
  {{- range .AdditionalNamespaces }}
        - --namespace={{ . }}
  {{- end }}
        env:
        - name: CURRENT_NAMESPACE
        valueFrom:
        fieldRef:
        fieldPath: metadata.namespace
{{ end -}}
`)

func target_v100DeploymentYamlTmplBytes() ([]byte, error) {
	return _target_v100DeploymentYamlTmpl, nil
}

func target_v100DeploymentYamlTmpl() (*asset, error) {
	bytes, err := target_v100DeploymentYamlTmplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "target_v1.0.0/deployment.yaml.tmpl", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _target_v100IssuerLetsencryptLiveYamlTmpl = []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: letsencrypt-live
  annotations:
    "acme.openshift.io/priority": "100"
  labels:
    managed-by: "openshift-acme"
    type: "CertIssuer"
data:
  "cert-issuer.types.acme.openshift.io": '{"type":"ACME","acmeCertIssuer":{"directoryUrl":"https://acme-v02.api.letsencrypt.org/directory"}}'
`)

func target_v100IssuerLetsencryptLiveYamlTmplBytes() ([]byte, error) {
	return _target_v100IssuerLetsencryptLiveYamlTmpl, nil
}

func target_v100IssuerLetsencryptLiveYamlTmpl() (*asset, error) {
	bytes, err := target_v100IssuerLetsencryptLiveYamlTmplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "target_v1.0.0/issuer-letsencrypt-live.yaml.tmpl", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _target_v100IssuerLetsencryptStagingYamlTmpl = []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: letsencrypt-staging
  annotations:
    "acme.openshift.io/priority": "50"
  labels:
    managed-by: "openshift-acme"
    type: "CertIssuer"
data:
  "cert-issuer.types.acme.openshift.io": '{"type":"ACME","acmeCertIssuer":{"directoryUrl":"https://acme-staging-v02.api.letsencrypt.org/directory"}}'
`)

func target_v100IssuerLetsencryptStagingYamlTmplBytes() ([]byte, error) {
	return _target_v100IssuerLetsencryptStagingYamlTmpl, nil
}

func target_v100IssuerLetsencryptStagingYamlTmpl() (*asset, error) {
	bytes, err := target_v100IssuerLetsencryptStagingYamlTmplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "target_v1.0.0/issuer-letsencrypt-staging.yaml.tmpl", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _target_v100NamespaceYamlTmpl = []byte(`apiVersion: v1
kind: Namespace
metadata:
  name: {{ .TargetNamespace }}
`)

func target_v100NamespaceYamlTmplBytes() ([]byte, error) {
	return _target_v100NamespaceYamlTmpl, nil
}

func target_v100NamespaceYamlTmpl() (*asset, error) {
	bytes, err := target_v100NamespaceYamlTmplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "target_v1.0.0/namespace.yaml.tmpl", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _target_v100PdbYamlTmpl = []byte(`apiVersion: policy/v1beta1
kind: PodDisruptionBudget
metadata:
  name: openshift-acme
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: openshift-acme
`)

func target_v100PdbYamlTmplBytes() ([]byte, error) {
	return _target_v100PdbYamlTmpl, nil
}

func target_v100PdbYamlTmpl() (*asset, error) {
	bytes, err := target_v100PdbYamlTmplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "target_v1.0.0/pdb.yaml.tmpl", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _target_v100RoleYamlTmpl = []byte(`apiVersion: rbac.authorization.k8s.io/v1
{{- if .ClusterWide }}
kind: ClusterRole
{{- else }}
kind: Role
{{- end }}
metadata:
  name: openshift-acme
  labels:
    app: openshift-acme
rules:
- apiGroups:
  - "route.openshift.io"
  resources:
  - routes
  verbs:
  - create
  - get
  - list
  - watch
  - update
  - patch
  - delete
  - deletecollection

- apiGroups:
  - "route.openshift.io"
  resources:
  - routes/custom-host
  verbs:
  - create

- apiGroups:
  - ""
  resources:
  - configmaps
  - services
  - secrets
  verbs:
  - create
  - get
  - list
  - watch
  - update
  - patch
  - delete

- apiGroups:
  - ""
  resources:
  - limitranges
  verbs:
  - get
  - list
  - watch

- apiGroups:
  - "apps"
  resources:
  - replicasets
  verbs:
  - create
  - get
  - list
  - watch
  - update
  - patch
  - delete

{{- if .ClusterWide }}

- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - update
  - patch
{{- end }}
`)

func target_v100RoleYamlTmplBytes() ([]byte, error) {
	return _target_v100RoleYamlTmpl, nil
}

func target_v100RoleYamlTmpl() (*asset, error) {
	bytes, err := target_v100RoleYamlTmplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "target_v1.0.0/role.yaml.tmpl", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _target_v100RolebindingYamlTmpl = []byte(`{{ range $i, $namespace := .AllNamespaces -}}
  {{- if ne $i 0 -}}
---
  {{ end -}}
apiVersion: rbac.authorization.k8s.io/v1
  {{- if $.ClusterWide }}
kind: ClusterRoleBinding
  {{- else }}
kind: RoleBinding
  {{- end }}
metadata:
  {{- if ne $namespace $.TargetNamespace }}
  namespace: {{ $namespace }}
  {{- end }}
  name: openshift-acme
roleRef:
  {{- if $.ClusterWide }}
  kind: ClusterRole
  {{- else }}
  kind: Role
  {{- end }}
  {{- if ne $namespace $.TargetNamespace }}
  namespace: {{ $namespace }}
  {{- end }}
  name: openshift-acme
subjects:
- kind: ServiceAccount
  {{- if $.ClusterWide }}
  namespace: {{ $.TargetNamespace }}
  {{- end }}
  name: openshift-acme
{{ end -}}
`)

func target_v100RolebindingYamlTmplBytes() ([]byte, error) {
	return _target_v100RolebindingYamlTmpl, nil
}

func target_v100RolebindingYamlTmpl() (*asset, error) {
	bytes, err := target_v100RolebindingYamlTmplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "target_v1.0.0/rolebinding.yaml.tmpl", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _target_v100ServiceaccountYamlTmpl = []byte(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: openshift-acme
  labels:
    app: openshift-acme
`)

func target_v100ServiceaccountYamlTmplBytes() ([]byte, error) {
	return _target_v100ServiceaccountYamlTmpl, nil
}

func target_v100ServiceaccountYamlTmpl() (*asset, error) {
	bytes, err := target_v100ServiceaccountYamlTmplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "target_v1.0.0/serviceaccount.yaml.tmpl", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"target_v1.0.0/deployment.yaml.tmpl":                 target_v100DeploymentYamlTmpl,
	"target_v1.0.0/issuer-letsencrypt-live.yaml.tmpl":    target_v100IssuerLetsencryptLiveYamlTmpl,
	"target_v1.0.0/issuer-letsencrypt-staging.yaml.tmpl": target_v100IssuerLetsencryptStagingYamlTmpl,
	"target_v1.0.0/namespace.yaml.tmpl":                  target_v100NamespaceYamlTmpl,
	"target_v1.0.0/pdb.yaml.tmpl":                        target_v100PdbYamlTmpl,
	"target_v1.0.0/role.yaml.tmpl":                       target_v100RoleYamlTmpl,
	"target_v1.0.0/rolebinding.yaml.tmpl":                target_v100RolebindingYamlTmpl,
	"target_v1.0.0/serviceaccount.yaml.tmpl":             target_v100ServiceaccountYamlTmpl,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"target_v1.0.0": {nil, map[string]*bintree{
		"deployment.yaml.tmpl":                 {target_v100DeploymentYamlTmpl, map[string]*bintree{}},
		"issuer-letsencrypt-live.yaml.tmpl":    {target_v100IssuerLetsencryptLiveYamlTmpl, map[string]*bintree{}},
		"issuer-letsencrypt-staging.yaml.tmpl": {target_v100IssuerLetsencryptStagingYamlTmpl, map[string]*bintree{}},
		"namespace.yaml.tmpl":                  {target_v100NamespaceYamlTmpl, map[string]*bintree{}},
		"pdb.yaml.tmpl":                        {target_v100PdbYamlTmpl, map[string]*bintree{}},
		"role.yaml.tmpl":                       {target_v100RoleYamlTmpl, map[string]*bintree{}},
		"rolebinding.yaml.tmpl":                {target_v100RolebindingYamlTmpl, map[string]*bintree{}},
		"serviceaccount.yaml.tmpl":             {target_v100ServiceaccountYamlTmpl, map[string]*bintree{}},
	}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}
