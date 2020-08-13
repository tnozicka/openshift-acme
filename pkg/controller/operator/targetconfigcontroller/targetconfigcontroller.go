package targetconfigcontroller

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	openshiftoperatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"github.com/tnozicka/openshift-acme/pkg/api"
	operatorv1clientset "github.com/tnozicka/openshift-acme/pkg/client/operator/clientset/versioned/typed/operator/v1"
	operatorv1informers "github.com/tnozicka/openshift-acme/pkg/client/operator/informers/externalversions/operator/v1"
	operatorv1lister "github.com/tnozicka/openshift-acme/pkg/client/operator/listers/operator/v1"
	v100_00_assets "github.com/tnozicka/openshift-acme/pkg/controller/operator/v100_00_assets"
	kubeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/kube"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const (
	ControllerName = "TargetConfigController"
	objectName     = "cluster"
	workQueueKey   = "key"
	resyncPeriod   = 15 * time.Minute

	deploymentConditionTypePrefix = "Deployment"
)

// TODO: move to const when https://github.com/openshift/api/pull/707 merges
var (
	deploymentAvailableConditionType   = deploymentConditionTypePrefix + openshiftoperatorv1.OperatorStatusTypeAvailable
	deploymentProgressingConditionType = deploymentConditionTypePrefix + openshiftoperatorv1.OperatorStatusTypeProgressing
	deploymentDegradedConditionType    = deploymentConditionTypePrefix + openshiftoperatorv1.OperatorStatusTypeDegraded
	targetConfigControllerDegraded     = "TargetConfigController" + openshiftoperatorv1.OperatorStatusTypeDegraded
)

type TargetConfigController struct {
	targetNamespace string

	kubeClient                 kubernetes.Interface
	kubeInformersForNamespaces kubeinformers.Interface

	operatorClient       operatorv1clientset.OperatorV1Interface
	acmeControllerLister operatorv1lister.ACMEControllerLister

	recorder record.EventRecorder

	queue workqueue.RateLimitingInterface

	cachesToSync []cache.InformerSynced
}

func NewTargetConfigController(
	targetNamespace string,
	operandImage string,
	kubeClient kubernetes.Interface,
	operatorClient operatorv1clientset.OperatorV1Interface,
	acmeControllerInformer operatorv1informers.ACMEControllerInformer,
	kubeInformersForNamespaces kubeinformers.Interface,
) *TargetConfigController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})

	c := &TargetConfigController{
		targetNamespace: targetNamespace,

		kubeClient:                 kubeClient,
		kubeInformersForNamespaces: kubeInformersForNamespaces,

		operatorClient:       operatorClient,
		acmeControllerLister: acmeControllerInformer.Lister(),

		recorder: eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: ControllerName}),

		queue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),

		cachesToSync: []cache.InformerSynced{
			acmeControllerInformer.Informer().HasSynced,
		},
	}

	kubeInformersForNamespaces.InformersFor(c.targetNamespace).Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())
	kubeInformersForNamespaces.InformersFor(c.targetNamespace).Apps().V1().Deployments().Informer().AddEventHandler(c.eventHandler())
	c.cachesToSync = append(
		c.cachesToSync,
		kubeInformersForNamespaces.InformersFor(c.targetNamespace).Core().V1().ConfigMaps().Informer().HasSynced,
		kubeInformersForNamespaces.InformersFor(c.targetNamespace).Apps().V1().Deployments().Informer().HasSynced,
	)

	return c
}

func (c *TargetConfigController) eventHandler() cache.ResourceEventHandler {
	resourceDesc := func(obj interface{}) string {
		m, ok := obj.(metav1.Object)
		if !ok {
			return fmt.Sprintf("%T", obj)
		}
		return fmt.Sprintf("%s/%s(%s)", m.GetNamespace(), m.GetName(), m.GetUID())
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			klog.V(5).Infof("Adding %s", resourceDesc(obj))
			c.queue.Add(workQueueKey)
		},
		UpdateFunc: func(old, new interface{}) {
			klog.V(5).Infof("Updating from %s to %s", resourceDesc(old), resourceDesc(new))
			c.queue.Add(workQueueKey)
		},
		DeleteFunc: func(obj interface{}) {
			klog.V(5).Infof("Deleting %s", resourceDesc(obj))
			c.queue.Add(workQueueKey)
		},
	}
}

