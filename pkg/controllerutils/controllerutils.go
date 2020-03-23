package controllerutils

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"

	"github.com/ghodss/yaml"
	"github.com/tnozicka/openshift-acme/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	_ "github.com/openshift/client-go/route/clientset/versioned/scheme"
	"github.com/tnozicka/openshift-acme/pkg/api"
	kubeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/kube"
)

func getIssuerConfigMapsForObject(obj metav1.ObjectMeta, globalIssuerNamesapce string, kubeInformersForNamespaces kubeinformers.Interface) ([]*corev1.ConfigMap, error) {
	// Lookup explicitly referenced issuer first. If explicitly referenced this should be the only match.
	issuerName, found := obj.Annotations[api.AcmeCertIssuerName]
	if found && len(issuerName) > 0 {
		issuerConfigMap, err := kubeInformersForNamespaces.InformersForOrGlobal(obj.Namespace).Core().V1().ConfigMaps().Lister().ConfigMaps(obj.Namespace).Get(issuerName)
		if err != nil {
			return nil, fmt.Errorf("can't get issuer %s/%s: %w", obj.Namespace, issuerName, err)
		}
		return []*corev1.ConfigMap{issuerConfigMap}, nil
	}

	var issuerConfigMaps []*corev1.ConfigMap

	localConfigMapList, err := kubeInformersForNamespaces.InformersForOrGlobal(obj.Namespace).Core().V1().ConfigMaps().Lister().ConfigMaps(obj.Namespace).List(api.AccountLabelSet.AsSelector())
	if err != nil {
		return nil, fmt.Errorf("can't look up local issuers: %w", err)
	}
	issuerConfigMaps = append(issuerConfigMaps, localConfigMapList...)

	globalConfigMapList, err := kubeInformersForNamespaces.InformersForOrGlobal(globalIssuerNamesapce).Core().V1().ConfigMaps().Lister().ConfigMaps(globalIssuerNamesapce).List(api.AccountLabelSet.AsSelector())
	if err != nil {
		return nil, fmt.Errorf("can't look up global issuers: %w", err)
	}
	issuerConfigMaps = append(issuerConfigMaps, globalConfigMapList...)

	if len(issuerConfigMaps) < 1 {
		return nil, fmt.Errorf("can't find any issuer")
	}

	sort.Slice(issuerConfigMaps, func(i, j int) bool {
		lhs := issuerConfigMaps[i]
		rhs := issuerConfigMaps[i]

		lhsPrio := 0
		lhsPrioString, ok := lhs.Annotations[api.AcmePriorityAnnotation]
		if ok && len(lhsPrioString) != 0 {
			v, err := strconv.Atoi(lhsPrioString)
			if err == nil {
				lhsPrio = v
			} else {
				klog.Warning(err)
			}
		}

		rhsPrio := 0
		rhsPrioString, ok := rhs.Annotations[api.AcmePriorityAnnotation]
		if ok && len(rhsPrioString) != 0 {
			v, err := strconv.Atoi(rhsPrioString)
			if err == nil {
				rhsPrio = v
			} else {
				klog.Warning(err)
			}
		}

		if lhsPrio < rhsPrio {
			return true
		}

		if lhs.CreationTimestamp.Time.After(rhs.CreationTimestamp.Time) {
			return true
		}

		return false
	})

	return issuerConfigMaps, nil
}

func IssuerForObject(obj metav1.ObjectMeta, globalIssuerNamespace string, kubeInformersForNamespaces kubeinformers.Interface) (*api.CertIssuer, *corev1.Secret, error) {
	issuerConfigMaps, err := getIssuerConfigMapsForObject(obj, globalIssuerNamespace, kubeInformersForNamespaces)
	if err != nil {
		return nil, nil, err
	}

	// TODO: Filter out non-matching issuers and solvers
	certIssuerCM := issuerConfigMaps[0]

	certIssuerData, ok := certIssuerCM.Data[api.CertIssuerDataKey]
	if !ok {
		return nil, nil, fmt.Errorf("configmap %s/%s is matching CertIssuer selectors %q but missing key %q", obj.Namespace, obj.Name, api.AccountLabelSet, api.CertIssuerDataKey)
	}

	certIssuer := &api.CertIssuer{}
	err = yaml.Unmarshal([]byte(certIssuerData), certIssuer)
	if err != nil {
		return nil, nil, fmt.Errorf("configmap %s/%s is matching CertIssuer selectors %q but contains invalid object: %w", obj.Namespace, obj.Name, api.AccountLabelSet, err)
	}

	if len(certIssuer.SecretName) == 0 {
		return certIssuer, nil, fmt.Errorf("cert issuer %s/%s is missing required secret", certIssuerCM.Namespace, certIssuerCM.Name)
	}

	secret, err := kubeInformersForNamespaces.InformersForOrGlobal(certIssuerCM.Namespace).Core().V1().Secrets().Lister().Secrets(certIssuerCM.Namespace).Get(certIssuer.SecretName)
	if err != nil {
		return nil, nil, err
	}

	return certIssuer, secret, nil
}

func ValidateExposedToken(url, expectedData string) error {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{Transport: tr}

	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("can't GET %q: %w", url, err)
	}
	defer response.Body.Close()

	// No response should be longer that this, we need to prevent against DoS
	buffer := make([]byte, 2048)
	n, err := response.Body.Read(buffer)
	if err != nil && err != io.EOF {
		return fmt.Errorf("can't read response body into buffer: %w", err)
	}
	body := string(buffer[:n])

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("getting %q return status code %d, expected %d: status %q: content head: %s", url, response.StatusCode, http.StatusOK, response.Status, util.FirstNLines(util.MaxNCharacters(body, 160), 5))
	}

	if body != expectedData {
		return fmt.Errorf("response body doesn't match expected data")
	}

	return nil
}
