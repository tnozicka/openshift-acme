package util

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/tnozicka/openshift-acme/test/e2e/framework"
)

var TestContext *framework.TestContextType = &framework.TestContext

func KubeConfigPath() string {
	return os.Getenv("KUBECONFIG")
}

func LookupBaseDomain() (string, bool) {
	return os.LookupEnv("E2E_DOMAIN")
}

func GetBaseDomain() string {
	domain, _ := LookupBaseDomain()
	return domain
}

func GenerateDomain(namespace, name string) string {
	domain, ok := LookupBaseDomain()
	if !ok || len(domain) == 0 {
		return ""
	}

	// CommonName is 64 bytes and we need a subdomain
	if len(domain) > 64-3 {
		panic("domain too long")
	}

	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%s", namespace, name)))
	subdomain := strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))[:64-1-len(domain)]

	return fmt.Sprintf("%s.%s", subdomain, domain)
}

func InitTest() {
	TestContext.KubeConfigPath = KubeConfigPath()
	framework.Logf("KubeConfigPath: %q", TestContext.KubeConfigPath)
	if TestContext.KubeConfigPath == "" {
		framework.Failf("You have to specify KubeConfigPath. (Use KUBECONFIG environment variable.)")
	}

	switch p := framework.DeleteTestingNSPolicyType(os.Getenv("DELETE_NS_POLICY")); p {
	case framework.DeleteTestingNSPolicyAlways,
		framework.DeleteTestingNSPolicyOnSuccess,
		framework.DeleteTestingNSPolicyNever:
		TestContext.DeleteTestingNSPolicy = framework.DeleteTestingNSPolicyType(p)
	case "":
		TestContext.DeleteTestingNSPolicy = framework.DeleteTestingNSPolicyAlways
	default:
		framework.Failf("Invalid DeleteTestingNSPolicy: %q", TestContext.DeleteTestingNSPolicy)
	}

	fixedNamespace := os.Getenv("E2E_FIXED_NAMESPACE")
	if len(fixedNamespace) != 0 {
		TestContext.CreateTestingNS = func(f *framework.Framework, name string, labels map[string]string) (*corev1.Namespace, error) {
			return &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fixedNamespace,
				},
			}, nil
		}
		TestContext.DeleteTestingNS = func(f *framework.Framework, ns *corev1.Namespace) error {
			var gracePeriod int64 = 0
			var propagation = metav1.DeletePropagationForeground
			err := f.RouteClientset().RouteV1().Routes(ns.Name).DeleteCollection(
				&metav1.DeleteOptions{
					GracePeriodSeconds: &gracePeriod,
					PropagationPolicy:  &propagation,
				}, metav1.ListOptions{
					LabelSelector: labels.Everything().String(),
				})
			if err != nil {
				return err
			}

			return nil
		}
	} else {
		TestContext.CreateTestingNS = framework.CreateTestingProjectAndChangeUser
	}

	framework.Logf("E2E_DOMAIN is %q", GetBaseDomain())
	framework.Logf("E2E_FIXED_NAMESPACE is %q", os.Getenv("E2E_FIXED_NAMESPACE"))
	framework.Logf("E2E_JUNITFILE is %q", os.Getenv("E2E_JUNITFILE"))
}

func ExecuteTest(t *testing.T, suite string) {
	gomega.RegisterFailHandler(ginkgo.Fail)

	junitFile := os.Getenv("E2E_JUNITFILE")
	if len(junitFile) > 0 {
		junitReporter := reporters.NewJUnitReporter(junitFile)
		ginkgo.RunSpecsWithDefaultAndCustomReporters(t, suite, []ginkgo.Reporter{junitReporter})
	} else {
		ginkgo.RunSpecs(t, suite)
	}

}