func (c *TargetConfigController) manageDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	deployment := DecodeAssetOrDie("v1.0.0/deployment.yaml").(*appsv1.Deployment)
	if deployment.Annotations == nil {
		deployment.Annotations = map[string]string{}
	}

	// TODO: substitute images, args, ...

	deployment.Annotations[api.ManagedDataHash] = HashObjectsOrDie(deployment)

	deploymentLister := c.kubeInformersForNamespaces.InformersFor(c.targetNamespace).Apps().V1().Deployments().Lister()
	existingDeployment, err := deploymentLister.Deployments(c.targetNamespace).Get(deployment.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		klog.V(2).Infof("Creating deployment %s/%s because it is missing.", c.targetNamespace, deployment.Name)
		// TODO: add expectations to avoid stale caches (AlreadyExistsError)
		deployment, err := c.kubeClient.AppsV1().Deployments(c.targetNamespace).Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}

		return deployment, nil
	}

	existingHash, ok := existingDeployment.Annotations[api.ManagedDataHash]
	if ok && existingHash == deployment.Annotations[api.ManagedDataHash] {
		return existingDeployment, nil
	}

	klog.V(2).Infof("Updating deployment %s/%s because it changed.", c.targetNamespace, deployment.Name)
	deployment.ResourceVersion = existingDeployment.ResourceVersion
	deployment, err = c.kubeClient.AppsV1().Deployments(c.targetNamespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	return deployment, nil
}

func (c *TargetConfigController) sync(ctx context.Context) error {
	klog.V(4).Infof("Started syncing %s", ControllerName)
	defer func() {
		klog.V(4).Infof("Finished syncing %s", ControllerName)
	}()

	operator, err := c.acmeControllerLister.Get(objectName)
	if err != nil {
		return fmt.Errorf("can't get operator config: %w", err)
	}

	switch operator.Spec.ManagementState {
	case openshiftoperatorv1.Managed:
	case openshiftoperatorv1.Unmanaged, openshiftoperatorv1.Removed:
		return nil
	default:
		c.recorder.Eventf(operator, "ManagementStateUnknown", "Unrecognized operator management state %q", string(operator.Spec.ManagementState))
		return nil
	}

	var reconciliationErrors []error

	status := operator.Status.DeepCopy()
	status.ObservedGeneration = operator.Generation

	if v1helpers.FindOperatorCondition(operator.Status.Conditions, deploymentAvailableConditionType) == nil {
		v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
			Type:   deploymentAvailableConditionType,
			Status: openshiftoperatorv1.ConditionUnknown,
		})
	}
	if v1helpers.FindOperatorCondition(operator.Status.Conditions, deploymentProgressingConditionType) == nil {
		v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
			Type:   deploymentProgressingConditionType,
			Status: openshiftoperatorv1.ConditionUnknown,
		})
	}

	if v1helpers.FindOperatorCondition(operator.Status.Conditions, deploymentDegradedConditionType) == nil {
		v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
			Type:   deploymentDegradedConditionType,
			Status: openshiftoperatorv1.ConditionUnknown,
		})
	}

	deployment, err := c.manageDeployment(ctx)
	if err != nil {
		reconciliationErrors = append(reconciliationErrors, fmt.Errorf("managing deployment: %w", err))
	} else {
		// Act only if the status is up-to-date
		if deployment.Status.ObservedGeneration == deployment.Generation {
			// FIXME: We are not able to determine how many available replicas are the new ones.
			//  (Needs structured status in kube.)
			status.AvailableReplicas = 0

			if deployment.Status.AvailableReplicas > 0 {
				v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
					Type:    deploymentAvailableConditionType,
					Status:  openshiftoperatorv1.ConditionTrue,
					Reason:  "AvailableReplica",
					Message: "At least one replica is available.",
				})
			} else {
				v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
					Type:    deploymentAvailableConditionType,
					Status:  openshiftoperatorv1.ConditionFalse,
					Reason:  "NoReplicasAvailable",
					Message: "No replicas are available.",
				})
			}

			if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
				v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
					Type:    deploymentProgressingConditionType,
					Status:  openshiftoperatorv1.ConditionTrue,
					Reason:  "DeploymentProgressing",
					Message: "Deployment is progressing.",
				})
			} else {
				v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
					Type:    deploymentProgressingConditionType,
					Status:  openshiftoperatorv1.ConditionFalse,
					Reason:  "DeploymentRolledOut",
					Message: "Deployment has finished the rollout.",
				})
			}

			if deployment.Status.Replicas == *deployment.Spec.Replicas &&
				deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas &&
				deployment.Status.AvailableReplicas == *deployment.Spec.Replicas {
				v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
					Type:    deploymentDegradedConditionType,
					Status:  openshiftoperatorv1.ConditionTrue,
					Reason:  "UnavailableReplicas.",
					Message: "Deployment has unavailable replicas.",
				})
			} else {
				v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
					Type:    deploymentDegradedConditionType,
					Status:  openshiftoperatorv1.ConditionFalse,
					Reason:  "AllReplicasUp",
					Message: "Deployment has all replicas up to date and available.",
				})

			}
		}
	}

	reconciliationError := errorutils.NewAggregate(reconciliationErrors)
	if reconciliationError != nil {
		klog.V(2).Info(reconciliationError)
		v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
			Type:    targetConfigControllerDegraded,
			Status:  openshiftoperatorv1.ConditionTrue,
			Reason:  "SynchronizationError",
			Message: reconciliationError.Error(),
		})
	} else {
		v1helpers.SetOperatorCondition(&operator.Status.Conditions, openshiftoperatorv1.OperatorCondition{
			Type:    targetConfigControllerDegraded,
			Status:  openshiftoperatorv1.ConditionFalse,
			Reason:  "AsExpected",
			Message: "AsExpected",
		})
	}

	// FIXME: DeepEqual + condition time
	timelessStatus := status.DeepCopy()
	for i := range timelessStatus.Conditions {
		cond := &timelessStatus.Conditions[i]
		oldCond := v1helpers.FindOperatorCondition(operator.Status.Conditions, cond.Type)
		if oldCond != nil {
			cond.LastTransitionTime = oldCond.LastTransitionTime
		}
	}
	if apiequality.Semantic.DeepEqual(timelessStatus, operator.Status) {
		return nil
	}

	_, err = c.operatorClient.ACMEControllers().UpdateStatus(ctx, operator, metav1.UpdateOptions{})
	if apierrors.IsConflict(err) {
		klog.V(2).Info(reconciliationError)
		c.queue.Add(workQueueKey)
		return nil
	} else if err != nil {
		return fmt.Errorf("can't update status: %w", err)
	}

	return nil
}

