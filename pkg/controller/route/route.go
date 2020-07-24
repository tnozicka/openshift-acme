package route

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base32"
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/ghodss/yaml"
	"golang.org/x/crypto/acme"
	"gopkg.in/inf.v0"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apierrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	routev1 "github.com/openshift/api/route/v1"
	routeclientset "github.com/openshift/client-go/route/clientset/versioned"
	_ "github.com/openshift/client-go/route/clientset/versioned/scheme"

	"github.com/tnozicka/openshift-acme/pkg/api"
	"github.com/tnozicka/openshift-acme/pkg/cert"
	"github.com/tnozicka/openshift-acme/pkg/controllerutils"
	"github.com/tnozicka/openshift-acme/pkg/helpers"
	kubeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/kube"
	routeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/route"
	routeutil "github.com/tnozicka/openshift-acme/pkg/route"
	"github.com/tnozicka/openshift-acme/pkg/util"
)

const (
	ControllerName           = "openshift-acme-controller"
	ExposerFileKey           = "exposer-file"
	RenewalStandardDeviation = 1
	RenewalMean              = 0
	AcmeTimeout              = 60 * time.Second
	// BackoffGCInterval is the time that has to pass before next iteration of backoff GC is run
	BackoffGCInterval = 1 * time.Minute
)

var (
	KeyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = routev1.SchemeGroupVersion.WithKind("Route")
)

type RouteController struct {
	annotation               string
	certOrderBackoffInitial  time.Duration
	certOrderBackoffMax      time.Duration
	certDefaultRSAKeyBitSize int
	exposerImage             string
	controllerNamespace      string

	kubeClient                 kubernetes.Interface
	kubeInformersForNamespaces kubeinformers.Interface

	routeClient                 routeclientset.Interface
	routeInformersForNamespaces routeinformers.Interface

	cachesToSync []cache.InformerSynced

	recorder record.EventRecorder

	queue                workqueue.RateLimitingInterface
	routesToSecretsQueue workqueue.RateLimitingInterface
}

func NewRouteController(
	annotation string,
	certOrderBackoffInitial time.Duration,
	certOrderBackoffMax time.Duration,
	certDefaultRSAKeyBitSize int,
	exposerImage string,
	controllerNamespace string,
	kubeClient kubernetes.Interface,
	kubeInformersForNamespaces kubeinformers.Interface,
	routeClient routeclientset.Interface,
	routeInformersForNamespaces routeinformers.Interface,
) *RouteController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})

	rc := &RouteController{
		annotation:               annotation,
		certOrderBackoffInitial:  certOrderBackoffInitial,
		certOrderBackoffMax:      certOrderBackoffMax,
		certDefaultRSAKeyBitSize: certDefaultRSAKeyBitSize,
		exposerImage:             exposerImage,
		controllerNamespace:      controllerNamespace,

		kubeClient:                 kubeClient,
		kubeInformersForNamespaces: kubeInformersForNamespaces,

		routeClient:                 routeClient,
		routeInformersForNamespaces: routeInformersForNamespaces,

		recorder: eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: ControllerName}),

		queue:                workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		routesToSecretsQueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	if len(routeInformersForNamespaces.Namespaces()) < 1 {
		panic("no namespace set up")
	}

	for _, namespace := range routeInformersForNamespaces.Namespaces() {
		klog.V(4).Infof("Setting up route informers for namespace %q", namespace)

		informers := routeInformersForNamespaces.InformersFor(namespace)

		informers.Route().V1().Routes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    rc.addRoute,
			UpdateFunc: rc.updateRoute,
			DeleteFunc: rc.deleteRoute,
		})
		rc.cachesToSync = append(rc.cachesToSync, informers.Route().V1().Routes().Informer().HasSynced)
	}

	if len(kubeInformersForNamespaces.Namespaces()) < 1 {
		panic("no namespace set up")
	}

	for _, namespace := range kubeInformersForNamespaces.Namespaces() {
		klog.V(4).Infof("Setting up kube informers for namespace %q", namespace)

		informers := kubeInformersForNamespaces.InformersFor(namespace)

		informers.Core().V1().Secrets().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: rc.updateSecret,
			DeleteFunc: rc.deleteSecret,
		})
		rc.cachesToSync = append(rc.cachesToSync, informers.Core().V1().Secrets().Informer().HasSynced)

		// FIXME: requeue on exposer objects
		rc.cachesToSync = append(rc.cachesToSync, informers.Core().V1().Services().Informer().HasSynced)

		informers.Apps().V1().ReplicaSets().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    rc.addReplicaSet,
			UpdateFunc: rc.updateReplicaSet,
			DeleteFunc: rc.deleteReplicaSet,
		})
		rc.cachesToSync = append(rc.cachesToSync, informers.Apps().V1().ReplicaSets().Informer().HasSynced)

		// We need to watch CM for global and local issuers
		rc.cachesToSync = append(rc.cachesToSync, informers.Core().V1().ConfigMaps().Informer().HasSynced)

		// We need to watch LimitRanges to respect Min and Max values on exposer pods
		rc.cachesToSync = append(rc.cachesToSync, informers.Core().V1().LimitRanges().Informer().HasSynced)
	}

	return rc
}

func (rc *RouteController) enqueueRoute(route *routev1.Route) {
	key, err := KeyFunc(route)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for route object: %w", err))
		return
	}

	rc.queue.Add(key)

	_, ok := GetSyncSecretName(route)
	if ok {
		rc.enqueueRouteToSecret(route)
	}
}

func (rc *RouteController) enqueueRouteToSecret(route *routev1.Route) {
	key, err := KeyFunc(route)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for route object: %w", err))
		return
	}

	rc.routesToSecretsQueue.Add(key)
}

func (rc *RouteController) addRoute(obj interface{}) {
	route := obj.(*routev1.Route)
	if !util.IsManaged(route, rc.annotation) {
		return
	}

	klog.V(4).Infof("Adding Route %s/%s RV=%s UID=%s", route.Namespace, route.Name, route.ResourceVersion, route.UID)
	rc.enqueueRoute(route)
}

func (rc *RouteController) updateRoute(old, cur interface{}) {
	oldRoute := old.(*routev1.Route)
	newRoute := cur.(*routev1.Route)

	if !util.IsManaged(newRoute, rc.annotation) {
		return
	}

	klog.V(4).Infof("Updating Route %s/%s RV=%s->%s UID=%s->%s", newRoute.Namespace, newRoute.Name, oldRoute.ResourceVersion, newRoute.ResourceVersion, oldRoute.UID, newRoute.UID)
	rc.enqueueRoute(newRoute)
}

