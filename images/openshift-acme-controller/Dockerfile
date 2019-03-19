FROM openshift/origin-release:golang-1.10 as bin
WORKDIR /go/src/github.com/tnozicka/openshift-acme
COPY . .
RUN make build

FROM centos:7
COPY --from=bin /go/src/github.com/tnozicka/openshift-acme/openshift-acme-controller /usr/bin/openshift-acme-controller
ENTRYPOINT ["/usr/bin/openshift-acme-controller"]
