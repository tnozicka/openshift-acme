package resourceapply

import (
	"context"

	"github.com/tnozicka/openshift-acme/pkg/api"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	appsv1listers "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/record"
)

func ApplyDeployment(ctx context.Context, client appsv1client.DeploymentsGetter, lister appsv1listers.DeploymentLister, recorder record.EventRecorder, deployment *appsv1.Deployment) (*appsv1.Deployment, bool, error) {
	required := deployment.DeepCopy()
	SetHashOrDie(required)

	actual, err := lister.Deployments(required.Namespace).Get(required.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, err
		}

		actual, err := client.Deployments(required.Namespace).Create(ctx, required, metav1.CreateOptions{})
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
	actual, err = client.Deployments(required.Namespace).Update(ctx, required, metav1.UpdateOptions{})
	if err != nil {
		reportUpdateEvent(recorder, required, err)
		return nil, false, err

	}
	reportUpdateEvent(recorder, actual, nil)
	return actual, true, nil
}
