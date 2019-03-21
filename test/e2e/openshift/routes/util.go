package routes

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	routegroup "github.com/openshift/api/route"
	routev1 "github.com/openshift/api/route/v1"
	routev1clientset "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	corev1clientset "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"

	"github.com/tnozicka/openshift-acme/pkg/cert"
)

func generateCertificate(hosts []string, notBefore, notAfter time.Time) (*cert.CertPemData, error) {
	certPemData := &cert.CertPemData{}

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return certPemData, fmt.Errorf("failed to generate serial number: %s", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"openshift-acme"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		DNSNames: hosts,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %s", err)
	}

	certBuffer := bytes.NewBuffer([]byte{})
	pem.Encode(certBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certPemData.Crt = certBuffer.Bytes()

	certPemData.Key = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	return certPemData, nil
}

func WaitForRouteCondition(ctx context.Context, c routev1clientset.RouteInterface, namespace string, name string, conditions ...watchtools.ConditionFunc) (*watch.Event, error) {
	fieldSelector := fields.OneTermEqualSelector("metadata.name", name).String()
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return c.List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return c.Watch(options)
		},
	}

	precondition := func(store cache.Store) (bool, error) {
		_, exists, err := store.Get(&metav1.ObjectMeta{Namespace: namespace, Name: name})
		if err != nil {
			return true, err
		}
		if !exists {
			// We need to make sure we see the object in the cache before we start waiting for events
			// or we would be waiting for the timeout if such object didn't exist.
			return true, apierrors.NewNotFound(routegroup.Resource("routes"), name)
		}

		return false, nil
	}

	return watchtools.UntilWithSync(ctx, lw, &routev1.Route{}, precondition, conditions...)
}

func WaitForSecretCondition(ctx context.Context, c corev1clientset.SecretInterface, namespace string, name string, conditions ...watchtools.ConditionFunc) (*watch.Event, error) {
	fieldSelector := fields.OneTermEqualSelector("metadata.name", name).String()
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return c.List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return c.Watch(options)
		},
	}

	precondition := func(store cache.Store) (bool, error) {
		_, exists, err := store.Get(&metav1.ObjectMeta{Namespace: namespace, Name: name})
		if err != nil {
			return true, err
		}
		if !exists {
			// We need to make sure we see the object in the cache before we start waiting for events
			// or we would be waiting for the timeout if such object didn't exist.
			return true, apierrors.NewNotFound(corev1.Resource("secrets"), name)
		}

		return false, nil
	}

	return watchtools.UntilWithSync(ctx, lw, &corev1.Secret{}, precondition, conditions...)
}
