package route

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/rand"
	"reflect"
	"time"

	"github.com/golang/glog"
	routev1 "github.com/openshift/api/route/v1"
	routeclientset "github.com/openshift/client-go/route/clientset/versioned"
	_ "github.com/openshift/client-go/route/clientset/versioned/scheme"
	routelistersv1 "github.com/openshift/client-go/route/listers/route/v1"
	"golang.org/x/crypto/acme"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	kcorelistersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/tnozicka/openshift-acme/pkg/acme/challengeexposers"
	acmeclient "github.com/tnozicka/openshift-acme/pkg/acme/client"
	acmeclientbuilder "github.com/tnozicka/openshift-acme/pkg/acme/client/builder"
	"github.com/tnozicka/openshift-acme/pkg/api"
	"github.com/tnozicka/openshift-acme/pkg/cert"
	routeutil "github.com/tnozicka/openshift-acme/pkg/route"
	"github.com/tnozicka/openshift-acme/pkg/util"
)

const (
	ControllerName = "openshift-acme-controller"
	// Raise this when we have separate rate limiting for ACME.
	// Now it will get eventually reconciled when informers re-sync or on edit.
	MaxRetries               = 2
	RenewalStandardDeviation = 1
	RenewalMean              = 0
	AcmeTimeout              = 60 * time.Second
)

var (
	KeyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = routev1.SchemeGroupVersion.WithKind("Route")
)

type RouteController struct {
	acmeClientFactory *acmeclientbuilder.SharedClientFactory

	// TODO: switch this for generic interface to allow other types like DNS01
	exposers map[string]challengeexposers.Interface

	routeIndexer cache.Indexer

	routeClientset routeclientset.Interface
	kubeClientset  kubernetes.Interface

	routeInformer  cache.SharedIndexInformer
	secretInformer cache.SharedIndexInformer

	routeLister  routelistersv1.RouteLister
	secretLister kcorelistersv1.SecretLister

	// routeInformerSynced returns true if the Route store has been synced at least once.
	// Added as a member to the struct to allow injection for testing.
	routeInformerSynced cache.InformerSynced

	// secretInformerSynced returns true if the Secret store has been synced at least once.
	// Added as a member to the struct to allow injection for testing.
	secretInformerSynced cache.InformerSynced

	recorder record.EventRecorder

	queue workqueue.RateLimitingInterface

	exposerIP     string
	exposerPort   int32
	selfNamespace string
	selfSelector  map[string]string

	defaultRouteTermination routev1.InsecureEdgeTerminationPolicyType

	labels 		  map[string]string
}

func NewRouteController(
	acmeClientFactory *acmeclientbuilder.SharedClientFactory,
	exposers map[string]challengeexposers.Interface,
	routeClientset routeclientset.Interface,
	kubeClientset kubernetes.Interface,
	routeInformer cache.SharedIndexInformer,
	secretInformer cache.SharedIndexInformer,
	exposerIP string,
	exposerPort int32,
	selfNamespace string,
	selfSelector map[string]string,
	defaultRouteTermination routev1.InsecureEdgeTerminationPolicyType,
	labels map[string]string,
) *RouteController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientset.CoreV1().Events("")})

	rc := &RouteController{
		acmeClientFactory: acmeClientFactory,

		exposers: exposers,

		routeIndexer: routeInformer.GetIndexer(),

		routeClientset: routeClientset,
		kubeClientset:  kubeClientset,

		routeInformer:  routeInformer,
		secretInformer: secretInformer,

		routeLister:  routelistersv1.NewRouteLister(routeInformer.GetIndexer()),
		secretLister: kcorelistersv1.NewSecretLister(secretInformer.GetIndexer()),

		routeInformerSynced:  routeInformer.HasSynced,
		secretInformerSynced: secretInformer.HasSynced,

		recorder: eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: ControllerName}),

		queue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),

		exposerIP:     exposerIP,
		exposerPort:   exposerPort,
		selfNamespace: selfNamespace,
		selfSelector:  selfSelector,

		defaultRouteTermination: defaultRouteTermination,
		labels: labels,
	}

	routeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    rc.addRoute,
		UpdateFunc: rc.updateRoute,
		DeleteFunc: rc.deleteRoute,
	})
	secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: rc.updateSecret,
		DeleteFunc: rc.deleteSecret,
	})

	return rc
}