func (rc *RouteController) deleteRoute(obj interface{}) {
	route, ok := obj.(*routev1.Route)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("object is not a Route neither tombstone: %#v", obj))
			return
		}
		route, ok = tombstone.Obj.(*routev1.Route)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Route %#v", obj))
			return
		}
	}

	if !util.IsManaged(route, rc.annotation) {
		// TODO: if it is the exposer route we need to requeue the parrent route

		klog.V(5).Infof("Skipping Route %s/%s RV=%s UID=%s", route.Namespace, route.Name, route.ResourceVersion, route.UID)
		return
	}

	klog.V(4).Infof("Deleting Route %s/%s RV=%s UID=%s", route.Namespace, route.Name, route.ResourceVersion, route.UID)
	rc.enqueueRoute(route)
}

func (rc *RouteController) enqueueOwningRoute(obj metav1.Object) {
	routeKey, ok := obj.GetAnnotations()[api.AcmeExposerKey]
	if !ok {
		return
	}

	objReadOnly, exists, err := rc.routeInformersForNamespaces.InformersForOrGlobal(obj.GetNamespace()).Route().V1().Routes().Informer().GetIndexer().GetByKey(routeKey)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", routeKey, err)
		return
	}
	if !exists {
		return
	}

	route := objReadOnly.(*routev1.Route)
	if !util.IsManaged(route, rc.annotation) {
		return
	}

	rc.queue.Add(routeKey)
}

func (rc *RouteController) addReplicaSet(obj interface{}) {
	rc.enqueueOwningRoute(obj.(*appsv1.ReplicaSet))
}

func (rc *RouteController) updateReplicaSet(old, cur interface{}) {
	rc.enqueueOwningRoute(old.(*appsv1.ReplicaSet))
	rc.enqueueOwningRoute(cur.(*appsv1.ReplicaSet))
}

func (rc *RouteController) deleteReplicaSet(obj interface{}) {
	rs, ok := obj.(*appsv1.ReplicaSet)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("object is not a ReplicaSet neither tombstone: %#v", obj))
			return
		}
		rs, ok = tombstone.Obj.(*appsv1.ReplicaSet)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a ReplicaSet %#v", obj))
			return
		}
	}

	rc.enqueueOwningRoute(rs)
}

func (rc *RouteController) updateSecret(old, cur interface{}) {
	oldSecret := old.(*corev1.Secret)
	newSecret := cur.(*corev1.Secret)

	newControllerRef := metav1.GetControllerOf(newSecret)
	if newControllerRef == nil {
		return
	}

	route := rc.resolveControllerRef(newSecret.Namespace, newControllerRef)
	if route == nil {
		return
	}

	klog.V(4).Infof("Updating Secret %s/%s RV=%s->%s UID=%s->%s.", newSecret.Namespace, newSecret.Name, oldSecret.ResourceVersion, newSecret.ResourceVersion, oldSecret.UID, newSecret.UID)

	secretName, ok := GetSyncSecretName(route)
	if ok && secretName == newSecret.Name {
		// We don't need to requeue Route for sync secret change
		rc.enqueueRouteToSecret(route)
	} else {
		// For other Secret changes (like the exposer one) we need to requeue the Route
		rc.enqueueRoute(route)
	}

	return
}

func (rc *RouteController) deleteSecret(obj interface{}) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("object is not a Secret neither tombstone: %#v", obj))
			return
		}
		secret, ok = tombstone.Obj.(*corev1.Secret)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Secret: %#v", obj))
			return
		}
	}

	controllerRef := metav1.GetControllerOf(secret)
	if controllerRef == nil {
		return
	}

	route := rc.resolveControllerRef(secret.Namespace, controllerRef)
	if route == nil {
		return
	}

	klog.V(4).Infof("Secret %s/%s deleted.", secret.Namespace, secret.Name)

	secretName, ok := GetSyncSecretName(route)
	if ok && secretName == secret.Name {
		// We don't need to requeue Route for sync secret change
		rc.enqueueRouteToSecret(route)
	} else {
		// For other Secret changes (like the exposer one) we need to requeue the Route
		rc.enqueueRoute(route)
	}
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (rc *RouteController) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *routev1.Route {
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}

	route, err := rc.routeInformersForNamespaces.InformersForOrGlobal(namespace).Route().V1().Routes().Lister().Routes(namespace).Get(controllerRef.Name)
	if err != nil {
		return nil
	}

	if route.UID != controllerRef.UID {
		return nil
	}

	return route
}

func needsCertKey(t time.Time, route *routev1.Route) (string, error) {
	if route.Spec.TLS == nil || route.Spec.TLS.Key == "" || route.Spec.TLS.Certificate == "" {
		return "Route is missing CertKey", nil
	}

	certPemData := &cert.CertPemData{
		Key: []byte(route.Spec.TLS.Key),
		Crt: []byte(route.Spec.TLS.Certificate),
	}
	certificate, err := certPemData.Certificate()
	if err != nil {
		return "", fmt.Errorf("can't decode certificate from Route %s/%s: %v", route.Namespace, route.Name, err)
	}

	err = certificate.VerifyHostname(route.Spec.Host)
	if err != nil {
		klog.V(5).Info(err)
		return "Existing certificate doesn't match hostname", nil
	}

	if !cert.IsValid(certificate, t) {
		return "Already expired", nil
	}

	// We need to trigger renewals before the certs expire
	remains := certificate.NotAfter.Sub(t)
	lifetime := certificate.NotAfter.Sub(certificate.NotBefore)

	// This is the deadline when we start renewing
	if remains <= lifetime/3 {
		return "In renewal period", nil
	}

	// In case many certificates were provisioned at specific time
	// We will try to avoid spikes by renewing randomly
	if remains <= lifetime/2 {
		// We need to randomize renewals to spread the load.
		// Closer to deadline, bigger chance
		s := rand.NewSource(t.UnixNano())
		r := rand.New(s)
		n := r.NormFloat64()*RenewalStandardDeviation + RenewalMean
		// We use left half of normal distribution (all negative numbers).
		if n < 0 {
			return "Proactive renewal", nil
		}
	}

	return "", nil
}

func (rc *RouteController) getStatus(routeReadOnly *routev1.Route) (*api.Status, error) {
	status := &api.Status{}
	if routeReadOnly.Annotations != nil {
		statusString := routeReadOnly.Annotations[api.AcmeStatusAnnotation]
		err := yaml.Unmarshal([]byte(statusString), status)
		if err != nil {
			return nil, fmt.Errorf("can't decode status annotation: %v", err)
		}
	}

	// TODO: verify it matches account hash

	// TODO: verify status signature

	return status, nil
}

