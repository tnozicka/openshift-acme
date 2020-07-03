package api

import (
	"k8s.io/apimachinery/pkg/labels"
)

const (
	ForwardingRouteSuffix = "acme"
	ExposerLabelName      = "acme.openshift.io/exposer"
	ExposerForLabelName   = "acme.openshift.io/exposer-for"
	ManagedDataHash       = "acme.openshift.io/managed-data-hash"
)

type AcmeState string

const (
	AcmeStateNeedsCert       = "NeedsCertificate"
	AcmeStateWaitingForAuthz = "WaitingForAuthz"
	AcmeStateOk              = "OK"
)

var (
	AccountLabelSet = labels.Set{
		"managed-by": "openshift-acme",
		"type":       "CertIssuer",
	}
)
