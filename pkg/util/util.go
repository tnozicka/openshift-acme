package util

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"reflect"
	"strings"

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/tnozicka/openshift-acme/pkg/api"
	routeutil "github.com/tnozicka/openshift-acme/pkg/route"
)

func IsTemporary(obj metav1.Object) bool {
	v, ok := obj.GetLabels()[api.AcmeTemporaryLabel]
	if ok && v == "true" {
		return true
	}

	return false
}

func IsManaged(obj metav1.Object, key string) bool {
	v, ok := obj.GetAnnotations()[key]
	if !ok || v != "true" {
		return false
	}

	// ignore temporary routes that inherit the TlsAcmeAnnotation from the real route
	if IsTemporary(obj) {
		return false
	}

	return true
}

func CertificateFromPEM(crt []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(crt)
	if block == nil {
		return nil, errors.New("no data found in Crt")
	}

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return certificate, nil
}

func RouteAdmittedFunc(event watch.Event) (bool, error) {
	switch event.Type {
	case watch.Added, watch.Modified:
		r := event.Object.(*routev1.Route)
		if routeutil.IsAdmitted(r) {
			return true, nil
		}
		return false, nil
	default:
		return true, fmt.Errorf("unexpected event - type: %s, obj: %#v", event.Type, event.Object)
	}
}

func RouteTLSChangedFunc(tls *routev1.TLSConfig) func(watch.Event) (bool, error) {
	return func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Added, watch.Modified:
			r := event.Object.(*routev1.Route)
			if !reflect.DeepEqual(r.Spec.TLS, tls) {
				return true, nil
			}

			return false, nil
		default:
			return true, fmt.Errorf("unexpected event - type: %s, obj: %#v", event.Type, event.Object)
		}
	}
}

func SecretDataChangedFunc(data map[string][]byte) func(watch.Event) (bool, error) {
	return func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Added, watch.Modified:
			secret := event.Object.(*corev1.Secret)
			if !reflect.DeepEqual(secret.Data, data) {
				return true, nil
			}

			return false, nil
		default:
			return true, fmt.Errorf("unexpected event - type: %s, obj: %#v", event.Type, event.Object)
		}
	}
}

func FirstNLines(s string, n int) string {
	if n < 1 {
		return ""
	}

	lines := strings.SplitN(s, "\n", n+1)
	c := len(lines)
	if c > n {
		c = n
	}

	return strings.Join(lines[:c], "\n")
}

func MaxNCharacters(s string, n int) string {
	if n < 1 {
		return ""
	}

	if len(s) <= n {
		return s
	}

	return s[:n]
}
