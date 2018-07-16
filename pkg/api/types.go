package api

const (
	TlsAcmeAnnotation                   = "kubernetes.io/tls-acme"
	TlsAcmePausedAnnotation             = "kubernetes.io/tls-acme-paused"
	AcmeAwaitingAuthzUrlAnnotation      = "kubernetes.io/tls-acme-awaiting-authorization-at-url"
	AcmeAwaitingAuthzUrlOwnerAnnotation = "kubernetes.io/tls-acme-awaiting-authorization-owner"
	// TlsSecretNameAnnotation describes the annotation used to determine the
	// name of the secret used to store the TLS certificate obtained
	TlsSecretNameAnnotation = "kubernetes.io/tls-acme-secret-name"
)
