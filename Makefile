#!/usr/bin/make -f
.PHONY: all
all: build


GO_BUILD_PACKAGES :=./cmd/...
GO_TEST_PACKAGES :=./cmd/... ./pkg/...

# we intentionaly don't specify this value because test are making changes to the cluster so we wan't user to configure it explicitely
GO_ET_KUBECONFIG :="<unspecified>"
TEST_FLAGS :=-ginkgo.v
GO_ET_DOMAIN :=""

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/library-go/alpha-build-machinery/make/, \
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
$(call build-image,openshift-acme-controller,./images/openshift-acme-controller/Dockerfile,.)


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


test-extended:
	go test $(GOFLAGS) ./test/e2e/openshift -args $(TEST_FLAGS)
.PHONY: test-extended
