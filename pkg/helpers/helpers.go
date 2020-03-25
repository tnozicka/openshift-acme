package helpers

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func PrivateKeyFromSecret(secret *corev1.Secret) (crypto.Signer, error) {
	keyPem, ok := secret.Data[corev1.TLSPrivateKeyKey]
	if !ok {
		return nil, fmt.Errorf("secret %s/%s is missing key %q", secret.Namespace, secret.Name, corev1.TLSPrivateKeyKey)
	}

	block, _ := pem.Decode(keyPem)
	if block == nil {
		return nil, fmt.Errorf("secret %s/%s has invalid PEM encoded private key", secret.Namespace, secret.Name)
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}
