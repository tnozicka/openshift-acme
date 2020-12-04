FROM registry.svc.ci.openshift.org/ocp/builder:rhel-8-golang-1.15-openshift-4.7 as builder
WORKDIR /go/src/github.com/tnozicka/openshift-acme
COPY . .
RUN make build GO_BUILD_PACKAGES=./cmd/openshift-acme-exposer --warn-undefined-variables

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
COPY --from=builder /go/src/github.com/tnozicka/openshift-acme/openshift-acme-exposer /usr/bin/openshift-acme-exposer
ENTRYPOINT ["/usr/bin/openshift-acme-exposer"]