func (rc *RouteController) enqueueRoute(route *routev1.Route) {
	key, err := KeyFunc(route)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for object %#v: %v", route, err))
		return
	}

	rc.queue.Add(key)
}

func (rc *RouteController) addRoute(obj interface{}) {
	route := obj.(*routev1.Route)
	if !util.IsManaged(route) {
		glog.V(5).Infof("Skipping Route %s/%s UID=%s RV=%s", route.Namespace, route.Name, route.UID, route.ResourceVersion)
		return
	}

	glog.V(4).Infof("Adding Route %s/%s UID=%s RV=%s", route.Namespace, route.Name, route.UID, route.ResourceVersion)
	rc.enqueueRoute(route)
}

func (rc *RouteController) updateRoute(old, cur interface{}) {
	oldRoute := old.(*routev1.Route)
	newRoute := cur.(*routev1.Route)

	if !util.IsManaged(newRoute) {
		glog.V(5).Infof("Skipping Route %s/%s UID=%s RV=%s", newRoute.Namespace, newRoute.Name, newRoute.UID, newRoute.ResourceVersion)
		return
	}

	glog.V(4).Infof("Updating Route from %s/%s UID=%s RV=%s to %s/%s UID=%s,RV=%s",
		oldRoute.Namespace, oldRoute.Name, oldRoute.UID, oldRoute.ResourceVersion,
		newRoute.Namespace, newRoute.Name, newRoute.UID, newRoute.ResourceVersion)

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

	if !util.IsManaged(route) {
		glog.V(5).Infof("Skipping Route %s/%s UID=%s RV=%s", route.Namespace, route.Name, route.UID, route.ResourceVersion)
		return
	}

	glog.V(4).Infof("Deleting Route %s/%s UID=%s RV=%s", route.Namespace, route.Name, route.UID, route.ResourceVersion)
	rc.enqueueRoute(route)
}

func (rc *RouteController) updateSecret(old, cur interface{}) {
	oldSecret := old.(*corev1.Secret)
	curSecret := cur.(*corev1.Secret)

	// Ignore periodic re-lists for Secrets.
	if oldSecret.ResourceVersion == curSecret.ResourceVersion {
		return
	}

	curControllerRef := metav1.GetControllerOf(curSecret)

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		route := rc.resolveControllerRef(curSecret.Namespace, curControllerRef)
		if route == nil {
			return
		}
		glog.V(4).Infof("Acme Secret %s/%s updated.", curSecret.Namespace, curSecret.Name)
		rc.enqueueRoute(route)
		return
	}
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

	glog.V(4).Infof("Secret %s/%s deleted.", secret.Namespace, secret.Name)
	rc.enqueueRoute(route)
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (rc *RouteController) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *routev1.Route {
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}

	route, err := rc.routeLister.Routes(namespace).Get(controllerRef.Name)
	if err != nil {
		return nil
	}

	if route.UID != controllerRef.UID {
		return nil
	}

	return route
}

