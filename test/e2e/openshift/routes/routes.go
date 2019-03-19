package routes

import (
	"context"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/tnozicka/openshift-acme/pkg/acme/client/builder"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

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

func DeleteACMEAccountIfRequested(f *framework.Framework, notFoundOK bool) error {
	namespace := exutil.DeleteAccountBetweenStepsInNamespace()
	if namespace == "" {
		return nil
	}
	name := "acme-account"

	// We need to deactivate account first because controller uses informer and might have it cached
	secret, err := f.KubeAdminClientSet().CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if !notFoundOK {
				return err
			}
		} else {
			return err
		}
	}

	client, err := builder.BuildClientFromSecret(secret)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client.DeactivateAccount(ctx, client.Account)

	var grace int64 = 0
	propagation := metav1.DeletePropagationForeground
	framework.Logf("Deleting account Secret %s/%s", namespace, name)
	err = f.KubeAdminClientSet().CoreV1().Secrets(namespace).Delete(name, &metav1.DeleteOptions{
		PropagationPolicy:  &propagation,
		GracePeriodSeconds: &grace,
	})
	if err != nil {
		return err
	}

	return nil
}

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

	tmpEndpoints, err := f.KubeClientSet().CoreV1().Endpoints(route.Namespace).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromValidatedSet(labels.Set{
			api.ExposerForLabelName: string(route.UID),
		}).String(),
	})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, tmpEndpoint := range tmpEndpoints.Items {
		o.Expect(tmpEndpoint.DeletionTimestamp).NotTo(o.BeNil())
	}
}

