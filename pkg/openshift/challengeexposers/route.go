package challengeexposers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/log"
	"github.com/tnozicka/openshift-acme/pkg/acme"
	oapi "github.com/tnozicka/openshift-acme/pkg/openshift/api"
	"github.com/tnozicka/openshift-acme/pkg/openshift/untypedclient"
	acmelib "golang.org/x/crypto/acme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	kerrors "k8s.io/client-go/pkg/api/errors"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/util/intstr"
)

type Route struct {
	UnderlyingExposer          acme.ChallengeExposer
	Client                     v1core.CoreV1Interface
	Namespace                  string
	SelfServiceEndpointSubsets []api_v1.EndpointSubset
}

func getDomainHash(domain string) string {
	hash := sha256.Sum256(bytes.TrimRight([]byte(domain), "\x00"))
	hashString := strings.ToLower(base32.HexEncoding.EncodeToString(hash[:])[:10])
	return hashString
}

func getTmpRouteName(domain string) string {
	return "acme-" + getDomainHash(domain)
}

func (r *Route) Expose(a *acmelib.Client, domain string, token string) error {
	// TODO: consider handling errors vs. logging in concurrent functions

	tmpName := getTmpRouteName(domain)
	namespace := r.Namespace

	maxTries := 10
	var wg sync.WaitGroup

	// Create temporary endpoints
	go func() {
		wg.Add(1)
		defer wg.Done()

		updateEndpoints := func(endpoints *api_v1.Endpoints) {
			endpoints.Subsets = r.SelfServiceEndpointSubsets
		}

		for i := 1; i <= maxTries; i++ {
			log.Debugf("Creating Endpoints %s/%s for exposing (%d/%d)", namespace, tmpName, i, maxTries)
			endpoints, err := r.Client.Endpoints(namespace).Get(tmpName)
			if err != nil {
				kerr, ok := err.(*kerrors.StatusError)
				if ok && kerr.Status().Code == 404 {
					// There are no endpoints present - this is good
					// (it means that previous object was properly cleaned)
					// we will create new one
					endpoints = &api_v1.Endpoints{
						ObjectMeta: api_v1.ObjectMeta{
							Name: tmpName,
						},
					}
					updateEndpoints(endpoints)

					endpoints, err = r.Client.Endpoints(namespace).Create(endpoints)
					if err != nil {
						kerr, ok := err.(*kerrors.StatusError)
						if ok && kerr.Status().Code == 409 {
							// Endpoints have been created in the meantime
							log.Warnf("route challenge exposer: creating endpoints %s/%s failed because of collision: %s", tmpName, namespace, err)
							continue
						} else {
							log.Errorf("route challenge exposer: creating endpoints %s/%s failed: %s", tmpName, namespace, err)
							return
						}
					}

					return
				} else {
					log.Errorf("route challenge exposer: reading endpoints %s/%s failed: %s", tmpName, namespace, err)
					return
				}
			}

			updateEndpoints(endpoints)

			endpoints, err = r.Client.Endpoints(namespace).Update(endpoints)
			if err != nil {
				kerr, ok := err.(*kerrors.StatusError)
				if ok && kerr.Status().Code == 409 {
					// There has been a change on endpoints
					log.Warnf("route challenge exposer: updating endpoints %s/%s failed because of collision: %s", tmpName, namespace, err)
					continue
				} else {
					log.Errorf("route challenge exposer: updating endpoints %s/%s failed: %s", tmpName, namespace, err)
					return
				}
			}

			return
		}
	}()

	// Create temporary service
	go func() {
		wg.Add(1)
		defer wg.Done()

		updateService := func(service *api_v1.Service) {
			service.Spec.ClusterIP = "None"
			service.Spec.Ports = []api_v1.ServicePort{
				{Name: "http", Protocol: "TCP", Port: 80, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 80}},
			}
		}

		for i := 1; i <= maxTries; i++ {
			log.Debugf("Creating Service %s/%s for exposing (%d/%d)", namespace, tmpName, i, maxTries)
			service, err := r.Client.Services(namespace).Get(tmpName)
			if err != nil {
				kerr, ok := err.(*kerrors.StatusError)
				if ok && kerr.Status().Code == 404 {
					// There is no service present - this is good
					// (it means that previous object was properly cleaned)
					// we will create new one
					service = &api_v1.Service{
						ObjectMeta: api_v1.ObjectMeta{
							Name: tmpName,
						},
					}
					updateService(service)

					service, err = r.Client.Services(namespace).Create(service)
					if err != nil {
						kerr, ok := err.(*kerrors.StatusError)
						if ok && kerr.Status().Code == 409 {
							// Service has been created in the meantime
							log.Warnf("route challenge exposer: creating service %s/%s failed because of collision: %s", tmpName, namespace, err)
							continue
						} else {
							log.Errorf("route challenge exposer: creating service %s/%s failed: %s", tmpName, namespace, err)
							return
						}
					}

					return
				} else {
					log.Errorf("route challenge exposer: reading service %s/%s failed: %s", tmpName, namespace, err)
					return
				}
			}

			updateService(service)

			service, err = r.Client.Services(namespace).Update(service)
			if err != nil {
				kerr, ok := err.(*kerrors.StatusError)
				if ok && kerr.Status().Code == 409 {
					// There has been a change on endpoint
					log.Warnf("route challenge exposer: updating service %s/%s failed because of collision: %s", tmpName, namespace, err)
					continue
				} else {
					log.Errorf("route challenge exposer: updating service %s/%s failed: %s", tmpName, namespace, err)
					return
				}
			}

			return
		}
	}()

	// Create temporary route
	go func() {
		wg.Add(1)
		defer wg.Done()

		typeUrl := fmt.Sprintf("/oapi/v1/namespaces/%s/routes", namespace)
		resourceUrl := fmt.Sprintf("/oapi/v1/namespaces/%s/routes/%s", namespace, tmpName)
		updateRoute := func(route *oapi.Route) {
			route.Spec.Host = domain
			route.Spec.Path = a.HTTP01ChallengePath(token)
			route.Spec.To.Kind = "Service"
			route.Spec.To.Name = tmpName
			route.Spec.To.Weight = 100
		}

		for i := 1; i <= maxTries; i++ {
			log.Debugf("Creating Route %s/%s for exposing (%d/%d)", namespace, tmpName, i, maxTries)

			var route oapi.Route
			rawRoute, err := untypedclient.Get(r.Client.RESTClient(), resourceUrl)
			if err != nil {
				kerr, ok := err.(*kerrors.StatusError)
				if ok && kerr.Status().Code == 404 {
					// There is no route present - this is good
					// (it means that previous object was properly cleaned)
					// we will create new one
					route.ObjectMeta = api_v1.ObjectMeta{
						Name: tmpName,
					}
					updateRoute(&route)

					payload, err := json.Marshal(&route)
					if err != nil {
						log.Errorf("route challenge exposer: marshaling Route failed: %s", err)
						return
					}
					rawRoute, err = untypedclient.Post(r.Client.RESTClient(), typeUrl, payload)
					if err != nil {
						kerr, ok := err.(*kerrors.StatusError)
						if ok && kerr.Status().Code == 409 {
							// Route has been created in the meantime
							log.Warnf("route challenge exposer: creating route %s/%s failed because of collision: %s", tmpName, namespace, err)
							continue
						} else {
							log.Errorf("route challenge exposer: creating route %s/%s failed: %s", tmpName, namespace, err)
							return
						}
					}

					return
				} else {
					log.Errorf("route challenge exposer: reading route %s/%s failed: %s", tmpName, namespace, err)
					return
				}
			} else {
				err := json.Unmarshal(rawRoute, &route)
				if err != nil {
					log.Errorf("route challenge exposer: unmarshaling Route %s/%s failed: %s", tmpName, namespace, err)
					return
				}
			}

			updateRoute(&route)

			payload, err := json.Marshal(route)
			if err != nil {
				log.Errorf("route challenge exposer: marshaling Route failed: %s", err)
				return
			}
			rawRoute, err = untypedclient.Patch(r.Client.RESTClient(), resourceUrl, payload)
			if err != nil {
				kerr, ok := err.(*kerrors.StatusError)
				if ok && kerr.Status().Code == 409 {
					// There has been a change on route
					log.Warnf("route challenge exposer: updating route %s/%s failed because of collision: %s", tmpName, namespace, err)
					continue
				} else {
					log.Errorf("route challenge exposer: updating route %s/%s failed: %s", tmpName, namespace, err)
					return
				}
			}

			return
		}
	}()

	wg.Wait()
	// FIXME: wait for route to be picked up by the router!!!
	time.Sleep(10 * time.Second)

	return r.UnderlyingExposer.Expose(a, domain, token)
}

func (r *Route) Remove(a *acmelib.Client, domain string, token string) error {
	// TODO: consider handling errors vs. logging in concurrent functions

	tmpName := getTmpRouteName(domain)
	namespace := r.Namespace

	var wg sync.WaitGroup

	// Remove service and endpoints
	// (endpoints are removed automatically when service is deleted)
	go func() {
		wg.Add(1)
		defer wg.Done()

		err := r.Client.Services(namespace).Delete(tmpName, &api_v1.DeleteOptions{})
		if err != nil {
			log.Errorf("route challenge exposer: deleting service '%s/%s' failed: %s", namespace, tmpName, err)
			return
		}
	}()

	// Remove route
	go func() {
		wg.Add(1)
		defer wg.Done()

		url := fmt.Sprintf("/oapi/v1/namespaces/%s/routes/%s", namespace, tmpName)
		body, err := untypedclient.Delete(r.Client.RESTClient(), url, []byte{})
		if err != nil {
			log.Errorf("route challenge exposer: deleting route '%s/%s': %s; %#v", namespace, tmpName, err, string(body))
			return
		}

	}()

	err := r.UnderlyingExposer.Remove(a, domain, token)

	wg.Wait()

	return err
}