// TODO: extract this function to be re-used by ingress controller
// FIXME: needs expectation protection
func (rc *RouteController) getState(t time.Time, route *routev1.Route, accountUrl string) api.AcmeState {
	if route.Annotations != nil {
		_, ok := route.Annotations[api.AcmeAwaitingAuthzUrlAnnotation]
		if ok {
			owner, ok := route.Annotations[api.AcmeAwaitingAuthzUrlOwnerAnnotation]
			if !ok {
				glog.Warning("Missing Route with %q annotation is missing %q annotation!", api.AcmeAwaitingAuthzUrlAnnotation, api.AcmeAwaitingAuthzUrlOwnerAnnotation)
				return api.AcmeStateNeedsCert
			}

			if owner != accountUrl {
				glog.Warning("%s mismatch: authorization owner is %q but current account is %q. This is likely because the acme-account was recreated in the meantime.", api.AcmeAwaitingAuthzUrlOwnerAnnotation, owner, accountUrl)
				return api.AcmeStateNeedsCert
			}

			return api.AcmeStateWaitingForAuthz
		}
	}

	if route.Spec.TLS == nil || route.Spec.TLS.Key == "" || route.Spec.TLS.Certificate == "" {
		return api.AcmeStateNeedsCert
	}

	certPemData := &cert.CertPemData{
		Key: []byte(route.Spec.TLS.Key),
		Crt: []byte(route.Spec.TLS.Certificate),
	}
	certificate, err := certPemData.Certificate()
	if err != nil {
		glog.Errorf("Failed to decode certificate from route %s/%s", route.Namespace, route.Name)
		return api.AcmeStateNeedsCert
	}

	err = certificate.VerifyHostname(route.Spec.Host)
	if err != nil {
		glog.Errorf("Certificate is invalid for route %s/%s with hostname %q", route.Namespace, route.Name, route.Spec.Host)
		return api.AcmeStateNeedsCert
	}

	if !cert.IsValid(certificate, t) {
		return api.AcmeStateNeedsCert
	}

	// We need to trigger renewals before the certs expire
	remains := certificate.NotAfter.Sub(t)
	lifetime := certificate.NotAfter.Sub(certificate.NotBefore)

	// This is the deadline when we start renewing
	if remains <= lifetime/3 {
		glog.Infof("Renewing cert because we reached a deadline of %s", remains)
		return api.AcmeStateNeedsCert
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
			glog.V(4).Infof("Renewing cert in advance with %s remaining to spread the load.", remains)
			return api.AcmeStateNeedsCert
		}
	}

	return api.AcmeStateOk
}

func (rc *RouteController) wrapExposers(exposers map[string]challengeexposers.Interface, route *routev1.Route) map[string]challengeexposers.Interface {
	wrapped := make(map[string]challengeexposers.Interface)

	for k, v := range exposers {
		if k == "http-01" {
			wrapped[k] = NewExposer(v, rc.routeClientset, rc.kubeClientset, rc.recorder, rc.exposerIP, rc.exposerPort, rc.selfNamespace, rc.selfSelector, route, rc.labels)
		} else {
			wrapped[k] = v
		}
	}

	return wrapped
}

