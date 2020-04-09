package routes

import (
	"context"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"
	watchtools "k8s.io/client-go/tools/watch"

	"github.com/tnozicka/openshift-acme/pkg/api"
	"github.com/tnozicka/openshift-acme/pkg/cert"
	"github.com/tnozicka/openshift-acme/pkg/util"
	"github.com/tnozicka/openshift-acme/test/e2e/framework"
	exutil "github.com/tnozicka/openshift-acme/test/e2e/openshift/util"
)

const (
	RouteAdmissionTimeout          = 15 * time.Second
	CertificateProvisioningTimeout = 5 * time.Minute
	SyncTimeout                    = 30 * time.Second
)

func validateSyncedSecret(f *framework.Framework, route *routev1.Route) {
	g.By("Waiting for Route's certificate to be synced to a Secret")
	// Routes don't support Generation
	time.Sleep(3 * time.Second)

	g.By("Validating the certificate is synced to a Secret")

	secret, err := f.KubeClientSet().CoreV1().Secrets(route.Namespace).Get(route.Name, metav1.GetOptions{})
	if route.Spec.TLS == nil {
		o.Expect(apierrors.IsNotFound(err)).To(o.BeTrue())
		return
	}
	o.Expect(err).NotTo(o.HaveOccurred())

	o.Expect(secret.DeletionTimestamp).To(o.BeNil())
	o.Expect(secret.OwnerReferences).To(o.HaveLen(1))
	o.Expect(secret.OwnerReferences[0].Kind).To(o.Equal("Route"))
	o.Expect(secret.OwnerReferences[0].Name).To(o.Equal(route.Name))
	o.Expect(secret.OwnerReferences[0].UID).To(o.Equal(route.UID))
	o.Expect(secret.OwnerReferences[0].Controller).NotTo(o.BeNil())
	o.Expect(*secret.OwnerReferences[0].Controller).To(o.BeTrue())

	o.Expect(secret.Type).To(o.Equal(corev1.SecretTypeTLS))

	o.Expect(secret.Data).NotTo(o.BeNil())
	o.Expect(string(secret.Data[corev1.TLSPrivateKeyKey])).To(o.Equal(route.Spec.TLS.Key))
	o.Expect(string(secret.Data[corev1.TLSCertKey])).To(o.Equal(route.Spec.TLS.Certificate))

}

func validateTemporaryObjectsAreDeleted(f *framework.Framework, route *routev1.Route) {
	g.By("Validating that temporary objects are deleted")

	// GC cleans objects asynchronously.
	// TODO: We should wait properly.
	time.Sleep(5 * time.Second)

	tmpRoutes, err := f.RouteClientset().RouteV1().Routes(route.Namespace).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromValidatedSet(labels.Set{
			api.ExposerForLabelName: string(route.UID),
		}).String(),
	})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, tmpRoute := range tmpRoutes.Items {
		o.Expect(tmpRoute.DeletionTimestamp).NotTo(o.BeNil())
	}

	tmpServices, err := f.KubeClientSet().CoreV1().Services(route.Namespace).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromValidatedSet(labels.Set{
			api.ExposerForLabelName: string(route.UID),
		}).String(),
	})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, tmpService := range tmpServices.Items {
		o.Expect(tmpService.DeletionTimestamp).NotTo(o.BeNil())
	}

	tmpReplicaSets, err := f.KubeClientSet().AppsV1().ReplicaSets(route.Namespace).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromValidatedSet(labels.Set{
			api.ExposerForLabelName: string(route.UID),
		}).String(),
	})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, tmpReplicaSet := range tmpReplicaSets.Items {
		o.Expect(tmpReplicaSet.DeletionTimestamp).NotTo(o.BeNil())
	}
}