var _ = g.Describe("Routes", func() {
	defer g.GinkgoRecover()
	f := framework.NewFramework("routes")

	g.It("should be provisioned with certificates", func() {
		namespace := f.Namespace()

		// ACME server will likely cache the validation for our domain and won't retry it so soon.
		err := DeleteACMEAccountIfRequested(f, true)

		g.By("creating new Route without TLS")
		route := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				// Test that the exposing goes fine even with a subdomain in name as other objects
				// whose name might be based on this one may not support it if not normalized.
				// See https://github.com/tnozicka/openshift-acme/issues/50
				Name: "subdomain.test",
				Annotations: map[string]string{
					"kubernetes.io/tls-acme": "true",
				},
			},
			Spec: routev1.RouteSpec{
				Host: exutil.GetDomain(),
				To: routev1.RouteTargetReference{
					Name: "non-existing",
				},
			},
		}
		route, err = f.RouteClientset().RouteV1().Routes(namespace).Create(route)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("waiting for Route to be admitted by the router")
		w, err := f.RouteClientset().RouteV1().Routes(namespace).Watch(metav1.SingleObject(route.ObjectMeta))
		o.Expect(err).NotTo(o.HaveOccurred())
		event, err := watch.Until(RouteAdmissionTimeout, w, util.RouteAdmittedFunc())
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for Route to be admitted by the router!")

		route = event.Object.(*routev1.Route)

		g.By("waiting for initial certificate to be provisioned")
		w, err = f.RouteClientset().RouteV1().Routes(namespace).Watch(metav1.SingleObject(route.ObjectMeta))
		o.Expect(err).NotTo(o.HaveOccurred())
		event, err = watch.Until(CertificateProvisioningTimeout, w, util.RouteTLSChangedFunc(route.Spec.TLS))
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for certificate to be provisioned!")

		route = event.Object.(*routev1.Route)
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

		// ACME server will likely cache the validation for our domain and won't retry it so soon.
		err = DeleteACMEAccountIfRequested(f, false)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("deleting the initial certificate and waiting for new one to be provisioned")
		routeCopy := route.DeepCopy()
		routeCopy.Spec.TLS = nil
		route, err = f.RouteClientset().RouteV1().Routes(namespace).Patch(route.Name, types.StrategicMergePatchType, []byte(`{"spec":{"tls":{"certificate":"","key":""}}}`))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(route.Spec.TLS).NotTo(o.BeNil())
		o.Expect(route.Spec.TLS.Certificate).To(o.BeEmpty())
		o.Expect(route.Spec.TLS.Key).To(o.BeEmpty())

		w, err = f.RouteClientset().RouteV1().Routes(namespace).Watch(metav1.SingleObject(route.ObjectMeta))
		o.Expect(err).NotTo(o.HaveOccurred())
		event, err = watch.Until(CertificateProvisioningTimeout, w, util.RouteTLSChangedFunc(route.Spec.TLS))
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for certificate to be re-provisioned!")

		g.By("validating the certificate")
		route = event.Object.(*routev1.Route)
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

		w, err = f.KubeClientSet().CoreV1().Secrets(secret.Namespace).Watch(metav1.SingleObject(secret.ObjectMeta))
		o.Expect(err).NotTo(o.HaveOccurred())
		event, err = watch.Until(SyncTimeout, w, util.SecretDataChangedFunc(secret.Data))
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for Secret to be synced!")

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

		// ACME server will likely cache the validation for our domain and won't retry it so soon.
		err := DeleteACMEAccountIfRequested(f, true)

		g.By("creating new Route with expired certificate")
		now := time.Now()
		notBefore := now.Add(-1 * time.Hour)
		notAfter := now.Add(-1 * time.Minute)
		certData, err := generateCertificate([]string{exutil.GetDomain()}, notBefore, notAfter)
		o.Expect(err).NotTo(o.HaveOccurred())
		certificate, err := certData.Certificate()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cert.IsValid(certificate, now)).To(o.BeFalse())

		route := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				Annotations: map[string]string{
					"kubernetes.io/tls-acme": "true",
				},
			},
			Spec: routev1.RouteSpec{
				Host: exutil.GetDomain(),
				To: routev1.RouteTargetReference{
					Name: "non-existing",
				},
				TLS: &routev1.TLSConfig{
					Termination:                   routev1.TLSTerminationEdge,
					InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyAllow,
					Key:         string(certData.Key),
					Certificate: string(certData.Crt),
				},
			},
		}
		route, err = f.RouteClientset().RouteV1().Routes(namespace).Create(route)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("waiting for Route to be admitted by the router")
		w, err := f.RouteClientset().RouteV1().Routes(namespace).Watch(metav1.SingleObject(route.ObjectMeta))
		o.Expect(err).NotTo(o.HaveOccurred())
		event, err := watch.Until(RouteAdmissionTimeout, w, util.RouteAdmittedFunc())
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for Route to be admitted by the router!")

		route = event.Object.(*routev1.Route)

		g.By("waiting for the certificate to be updated")
		w, err = f.RouteClientset().RouteV1().Routes(namespace).Watch(metav1.SingleObject(route.ObjectMeta))
		o.Expect(err).NotTo(o.HaveOccurred())
		event, err = watch.Until(CertificateProvisioningTimeout, w, util.RouteTLSChangedFunc(route.Spec.TLS))
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for certificate to be provisioned!")

		g.By("validating for the updated certificate")
		route = event.Object.(*routev1.Route)

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

		// ACME server will likely cache the validation for our domain and won't retry it so soon.
		err := DeleteACMEAccountIfRequested(f, true)

		g.By("creating new Route with unmatching certificate")
		domain := "test.local"
		o.Expect(domain).NotTo(o.Equal(exutil.GetDomain()))

		now := time.Now()
		notBefore := now.Add(-1 * time.Hour)
		notAfter := now.Add(1 * time.Hour)
		certData, err := generateCertificate([]string{domain}, notBefore, notAfter)
		o.Expect(err).NotTo(o.HaveOccurred())
		certificate, err := certData.Certificate()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(certificate.DNSNames[0]).NotTo(o.Equal(exutil.GetDomain()))
		o.Expect(cert.IsValid(certificate, now)).To(o.BeTrue())

		route := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				Annotations: map[string]string{
					"kubernetes.io/tls-acme": "true",
				},
			},
			Spec: routev1.RouteSpec{
				Host: exutil.GetDomain(),
				To: routev1.RouteTargetReference{
					Name: "non-existing",
				},
				TLS: &routev1.TLSConfig{
					Termination:                   routev1.TLSTerminationEdge,
					InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyAllow,
					Key:         string(certData.Key),
					Certificate: string(certData.Crt),
				},
			},
		}
		route, err = f.RouteClientset().RouteV1().Routes(namespace).Create(route)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("waiting for Route to be admitted by the router")
		w, err := f.RouteClientset().RouteV1().Routes(namespace).Watch(metav1.SingleObject(route.ObjectMeta))
		o.Expect(err).NotTo(o.HaveOccurred())
		event, err := watch.Until(RouteAdmissionTimeout, w, util.RouteAdmittedFunc())
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for Route to be admitted by the router!")

		route = event.Object.(*routev1.Route)
		g.By("waiting for certificate to be updated")
		w, err = f.RouteClientset().RouteV1().Routes(namespace).Watch(metav1.SingleObject(route.ObjectMeta))
		o.Expect(err).NotTo(o.HaveOccurred())
		event, err = watch.Until(CertificateProvisioningTimeout, w, util.RouteTLSChangedFunc(route.Spec.TLS))
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to wait for certificate to be provisioned!")

		g.By("validating updated certificate")
		route = event.Object.(*routev1.Route)

		o.Expect(route.Spec.TLS).NotTo(o.BeNil())

		certificate, err = util.CertificateFromPEM([]byte(route.Spec.TLS.Certificate))
		o.Expect(err).NotTo(o.HaveOccurred())

		now = time.Now()
		o.Expect(now.Before(certificate.NotBefore)).To(o.BeFalse())
		o.Expect(now.After(certificate.NotAfter)).To(o.BeFalse())
		o.Expect(cert.IsValid(certificate, now)).To(o.BeTrue())
		o.Expect(certificate.DNSNames[0]).To(o.Equal(exutil.GetDomain()))

		validateSyncedSecret(f, route)
		validateTemporaryObjectsAreDeleted(f, route)
	})
})