// handle is the business logic of the controller.
// In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
// This function is not meant to be invoked concurrently with the same key.
// TODO: extract common parts to be re-used by ingress controller
func (rc *RouteController) handle(key string) error {
	startTime := time.Now()
	glog.V(4).Infof("Started syncing Route %q (%v)", key, startTime)
	defer func() {
		glog.V(4).Infof("Finished syncing Route %q (%v)", key, time.Since(startTime))
	}()

	objReadOnly, exists, err := rc.routeIndexer.GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		glog.V(4).Infof("Route %s does not exist anymore\n", key)
		return nil
	}

	// Deep copy to avoid mutating the cache
	routeReadOnly := objReadOnly.(*routev1.Route)

	// Don't act on objects that are being deleted
	if routeReadOnly.DeletionTimestamp != nil {
		return nil
	}

	// We have to check if Route is admitted to be sure it owns the domain!
	if !routeutil.IsAdmitted(routeReadOnly) {
		glog.V(4).Infof("Skipping Route %s because it's not admitted", key)
		return nil
	}

	if routeReadOnly.Annotations[api.TlsAcmePausedAnnotation] == "true" {
		glog.V(4).Infof("Skipping Route %s because it is paused", key)

		// TODO: reconcile (e.g. related secrets)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), AcmeTimeout)
	defer cancel()
	client, err := rc.acmeClientFactory.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get ACME client: %v", err)
	}
	state := rc.getState(startTime, routeReadOnly, client.Account.URI)
	// FIXME: this state machine needs protection with expectations
	// (informers may not be synced yet with recent state transition updates)
	switch state {
	case api.AcmeStateNeedsCert:
		// TODO: Add TTL based lock to allow only one domain to enter this stage

		// FIXME: definitely protect with expectations
		authorization, err := client.Client.Authorize(ctx, routeReadOnly.Spec.Host)
		if err != nil {
			return fmt.Errorf("failed to authorize domain %q: %v", routeReadOnly.Spec.Host, err)
		}
		glog.V(4).Infof("Created authorization %q for Route %s", authorization.URI, key)

		if authorization.Status == acme.StatusValid {
			glog.V(4).Infof("Authorization %q for Route %s is already valid", authorization.URI, key)
		}

		route := routeReadOnly.DeepCopy()
		if route.Annotations == nil {
			route.Annotations = make(map[string]string)
		}

		route.Annotations[api.AcmeAwaitingAuthzUrlAnnotation] = authorization.URI
		route.Annotations[api.AcmeAwaitingAuthzUrlOwnerAnnotation] = client.Account.URI

		// TODO: convert to PATCH to avoid loosing time and rate limits on update collisions
		_, err = rc.routeClientset.RouteV1().Routes(route.Namespace).Update(route)
		if err != nil {
			glog.Errorf("Failed to update Route %s: %v. Revoking authorization %q so it won't stay pending.", key, err, authorization.URI)
			// We need to try to cancel the authorization so we don't leave pending authorization behind and get rate limited
			acmeErr := client.Client.RevokeAuthorization(ctx, authorization.URI)
			if acmeErr != nil {
				glog.Errorf("Failed to revoke authorization %q: %v", acmeErr)
			}

			return fmt.Errorf("failed to update authorizationURI: %v", err)
		}

		return nil

	case api.AcmeStateWaitingForAuthz:
		ctx, cancel := context.WithTimeout(context.Background(), AcmeTimeout)
		defer cancel()

		client, err := rc.acmeClientFactory.GetClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to get ACME client: %v", err)
		}

		authorizationUri := routeReadOnly.Annotations[api.AcmeAwaitingAuthzUrlAnnotation]
		authorization, err := client.Client.GetAuthorization(ctx, authorizationUri)
		// TODO: emit an event but don't fail as user can set it
		if err != nil {
			return fmt.Errorf("failed to get ACME authorization: %v", err)
		}

		glog.V(4).Infof("Route %q: authorization state is %q", key, authorization.Status)

		exposers := rc.wrapExposers(rc.exposers, routeReadOnly)

		switch authorization.Status {
		case acme.StatusPending:
			authorization, err := client.AcceptAuthorization(ctx, authorization, routeReadOnly.Spec.Host, exposers, rc.labels)
			if err != nil {
				return fmt.Errorf("failed to accept ACME authorization: %v", err)
			}

			if authorization.Status == acme.StatusPending {
				glog.V(4).Infof("Re-queuing Route %q due to pending authorization", key)

				// TODO: get this value from authorization when this is fixed
				// https://github.com/golang/go/issues/22457
				retryAfter := 5 * time.Second
				rc.queue.AddAfter(key, retryAfter)

				// Don't count this as requeue, reset counter
				rc.queue.Forget(key)

				return nil
			}

			if authorization.Status != acme.StatusValid {
				return fmt.Errorf("route %q - authorization has transitioned to unexpected state %q", key, authorization.Status)
			}

			fallthrough

		case acme.StatusValid:
			glog.V(4).Infof("Authorization %q for Route %s successfully validated", authorization.URI, key)
			// provision cert
			template := x509.CertificateRequest{
				Subject: pkix.Name{
					CommonName: routeReadOnly.Spec.Host,
				},
			}
			template.DNSNames = append(template.DNSNames, routeReadOnly.Spec.Host)
			privateKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
			if err != nil {
				return fmt.Errorf("failed to generate RSA key: %v", err)
			}

			csr, err := x509.CreateCertificateRequest(cryptorand.Reader, &template, privateKey)
			if err != nil {
				return fmt.Errorf("failed to create certificate request: %v", err)
			}

			// TODO: protect with expectations
			// TODO: aks to split CreateCert func in acme library to avoid embedded pooling
			der, certUrl, err := client.Client.CreateCert(ctx, csr, 0, true)
			if err != nil {
				return fmt.Errorf("failed to create ACME certificate: %v", err)
			}
			glog.V(4).Infof("Route %q - created certificate available at %s", key, certUrl)

			certPemData, err := cert.NewCertificateFromDER(der, privateKey)
			if err != nil {
				return fmt.Errorf("failed to convert certificate from DER to PEM: %v", err)
			}

			route := routeReadOnly.DeepCopy()
			if route.Spec.TLS == nil {
				route.Spec.TLS = &routev1.TLSConfig{
					// Defaults
					InsecureEdgeTerminationPolicy: rc.defaultRouteTermination,
					Termination:                   routev1.TLSTerminationEdge,
				}
			}
			route.Spec.TLS.Key = string(certPemData.Key)
			route.Spec.TLS.Certificate = string(certPemData.Crt)

			delete(route.Annotations, api.AcmeAwaitingAuthzUrlAnnotation)

			updatedRoute, err := rc.routeClientset.RouteV1().Routes(route.Namespace).Update(route)
			if err != nil {
				return fmt.Errorf("failed to update route %s/%s with new certificates: %v", route.Namespace, route.Name, err)
			}

			rc.recorder.Event(updatedRoute, corev1.EventTypeNormal, "AcmeCertificateProvisioned", "Successfully provided new certificate")

			// Clean up tmp objects on success.
			// We should make this more smart when we support more exposers.
			for _, exposer := range exposers {
				exposer.Remove(routeReadOnly.Spec.Host)
			}

		case acme.StatusInvalid:
			rc.recorder.Eventf(routeReadOnly, corev1.EventTypeWarning, "AcmeFailedAuthorization", "Acme provider failed to validate domain %q: %s", routeReadOnly.Spec.Host, acmeclient.GetAuthorizationErrors(authorization))

			route := routeReadOnly.DeepCopy()
			delete(route.Annotations, api.AcmeAwaitingAuthzUrlAnnotation)
			// TODO: remove force pausing when we have ACME rate limiter
			route.Annotations[api.TlsAcmePausedAnnotation] = "true"
			route, err = rc.routeClientset.RouteV1().Routes(route.Namespace).Update(route)
			if err != nil {
				return fmt.Errorf("failed to pause Route: %v", err)
			}

		case acme.StatusRevoked:
			rc.recorder.Eventf(routeReadOnly, corev1.EventTypeWarning, "AcmeRevokedAuthorization", "Acme authorization has been revoked for domain %q", routeReadOnly.Spec.Host)

		case "deactivated":
			glog.V(4).Infof("Authorization %q is %s.", authorization.URI, authorization.Status)

		case acme.StatusProcessing:
			fallthrough
		default:
			return fmt.Errorf("unknow authorization status %s", authorization.Status)
		}

	case api.AcmeStateOk:
	default:
		return fmt.Errorf("failed to determine state for Route: %#v", routeReadOnly)
	}

	err = rc.syncSecret(routeReadOnly)
	if err != nil {
		return fmt.Errorf("failed to sync secret for Route %s/%s: %v", routeReadOnly.Namespace, routeReadOnly.Name, err)
	}

	return nil
}

