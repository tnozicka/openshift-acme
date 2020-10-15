package resourceapply

import (
	"context"

	"github.com/tnozicka/openshift-acme/pkg/api"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policyv1beta1client "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
	policyv1beta1listers "k8s.io/client-go/listers/policy/v1beta1"
	"k8s.io/client-go/tools/record"
)

func ApplyPodDisruptionBudget(ctx context.Context, client policyv1beta1client.PodDisruptionBudgetsGetter, lister policyv1beta1listers.PodDisruptionBudgetLister, recorder record.EventRecorder, pdb *policyv1beta1.PodDisruptionBudget) (*policyv1beta1.PodDisruptionBudget, bool, error) {
	required := pdb.DeepCopy()
	SetHashOrDie(required)

	actual, err := lister.PodDisruptionBudgets(required.Namespace).Get(required.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, err
		}

		actual, err := client.PodDisruptionBudgets(required.Namespace).Create(ctx, required, metav1.CreateOptions{})
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
	actual, err = client.PodDisruptionBudgets(required.Namespace).Update(ctx, required, metav1.UpdateOptions{})
	if err != nil {
		reportUpdateEvent(recorder, required, err)
		return nil, false, err

	}
	reportUpdateEvent(recorder, actual, nil)
	return actual, true, nil
}