func (c *TargetConfigController) processNextItem(ctx context.Context) bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.sync(ctx)
	if err == nil {
		c.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("syncing key '%v' failed: %v", key, err))
	c.queue.AddRateLimited(key)

	return true
}

func (c *TargetConfigController) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

func (c *TargetConfigController) Run(ctx context.Context) {
	defer utilruntime.HandleCrash()

	klog.Infof("Starting %s", ControllerName)
	var wg sync.WaitGroup
	defer func() {
		klog.Info("Shutting down %s", ControllerName)
		c.queue.ShutDown()
		wg.Wait()
		klog.Info("%s shut down", ControllerName)
	}()

	ok := cache.WaitForNamedCacheSync(ControllerName, ctx.Done(), c.cachesToSync...)
	if !ok {
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		wait.UntilWithContext(ctx, func(ctx context.Context) {
			c.queue.Add(workQueueKey)
		}, resyncPeriod)
	}()

	<-ctx.Done()
}

func DecodeAssetOrDie(path string) runtime.Object {
	obj, err := runtime.Decode(
		scheme.Codecs.UniversalDeserializer(),
		v100_00_assets.MustAsset(path),
	)
	if err != nil {
		panic(err)
	}

	return obj
}

func HashObjectsOrDie(objects ...runtime.Object) string {
	hasher := sha512.New()

	for _, obj := range objects {
		data, err := json.Marshal(obj)
		if err != nil {
			panic(err)
		}

		_, err = hasher.Write(data)
		if err != nil {
			panic(err)
		}
	}

	return string(hasher.Sum(nil))
}
