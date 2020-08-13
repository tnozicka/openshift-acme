module github.com/tnozicka/openshift-acme

go 1.13

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/ghodss/yaml v1.0.0
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/google/go-cmp v0.4.0
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.2-0.20191230164726-a31eda7afd3c
	github.com/openshift/api v0.0.0-20200803131051-87466835fcc0
	github.com/openshift/build-machinery-go v0.0.0-20200731024703-cd7e6e844b55
	github.com/openshift/client-go v0.0.0-20200729195840-c2b1adc6bed6
	github.com/openshift/library-go v0.0.0-20200807122248-f5cb4d19a4fe
	github.com/prometheus/client_golang v1.7.1
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	gopkg.in/inf.v0 v0.9.1
	k8s.io/api v0.19.0-rc.2
	k8s.io/apimachinery v0.19.0-rc.2
	k8s.io/apiserver v0.19.0-rc.2
	k8s.io/client-go v0.19.0-rc.2
	k8s.io/code-generator v0.19.0-rc.2
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-tools v0.3.1-0.20200811133417-0107350c4ee7

)

replace github.com/openshift/build-machinery-go => github.com/tnozicka/build-machinery-go v0.0.0-20200813151022-40b80b29a377
