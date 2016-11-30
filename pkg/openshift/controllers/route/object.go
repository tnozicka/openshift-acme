package route

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-playground/log"
	"github.com/tnozicka/openshift-acme/pkg/acme"
	"github.com/tnozicka/openshift-acme/pkg/cert"
	oapi "github.com/tnozicka/openshift-acme/pkg/openshift/api"
	oschallengeexposers "github.com/tnozicka/openshift-acme/pkg/openshift/challengeexposers"
	"github.com/tnozicka/openshift-acme/pkg/openshift/untypedclient"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	kerrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func AcmeRouteHash(r oapi.Route) string {
	s := ""

	s += r.Spec.Host

	if r.Spec.Tls != nil {
		s += r.Spec.Tls.Key
		s += r.Spec.Tls.Certificate
	}

	v, found := r.Annotations["kubernetes.io/tls-acme"]
	if found {
		s += v
	}

	hash := sha256.Sum256([]byte(s))

	return base64.StdEncoding.EncodeToString(hash[:])
}

type RouteObject struct {
	route                      oapi.Route
	client                     v1core.CoreV1Interface
	selfService                ServiceID
	exposers                   map[string]acme.ChallengeExposer
	SelfServiceEndpointSubsets []api_v1.EndpointSubset
}

func (o *RouteObject) GetDomains() []string {
	return []string{o.route.Spec.Host}
}

func (o *RouteObject) GetSecretName() string {
	secretName, found := o.route.Annotations["kubernetes.io/tls-acme-secretname"]
	if !found {
		return "acme." + o.GetName()
	}
	return secretName
}

func (o *RouteObject) GetUID() string {
	return fmt.Sprintf("route/%s/%s", o.GetNamespace(), o.GetName())
}

func (o *RouteObject) GetCertificate() *cert.Certificate {
	c := &cert.Certificate{}

	if o.route.Spec.Tls != nil {
		c.Key = []byte(o.route.Spec.Tls.Key)
		c.Crt = []byte(o.route.Spec.Tls.Certificate)
	}

	return c
}

func (o *RouteObject) GetName() string {
	return o.route.Name
}

func (o *RouteObject) GetNamespace() string {
	return o.route.Namespace
}

func (o *RouteObject) GetAcmeHash() string {
	return AcmeRouteHash(o.route)
}

func (o *RouteObject) GetExposers() map[string]acme.ChallengeExposer {
	exposers := make(map[string]acme.ChallengeExposer)

	http01, found := o.exposers["http-01"]
	if found {
		routeHttp01 := oschallengeexposers.Route{
			UnderlyingExposer:          http01,
			Client:                     o.client,
			Namespace:                  o.GetNamespace(),
			SelfServiceEndpointSubsets: o.SelfServiceEndpointSubsets,
		}
		exposers["http-01"] = &routeHttp01
	}

	return exposers
}

func (o *RouteObject) UpdateCertificate(c *cert.Certificate) error {
	o.route.Annotations["kubernetes.io/tls-acme.last-update-time"] = time.Now().Format(time.RFC3339)
	name := o.GetName()
	namespace := o.GetNamespace()

	var secretExists bool
	secret, err := o.client.Secrets(namespace).Get(o.GetSecretName())
	if err != nil {
		if kerrors.IsNotFound(err) {
			secretExists = false
		} else {
			return err
		}
	} else {
		secretExists = true
	}

	// create a secret representing the certificate as well
	// with routes it is not necessary but it is consistent with how ingress works
	// also this secret can be mounted into pods for TLS passthrough
	// TODO: replace Opaque with proper type if there is one
	secret.Type = "Opaque"
	secret.Name = o.GetSecretName()
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations["kubernetes.io/tls-acme.last-update-time"] = time.Now().Format(time.RFC3339)
	secret.Annotations["kubernetes.io/tls-acme.valid-not-before"] = time.Now().Format(time.RFC3339)
	secret.Annotations["kubernetes.io/tls-acme.valid-not-after"] = time.Now().Format(time.RFC3339)
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data["tls.key"] = c.Key
	secret.Data["tls.crt"] = c.Crt

	if !secretExists {
		log.Infof("Creating new secret '%s' in namespace '%s' for route '%s'", secret.Name, secret.Namespace, name)
		_, err = o.client.Secrets(namespace).Create(secret)
	} else {
		log.Infof("Updating secret '%s' in namespace '%s' for route '%s'", secret.Name, secret.Namespace, name)
		// TODO: consider using PATCH in the future
		_, err = o.client.Secrets(namespace).Update(secret)
	}
	if err != nil {
		log.Error(err)
		return err
	}

	// update the route
	route := &o.route
	if route.Annotations == nil {
		route.Annotations = map[string]string{}
	}
	route.Annotations["kubernetes.io/tls-acme.last-update-time"] = time.Now().Format(time.RFC3339)
	secret.Annotations["kubernetes.io/tls-acme.valid-not-before"] = time.Now().Format(time.RFC3339)
	secret.Annotations["kubernetes.io/tls-acme.valid-not-after"] = time.Now().Format(time.RFC3339)
	if route.Spec.Tls == nil {
		route.Spec.Tls = &oapi.TlsConfig{}
	}
	route.Spec.Tls.Key = string(c.Key)
	route.Spec.Tls.Certificate = string(c.Crt)
	route.Annotations["kubernetes.io/tls-acme.hash"] = o.GetAcmeHash()

	url := fmt.Sprintf("/oapi/v1/namespaces/%s/routes/%s", namespace, name)
	data, err := json.Marshal(route)
	if err != nil {
		log.Error(err)
		return err
	}
	log.Infof("Updating route '%s' in namespace '%s'", name, namespace)
	// TODO: use PATCH in the future to avoid 409
	route.ResourceVersion = ""
	route.CreationTimestamp = unversioned.Time{}
	route.SelfLink = ""
	route.UID = ""
	body, err := untypedclient.Put(o.client.RESTClient(), url, data)
	if err != nil {
		return fmt.Errorf("%s; detail: '%s'", err, string(body))
	}
	log.Infof("Route '%s' in namespace '%s' UPDATED.", name, namespace)

	return nil
}
