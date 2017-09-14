package route

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-playground/log"
	"github.com/tnozicka/openshift-acme/pkg/acme"
	oapi "github.com/tnozicka/openshift-acme/pkg/openshift/api"
	acme_controller "github.com/tnozicka/openshift-acme/pkg/openshift/controllers/acme"
	"github.com/tnozicka/openshift-acme/pkg/openshift/untypedclient"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/pkg/api/unversioned"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

type ServiceID struct {
	Name      string
	Namespace string
}

type RouteController struct {
	client      v1core.CoreV1Interface
	ctx         context.Context
	acme        *acme_controller.AcmeController
	exposers    map[string]acme.ChallengeExposer
	wg          sync.WaitGroup
	selfService ServiceID
	// TODO: update IP and port in a goroutine if someone were to change them; protect by RW mutex
	selfServiceEndpointSubsets []api_v1.EndpointSubset
	watchNamespaces            []string
	resourceVersions           map[string]string // namespace => resourceVersion
}

func NewRouteController(ctx context.Context, client v1core.CoreV1Interface, acme *acme_controller.AcmeController,
	exposers map[string]acme.ChallengeExposer, selfService ServiceID, watchNamespaces []string) (rc RouteController, err error) {
	rc.client = client
	rc.acme = acme
	rc.exposers = exposers
	rc.ctx = ctx
	rc.selfService = selfService
	err = rc.UpdateSelfServiceEndpointSubsets()
	if err != nil {
		return
	}
	rc.watchNamespaces = watchNamespaces

	rc.resourceVersions = make(map[string]string)
	// we have to initialize the array here to make subsequent access race free
	for _, namespace := range watchNamespaces {
		rc.resourceVersions[namespace] = "0"
	}

	return
}

func (rc *RouteController) doWatchIteration(namespace string) error {
	var url string
	if namespace == "" {
		url = "/oapi/v1/watch/routes"
	} else {
		url = fmt.Sprintf("/oapi/v1/watch/namespaces/%s/routes", namespace)
	}
	w, err := untypedclient.Watch(rc.client.RESTClient(), url+"?resourceVersion="+rc.resourceVersions[namespace])
	if err != nil {
		return fmt.Errorf("watch failed: %s", err)
	}
	defer w.Stop()

	for {
		select {
		case <-rc.ctx.Done():
			return nil
		case rawEvent, ok := <-w.ResultChan():
			if !ok {
				return errors.New("RouteController ResultChannel closed")
			}

			var event oapi.Event
			if err := json.Unmarshal(rawEvent, &event); err != nil {
				log.Error(err)
				return fmt.Errorf("watch failed to unmarshal event: %s", err)
			}

			switch event.Type {
			case "ERROR":
				var status unversioned.Status
				if err := json.Unmarshal(event.Object, &status); err != nil {
					log.Error(err)
					return fmt.Errorf("RouteController: failed to unmarshal Status: '%s'", err)
				}

				if status.Code == 410 {
					log.Warnf("RouteController: resetting resourceVersion: caused by 'ERROR' (%#v)", status)
					rc.resourceVersions[namespace] = "0"
					// FIXME: clean db because some objects could escape getting deleted this way
					continue
				}

				err := fmt.Errorf("RouteController: unknown 'ERROR' (%s)", event.Object)
				log.Error(err)
				return err
			default:
				return fmt.Errorf("Unknown Route event '%s'", event.Type)
			case "ADDED", "MODIFIED", "DELETED":
				break
			}

			var route oapi.Route
			if err := json.Unmarshal(event.Object, &route); err != nil {
				log.Error(err)
				return fmt.Errorf("RouteController: failed to unmarshal Route: '%s'", err)
			}

			if route.Annotations["kubernetes.io/tls-acme"] != "true" {
				rc.resourceVersions[namespace] = route.ResourceVersion
				continue
			}

			log.Debugf("Type: %s", event.Type)
			// We need to check first if the route has been admitted by the router.
			// The assumption is that we wait for all ingresses
			admittedSet := false
			admittedValue := true
			for _, ingress := range route.Status.Ingress {
				for _, condition := range ingress.Conditions {
					if condition.Type == "Admitted" {
						admittedSet = true
						if condition.Status != "True" {
							admittedValue = false
						}
					}
				}
			}
			if !(admittedSet && admittedValue) {
				log.Debugf("RouteController: skipping route (not admitted) [admittedSet=%t, admittedValue=%t]", admittedSet, admittedValue)
				rc.resourceVersions[namespace] = route.ResourceVersion
				continue
			}

			switch event.Type {
			case "ADDED", "MODIFIED":
				log.Debugf("RouteController: processing route '%s'", route.Spec.Host)
				err = rc.acme.Manage(&RouteObject{
					route:                      route,
					client:                     rc.client,
					exposers:                   rc.exposers,
					SelfServiceEndpointSubsets: rc.selfServiceEndpointSubsets,
				})
				if err != nil {
					return fmt.Errorf("acme.Manage failed: %s", err)
				}
			case "DELETED":
				err = rc.acme.Done(&RouteObject{
					route:                      route,
					client:                     rc.client,
					exposers:                   rc.exposers,
					SelfServiceEndpointSubsets: rc.selfServiceEndpointSubsets,
				})
				if err != nil {
					return fmt.Errorf("acme.Done failed: %s", err)
				}
			default:
				return fmt.Errorf("Unknown Route event '%s'", event.Type)
			}

			rc.resourceVersions[namespace] = route.ResourceVersion
		}
	}
}