func (rc *RouteController) updateStatus(routeReadOnly *routev1.Route, status *api.Status) error {
	var oldRouteReadOnly *routev1.Route
	var err error
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if oldRouteReadOnly == nil {
			oldRouteReadOnly = routeReadOnly
		} else {
			oldRouteReadOnly, err = rc.routeClient.RouteV1().Routes(routeReadOnly.Namespace).Get(routeReadOnly.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
		}

		newRoute := oldRouteReadOnly.DeepCopy()

		err := setStatus(&newRoute.ObjectMeta, status)
		if err != nil {
			return fmt.Errorf("can't set status: %w", err)
		}

		if reflect.DeepEqual(newRoute, oldRouteReadOnly) {
			return nil
		}

		klog.V(4).Info(spew.Sprintf("Updating status for Route %s/%s to %#v", newRoute.Namespace, newRoute.Name, status))
		// The controller is the sole owner of the status.
		// Use Patch so we don't loose ACME information due to conflicts on the object. (e.g. on stale caches)

		_, err = rc.routeClient.RouteV1().Routes(newRoute.Namespace).Update(newRoute)
		return err
	})
	if err != nil {
		return fmt.Errorf("can't update status: %w", err)
	}
	return nil
}

func (rc *RouteController) sync(ctx context.Context, key string) error {
	klog.V(4).Infof("Started syncing Route %q", key)
	defer func() {
		klog.V(4).Infof("Finished syncing Route %q", key)
	}()

	namespace, _, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(err)
		return err
	}

	objReadOnly, exists, err := rc.routeInformersForNamespaces.InformersForOrGlobal(namespace).Route().V1().Routes().Informer().GetIndexer().GetByKey(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		klog.V(4).Infof("Route %s does not exist anymore\n", key)
		return nil
	}

	routeReadOnly := objReadOnly.(*routev1.Route)

	// Don't act on objects that are being deleted.
	if routeReadOnly.DeletionTimestamp != nil {
		return nil
	}

	// Although we check when adding the Route into the queue it might have been waiting for a while and edited
	if !util.IsManaged(routeReadOnly, rc.annotation) {
		klog.V(4).Infof("Skipping Route %s/%s UID=%s RV=%s", routeReadOnly.Namespace, routeReadOnly.Name, routeReadOnly.UID, routeReadOnly.ResourceVersion)
		return nil
	}

	// We have to check if Route is admitted to be sure it owns the domain!
	if !routeutil.IsAdmitted(routeReadOnly) {
		klog.V(4).Infof("Skipping Route %s because it's not admitted", key)
		return nil
	}

	status, err := rc.getStatus(routeReadOnly)
	if err != nil {
		return fmt.Errorf("can't get status: %v", err)
	}

	// TODO: Update status values e.g. for cert validity, next planned update range
	backoff := time.Duration(0)
	for i := 0; i < status.ProvisioningStatus.Failures; i++ {
		backoff *= rc.certOrderBackoffInitial
		if backoff >= rc.certOrderBackoffMax {
			backoff = rc.certOrderBackoffMax
			break
		}
	}
	status.ProvisioningStatus.EarliestAttemptAt = status.ProvisioningStatus.StartedAt.Add(backoff)

	reason, err := needsCertKey(time.Now(), routeReadOnly)
	if err != nil {
		return err
	}

	if len(reason) == 0 {
		klog.V(4).Infof("Route %q doesn't need new certificate.", key)
		return rc.updateStatus(routeReadOnly, status)
	}

	klog.V(2).Infof("Route %q needs new certificate: %v", key, reason)

	// We need new cert, clean the previous order if present
	switch status.ProvisioningStatus.OrderStatus {
	case "", acme.StatusValid:
		status.ProvisioningStatus.OrderURI = ""
		status.ProvisioningStatus.OrderStatus = ""

	case acme.StatusInvalid, acme.StatusExpired, acme.StatusRevoked, acme.StatusDeactivated:
		delay := status.ProvisioningStatus.EarliestAttemptAt.Sub(time.Now())
		klog.Infof("route %s, now: %v, EarliestAttemptAt: %v, delay: %v", key, time.Now(), status.ProvisioningStatus.EarliestAttemptAt, delay)
		if delay > 0 {
			klog.V(2).Infof("Retrying validation for Route %s got rate limited, next attempt in %v", key, delay)
			rc.queue.AddAfter(key, delay)
			return rc.updateStatus(routeReadOnly, status)
		}

		status.ProvisioningStatus.OrderURI = ""
		status.ProvisioningStatus.OrderStatus = ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), AcmeTimeout)
	defer cancel()

	certIssuer, certIssuerSecret, err := controllerutils.IssuerForObject(routeReadOnly.ObjectMeta, rc.controllerNamespace, rc.kubeInformersForNamespaces)
	if err != nil {
		return fmt.Errorf("can't get cert issuer: %w", err)
	}
	switch certIssuer.Type {
	case api.CertIssuerTypeAcme:
		break
	default:
		return fmt.Errorf("unsupported cert issuer type %q", certIssuer.Type)
	}

	acmeIssuer := certIssuer.AcmeCertIssuer
	if acmeIssuer == nil {
		return fmt.Errorf("ACME issuer is missing AcmeCertIssuer spec")
	}

	acmeClient := &acme.Client{
		DirectoryURL: acmeIssuer.DirectoryURL,
		UserAgent:    "github.com/tnozicka/openshift-acme",
	}
	klog.V(4).Infof("Using ACME client with DirectoryURL %q", acmeClient.DirectoryURL)

	acmeClient.Key, err = helpers.PrivateKeyFromSecret(certIssuerSecret)
	if err != nil {
		return err
	}

	domain := routeReadOnly.Spec.Host

	if len(status.ProvisioningStatus.OrderURI) == 0 {
		order, err := acmeClient.AuthorizeOrder(ctx, acme.DomainIDs(domain))
		if err != nil {
			return err
		}
		// TODO: convert into event
		klog.V(1).Infof("Created Order %q for Route %q", order.URI, key)

		// We need to store the order URI immediately to prevent loosing it on error.
		// Updating the route will make it requeue.
		status.ProvisioningStatus.StartedAt = time.Now()
		status.ProvisioningStatus.OrderURI = order.URI
		status.ProvisioningStatus.OrderStatus = order.Status
		return rc.updateStatus(routeReadOnly, status)
	}

	order, err := acmeClient.GetOrder(ctx, status.ProvisioningStatus.OrderURI)
	if err != nil {
		acmeErr, ok := err.(*acme.Error)
		if !ok || acmeErr.StatusCode != http.StatusNotFound {
			return err
		}

		// The order URI doesn't exist. Delete OrderUri and update the status.
		klog.Warningf("Route %q: Found invalid OrderURI %q, removing it.", key, status.ProvisioningStatus.OrderURI)
		status.ProvisioningStatus.OrderURI = ""
		return rc.updateStatus(routeReadOnly, status)
	}
	// TODO: acme or golang should fill in the value
	order.URI = status.ProvisioningStatus.OrderURI

	previousOrderStatus := status.ProvisioningStatus.OrderStatus
	status.ProvisioningStatus.OrderStatus = order.Status

	klog.V(4).Infof("Route %q: Order %q is in %q state", key, order.URI, order.Status)

	switch order.Status {
	case acme.StatusPending:
		// Satisfy all pending authorizations.
		klog.V(4).Infof("Route %q: Order %q contains %d authorization(s)", key, order.URI, len(order.AuthzURLs))

		for _, authzURL := range order.AuthzURLs {
			authz, err := acmeClient.GetAuthorization(ctx, authzURL)
			if err != nil {
				return err
			}

			klog.V(4).Infof("Route %q: order %q: authz %q: is in %q state", key, order.URI, authz.URI, authz.Status)

			switch authz.Status {
			case acme.StatusPending:
				break

			case acme.StatusValid, acme.StatusInvalid, acme.StatusDeactivated, acme.StatusExpired, acme.StatusRevoked:
				continue

			default:
				return fmt.Errorf("route %q: order %q: authz %q has invalid status %q", key, order.URI, authz.URI, authz.Status)
			}

			// Authz is Pending

			var challenge *acme.Challenge
			for _, c := range authz.Challenges {
				if c.Type == "http-01" {
					challenge = c
				}
			}

			if challenge == nil {
				// TODO: emit an event
				return fmt.Errorf("route %q: unable to satisfy authorization %q for domain %q: no viable challenge type found in %v", key, authz.URI, domain, authz.Challenges)
			}

			klog.V(4).Infof("route %q: order %q: authz %q: challenge %q is in %q state", key, order.URI, authz.URI, authz.Status, challenge.Status)

			switch challenge.Status {
			case acme.StatusPending:
				id := getID(routeReadOnly.Name, order.URI, authzURL, challenge.URI)
				tmpName := getTemporaryName(id)

				challengePath := acmeClient.HTTP01ChallengePath(challenge.Token)
				challengeResponse, err := acmeClient.HTTP01ChallengeResponse(challenge.Token)
				if err != nil {
					return err
				}

				/*
				 * Route
				 */
				trueVal := true
				desiredExposerRoute := routeReadOnly.DeepCopy()
				filterOutAnnotations(desiredExposerRoute.Annotations)
				filterOutLabels(desiredExposerRoute.Labels, desiredExposerRoute.Annotations)

				desiredExposerRoute.Name = tmpName
				desiredExposerRoute.ResourceVersion = ""
				desiredExposerRoute.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: routev1.SchemeGroupVersion.String(),
						Kind:       "Route",
						Name:       routeReadOnly.Name,
						UID:        routeReadOnly.UID,
						Controller: &trueVal,
					},
				}
				if desiredExposerRoute.Annotations == nil {
					desiredExposerRoute.Annotations = map[string]string{}
				}
				desiredExposerRoute.Annotations[api.AcmeExposerId] = id
				desiredExposerRoute.Annotations[api.AcmeExposerKey] = key
				if desiredExposerRoute.Labels == nil {
					desiredExposerRoute.Labels = map[string]string{}
				}
				desiredExposerRoute.Labels[api.AcmeTemporaryLabel] = "true"
				desiredExposerRoute.Labels[api.AcmeExposerUID] = string(routeReadOnly.UID)
				desiredExposerRoute.Spec.Path = acmeClient.HTTP01ChallengePath(challenge.Token)
				desiredExposerRoute.Spec.Port = nil
				desiredExposerRoute.Spec.TLS = &routev1.TLSConfig{
					Termination:                   "edge",
					InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyAllow,
				}
				desiredExposerRoute.Spec.To = routev1.RouteTargetReference{
					Kind: "Service",
					Name: tmpName,
				}

				exposerRoute, err := rc.routeInformersForNamespaces.InformersForOrGlobal(routeReadOnly.Namespace).Route().V1().Routes().Lister().Routes(routeReadOnly.Namespace).Get(desiredExposerRoute.Name)
				if err != nil {
					if !kapierrors.IsNotFound(err) {
						return err
					}

					klog.V(2).Infof("Exposer route %s/%s not found, creating new one.", routeReadOnly.Namespace, desiredExposerRoute.Name)

					exposerRoute, err = rc.routeClient.RouteV1().Routes(routeReadOnly.Namespace).Create(desiredExposerRoute)
					if err != nil {
						return err
					}
					klog.V(2).Infof("Created exposer Route %s/%s for Route %s", exposerRoute.Namespace, exposerRoute.Name, key)
				}

				if !metav1.IsControlledBy(exposerRoute, routeReadOnly) {
					klog.Infof("%#v", exposerRoute)
					return fmt.Errorf("exposer Route %s/%s already exists and isn't owned by route %s", exposerRoute.Namespace, exposerRoute.Name, key)
				}

				// Check the id to avoid collisions
				exposerRouteId, ok := exposerRoute.Annotations[api.AcmeExposerId]
				if !ok {
					return fmt.Errorf("exposer route %s/%s misses exposer id", exposerRoute.Namespace, exposerRoute.Name)
				} else if exposerRouteId != id {
					return fmt.Errorf("exposer route %s/%s id missmatch: expected %q, got %q", exposerRoute.Namespace, exposerRoute.Name, id, exposerRouteId)
				}

				ownerRefToExposerRoute := metav1.OwnerReference{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Route",
					Name:       exposerRoute.Name,
					UID:        exposerRoute.UID,
					Controller: &trueVal,
				}

				/*
				 * Secret
				 */
				desiredExposerSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:            tmpName,
						OwnerReferences: []metav1.OwnerReference{ownerRefToExposerRoute},
						Annotations: map[string]string{
							api.AcmeExposerId:  id,
							api.AcmeExposerKey: key,
						},
						Labels: map[string]string{
							api.AcmeTemporaryLabel: "true",
							api.AcmeExposerUID:     string(routeReadOnly.UID),
						},
					},
					StringData: map[string]string{
						ExposerFileKey: challengePath + " " + challengeResponse,
					},
				}
				exposerSecret, err := rc.kubeInformersForNamespaces.InformersForOrGlobal(routeReadOnly.Namespace).Core().V1().Secrets().Lister().Secrets(routeReadOnly.Namespace).Get(desiredExposerSecret.Name)
				if err != nil {
					if !kapierrors.IsNotFound(err) {
						return err
					}

					klog.V(2).Infof("Exposer secret %s/%s not found, creating new one.", routeReadOnly.Namespace, desiredExposerSecret.Name)

					exposerSecret, err = rc.kubeClient.CoreV1().Secrets(routeReadOnly.Namespace).Create(desiredExposerSecret)
					if err != nil {
						return err
					}
				}

				if !metav1.IsControlledBy(exposerSecret, exposerRoute) {
					return fmt.Errorf("secret %s/%s already exists and isn't owned by expúoser route %s/%s", exposerSecret.Namespace, exposerSecret.Name, exposerRoute.Namespace, exposerRoute.Name)
				}

				// Check the id to avoid collisions
				exposerSecretId, ok := exposerSecret.Annotations[api.AcmeExposerId]
				if !ok {
					return fmt.Errorf("exposer secret %s/%s misses exposer id", exposerRoute.Namespace, exposerRoute.Name)
				} else if exposerSecretId != id {
					return fmt.Errorf("exposer secret %s/%s id missmatch: expected %q, got %q", exposerRoute.Namespace, exposerRoute.Name, id, exposerSecretId)
				}

				/*
				 * ReplicaSet
				 */
				var replicas int32 = 2
				podLabels := map[string]string{
					"app": tmpName,
				}
				podSelector := &metav1.LabelSelector{
					MatchLabels: podLabels,
				}
				desiredExposerRS := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:            tmpName,
						OwnerReferences: []metav1.OwnerReference{ownerRefToExposerRoute},
						Annotations: map[string]string{
							api.AcmeExposerId:  id,
							api.AcmeExposerKey: key,
						},
						Labels: map[string]string{
							api.AcmeTemporaryLabel: "true",
							api.AcmeExposerUID:     string(routeReadOnly.UID),
						},
					},
					Spec: appsv1.ReplicaSetSpec{
						Replicas: &replicas,
						Selector: podSelector,
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: podLabels,
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "exposer",
										Image: rc.exposerImage,
										Command: []string{
											"openshift-acme-exposer",
										},
										Args: []string{
											"--response-file=/etc/openshift-acme-exposer/" + ExposerFileKey,
										},
										Ports: []corev1.ContainerPort{
											{
												Name:          "http",
												Protocol:      corev1.ProtocolTCP,
												ContainerPort: 5000,
											},
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "exposer-data",
												ReadOnly:  true,
												MountPath: "/etc/openshift-acme-exposer",
											},
										},
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceCPU:    *resource.NewMilliQuantity(5, resource.DecimalSI),
												corev1.ResourceMemory: *resource.NewQuantity(50*(1024*1024), resource.BinarySI),
											},
											Limits: corev1.ResourceList{
												corev1.ResourceCPU:    *resource.NewMilliQuantity(100, resource.DecimalSI),
												corev1.ResourceMemory: *resource.NewQuantity(50*(1024*1024), resource.BinarySI),
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "exposer-data",
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: exposerSecret.Name,
											},
										},
									},
								},
							},
						},
					},
				}

				limitRanges, err := rc.kubeInformersForNamespaces.InformersForOrGlobal(routeReadOnly.Namespace).Core().V1().LimitRanges().Lister().LimitRanges(routeReadOnly.Namespace).List(labels.Everything())
				if err != nil {
					return err
				}

				err = adjustContainerResourceRequirements(&desiredExposerRS.Spec.Template.Spec.Containers[0].Resources, limitRanges)
				if err != nil {
					rc.recorder.Eventf(routeReadOnly, corev1.EventTypeWarning, "ExposerPodResourceRequirementsError", err.Error())
					return nil
				}

				exposerRS, err := rc.kubeInformersForNamespaces.InformersForOrGlobal(routeReadOnly.Namespace).Apps().V1().ReplicaSets().Lister().ReplicaSets(routeReadOnly.Namespace).Get(desiredExposerRS.Name)
				if err != nil {
					if !kapierrors.IsNotFound(err) {
						return err
					}

					klog.V(2).Infof("Exposer replica set %s/%s not found, creating new one.", routeReadOnly.Namespace, desiredExposerRS.Name)

					exposerRS, err = rc.kubeClient.AppsV1().ReplicaSets(routeReadOnly.Namespace).Create(desiredExposerRS)
					if err != nil {
						return err
					}
				}

				if !metav1.IsControlledBy(exposerRS, exposerRoute) {
					return fmt.Errorf("RS %s/%s already exists and isn't owned by exposer route %s/%s", exposerRS.Namespace, exposerRS.Name, exposerRoute.Namespace, exposerRoute.Name)
				}

				// Check the id to avoid collisions
				exposerRSId, ok := exposerRS.Annotations[api.AcmeExposerId]
				if !ok {
					return fmt.Errorf("exposer RS %s/%s misses exposer id", exposerRoute.Namespace, exposerRoute.Name)
				} else if exposerRSId != id {
					return fmt.Errorf("exposer RS %s/%s id missmatch: expected %q, got %q", exposerRoute.Namespace, exposerRoute.Name, id, exposerRSId)
				}

				/*
				 * Service
				 */
				desiredExposerService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:            tmpName,
						OwnerReferences: []metav1.OwnerReference{ownerRefToExposerRoute},
						Annotations: map[string]string{
							api.AcmeExposerId:  id,
							api.AcmeExposerKey: key,
						},
						Labels: map[string]string{
							api.AcmeTemporaryLabel: "true",
							api.AcmeExposerUID:     string(routeReadOnly.UID),
						},
					},
					Spec: corev1.ServiceSpec{
						Selector: podLabels,
						Type:     corev1.ServiceTypeClusterIP,
						Ports: []corev1.ServicePort{
							{
								Name:       "http",
								Protocol:   corev1.ProtocolTCP,
								Port:       80,
								TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 5000},
							},
						},
					},
				}
				exposerService, err := rc.kubeInformersForNamespaces.InformersForOrGlobal(routeReadOnly.Namespace).Core().V1().Services().Lister().Services(routeReadOnly.Namespace).Get(desiredExposerService.Name)
				if err != nil {
					if !kapierrors.IsNotFound(err) {
						return err
					}

					klog.V(2).Infof("Exposer service %s/%s not found, creating new one.", routeReadOnly.Namespace, desiredExposerService.Name)

					exposerService, err = rc.kubeClient.CoreV1().Services(routeReadOnly.Namespace).Create(desiredExposerService)
					if err != nil {
						return err
					}
				}

				if !metav1.IsControlledBy(exposerService, exposerRoute) {
					return fmt.Errorf("service %s/%s already exists and isn't owned by exposer route %s/%s", exposerService.Namespace, exposerService.Name, exposerRoute.Namespace, exposerRoute.Name)
				}

				// Check the id to avoid collisions
				exposerServiceId, ok := exposerService.Annotations[api.AcmeExposerId]
				if !ok {
					return fmt.Errorf("exposer service %s/%s misses exposer id", exposerRoute.Namespace, exposerRoute.Name)
				} else if exposerServiceId != id {
					return fmt.Errorf("exposer service %s/%s id missmatch: expected %q, got %q", exposerRoute.Namespace, exposerRoute.Name, id, exposerServiceId)
				}

				// TODO: id admitted=false we should stop trying and report event
				if !routeutil.IsAdmitted(exposerRoute) {
					klog.V(4).Infof("exposer Route %s/%s isn't admitted yet", exposerRoute.Namespace, exposerRoute.Name)
					rc.queue.AddAfter(key, 15*time.Second) // FIXME: set up event handlers
					break
				}

				// TODO: wait for pods to run and report into status, requeue
				// For now, the server is bound to retry the verification by RFC8555
				// so on happy path there shouldn't be issues. But pods can get stuck
				// on scheduling, quota, resources, ... and we want to know why the validation fails.
				if exposerRS.Status.ObservedGeneration != exposerRS.Generation ||
					exposerRS.Status.AvailableReplicas != exposerRS.Status.Replicas {
					klog.V(4).Infof("exposer ReplicaSet %s/%s isn't available yet", exposerRS.Namespace, exposerRS.Name)
					rc.queue.AddAfter(key, 15*time.Second) // FIXME: set up event handlers
					break
				}

				url := "http://" + domain + challengePath
				err = controllerutils.ValidateExposedToken(url, challengeResponse)
				if err != nil {
					klog.Infof("Can't self validate exposed token before accepting the challenge: %v", err)
					// We are waiting for external event, make sure we requeue
					rc.queue.AddAfter(key, 15*time.Second)
					break
				}

				_, err = acmeClient.Accept(ctx, challenge)
				if err != nil {
					return err
				}
				klog.V(2).Infof("Accepted challenge for Route %s.", key)

				// We are waiting for external event, make sure we requeue
				rc.queue.AddAfter(key, 15*time.Second)

			case acme.StatusProcessing, acme.StatusValid, acme.StatusInvalid:
				// These states will manifest into global order state over time.
				// We only need to attend to pending states.
				// We could possibly report events for those but is seems too fine grained for now.

				// We are waiting for external event, make sure we requeue
				rc.queue.AddAfter(key, 15*time.Second)

			default:
				return fmt.Errorf("route %q: order %q: authz %q: invalid status %q for challenge %q", key, order.URI, authz.URI, challenge.Status, challenge.URI)
			}
		}

		return rc.updateStatus(routeReadOnly, status)

	case acme.StatusProcessing:
		// TODO: backoff but capped at some reasonable time
		rc.queue.AddAfter(key, 15*time.Second)

		klog.V(4).Infof("Route %q: Order %q: Waiting to be validated by ACME server", key, order)

		return rc.updateStatus(routeReadOnly, status)

	case acme.StatusReady:
		// TODO: fix the golang acme lib
		// Unfortunately the golang acme lib actively waits in 'CreateOrderCert'
		// so we can't take the appropriate asynchronous action here.

		klog.V(3).Infof("Route %q: Order %q successfully validated", key, order.URI)
		template := x509.CertificateRequest{
			DNSNames: []string{routeReadOnly.Spec.Host},
		}
		privateKey, err := rsa.GenerateKey(cryptorand.Reader, rc.certDefaultRSAKeyBitSize)
		if err != nil {
			return fmt.Errorf("failed to generate RSA key: %v", err)
		}

		csr, err := x509.CreateCertificateRequest(cryptorand.Reader, &template, privateKey)
		if err != nil {
			return fmt.Errorf("failed to create certificate request: %v", err)
		}

		// Send CSR
		// FIXME: Unfortunately golang also waits in this method for the cert creation
		//  although that should be asynchronous. Requires fixing golang lib. (The helpers used are private.)
		der, certUrl, err := acmeClient.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
		if err != nil {
			return fmt.Errorf("can't create cert order: %w", err)
		}

		klog.V(4).Infof("Route %q: Order %q: Certificate available at %q", key, order.URI, certUrl)

		certPemData, err := cert.NewCertificateFromDER(der, privateKey)
		if err != nil {
			return fmt.Errorf("can't convert certificate from DER to PEM: %v", err)
		}

		route := routeReadOnly.DeepCopy()

		// unfortunatly golang acmeClient.CreateOrderCert waits internally for transitioning state
		// to valid and we need to reflect it in our state machine because we don't get back
		// into the provisioning phase again after the certs are updated and valid.
		status.ProvisioningStatus.OrderStatus = acme.StatusValid

		// We are updating the route and to avoid conflicts later we will also update the status together
		err = setStatus(&route.ObjectMeta, status)
		if err != nil {
			return fmt.Errorf("can't set status: %w", err)
		}

		if route.Spec.TLS == nil {
			route.Spec.TLS = &routev1.TLSConfig{
				// Defaults
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
				Termination:                   routev1.TLSTerminationEdge,
			}
		}
		route.Spec.TLS.Key = string(certPemData.Key)
		route.Spec.TLS.Certificate = string(certPemData.Crt)

		// TODO: consider RetryOnConflict with rechecking the managed annotation
		_, err = rc.routeClient.RouteV1().Routes(routeReadOnly.Namespace).Update(route)
		if err != nil {
			return fmt.Errorf("can't update route %s/%s with new certificates: %v", routeReadOnly.Namespace, route.Name, err)
		}

		err = rc.CleanupExposerObjects(routeReadOnly)
		if err != nil {
			klog.Errorf("Can't cleanup exposer objects: %v", err)
		}

		// We have already updated the status when updating the Route.
		return nil

	case acme.StatusValid:
		// TODO: fix the golang acme lib
		// Unfortunately the golang acme lib actively waits in 'CreateOrderCert'
		// so we can't take the appropriate asynchronous action here.
		// The logic is included in handling acme.StatusReady
		return nil

	case acme.StatusInvalid:
		rc.recorder.Eventf(routeReadOnly, corev1.EventTypeWarning, "AcmeFailedOrder", "Order %q for domain %q failed: %v", order.URI, routeReadOnly.Spec.Host, order.Error)

		if status.ProvisioningStatus.OrderStatus != previousOrderStatus {
			status.ProvisioningStatus.Failures += 1
		}
		err = rc.CleanupExposerObjects(routeReadOnly)
		if err != nil {
			klog.Errorf("Can't cleanup exposer objects: %v", err)
		}
		return rc.updateStatus(routeReadOnly, status)

	case acme.StatusExpired, acme.StatusRevoked, acme.StatusDeactivated:
		if status.ProvisioningStatus.OrderStatus != previousOrderStatus {
			status.ProvisioningStatus.Failures += 1
		}
		err = rc.CleanupExposerObjects(routeReadOnly)
		if err != nil {
			klog.Errorf("Can't cleanup exposer objects: %v", err)
		}
		return rc.updateStatus(routeReadOnly, status)

	default:
		return fmt.Errorf("route %q: invalid new order status %q; order URL: %q", key, order.Status, order.URI)
	}
}