func (rc *RouteController) syncSecret(routeReadOnly *routev1.Route) error {
	// TODO: consider option of choosing a oldSecret name using an annotation
	secretName := routeReadOnly.Name

	secretExists := true
	oldSecret, err := rc.secretLister.Secrets(routeReadOnly.Namespace).Get(secretName)
	if err != nil {
		if !kapierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get Secret %s/%s: %v", routeReadOnly.Namespace, secretName, err)
		}
		secretExists = false
	}

	// We need to make sure we can modify this oldSecret (has our controllerRef)
	if secretExists {
		controllerRef := GetControllerRef(&oldSecret.ObjectMeta)
		if controllerRef == nil || controllerRef.UID != routeReadOnly.UID {
			rc.recorder.Eventf(routeReadOnly, corev1.EventTypeWarning, "CollidingSecret", "Can't sync certificates for Route %s/%s into Secret %s/%s because it already exists and isn't owned by the Route!", routeReadOnly.Namespace, routeReadOnly.Name, routeReadOnly.Namespace, secretName)
			return nil
		}
	}

	if routeReadOnly.Spec.TLS == nil {
		if !secretExists {
			return nil
		}

		var gracePeriod int64 = 0
		propagationPolicy := metav1.DeletePropagationBackground
		preconditions := metav1.Preconditions{
			UID: &oldSecret.UID,
		}
		err := rc.kubeClientset.CoreV1().Secrets(routeReadOnly.Namespace).Delete(secretName, &metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
			PropagationPolicy:  &propagationPolicy,
			Preconditions:      &preconditions,
		})
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete oldSecret %s/%s: %s", routeReadOnly.Namespace, secretName, err)
			}
		}

		return nil
	}

	// Route has TLS; we need to sync it into a Secret
	var newSecret *corev1.Secret
	if secretExists {
		newSecret = oldSecret.DeepCopy()
	} else {
		newSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: secretName,
			},
		}
	}

	trueVal := true
	newSecret.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: controllerKind.GroupVersion().String(),
			Kind:       controllerKind.Kind,
			Name:       routeReadOnly.Name,
			UID:        routeReadOnly.UID,
			Controller: &trueVal,
		},
	}

	newSecret.Type = corev1.SecretTypeTLS

	if newSecret.Data == nil {
		newSecret.Data = make(map[string][]byte)
	}
	newSecret.Data[corev1.TLSCertKey] = []byte(routeReadOnly.Spec.TLS.Certificate)
	newSecret.Data[corev1.TLSPrivateKeyKey] = []byte(routeReadOnly.Spec.TLS.Key)

	if !secretExists {
		_, err = rc.kubeClientset.CoreV1().Secrets(routeReadOnly.Namespace).Create(newSecret)
		if err != nil {
			return fmt.Errorf("failed to create Secret %s/%s with TLS data: %v", routeReadOnly.Namespace, newSecret.Name, err)
		}

		return nil
	}

	if !reflect.DeepEqual(oldSecret, newSecret) {
		_, err = rc.kubeClientset.CoreV1().Secrets(routeReadOnly.Namespace).Update(newSecret)
		if err != nil {
			return fmt.Errorf("failed to update Secret %s/%s with TLS data: %v", routeReadOnly.Namespace, newSecret.Name, err)
		}
	}

	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (rc *RouteController) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		rc.queue.Forget(key)
		return
	}

	if rc.queue.NumRequeues(key) < MaxRetries {
		glog.Infof("Error syncing Route %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		rc.queue.AddRateLimited(key)
		return
	}

	rc.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	glog.Infof("Dropping Route %q out of the queue: %v", key, err)
}

func (rc *RouteController) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := rc.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two Routes with the same key are never processed in
	// parallel.
	defer rc.queue.Done(key)

	// Invoke the method containing the business logic
	err := rc.handle(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	rc.handleErr(err, key)
	return true
}

func (rc *RouteController) runWorker() {
	for rc.processNextItem() {
	}
}

func (rc *RouteController) Run(workers int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer rc.queue.ShutDown()

	glog.Info("Starting Route controller")

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, rc.routeInformerSynced, rc.secretInformerSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	glog.Info("Starting Route controller: informer caches synced")

	for i := 0; i < workers; i++ {
		go wait.Until(rc.runWorker, time.Second, stopCh)
	}

	<-stopCh

	glog.Info("Stopping Route controller")
}
