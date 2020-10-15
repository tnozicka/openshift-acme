package resourceapply

import (
	"context"

	"github.com/tnozicka/openshift-acme/pkg/api"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/record"
)

func ApplyClusterRole(ctx context.Context, client rbacv1client.ClusterRolesGetter, lister rbacv1listers.ClusterRoleLister, recorder record.EventRecorder, clusterRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, bool, error) {
	required := clusterRole.DeepCopy()
	SetHashOrDie(required)

	actual, err := lister.Get(required.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, err
		}

		actual, err := client.ClusterRoles().Create(ctx, required, metav1.CreateOptions{})
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
	actual, err = client.ClusterRoles().Update(ctx, required, metav1.UpdateOptions{})
	if err != nil {
		reportUpdateEvent(recorder, required, err)
		return nil, false, err

	}
	reportUpdateEvent(recorder, actual, nil)
	return actual, true, nil
}

func ApplyClusterRoleBinding(ctx context.Context, client rbacv1client.ClusterRoleBindingsGetter, lister rbacv1listers.ClusterRoleBindingLister, recorder record.EventRecorder, clusterRoleBinding *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, bool, error) {
	required := clusterRoleBinding.DeepCopy()
	SetHashOrDie(required)

	actual, err := lister.Get(required.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, err
		}

		actual, err := client.ClusterRoleBindings().Create(ctx, required, metav1.CreateOptions{})
		reportCreateEvent(recorder, required, err)
		return actual, err == nil, err
	}

	if actual.Annotations != nil && actual.Annotations[api.ManagedDataHash] == required.Annotations[api.ManagedDataHash] {
		return actual, false, nil
	}

	required.ResourceVersion = actual.ResourceVersion
	actual, err = client.ClusterRoleBindings().Update(ctx, required, metav1.UpdateOptions{})
	if err != nil {
		reportUpdateEvent(recorder, required, err)
		return nil, false, err

	}
	reportUpdateEvent(recorder, actual, nil)
	return actual, true, nil
}