var _ = g.Describe("Routes", func() {
	defer g.GinkgoRecover()
	f := framework.NewFramework("routes")

	g.It("should be provisioned with certificates", func() {
		namespace := f.Namespace()

		// Create a limit range so we know creating Pods work in such environment
		_, err := f.KubeAdminClientSet().CoreV1().LimitRanges(namespace).Create(&corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type: corev1.LimitTypePod,
						Min: corev1.ResourceList{
							corev1.ResourceCPU:    *resource.NewMilliQuantity(100, resource.DecimalSI),       // higher then our regular ask
							corev1.ResourceMemory: *resource.NewQuantity(100*(1024*1024), resource.BinarySI), // higher then our regular ask
						},
					},
				},
			},
		})
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("creating new Route without TLS")
		name := names.SimpleNameGenerator.GenerateName("test-")
		route := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Annotations: map[string]string{
					"kubernetes.io/tls-acme":        "true",
					"acme.openshift.io/secret-name": "",
				},
			},
			Spec: routev1.RouteSpec{
				Host: exutil.GenerateDomain(namespace, name),
				To: routev1.RouteTargetReference{
					Name: "non-existing",
				},
			},
		}
		route, err = f.RouteClientset().RouteV1().Routes(namespace).Create(route)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("waiting for Route to be admitted by the router")
		{
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), RouteAdmissionTimeout)
			defer cancel()
			event, err := WaitForRouteCondition(
				ctx,
				f.RouteClientset().RouteV1().Routes(namespace),
				route.Namespace,
				route.Name,
				util.RouteAdmittedFunc,
			)
			o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for Route to be admitted by the router!")

			route = event.Object.(*routev1.Route)
		}

		g.By("waiting for initial certificate to be provisioned")
		{
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), CertificateProvisioningTimeout)
			defer cancel()
			event, err := WaitForRouteCondition(
				ctx,
				f.RouteClientset().RouteV1().Routes(namespace),
				route.Namespace,
				route.Name,
				util.RouteTLSChangedFunc(route.Spec.TLS),
			)
			o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for certificate to be provisioned!")

			route = event.Object.(*routev1.Route)
		}
		o.Expect(route.Spec.TLS).NotTo(o.BeNil())

		o.Expect(route.Spec.TLS.Termination).To(o.Equal(routev1.TLSTerminationEdge))

		crt, err := util.CertificateFromPEM([]byte(route.Spec.TLS.Certificate))
		o.Expect(err).NotTo(o.HaveOccurred())

		now := time.Now()
		o.Expect(now.Before(crt.NotBefore)).To(o.BeFalse())
		o.Expect(now.After(crt.NotAfter)).To(o.BeFalse())
		o.Expect(cert.IsValid(crt, now)).To(o.BeTrue())

		validateSyncedSecret(f, route)
		validateTemporaryObjectsAreDeleted(f, route)

		g.By("deleting the initial certificate and waiting for new one to be provisioned")
		routeCopy := route.DeepCopy()
		routeCopy.Spec.TLS = nil
		route, err = f.RouteClientset().RouteV1().Routes(namespace).Patch(route.Name, types.StrategicMergePatchType, []byte(`{"spec":{"tls":{"certificate":"","key":""}}}`))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(route.Spec.TLS).NotTo(o.BeNil())
		o.Expect(route.Spec.TLS.Certificate).To(o.BeEmpty())
		o.Expect(route.Spec.TLS.Key).To(o.BeEmpty())

		{
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), CertificateProvisioningTimeout)
			defer cancel()
			event, err := WaitForRouteCondition(
				ctx,
				f.RouteClientset().RouteV1().Routes(namespace),
				route.Namespace,
				route.Name,
				util.RouteTLSChangedFunc(route.Spec.TLS),
			)
			o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for certificate to be re-provisioned!")

			route = event.Object.(*routev1.Route)
		}
		g.By("validating the certificate")
		o.Expect(route.Spec.TLS).NotTo(o.BeNil())

		o.Expect(route.Spec.TLS.Termination).To(o.Equal(routev1.TLSTerminationEdge))

		crt, err = util.CertificateFromPEM([]byte(route.Spec.TLS.Certificate))
		o.Expect(err).NotTo(o.HaveOccurred())

		now = time.Now()
		o.Expect(now.Before(crt.NotBefore)).To(o.BeFalse())
		o.Expect(now.After(crt.NotAfter)).To(o.BeFalse())
		o.Expect(cert.IsValid(crt, now)).To(o.BeTrue())

		validateSyncedSecret(f, route)
		validateTemporaryObjectsAreDeleted(f, route)

		g.By("updating the synced Secret and seeing it reconciled")
		secret, err := f.KubeClientSet().CoreV1().Secrets(route.Namespace).Patch(route.Name, types.StrategicMergePatchType, []byte(`{"data":{"tls.key":"", "tls.crt":""}}`))
		o.Expect(err).NotTo(o.HaveOccurred())

		{
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), SyncTimeout)
			defer cancel()
			_, err := WaitForSecretCondition(
				ctx,
				f.KubeClientSet().CoreV1().Secrets(route.Namespace),
				route.Namespace,
				route.Name,
				util.SecretDataChangedFunc(secret.Data),
			)
			o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for Secret to be synced!")
		}

		validateSyncedSecret(f, route)
		validateTemporaryObjectsAreDeleted(f, route)

		g.By("deleting the synced Secret and seeing it recreated")
		foregroundDeletion := metav1.DeletePropagationForeground
		gracePeriod := int64(0)
		err = f.KubeClientSet().CoreV1().Secrets(route.Namespace).Delete(route.Name, &metav1.DeleteOptions{
			PropagationPolicy:  &foregroundDeletion,
			GracePeriodSeconds: &gracePeriod,
		})
		o.Expect(err).NotTo(o.HaveOccurred())

		validateSyncedSecret(f, route)
		validateTemporaryObjectsAreDeleted(f, route)
	})

	g.It("should have expired certificates replaced", func() {
		namespace := f.Namespace()

		g.By("creating new Route with expired certificate")
		now := time.Now()
		notBefore := now.Add(-1 * time.Hour)
		notAfter := now.Add(-1 * time.Minute)
		name := names.SimpleNameGenerator.GenerateName("test-")
		domain := exutil.GenerateDomain(namespace, name)
		certData, err := generateCertificate([]string{domain}, notBefore, notAfter)
		o.Expect(err).NotTo(o.HaveOccurred())
		certificate, err := certData.Certificate()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cert.IsValid(certificate, now)).To(o.BeFalse())

		route := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Annotations: map[string]string{
					"kubernetes.io/tls-acme":        "true",
					"acme.openshift.io/secret-name": "",
				},
			},
			Spec: routev1.RouteSpec{
				Host: domain,
				To: routev1.RouteTargetReference{
					Name: "non-existing",
				},
				TLS: &routev1.TLSConfig{
					Termination:                   routev1.TLSTerminationEdge,
					InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyAllow,
					Key:                           string(certData.Key),
					Certificate:                   string(certData.Crt),
				},
			},
		}
		route, err = f.RouteClientset().RouteV1().Routes(namespace).Create(route)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("waiting for Route to be admitted by the router")
		{
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), RouteAdmissionTimeout)
			defer cancel()
			event, err := WaitForRouteCondition(
				ctx,
				f.RouteClientset().RouteV1().Routes(namespace),
				route.Namespace,
				route.Name,
				util.RouteAdmittedFunc,
			)
			o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for Route to be admitted by the router!")

			route = event.Object.(*routev1.Route)
		}

		g.By("waiting for the certificate to be updated")
		{
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), CertificateProvisioningTimeout)
			defer cancel()
			event, err := WaitForRouteCondition(
				ctx,
				f.RouteClientset().RouteV1().Routes(namespace),
				route.Namespace,
				route.Name,
				util.RouteTLSChangedFunc(route.Spec.TLS),
			)
			o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for certificate to be provisioned!")

			route = event.Object.(*routev1.Route)
		}
		o.Expect(route.Spec.TLS).NotTo(o.BeNil())

		crt, err := util.CertificateFromPEM([]byte(route.Spec.TLS.Certificate))
		o.Expect(err).NotTo(o.HaveOccurred())

		now = time.Now()
		o.Expect(now.Before(crt.NotBefore)).To(o.BeFalse())
		o.Expect(now.After(crt.NotAfter)).To(o.BeFalse())
		o.Expect(cert.IsValid(crt, now)).To(o.BeTrue())

		validateSyncedSecret(f, route)
		validateTemporaryObjectsAreDeleted(f, route)
	})

	g.It("should have unmatching certificates replaced", func() {
		namespace := f.Namespace()

		g.By("creating new Route with unmatching certificate")
		name := names.SimpleNameGenerator.GenerateName("test-")
		domain := exutil.GenerateDomain(namespace, name)
		unmathchingDomain := "test.local"
		o.Expect(unmathchingDomain).NotTo(o.Equal(domain))

		now := time.Now()
		notBefore := now.Add(-1 * time.Hour)
		notAfter := now.Add(1 * time.Hour)
		certData, err := generateCertificate([]string{unmathchingDomain}, notBefore, notAfter)
		o.Expect(err).NotTo(o.HaveOccurred())
		certificate, err := certData.Certificate()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(certificate.DNSNames[0]).NotTo(o.Equal(domain))
		o.Expect(cert.IsValid(certificate, now)).To(o.BeTrue())

		route := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Annotations: map[string]string{
					"kubernetes.io/tls-acme":        "true",
					"acme.openshift.io/secret-name": "",
				},
			},
			Spec: routev1.RouteSpec{
				Host: domain,
				To: routev1.RouteTargetReference{
					Name: "non-existing",
				},
				TLS: &routev1.TLSConfig{
					Termination:                   routev1.TLSTerminationEdge,
					InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyAllow,
					Key:                           string(certData.Key),
					Certificate:                   string(certData.Crt),
				},
			},
		}
		route, err = f.RouteClientset().RouteV1().Routes(namespace).Create(route)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("waiting for Route to be admitted by the router")
		{
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), RouteAdmissionTimeout)
			defer cancel()
			event, err := WaitForRouteCondition(
				ctx,
				f.RouteClientset().RouteV1().Routes(namespace),
				route.Namespace,
				route.Name,
				util.RouteAdmittedFunc,
			)
			o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for Route to be admitted by the router!")

			route = event.Object.(*routev1.Route)
		}
		g.By("waiting for certificate to be updated")
		{
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), CertificateProvisioningTimeout)
			defer cancel()
			event, err := WaitForRouteCondition(
				ctx,
				f.RouteClientset().RouteV1().Routes(namespace),
				route.Namespace,
				route.Name,
				util.RouteTLSChangedFunc(route.Spec.TLS),
			)
			o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for certificate to be provisioned!")

			route = event.Object.(*routev1.Route)
		}

		g.By("validating updated certificate")
		o.Expect(route.Spec.TLS).NotTo(o.BeNil())

		certificate, err = util.CertificateFromPEM([]byte(route.Spec.TLS.Certificate))
		o.Expect(err).NotTo(o.HaveOccurred())

		now = time.Now()
		o.Expect(now.Before(certificate.NotBefore)).To(o.BeFalse())
		o.Expect(now.After(certificate.NotAfter)).To(o.BeFalse())
		o.Expect(cert.IsValid(certificate, now)).To(o.BeTrue())
		o.Expect(certificate.DNSNames[0]).To(o.Equal(domain))

		validateSyncedSecret(f, route)
		validateTemporaryObjectsAreDeleted(f, route)
	})
})
