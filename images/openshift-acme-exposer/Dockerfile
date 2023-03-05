FROM docker.io/golang:1.19.6-alpine3.17 AS builder

ARG APP=openshift-acme-exposer
WORKDIR /workspace

COPY Makefile .
COPY cmd cmd
COPY go.mod .
COPY go.sum .
COPY pkg pkg

RUN go mod download
RUN go mod vendor

RUN CGO_ENABLED=0 go build -mod=vendor -trimpath -o $APP -ldflags '-w -extldflags "-static"' github.com/tnozicka/openshift-acme/cmd/$APP

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=builder /workspace/$APP /usr/local/bin/$APP

ENTRYPOINT ["/usr/local/bin/openshift-acme-exposer"]
