package extended

import (
	"flag"
	"fmt"
	//"os/exec"
	"encoding/json"
	"os"
	"testing"
	"time"

	oapi "github.com/tnozicka/openshift-acme/pkg/openshift/api"
	"github.com/tnozicka/openshift-acme/pkg/openshift/untypedclient"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/unversioned"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/clientcmd"
)

var namespace string
var domain *string
var clientset *kubernetes.Clientset

//var oc func(...string) error

//func prefixNamespace(namespace string) string {
//	return "openshift-acme-test-prefix-" + namespace
//}

func TestMain(m *testing.M) {
	kubeconfigPath := flag.String("kubeconfig", "", "kubeconfig path")
	domain = flag.String("domain", "", "domain to use - must be routed to testing OpenShift instance")
	flag.Parse()
	restConfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfigPath)
	if err != nil {
		panic(fmt.Errorf("failed to create client config from kubeconfig path %q: %s", *kubeconfigPath, err))
	}
	clientset, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		panic(fmt.Errorf("failed to create clientset: %s", err))
	}

	kubeconfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: *kubeconfigPath},
		&clientcmd.ConfigOverrides{},
	).RawConfig()
	if err != nil {
		panic(fmt.Errorf("failed to create config: %s", err))
	}

	kctx, found := kubeconfig.Contexts[kubeconfig.CurrentContext]
	if !found {
		panic(fmt.Errorf("current context not found: %s", err))
	}

	namespace = kctx.Namespace

	if *domain == "" {
		panic(fmt.Errorf("You have to specify domain"))
	}

	//oc = func(args ...string) error {
	//	cmd := exec.Command("oc", append([]string{"--config", *kubeconfig}, args...)...)
	//	return cmd.Run()
	//}

	os.Exit(m.Run())
}

func TestBasic(t *testing.T) {
	//namespace := prefixNamespace("")
	//err := oc("new-project", namespace)
	//if err != nil {
	//	t.Fatal(err)
	//}

	name := "test"
	route := &oapi.Route{
		ObjectMeta: api_v1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"kubernetes.io/tls-acme": "true",
			},
		},
		Spec: oapi.RouteSpec{
			Host: *domain,
			To: oapi.RouteTargetReference{
				Name: "non-existing",
			},
		},
	}

	payload, err := json.Marshal(&route)
	if err != nil {
		t.Fatalf("marshaling Route failed: %s", err)
		return
	}

	routesUrl := fmt.Sprintf("/oapi/v1/namespaces/%s/routes", namespace)
	resp, err := untypedclient.Post(clientset.CoreV1().RESTClient(), routesUrl, payload)
	if err != nil {
		t.Fatalf("%s: %#v", err, string(resp))
	}

	watchUrl := fmt.Sprintf("/oapi/v1/watch/namespaces/%s/routes/%s", namespace, name)
	w, err := untypedclient.Watch(clientset.CoreV1().RESTClient(), watchUrl)
	if err != nil {
		t.Fatalf("watch failed: %s", err)
	}
	defer w.Stop()

	timeout := 60 * time.Second
loop:
	for {
		select {
		case <-time.After(timeout):
			t.Fatalf("Route wasn't updated with TLS before timeout (%s)", timeout)
		case rawEvent, ok := <-w.ResultChan():
			if !ok {
				t.Fatal("RouteController ResultChannel closed")
			}

			var event oapi.Event
			if err := json.Unmarshal(rawEvent, &event); err != nil {
				t.Fatal(err)
			}

			switch event.Type {
			case "ADDED", "MODIFIED":
				// this is the one we are waiting for
				break
			case "ERROR":
				var status unversioned.Status
				if err := json.Unmarshal(event.Object, &status); err != nil {
					t.Fatalf("RouteController: failed to unmarshal Status: '%s'", err)
				}
				t.Fatalf("RouteController: unknown 'ERROR' (%s)", event.Object)
			default:
				t.Fatalf("Unexpected Route event '%s'", event.Type)
			}

			var route oapi.Route
			if err := json.Unmarshal(event.Object, &route); err != nil {
				t.Fatal(err)
			}

			t.Logf("Route: %#v", route)

			if route.Spec.Tls != nil {
				t.Logf("%#v", *route.Spec.Tls)
				break loop
			}
		}
	}
	t.Logf("finished")
}
