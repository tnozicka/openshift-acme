#!/usr/bin/make -f
.PHONY: all
all: build


GO_BUILD_PACKAGES :=./cmd/...
GO_TEST_PACKAGES :=./cmd/... ./pkg/...

IMAGE_REGISTRY :=quay.io

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/deps.mk \
	targets/openshift/images.mk \
)

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target suffix
# $2 - Dockerfile path
# $3 - context directory for image build
# It will generate target "image-$(1)" for builing the image an binding it as a prerequisite to target "images".
$(call build-image,openshift-acme-controller,$(IMAGE_REGISTRY)/tnozicka/openshift-acme:controller,./images/openshift-acme-controller/Dockerfile,.)
$(call build-image,openshift-acme-exposer,$(IMAGE_REGISTRY)/tnozicka/openshift-acme:exposer, ./images/openshift-acme-exposer/Dockerfile,.)


verify-deploy-files:
	hack/diff-deploy-files.sh $(shell mktemp -d)
.PHONY: verify-deploy-files

verify: verify-deploy-files
.PHONY: verify

update-deploy-files:
	mv ./deploy/.diffs/* $(shell mktemp -d) || true
	hack/diff-deploy-files.sh ./deploy/.diffs
.PHONY: update-deploy-files

update: update-deploy-files
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
