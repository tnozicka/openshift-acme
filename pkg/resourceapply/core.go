package resourceapply

import (
	"context"

	"github.com/tnozicka/openshift-acme/pkg/api"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/record"
)

func ApplyNamespace(ctx context.Context, client corev1client.NamespacesGetter, lister corev1listers.NamespaceLister, recorder record.EventRecorder, namespace *corev1.Namespace) (*corev1.Namespace, bool, error) {
	required := namespace.DeepCopy()
	SetHashOrDie(required)

	actual, err := lister.Get(required.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, err
		}

		actual, err := client.Namespaces().Create(ctx, required, metav1.CreateOptions{})
		if err != nil {
			reportCreateEvent(recorder, required, err)
			return nil, false, err

		}
		reportCreateEvent(recorder, actual, nil)
		return actual, true, nil
	}

	if actual.Annotations != nil && actual.Annotations[api.ManagedDataHash] == required.Annotations[api.ManagedDataHash] {
		return actual, false, nil
	}

	required.ResourceVersion = actual.ResourceVersion
	actual, err = client.Namespaces().Update(ctx, required, metav1.UpdateOptions{})
	if err != nil {
		reportUpdateEvent(recorder, required, err)
		return nil, false, err

	}
	reportUpdateEvent(recorder, actual, nil)
	return actual, true, nil
}

func ApplyConfigMap(ctx context.Context, client corev1client.ConfigMapsGetter, lister corev1listers.ConfigMapLister, recorder record.EventRecorder, configMap *corev1.ConfigMap) (*corev1.ConfigMap, bool, error) {
	required := configMap.DeepCopy()
	SetHashOrDie(required)

	actual, err := lister.ConfigMaps(required.Namespace).Get(required.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, err
		}

		actual, err := client.ConfigMaps(required.Namespace).Create(ctx, required, metav1.CreateOptions{})
		if err != nil {
			reportCreateEvent(recorder, required, err)
			return nil, false, err

		}
		reportCreateEvent(recorder, actual, nil)
		return actual, true, nil
	}

	if actual.Annotations != nil && actual.Annotations[api.ManagedDataHash] == required.Annotations[api.ManagedDataHash] {
		return actual, false, nil
	}

	required.ResourceVersion = actual.ResourceVersion
	actual, err = client.ConfigMaps(required.Namespace).Update(ctx, required, metav1.UpdateOptions{})
	if err != nil {
		reportUpdateEvent(recorder, required, err)
		return nil, false, err

	}
	reportUpdateEvent(recorder, actual, nil)
	return actual, true, nil
}

func ApplyServiceAccount(ctx context.Context, client corev1client.ServiceAccountsGetter, lister corev1listers.ServiceAccountLister, recorder record.EventRecorder, serviceAccount *corev1.ServiceAccount) (*corev1.ServiceAccount, bool, error) {
	required := serviceAccount.DeepCopy()
	SetHashOrDie(required)

	actual, err := lister.ServiceAccounts(required.Namespace).Get(required.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, err
		}

		actual, err := client.ServiceAccounts(required.Namespace).Create(ctx, required, metav1.CreateOptions{})
		if err != nil {
			reportCreateEvent(recorder, required, err)
			return nil, false, err

		}
		reportCreateEvent(recorder, actual, nil)
		return actual, true, nil
	}

	if actual.Annotations != nil && actual.Annotations[api.ManagedDataHash] == required.Annotations[api.ManagedDataHash] {
		return actual, false, nil
	}

	required.ResourceVersion = actual.ResourceVersion
	actual, err = client.ServiceAccounts(required.Namespace).Update(ctx, required, metav1.UpdateOptions{})
	if err != nil {
		reportUpdateEvent(recorder, required, err)
		return nil, false, err

	}
	reportUpdateEvent(recorder, actual, nil)
	return actual, true, nil
}
