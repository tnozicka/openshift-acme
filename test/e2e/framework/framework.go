package framework

import (
	"fmt"
	"io/ioutil"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	"k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kclientcmd "k8s.io/client-go/tools/clientcmd"
	kclientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	oauthv1 "github.com/openshift/api/oauth/v1"
	userv1 "github.com/openshift/api/user/v1"
	oauthclientset "github.com/openshift/client-go/oauth/clientset/versioned"
	projectclientset "github.com/openshift/client-go/project/clientset/versioned"
	routeclientset "github.com/openshift/client-go/route/clientset/versioned"
	userclientset "github.com/openshift/client-go/user/clientset/versioned"
)

type Framework struct {
	name               string
	namespace          *v1.Namespace
	namespacesToDelete []*v1.Namespace

	clientConfig      *rest.Config
	adminClientConfig *rest.Config
	username          string
}

func NewFramework(project string) *Framework {
	uniqueProject := names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%s-", project))
	f := &Framework{
		name:     uniqueProject,
		username: "admin",
	}

	g.BeforeEach(f.BeforeEach)
	g.AfterEach(f.AfterEach)

	return f
}

func (f *Framework) Namespace() string {
	return f.namespace.Name
}

func (f *Framework) AddNamespace(namespace *v1.Namespace) {
	f.namespace = namespace
	f.namespacesToDelete = append(f.namespacesToDelete, namespace)
}

func (f *Framework) Username() string {
	return f.username
}

func (f *Framework) AdminClientConfig() *rest.Config {
	if f.adminClientConfig == nil {
		var err error
		f.adminClientConfig, err = kclientcmd.NewNonInteractiveDeferredLoadingClientConfig(&kclientcmd.ClientConfigLoadingRules{ExplicitPath: TestContext.KubeConfigPath}, &kclientcmd.ConfigOverrides{}).ClientConfig()
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	return f.adminClientConfig
}

func (f *Framework) ClientConfig() *rest.Config {
	if f.clientConfig == nil {
		f.clientConfig = f.AdminClientConfig()
	}

	return f.clientConfig
}

func (f *Framework) KubeClientSet() *kubernetes.Clientset {
	clientSet, err := kubernetes.NewForConfig(f.ClientConfig())
	o.Expect(err).NotTo(o.HaveOccurred())
	return clientSet
}

func (f *Framework) KubeAdminClientSet() *kubernetes.Clientset {
	clientSet, err := kubernetes.NewForConfig(f.AdminClientConfig())
	o.Expect(err).NotTo(o.HaveOccurred())
	return clientSet
}

func (f *Framework) CreateNamespace(name string, labels map[string]string) (*v1.Namespace, error) {
	createTestingNS := TestContext.CreateTestingNS
	if createTestingNS == nil {
		createTestingNS = CreateTestingNamespace
	}

	if labels == nil {
		labels = map[string]string{}
	}
	labels["e2e"] = "openshift-acme"

	ns, err := createTestingNS(f, name, labels)
	if err != nil {
		return nil, err
	}

	f.AddNamespace(ns)

	return ns, nil
}

func (f *Framework) ChangeUser(username string, namespace string) {
	if username == "admin" {
		f.clientConfig = f.AdminClientConfig()
		return
	}

	// We need to reset the user
	f.clientConfig = nil

	user, err := f.UserClientset().UserV1().Users().Create(&userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: username,
		},
	})
	o.Expect(err).NotTo(o.HaveOccurred())

	token, err := f.OAuthClientset().OauthV1().OAuthAccessTokens().Create(&oauthv1.OAuthAccessToken{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-padding_padding_padding_padding_padding_padding_padding", username)},
		UserName:   user.Name,
		UserUID:    string(user.UID),
		ClientName: "openshift-challenging-client",
		ExpiresIn:  90000,
	})
	o.Expect(err).NotTo(o.HaveOccurred())

	f.clientConfig = rest.AnonymousClientConfig(f.AdminClientConfig())
	f.clientConfig.BearerToken = token.Name

	// Create Kubeconfig
	kubeConfig := kclientcmdapi.NewConfig()

	credentials := kclientcmdapi.NewAuthInfo()
	credentials.Token = f.clientConfig.BearerToken
	credentials.ClientCertificate = f.clientConfig.TLSClientConfig.CertFile
	if len(credentials.ClientCertificate) == 0 {
		credentials.ClientCertificateData = f.clientConfig.TLSClientConfig.CertData
	}
	credentials.ClientKey = f.clientConfig.TLSClientConfig.KeyFile
	if len(credentials.ClientKey) == 0 {
		credentials.ClientKeyData = f.clientConfig.TLSClientConfig.KeyData
	}
	kubeConfig.AuthInfos[user.Name] = credentials

	cluster := kclientcmdapi.NewCluster()
	cluster.Server = f.clientConfig.Host
	cluster.CertificateAuthority = f.clientConfig.CAFile
	if len(cluster.CertificateAuthority) == 0 {
		cluster.CertificateAuthorityData = f.clientConfig.CAData
	}
	cluster.InsecureSkipTLSVerify = f.clientConfig.Insecure
	kubeConfig.Clusters["test"] = cluster

	context := kclientcmdapi.NewContext()
	context.Cluster = "test"
	context.AuthInfo = user.Name
	context.Namespace = namespace
	kubeConfig.Contexts["test"] = context
	kubeConfig.CurrentContext = "test"

	tmpFile, err := ioutil.TempFile("", fmt.Sprintf("%s-kubeconfig-", username))
	o.Expect(err).NotTo(o.HaveOccurred())

	err = kclientcmd.WriteToFile(*kubeConfig, tmpFile.Name())
	o.Expect(err).NotTo(o.HaveOccurred())

	f.username = username
	Logf("ConfigPath is now %q", tmpFile.Name())
}

