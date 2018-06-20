package route

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	routev1 "github.com/openshift/api/route/v1"
	routeclientset "github.com/openshift/client-go/route/clientset/versioned"
	routeutil "github.com/tnozicka/openshift-acme/pkg/route"
	"github.com/tnozicka/openshift-acme/pkg/util"
	"golang.org/x/crypto/acme"
	corev1 "k8s.io/api/core/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"

	"github.com/tnozicka/openshift-acme/pkg/acme/challengeexposers"
	"github.com/tnozicka/openshift-acme/pkg/api"
)

const (
	RouterAdmitTimeout     = 30 * time.Second
	GeneratedAnnotation    = "acme.openshift.io/generated"
	maxNameLength          = 63
	randomLength           = 5
	maxGeneratedNameLength = maxNameLength - randomLength
)

// TODO: move to a package
func GetControllerRef(o *metav1.ObjectMeta) *metav1.OwnerReference {
	for _, ref := range o.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return &ref
		}
	}
	return nil
}

type Exposer struct {
	underlyingExposer challengeexposers.Interface
	routeClientset    routeclientset.Interface
	kubeClientset     kubernetes.Interface
	recorder          record.EventRecorder
	exposerIP         string
	exposerPort       int32
	selfNamespace     string
	selfSelector      map[string]string
	route             *routev1.Route
	labels            map[string]string
}

var _ challengeexposers.Interface = &Exposer{}

func NewExposer(underlyingExposer challengeexposers.Interface,
	routeClientset routeclientset.Interface,
	kubeClientset kubernetes.Interface,
	recorder record.EventRecorder,
	exposerIP string,
	exposerPort int32,
	selfNamespace string,
	selfSelector map[string]string,
	route *routev1.Route,
	labels map[string]string,
) *Exposer {
	return &Exposer{
		underlyingExposer: underlyingExposer,
		routeClientset:    routeClientset,
		kubeClientset:     kubeClientset,
		recorder:          recorder,
		exposerIP:         exposerIP,
		exposerPort:       exposerPort,
		selfNamespace:     selfNamespace,
		selfSelector:      selfSelector,
		route:             route,
		labels:            labels,
	}
}

func (e *Exposer) cleanupTmpObjects() error {
	// All ownerRefs are bound to the temporary Route, so it's enough to delete only it
	// and GC will take care of the rest.
	routes, err := e.routeClientset.RouteV1().Routes(e.route.Namespace).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromValidatedSet(labels.Set{
			api.ExposerForLabelName: string(e.route.UID),
		}).String(),
	})
	if err != nil {
		if kapierrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("failed to list old exposing Routes: %v", err)
	}

	var routesToDelete []*routev1.Route
	for _, route := range routes.Items {
		controllerRef := metav1.GetControllerOf(&route)
		if controllerRef == nil {
			glog.V(2).Infof("Ignoring Route %s/%s with missing controllerRef.", route.Namespace, route.Name)
			continue
		}

		if controllerRef.UID != e.route.UID {
			glog.V(2).Infof("Ignoring Route %s/%s with unmatching controllerRef.", route.Namespace, route.Name)
			continue
		}

		routesToDelete = append(routesToDelete, &route)
	}

	for _, route := range routes.Items {
		if route.DeletionTimestamp != nil {
			continue
		}

		err = e.routeClientset.RouteV1().Routes(e.route.Namespace).Delete(route.Name, &metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{
				UID: &route.UID,
			},
		})
		if err != nil && !(kapierrors.IsNotFound(err) || kapierrors.IsConflict(err)) {
			return fmt.Errorf("failed to delete old exposing Route %s/%s: %v", route.Namespace, route.Name, err)
		}
	}

	return nil
}

func createTemporaryExposerName(routeName string) string {
	baseName := fmt.Sprintf("%s-%s-", routeName, api.ForwardingRouteSuffix)

	// We need to normalize the name from possible DNSSubdomain (allowed for Route's name)
	// to DNSLabel (allowed for regular Kubernetes objects)
	baseName = strings.Replace(baseName, ".", "-", -1)

	if len(baseName) > maxGeneratedNameLength {
		baseName = baseName[:maxGeneratedNameLength]
	}

	return fmt.Sprintf("%s%s", baseName, utilrand.String(randomLength))
}

