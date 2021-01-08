#!/usr/bin/make -f
.PHONY: all
all: build


GO_BUILD_PACKAGES :=./cmd/...
GO_TEST_PACKAGES :=./cmd/... ./pkg/...

CONTROLLER_NAMESPACE :=acme-controller

IMAGE_REF :=quay.io/tnozicka/openshift-acme
CONTROLLER_IMAGE :=$(IMAGE_REF):controller
EXPOSER_IMAGE :=$(IMAGE_REF):exposer
OPERATOR_IMAGE :=$(IMAGE_REF):operator

YQ :=$(GO) run github.com/mikefarah/yq
OLM_CSV_FILES :=$(wildcard deploy/olm/*/*.clusterserviceversion.yaml)

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
$(call build-image,openshift-acme-controller,$(CONTROLLER_IMAGE),./images/openshift-acme-controller/Dockerfile,.)
$(call build-image,openshift-acme-exposer,$(EXPOSER_IMAGE), ./images/openshift-acme-exposer/Dockerfile,.)
$(call build-image,openshift-acme-operator,$(OPERATOR_IMAGE), ./images/openshift-acme-operator/Dockerfile,.)

# This will call a macro called "add-bindata" which will generate bindata specific targets based on the parameters:
# $0 - macro name
# $1 - target suffix
# $2 - input dirs
# $3 - prefix
# $4 - pkg
# $5 - output
# It will generate targets {update,verify}-bindata-$(1) logically grouping them in unsuffixed versions of these targets
# and also hooked into {update,verify}-generated for broader integration.
$(call add-bindata,v1.0.0,./bindata/operator/target_v1.0.0/...,bindata/operator,target_v100,pkg/controller/operator/assets/target_v100/bindata.go)

# $1 - target name
# $2 - apis
# $3 - manifests
# $4 - output
$(call add-crd-gen,operator,./pkg/api/acme/v1,./pkg/api/acme/v1,./pkg/api/acme/v1)


CODEGEN_PKG ?=./vendor/k8s.io/code-generator
CODEGEN_HEADER_FILE ?=/dev/null
CODEGEN_APIS_PACKAGE ?=$(GO_PACKAGE)/pkg/api
CODEGEN_GROUPS_VERSIONS ?="acme/v1"
define run-codegen
	GOPATH=$(GOPATH) $(GO) run $(GO_MOD_FLAGS) "$(CODEGEN_PKG)/cmd/$(1)" --go-header-file='$(CODEGEN_HEADER_FILE)' $(2)

endef

define run-deepcopy-gen
	$(call run-codegen,deepcopy-gen,--input-dirs='github.com/tnozicka/openshift-acme/pkg/api/acme/v1' --output-file-base='zz_generated.deepcopy' --bounding-dirs='github.com/tnozicka/openshift-acme/pkg/api/' $(1))

endef

define run-client-gen
	$(call run-codegen,client-gen,--clientset-name=versioned --input-base="./" --input='github.com/tnozicka/openshift-acme/pkg/api/acme/v1' --output-package='github.com/tnozicka/openshift-acme/pkg/client/acme/clientset' $(1))

endef

define run-lister-gen
	$(call run-codegen,lister-gen,--input-dirs='github.com/tnozicka/openshift-acme/pkg/api/acme/v1' --output-package='github.com/tnozicka/openshift-acme/pkg/client/acme/listers' $(1))

endef

define run-informer-gen
	$(call run-codegen,informer-gen,--input-dirs='github.com/tnozicka/openshift-acme/pkg/api/acme/v1' --output-package='github.com/tnozicka/openshift-acme/pkg/client/acme/informers' --versioned-clientset-package "github.com/tnozicka/openshift-acme/pkg/client/acme/clientset/versioned" --listers-package="github.com/tnozicka/openshift-acme/pkg/client/acme/listers" $(1))

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

run-render =$(GO) run ./cmd/openshift-acme-operator render target --controller-image=$(CONTROLLER_IMAGE) --exposer-image=$(EXPOSER_IMAGE) --namespace=$(CONTROLLER_NAMESPACE) $(1)
define render-deploy-files
	mkdir "$(1)"/{cluster-wide,single-namespace,specific-namespaces} 2>/dev/null || true
	$(call run-render,--output-dir="$(1)"/cluster-wide --cluster-wide=true )
	$(call run-render,--output-dir="$(1)"/single-namespace --cluster-wide=false )
	$(call run-render,--output-dir="$(1)"/specific-namespaces --cluster-wide=false --additional-namespace=foo --additional-namespace=bar )

endef

verify-deploy-files: TMP_DIR := $(shell mktemp -d)
verify-deploy-files:
	$(call render-deploy-files,$(TMP_DIR))
	set -eu; for d in $$( ls $(TMP_DIR) ); do diff -Naup "$(TMP_DIR)"/$${d} ./deploy/$${d}; done

.PHONY: verify-deploy-files

update-deploy-files:
	$(call render-deploy-files,./deploy)
.PHONY: update-deploy-files

define run-olm-csv-injection
	$(YQ) ea '\
select(fi==0).spec.install.spec.deployment.name = select(fi==1).metadata.name | \
select(fi==0).spec.install.spec.deployment.spec = select(fi==1).spec | \
( ( select(fi==0).spec.install.spec.deployment.spec.template.spec.containers[0].env[] | select(.name == "ACME_CONTROLLER_IMAGE") ).value = ( select(fi=0).spec.relatedImages[] | select(.name == "acme-controller") ).image ) | select(fileIndex == 0)' \
deploy/olm/0.9/acme-operator.clusterserviceversion.yaml deploy/operator/50_deployment.yaml


endef

verify-olm: TMP_DIR := $(shell mktemp -d)
verify-olm:
#	$(call run-olm-csv-injection,$(TMP_DIR))
#	set -eu; for d in $$( ls $(TMP_DIR) ); do diff -Naup "$(TMP_DIR)"/$${d} ./deploy/$${d}; done
.PHONY: verify-olm

update-olm:
	$(for f,$(OLM_CSV_FILES),$(call run-olm-csv-injection,$(f)))
.PHONY: update-olm

verify: verify-deploy-files verify-codegen verify-olm
.PHONY: verify

update: update-deploy-files update-codegen update-olm
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