func (rc *RouteController) syncRouteToSecret(ctx context.Context, key string) error {
	klog.V(4).Infof("Started syncing Route (to Secret) %q", key)
	defer func() {
		klog.V(4).Infof("Finished syncing Route (to Secret) %q", key)
	}()

	namespace, _, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(err)
		return err
	}

	routeObjReadOnly, exists, err := rc.routeInformersForNamespaces.InformersForOrGlobal(namespace).Route().V1().Routes().Informer().GetIndexer().GetByKey(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}
	if !exists {
		klog.V(4).Infof("Route %s does not exist anymore\n", key)
		return nil
	}
	routeReadOnly := routeObjReadOnly.(*routev1.Route)

	// Don't act on objects that are being deleted.
	if routeReadOnly.DeletionTimestamp != nil {
		return nil
	}

	if routeReadOnly.Spec.TLS == nil {
		return nil
	}

	secretName, ok := GetSyncSecretName(routeReadOnly)
	if !ok {
		return nil
	}

	secretObjReadOnly, exists, err := rc.kubeInformersForNamespaces.InformersForOrGlobal(namespace).Core().V1().Secrets().Informer().GetIndexer().GetByKey(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}
	var secretReadOnly *corev1.Secret
	var secret *corev1.Secret
	if exists {
		secretReadOnly = secretObjReadOnly.(*corev1.Secret)

		controllerRef := metav1.GetControllerOf(secretReadOnly)
		if controllerRef == nil {
			rc.recorder.Eventf(routeReadOnly, corev1.EventTypeWarning, "CollidingSecret", "Can't sync certificates for Route %s/%s into Secret %s/%s because it already exists and isn't owned by the Route!", routeReadOnly.Namespace, routeReadOnly.Name, routeReadOnly.Namespace, secretName)
			return fmt.Errorf("secret %s/%s already exists with no owner", secretReadOnly.Namespace, secretReadOnly.Name)
		}
		owningRoute := rc.resolveControllerRef(routeReadOnly.Namespace, controllerRef)
		if owningRoute == nil {
			rc.recorder.Eventf(routeReadOnly, corev1.EventTypeWarning, "CollidingSecret", "Can't sync certificates for Route %s/%s into Secret %s/%s because it already exists and isn't owned by the Route!", routeReadOnly.Namespace, routeReadOnly.Name, routeReadOnly.Namespace, secretName)
			return fmt.Errorf("secret %s/%s already exists and isn't owned by us", secretReadOnly.Namespace, secretReadOnly.Name)
		}

		secret = secretReadOnly.DeepCopy()
	} else {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: secretName,
			},
		}
	}

	secret.Type = corev1.SecretTypeTLS

	trueVal := true
	secret.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: controllerKind.GroupVersion().String(),
			Kind:       controllerKind.Kind,
			Name:       routeReadOnly.Name,
			UID:        routeReadOnly.UID,
			Controller: &trueVal,
		},
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[corev1.TLSCertKey] = []byte(routeReadOnly.Spec.TLS.Certificate)
	secret.Data[corev1.TLSPrivateKeyKey] = []byte(routeReadOnly.Spec.TLS.Key)

	if !exists {
		_, err = rc.kubeClient.CoreV1().Secrets(routeReadOnly.Namespace).Create(secret)
		if err != nil {
			return fmt.Errorf("can't create Secret %s/%s: %v", routeReadOnly.Namespace, secret.Name, err)
		}
	} else {
		if !reflect.DeepEqual(secret, secretReadOnly) {
			_, err = rc.kubeClient.CoreV1().Secrets(routeReadOnly.Namespace).Update(secret)
			if err != nil {
				return fmt.Errorf("failed to update Secret %s/%s with TLS data: %v", routeReadOnly.Namespace, secret.Name, err)
			}
		}
	}

	return nil
}