func (rc *RouteController) watch(namespace string) {
	defer rc.wg.Done()
	log.Infof("RouteController: watching namespace '%s'", namespace)

	for {
		select {
		case <-rc.ctx.Done():
			return
		default:
		}

		err := rc.doWatchIteration(namespace)
		if err == nil {
			break // cancelling due to ctx.Done() from doWatchIteration
		}

		log.Errorf("RouteController: doWatchIteration failed: %s", err)
		// TODO: raise error counter for health check

		// TODO: exponential backoff
		select {
		case <-rc.ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (rc *RouteController) Start() {
	rc.Wait() // make sure it can't be started twice at the same time

	for _, namespace := range rc.watchNamespaces {
		rc.wg.Add(1)
		go rc.watch(namespace)
	}

	go func() {
		rc.wg.Wait()
		log.Info("RouteController finished")
	}()
}

func (rc *RouteController) Wait() {
	rc.wg.Wait()
}

func (rc *RouteController) UpdateSelfServiceEndpointSubsets() (err error) {
	service, err := rc.client.Services(rc.selfService.Namespace).Get(rc.selfService.Name)
	if err != nil {
		return fmt.Errorf("RouteController could not find its own service: '%s'", err)
	}

	switch service.Spec.ClusterIP {
	case "":
		return errors.New("unable to detect selfServiceIP: clusterIP=''")
	case "None":
		// this is a headless service; go for endpoints directly
		// usually a case for development setups
		endpoints, err := rc.client.Endpoints(rc.selfService.Namespace).Get(rc.selfService.Name)
		if err != nil {
			return fmt.Errorf("RouteController could not find corresponding endpoints to its own service: '%s'", err)
		}
		// TODO: check if there are any subsets and make sure there are valid
		rc.selfServiceEndpointSubsets = endpoints.Subsets
	default:
		// for regular service we will use static and load-balanced ClusterIP
		endpoints, err := rc.client.Endpoints(rc.selfService.Namespace).Get(rc.selfService.Name)
		if err == nil {
			rc.selfServiceEndpointSubsets = endpoints.Subsets
		} else {
			ports := []api_v1.EndpointPort{}
			for _, svc_port := range service.Spec.Ports {
				ports = append(ports, api_v1.EndpointPort{Port: svc_port.Port})
			}
			rc.selfServiceEndpointSubsets = []api_v1.EndpointSubset{
				{
					Addresses: []api_v1.EndpointAddress{
						{
							IP: service.Spec.ClusterIP,
						},
					},
					Ports: ports,
				},
			}
		}
	}

	log.Debugf("Detected subsets for selfService: '%+v'", rc.selfServiceEndpointSubsets)

	return nil
}