func (e *Exposer) Expose(c *acme.Client, domain string, token string) error {
	err := e.cleanupTmpObjects()
	if err != nil {
		return fmt.Errorf("failed to cleanup temporary exposing objects before creating new ones: %v", err)
	}

	trueVal := true

	exposerName := createTemporaryExposerName(e.route.Name)

	exposerLabels := map[string]string{
		api.ExposerLabelName:    "true",
		api.ExposerForLabelName: string(e.route.UID),
	}

	exposerLabels = labels.Merge(labels.Set(e.labels), labels.Set(exposerLabels))

	// Route can only point to a Service in the same namespace
	// but we need to redirect ACME challenge to this controller
	// usually deployed in a different namespace.
	// We avoid this limitation by creating a forwarding service and manual endpoints if needed.

	/*
		Route

		Create Route to accept the traffic for ACME challenge.
	*/
	routeDef := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name: exposerName,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: routev1.SchemeGroupVersion.String(),
					Kind:       "Route",
					Name:       e.route.Name,
					UID:        e.route.UID,
					Controller: &trueVal,
				},
			},
			Labels: exposerLabels,
		},
		Spec: routev1.RouteSpec{
			Host: domain,
			Path: c.HTTP01ChallengePath(token),
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: exposerName,
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationEdge,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyAllow,
			},
		},
	}
	// TODO: Remove after https://github.com/openshift/origin/issues/14950 is fixed in all supported OpenShift versions
	if e.route.Spec.TLS != nil && e.route.Spec.TLS.InsecureEdgeTerminationPolicy == routev1.InsecureEdgeTerminationPolicyRedirect {
		routeDef.Spec.TLS.InsecureEdgeTerminationPolicy = routev1.InsecureEdgeTerminationPolicyRedirect
	}

	route, err := e.routeClientset.RouteV1().Routes(e.route.Namespace).Create(routeDef)
	if err != nil {
		return fmt.Errorf("failed to create exposing Route %s/%s: %v", routeDef.Namespace, routeDef.Name, err)
	}

	ownerRefToExposingRoute := metav1.OwnerReference{
		APIVersion: corev1.SchemeGroupVersion.String(),
		Kind:       "Route",
		Name:       route.Name,
		UID:        route.UID,
	}

	/*
	   Service
	*/
	serviceDef := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            exposerName,
			OwnerReferences: []metav1.OwnerReference{ownerRefToExposingRoute},
			Labels:          exposerLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
		},
	}

	// We need to avoid requiring "endpoints/restricted" for regular user in single-namespace use case.
	unprivilegedSameNamespace := e.route.Namespace == e.selfNamespace && e.selfSelector != nil

	// If we are in the same namespace as the controller, and self selector is set, point it directly to the pod using a selector.
	// The selector shall be unique to this pod.
	if unprivilegedSameNamespace {
		serviceDef.Spec.Selector = e.selfSelector
		serviceDef.Spec.Ports = []corev1.ServicePort{
			{
				Name: "http",
				// Port that the controller http-01 exposer listens on
				Port:     e.exposerPort,
				Protocol: corev1.ProtocolTCP,
			},
		}
		glog.V(4).Infof("Using unprivileged traffic redirection for exposing Service %s/%s", e.route.Namespace, serviceDef.Name)
	}

	service, err := e.kubeClientset.CoreV1().Services(e.route.Namespace).Create(serviceDef)
	if err != nil {
		return fmt.Errorf("failed to create exposing Service %s/%s: %v", serviceDef.Namespace, serviceDef.Name, err)
	}

	if !unprivilegedSameNamespace {
		/*
			Endpoints

			Create endpoints which can point any namespace.
		*/
		endpointsDef := &corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:            service.Name,
				OwnerReferences: []metav1.OwnerReference{ownerRefToExposingRoute},
				Labels:          exposerLabels,
			},
			Subsets: []corev1.EndpointSubset{
				{
					Addresses: []corev1.EndpointAddress{
						{
							IP: e.exposerIP,
						},
					},
					Ports: []corev1.EndpointPort{
						{
							Name: "http",
							// Port that the controller http-01 exposer listens on
							Port:     e.exposerPort,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			},
		}
		_, err = e.kubeClientset.CoreV1().Endpoints(e.route.Namespace).Create(endpointsDef)
		if err != nil {
			return fmt.Errorf("failed to create exposing Endpoints %s/%s: %v", e.route.Namespace, endpointsDef.Name, err)
		}
	}

	glog.V(4).Infof("Waiting for exposing route %s/%s to be admitted.", route.Namespace, route.Name)

	if !routeutil.IsAdmitted(route) {
		// TODO: switch to informer to avoid broken watches
		watcher, err := e.routeClientset.RouteV1().Routes(e.route.Namespace).Watch(metav1.SingleObject(route.ObjectMeta))
		if err != nil {
			return fmt.Errorf("failed to create watcher for Route %s/%s: %v", e.route.Namespace, e.route.Name, err)
		}

		_, err = watch.Until(RouterAdmitTimeout, watcher, func(event watch.Event) (bool, error) {
			switch event.Type {
			case watch.Modified:
				exposingRoute := event.Object.(*routev1.Route)
				if routeutil.IsAdmitted(exposingRoute) {
					return true, nil
				}

				return false, nil
			default:
				return true, fmt.Errorf("unexpected event type %q while waiting for Route %s/%s to be admitted",
					event.Type, route.Namespace, route.Name)
			}
		})
		if err != nil {
			return fmt.Errorf("exceeded timeout %v while waiting for Route %s/%s to be admitted: %v", RouterAdmitTimeout, route.Namespace, route.Name, err)
		}
	}
	glog.V(4).Infof("Exposing route %s/%s has been admitted. Ingresses: %#v", route.Namespace, route.Name, route.Status.Ingress)

	err = e.underlyingExposer.Expose(c, domain, token)
	if err != nil {
		return fmt.Errorf("failed to expose challenge for Route %s/%s: ", e.route.Namespace, e.route.Name)
	}

	// We need to wait for Route to be accessible on the Router because because Route can be admitted but not exposed yet.
	glog.V(4).Infof("Waiting for route %s/%s to be exposed on the router.", route.Namespace, route.Name)

	url := "http://" + domain + c.HTTP01ChallengePath(token)
	key, err := c.HTTP01ChallengeResponse(token)
	if err != nil {
		return fmt.Errorf("failed to compute key: %v", err)
	}
	// FIXME: this can DOS the workers and needs to become asynchronous using the queue
	err = wait.ExponentialBackoff(
		wait.Backoff{
			Duration: 1 * time.Second,
			Factor:   1.3,
			Jitter:   0.2,
			Steps:    22,
		},
		func() (bool, error) {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			}
			client := &http.Client{Transport: tr}

			response, err := client.Get(url)
			if err != nil {
				glog.Warningf("Failed to GET %q: %v", url, err)
				return false, nil
			}

			defer response.Body.Close()

			// No response should be longer that this, we need to prevent against DoS
			buffer := make([]byte, 2048)
			n, err := response.Body.Read(buffer)
			if err != nil && err != io.EOF {
				glog.Warningf("Failed to read response body into buffer: %v", err)
				return false, nil
			}
			body := string(buffer[:n])

			if response.StatusCode != http.StatusOK {
				glog.V(3).Infof("Failed to GET %q: %s: %s", url, response.Status, util.FirstNLines(util.MaxNCharacters(body, 160), 5))
				return false, nil
			}

			if body != key {
				glog.V(3).Infof("Key for route %s/%s is not yet exposed.", route.Namespace, route.Name)
				return false, nil
			}

			return true, nil
		},
	)
	if err != nil {
		e.recorder.Event(e.route, "Controller failed to verify that exposing Route is accessible. It will continue with ACME validation but chances are that either exposing failed or your domain can't be reached from inside the cluster.", corev1.EventTypeWarning, "ExposingRouteNotVerified")
	} else {
		glog.V(4).Infof("Exposing Route %s/%s is accessible and contains correct response.", route.Namespace, route.Name)
	}

	return nil
}

func (e *Exposer) Remove(domain string) error {
	var err error
	var errs []error

	err = e.cleanupTmpObjects()
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to cleanup temporary exposing objects: %v", err))
	}

	err = e.underlyingExposer.Remove(domain)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to remove domain and token for Route %s/%s from underlying exposer: %v", e.route.Namespace, e.route.Name, err))
	}

	return utilerrors.NewAggregate(errs)
}