func (rc *RouteController) CleanupExposerObjects(route *routev1.Route) error {
	var gracePeriod int64 = 0
	propagationPolicy := metav1.DeletePropagationBackground
	klog.V(3).Infof("Cleaning up temporary exposer for Route %s/%s (UID=%s)", route.Namespace, route.Name, route.UID)
	err := rc.routeClient.RouteV1().Routes(route.Namespace).DeleteCollection(
		&metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
			PropagationPolicy:  &propagationPolicy,
		},
		metav1.ListOptions{
			LabelSelector: labels.SelectorFromValidatedSet(labels.Set{
				api.AcmeExposerUID: string(route.UID),
			}).String(),
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func (rc *RouteController) processNextRouteItem(ctx context.Context) bool {
	key, quit := rc.queue.Get()
	if quit {
		return false
	}
	defer rc.queue.Done(key)

	err := rc.sync(ctx, key.(string))
	if err == nil {
		rc.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with : %v", key, err))
	rc.queue.AddRateLimited(key)

	return true
}

func (rc *RouteController) processNextRouteToSecretItem(ctx context.Context) bool {
	key, quit := rc.routesToSecretsQueue.Get()
	if quit {
		return false
	}
	defer rc.routesToSecretsQueue.Done(key)

	err := rc.syncRouteToSecret(ctx, key.(string))
	if err == nil {
		rc.routesToSecretsQueue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with : %v", key, err))
	rc.routesToSecretsQueue.AddRateLimited(key)

	return true
}

func (rc *RouteController) runRouteWorker(ctx context.Context) {
	for rc.processNextRouteItem(ctx) {
	}
}

func (rc *RouteController) runSecretWorker(ctx context.Context) {
	for rc.processNextRouteToSecretItem(ctx) {
	}
}

func (rc *RouteController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	var wg sync.WaitGroup
	klog.Info("Starting Route controller")
	defer func() {
		klog.Info("Shutting down Route controller")
		rc.queue.ShutDown()
		rc.routesToSecretsQueue.ShutDown()
		wg.Wait()
		klog.Info("Route controller shut down")
	}()

	// Wait for all involved caches to be synced, before processing items from the queue is started
	synced := cache.WaitForNamedCacheSync("route controller", ctx.Done(), rc.cachesToSync...)
	if !synced {
		return
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wait.UntilWithContext(ctx, rc.runRouteWorker, time.Second)
		}()
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wait.UntilWithContext(ctx, rc.runSecretWorker, time.Second)
		}()
	}

	<-ctx.Done()
}

func getTemporaryName(key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("exposer-%s", strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])))
}

