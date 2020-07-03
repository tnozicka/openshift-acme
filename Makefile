#!/usr/bin/make -f
.PHONY: all
all: build


GO_BUILD_PACKAGES :=./cmd/...
GO_TEST_PACKAGES :=./cmd/... ./pkg/...

IMAGE_REGISTRY :=quay.io

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/bindata.mk \
	targets/openshift/deps.mk \
	targets/openshift/images.mk \
	targets/openshift/crd-schema-gen.mk \
)

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target suffix
# $2 - Dockerfile path
# $3 - context directory for image build
# It will generate target "image-$(1)" for builing the image an binding it as a prerequisite to target "images".
$(call build-image,openshift-acme-controller,$(IMAGE_REGISTRY)/tnozicka/openshift-acme:controller,./images/openshift-acme-controller/Dockerfile,.)
$(call build-image,openshift-acme-exposer,$(IMAGE_REGISTRY)/tnozicka/openshift-acme:exposer, ./images/openshift-acme-exposer/Dockerfile,.)

# This will call a macro called "add-bindata" which will generate bindata specific targets based on the parameters:
# $0 - macro name
# $1 - target suffix
# $2 - input dirs
# $3 - prefix
# $4 - pkg
# $5 - output
# It will generate targets {update,verify}-bindata-$(1) logically grouping them in unsuffixed versions of these targets
# and also hooked into {update,verify}-generated for broader integration.
$(call add-bindata,v1.0.0,./bindata/v1.0.0/...,bindata,v100_0_assets,pkg/controller/operator/v100_00_assets/bindata.go)

# $1 - target name
# $2 - apis
# $3 - manifests
# $4 - output
$(call add-crd-gen,operator,./pkg/api/operator/v1,./pkg/api/operator/v1,./pkg/api/operator/v1)


CODEGEN_PKG ?=./vendor/k8s.io/code-generator
CODEGEN_HEADER_FILE ?=/dev/null
CODEGEN_APIS_PACKAGE ?=$(GO_PACKAGE)/pkg/api
CODEGEN_GROUPS_VERSIONS ?="operator/v1"
define run-codegen
	GOPATH=$(GOPATH) $(GO) run "$(CODEGEN_PKG)/cmd/$(1)" --go-header-file='$(CODEGEN_HEADER_FILE)' $(2)

endef

define run-deepcopy-gen
	$(call run-codegen,deepcopy-gen,--input-dirs='github.com/tnozicka/openshift-acme/pkg/api/operator/v1' --output-file-base='zz_generated.deepcopy' --bounding-dirs='github.com/tnozicka/openshift-acme/pkg/api/' $(1))

endef

define run-client-gen
	$(call run-codegen,client-gen,--clientset-name=versioned --input-base="./" --input='github.com/tnozicka/openshift-acme/pkg/api/operator/v1' --output-package='github.com/tnozicka/openshift-acme/pkg/client/operator/clientset' $(1))

endef

define run-lister-gen
	$(call run-codegen,lister-gen,--input-dirs='github.com/tnozicka/openshift-acme/pkg/api/operator/v1' --output-package='github.com/tnozicka/openshift-acme/pkg/client/operator/listers' $(1))

endef

define run-informer-gen
	$(call run-codegen,informer-gen,--input-dirs='github.com/tnozicka/openshift-acme/pkg/api/operator/v1' --output-package='github.com/tnozicka/openshift-acme/pkg/client/operator/informers' $(1))

endef

update-codegen:
	$(call run-deepcopy-gen,)
	$(call run-client-gen,)
	$(call run-lister-gen,)
	$(call run-informer-gen,)
.PHONY: update-codegen

verify-codegen:
	$(call run-deepcopy-gen,--verify-only)
	$(call run-client-gen,--verify-only)
	$(call run-lister-gen,--verify-only)
	$(call run-informer-gen,--verify-only)
.PHONY: verify-codegen


verify-deploy-files:
	hack/diff-deploy-files.sh $(shell mktemp -d)
.PHONY: verify-deploy-files

update-deploy-files:
	mv ./deploy/.diffs/* $(shell mktemp -d) || true
	hack/diff-deploy-files.sh ./deploy/.diffs
.PHONY: update-deploy-files


verify: verify-deploy-files verify-codegen
.PHONY: verify

update: update-deploy-files update-codegen
.PHONY: update

test-e2e: export E2E_DOMAIN ?=$(shell oc get ingresses.config.openshift.io cluster --template='{{.spec.domain}}')
test-e2e: export E2E_CONTROLLER_NAMESPACE?=acme-controller
test-e2e: export E2E_FIXED_NAMESPACE?=
test-e2e: export E2E_ARGS :=-args -ginkgo.progress -ginkgo.v
test-e2e: export E2E_JUNIT ?=
#test-e2e: export E2E_FIXED_NAMESPACE:=$(E2E_FIXED_NAMESPACE)
test-e2e: GO_TEST_PACKAGES:=./test/e2e/openshift
# FIXME: needs a change in openshift/build-machinery-go
test-e2e: GO_TEST_PACKAGES+= $(E2E_ARGS)
test-e2e: GO_TEST_FLAGS:=-v
test-e2e: test-unit
test-e2e:
.PHONY: test-extended

ci-test-e2e-cluster-wide:
	$(MAKE) --no-print-directory test-e2e E2E_CONTROLLER_NAMESPACE:=acme-controller E2E_FIXED_NAMESPACE:=
.PHONY: ci-test-e2e-cluster-wide

ci-test-e2e-single-namespace:
	$(MAKE) --no-print-directory test-e2e E2E_CONTROLLER_NAMESPACE:=acme-controller E2E_FIXED_NAMESPACE:=acme-controller
.PHONY: ci-test-e2e-single-namespace

ci-test-e2e-specific-namespaces:
	$(MAKE) --no-print-directory test-e2e E2E_CONTROLLER_NAMESPACE:=acme-controller E2E_FIXED_NAMESPACE:=acme-controller
	$(MAKE) --no-print-directory test-e2e E2E_CONTROLLER_NAMESPACE:=acme-controller E2E_FIXED_NAMESPACE:=test
.PHONY: ci-test-e2e-specific-namespaces