func (f *Framework) DeleteNamespace(ns *v1.Namespace) error {
	deleteTestingNS := TestContext.DeleteTestingNS
	if deleteTestingNS == nil {
		deleteTestingNS = DeleteNamespace
	}

	err := deleteTestingNS(f, ns)
	if err != nil {
		return err
	}

	return nil
}

func (f *Framework) BeforeEach() {
	g.By("Building a namespace api object")
	_, err := f.CreateNamespace(f.name, nil)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (f *Framework) AfterEach() {
	defer func() {
		nsDeletionErrors := map[string]error{}

		if TestContext.DeleteTestingNSPolicy == DeleteTestingNSPolicyNever ||
			(TestContext.DeleteTestingNSPolicy == DeleteTestingNSPolicyOnSuccess && g.CurrentGinkgoTestDescription().Failed) {
			return
		}

		for _, ns := range f.namespacesToDelete {
			err := f.DeleteNamespace(ns)
			if err != nil {
				nsDeletionErrors[ns.Name] = err
				continue
			}
		}

		// Prevent reuse
		f.namespace = nil
		f.namespacesToDelete = nil

		if len(nsDeletionErrors) > 0 {
			messages := []string{}
			for namespaceKey, namespaceErr := range nsDeletionErrors {
				messages = append(messages, fmt.Sprintf("Couldn't delete ns: %q: %s (%#v)", namespaceKey, namespaceErr, namespaceErr))
			}
			Failf(strings.Join(messages, ","))
		}
	}()

	// Print events if the test failed.
	if g.CurrentGinkgoTestDescription().Failed {
		for _, ns := range f.namespacesToDelete {
			g.By(fmt.Sprintf("Collecting events from namespace %q.", ns.Name))
			DumpEventsInNamespace(f.KubeClientSet(), ns.Name)
		}
	}
}

func (f *Framework) OAuthClientset() oauthclientset.Interface {
	clientset, err := oauthclientset.NewForConfig(f.ClientConfig())
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create oauth clientset")

	return clientset
}

func (f *Framework) UserClientset() userclientset.Interface {
	clientset, err := userclientset.NewForConfig(f.ClientConfig())
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create oauth clientset")

	return clientset
}

func (f *Framework) RouteClientset() routeclientset.Interface {
	clientset, err := routeclientset.NewForConfig(f.ClientConfig())
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create route clientset")

	return clientset
}

func (f *Framework) ProjectClientset() projectclientset.Interface {
	clientset, err := projectclientset.NewForConfig(f.ClientConfig())
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create project clientset")

	return clientset
}