func getID(routeName, orderURI, authzURL, challengeURI string) string {
	return routeName + ":" + orderURI + ":" + authzURL + ":" + challengeURI
}

func setStatus(obj *metav1.ObjectMeta, status *api.Status) error {
	status.ObservedGeneration = obj.Generation

	// TODO: sign the status

	bytes, err := yaml.Marshal(status)
	if err != nil {
		return fmt.Errorf("can't encode status annotation: %v", err)
	}

	metav1.SetMetaDataAnnotation(obj, api.AcmeStatusAnnotation, string(bytes))

	return nil
}

func GetSyncSecretName(route *routev1.Route) (string, bool) {
	secretName, ok := route.Annotations[api.AcmeSecretName]
	if !ok {
		return "", false
	}

	// Empty string means default to the same name as the route.
	if len(secretName) == 0 {
		secretName = route.Name
	}

	return secretName, true
}

func adjustContainerResourceRequirements(requirements *corev1.ResourceRequirements, limitRanges []*corev1.LimitRange) error {
	var errors []error

	for _, limitRange := range limitRanges {
		for _, limitRangeItem := range limitRange.Spec.Limits {
			if limitRangeItem.Type != corev1.LimitTypeContainer && limitRangeItem.Type != corev1.LimitTypePod {
				continue
			}

			memoryRangeMin, memoryRangeMinPresent := limitRangeItem.Min[corev1.ResourceMemory]
			memoryRangeMax, memoryRangeMaxPresent := limitRangeItem.Max[corev1.ResourceMemory]

			for _, r := range []corev1.ResourceList{requirements.Requests, requirements.Limits} {
				memory, memoryPresent := r[corev1.ResourceMemory]
				if memoryPresent {
					if memoryRangeMinPresent {
						if memory.Cmp(memoryRangeMin) == -1 { // less
							r[corev1.ResourceMemory] = memoryRangeMin
						}
					}

					if memoryRangeMaxPresent {
						if memory.Cmp(memoryRangeMax) == 1 { // more
							errors = append(errors, fmt.Errorf("memory ask for %s is higher then maximum memory from limitrange %s/%s", memory.String(), limitRange.Namespace, limitRange.Name))
						}
					}
				}
			}

			memoryRangeRequestRatio, memoryRangeRequestRatioPresent := limitRangeItem.MaxLimitRequestRatio[corev1.ResourceMemory]
			memoryRequest, memoryRequestPresent := requirements.Requests[corev1.ResourceMemory]
			memoryLimit, memoryLimitPresent := requirements.Limits[corev1.ResourceMemory]
			if memoryRangeRequestRatioPresent && memoryRequestPresent && memoryLimitPresent {
				observerRatio := new(inf.Dec).QuoRound(memoryLimit.AsDec(), memoryRequest.AsDec(), 3, inf.RoundHalfDown)
				if observerRatio.Cmp(memoryRangeRequestRatio.AsDec()) == 1 { // more
					res := new(inf.Dec).QuoRound(memoryLimit.AsDec(), memoryRangeRequestRatio.AsDec(), 3, inf.RoundHalfDown)
					unscaled, _ := res.Unscaled()
					requirements.Requests[corev1.ResourceMemory] = *resource.NewScaledQuantity(unscaled, resource.Scale(res.Scale()*-1))
				}
			}

			cpuRangeMin, cpuRangeMinPresent := limitRangeItem.Min[corev1.ResourceCPU]
			cpuRangeMax, cpuRangeMaxPresent := limitRangeItem.Max[corev1.ResourceCPU]

			for _, r := range []corev1.ResourceList{requirements.Requests, requirements.Limits} {
				cpu, cpuPresent := r[corev1.ResourceCPU]
				if cpuPresent {
					if cpuRangeMinPresent {
						if cpu.Cmp(cpuRangeMin) == -1 { // less
							r[corev1.ResourceCPU] = cpuRangeMin
						}
					}

					if cpuRangeMaxPresent {
						if cpu.Cmp(cpuRangeMax) == 1 { // more
							errors = append(errors, fmt.Errorf("cpu ask for %s is higher then maximum cpu from limitrange %s/%s", cpu.String(), limitRange.Namespace, limitRange.Name))
						}
					}
				}
			}

			cpuRangeRequestRatio, cpuRangeRequestRatioPresent := limitRangeItem.MaxLimitRequestRatio[corev1.ResourceCPU]
			cpuRequest, cpuRequestPresent := requirements.Requests[corev1.ResourceCPU]
			cpuLimit, cpuLimitPresent := requirements.Limits[corev1.ResourceCPU]
			if cpuRangeRequestRatioPresent && cpuRequestPresent && cpuLimitPresent {
				observerRatio := new(inf.Dec).QuoRound(cpuLimit.AsDec(), cpuRequest.AsDec(), 3, inf.RoundHalfDown)
				if observerRatio.Cmp(cpuRangeRequestRatio.AsDec()) == 1 { // more
					res := new(inf.Dec).QuoRound(cpuLimit.AsDec(), cpuRangeRequestRatio.AsDec(), 3, inf.RoundHalfDown)
					unscaled, _ := res.Unscaled()
					requirements.Requests[corev1.ResourceCPU] = *resource.NewScaledQuantity(unscaled, resource.Scale(res.Scale()*-1))
				}
			}
		}
	}

	return apierrors.NewAggregate(errors)
}

func filterOutAnnotations(annotations map[string]string) {
	if annotations == nil {
		return
	}

	// don't copy haproxy.router.openshift.io/ip_whitelist so http-01 validation works
	delete(annotations, "haproxy.router.openshift.io/ip_whitelist")

	regexString, ok := annotations[api.AcmeExposerHttpFilterOutAnnotationsAnnotation]
	if !ok || len(regexString) == 0 {
		return
	}

	r, err := regexp.Compile(regexString)
	if err != nil {
		klog.V(2).Infof("invalid regex: %q", regexString)
		return
	}

	for k := range annotations {
		if r.MatchString(k) {
			delete(annotations, k)
		}
	}
}

func filterOutLabels(labels map[string]string, annotations map[string]string) {
	if labels == nil {
		return
	}

	regexString, ok := annotations[api.AcmeExposerHttpFilterOutLabelsAnnotation]
	if !ok || len(regexString) == 0 {
		return
	}

	r, err := regexp.Compile(regexString)
	if err != nil {
		klog.V(2).Infof("invalid regex: %q", regexString)
		return
	}

	for k := range labels {
		if r.MatchString(k) {
			delete(labels, k)
		}
	}
}
