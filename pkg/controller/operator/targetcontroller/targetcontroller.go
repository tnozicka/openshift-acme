package targetcontroller

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	openshiftoperatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"github.com/tnozicka/openshift-acme/pkg/assetutil"
	acmev1clientset "github.com/tnozicka/openshift-acme/pkg/client/acme/clientset/versioned/typed/acme/v1"
	acmev1informers "github.com/tnozicka/openshift-acme/pkg/client/acme/informers/externalversions/acme/v1"
	acmev1lister "github.com/tnozicka/openshift-acme/pkg/client/acme/listers/acme/v1"
	"github.com/tnozicka/openshift-acme/pkg/controller/operator/assets"
	"github.com/tnozicka/openshift-acme/pkg/controller/operator/assets/target_v100"
	kubeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/kube"
	"github.com/tnozicka/openshift-acme/pkg/resourceapply"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
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
	ControllerName = "TargetController"
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
	targetConfigControllerDegraded     = "TargetController" + openshiftoperatorv1.OperatorStatusTypeDegraded
)

type TargetController struct {
	targetNamespace        string
	operandControllerImage string
	operandExposerImage    string
	stagingIssuersOnly     bool

	kubeClient                 kubernetes.Interface
	kubeInformersForNamespaces kubeinformers.Interface

	operatorClient       acmev1clientset.AcmeV1Interface
	acmeControllerLister acmev1lister.ACMEControllerLister

	recorder record.EventRecorder

	queue workqueue.RateLimitingInterface

	cachesToSync []cache.InformerSynced
}

func NewTargetController(
	targetNamespace string,
	operandControllerImage string,
	operandExposerImage string,
	stagingIssuersOnly bool,
	kubeClient kubernetes.Interface,
	operatorClient acmev1clientset.AcmeV1Interface,
	acmeControllerInformer acmev1informers.ACMEControllerInformer,
	kubeInformersForNamespaces kubeinformers.Interface,
) *TargetController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})

	c := &TargetController{
		targetNamespace:        targetNamespace,
		operandControllerImage: operandControllerImage,
		operandExposerImage:    operandExposerImage,
		stagingIssuersOnly:     stagingIssuersOnly,

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

	kubeInformersForNamespaces.InformersFor(corev1.NamespaceAll).Core().V1().Namespaces().Informer().AddEventHandler(c.eventHandler())
	kubeInformersForNamespaces.InformersFor(c.targetNamespace).Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())
	kubeInformersForNamespaces.InformersFor(c.targetNamespace).Core().V1().ServiceAccounts().Informer().AddEventHandler(c.eventHandler())
	kubeInformersForNamespaces.InformersFor(c.targetNamespace).Apps().V1().Deployments().Informer().AddEventHandler(c.eventHandler())
	kubeInformersForNamespaces.InformersFor(c.targetNamespace).Rbac().V1().ClusterRoles().Informer().AddEventHandler(c.eventHandler())
	kubeInformersForNamespaces.InformersFor(c.targetNamespace).Rbac().V1().ClusterRoleBindings().Informer().AddEventHandler(c.eventHandler())
	kubeInformersForNamespaces.InformersFor(c.targetNamespace).Policy().V1beta1().PodDisruptionBudgets().Informer().AddEventHandler(c.eventHandler())

	c.cachesToSync = append(
		c.cachesToSync,
		kubeInformersForNamespaces.InformersFor(corev1.NamespaceAll).Core().V1().Namespaces().Informer().HasSynced,
		kubeInformersForNamespaces.InformersFor(c.targetNamespace).Core().V1().ConfigMaps().Informer().HasSynced,
		kubeInformersForNamespaces.InformersFor(c.targetNamespace).Core().V1().ServiceAccounts().Informer().HasSynced,
		kubeInformersForNamespaces.InformersFor(c.targetNamespace).Apps().V1().Deployments().Informer().HasSynced,
		kubeInformersForNamespaces.InformersFor(c.targetNamespace).Rbac().V1().ClusterRoles().Informer().HasSynced,
		kubeInformersForNamespaces.InformersFor(c.targetNamespace).Rbac().V1().ClusterRoleBindings().Informer().HasSynced,
		kubeInformersForNamespaces.InformersFor(c.targetNamespace).Policy().V1beta1().PodDisruptionBudgets().Informer().HasSynced,
	)

	return c
}

func (c *TargetController) eventHandler() cache.ResourceEventHandler {
	resourceDesc := func(obj interface{}) string {
		m, ok := obj.(metav1.Object)
		if !ok {
			return fmt.Sprintf("%T", obj)
		}
		return fmt.Sprintf("%s/%s(%s)", m.GetNamespace(), m.GetName(), m.GetUID())
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			klog.V(5).Infof("Adding %T %s", obj, resourceDesc(obj))
			c.queue.Add(workQueueKey)
		},
		UpdateFunc: func(old, new interface{}) {
			klog.V(5).Infof("Updating %T from %s to %s", old, resourceDesc(old), resourceDesc(new))
			c.queue.Add(workQueueKey)
		},
		DeleteFunc: func(obj interface{}) {
			klog.V(5).Infof("Deleting %T %s", obj, resourceDesc(obj))
			c.queue.Add(workQueueKey)
		},
	}
}

func (c *TargetController) templateContext() *assets.Data {
	d := &assets.Data{
		ClusterWide:     false,
		TargetNamespace: c.targetNamespace,
		ControllerImage: c.operandControllerImage,
		ExposerImage:    c.operandExposerImage,
	}

	for _, n := range c.kubeInformersForNamespaces.Namespaces() {
		if n == corev1.NamespaceAll {
			d.ClusterWide = true
			d.AdditionalNamespaces = nil
			break
		}

		d.AdditionalNamespaces = append(d.AdditionalNamespaces, n)
	}

	return d
}

func (c *TargetController) ensureTargetNamespace(ctx context.Context) error {
	namespace := DecodeAssetTemplateOrDie("target_v1.0.0/namespace.yaml.tmpl", c.templateContext()).(*corev1.Namespace)

	namespaceLister := c.kubeInformersForNamespaces.InformersFor(corev1.NamespaceAll).Core().V1().Namespaces().Lister()

	_, _, err := resourceapply.ApplyNamespace(ctx, c.kubeClient.CoreV1(), namespaceLister, c.recorder, namespace)

	return err
}

func (c *TargetController) ensurePDB(ctx context.Context) error {
	pdb := DecodeAssetTemplateOrDie("target_v1.0.0/pdb.yaml.tmpl", c.templateContext()).(*policyv1beta1.PodDisruptionBudget)
	pdb.Namespace = c.targetNamespace

	pdbLister := c.kubeInformersForNamespaces.InformersFor(c.targetNamespace).Policy().V1beta1().PodDisruptionBudgets().Lister()

	_, _, err := resourceapply.ApplyPodDisruptionBudget(ctx, c.kubeClient.PolicyV1beta1(), pdbLister, c.recorder, pdb)
	return err
}

func (c *TargetController) ensureServiceAccount(ctx context.Context) error {
	serviceAccount := DecodeAssetTemplateOrDie("target_v1.0.0/serviceaccount.yaml.tmpl", c.templateContext()).(*corev1.ServiceAccount)
	serviceAccount.Namespace = c.targetNamespace

	serviceAccountLister := c.kubeInformersForNamespaces.InformersFor(c.targetNamespace).Core().V1().ServiceAccounts().Lister()

	_, _, err := resourceapply.ApplyServiceAccount(ctx, c.kubeClient.CoreV1(), serviceAccountLister, c.recorder, serviceAccount)
	return err
}

func (c *TargetController) ensureClusterRole(ctx context.Context) error {
	clusterRole := DecodeAssetTemplateOrDie("target_v1.0.0/role.yaml.tmpl", c.templateContext()).(*rbacv1.ClusterRole)

	clusterRoleLister := c.kubeInformersForNamespaces.InformersFor(c.targetNamespace).Rbac().V1().ClusterRoles().Lister()

	_, _, err := resourceapply.ApplyClusterRole(ctx, c.kubeClient.RbacV1(), clusterRoleLister, c.recorder, clusterRole)
	return err
}

func (c *TargetController) ensureClusterRoleBinding(ctx context.Context) error {
	clusterRoleBinding := DecodeAssetTemplateOrDie("target_v1.0.0/rolebinding.yaml.tmpl", c.templateContext()).(*rbacv1.ClusterRoleBinding)

	clusterRoleBindingLister := c.kubeInformersForNamespaces.InformersFor(c.targetNamespace).Rbac().V1().ClusterRoleBindings().Lister()

	_, _, err := resourceapply.ApplyClusterRoleBinding(ctx, c.kubeClient.RbacV1(), clusterRoleBindingLister, c.recorder, clusterRoleBinding)
	return err
}

func (c *TargetController) ensureIssuer(ctx context.Context, assetName string) error {
	issuer := DecodeAssetTemplateOrDie(assetName, c.templateContext()).(*corev1.ConfigMap)
	issuer.Namespace = c.targetNamespace

	configMapLister := c.kubeInformersForNamespaces.InformersFor(c.targetNamespace).Core().V1().ConfigMaps().Lister()

	_, _, err := resourceapply.ApplyConfigMap(ctx, c.kubeClient.CoreV1(), configMapLister, c.recorder, issuer)
	return err
}

func (c *TargetController) ensureIssuers(ctx context.Context) error {
	issuers := []string{
		"target_v1.0.0/issuer-letsencrypt-staging.yaml.tmpl",
	}

	if !c.stagingIssuersOnly {
		issuers = append(issuers, []string{
			"target_v1.0.0/issuer-letsencrypt-live.yaml.tmpl",
		}...)
	}

	var errors []error
	for _, issuerAsset := range issuers {
		errors = append(errors, c.ensureIssuer(ctx, issuerAsset))
	}

	return errorutils.NewAggregate(errors)
}

func (c *TargetController) manageDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	deployment := DecodeAssetTemplateOrDie("target_v1.0.0/deployment.yaml.tmpl", c.templateContext()).(*appsv1.Deployment)
	deployment.Namespace = c.targetNamespace

	deploymentLister := c.kubeInformersForNamespaces.InformersFor(c.targetNamespace).Apps().V1().Deployments().Lister()

	actual, _, err := resourceapply.ApplyDeployment(ctx, c.kubeClient.AppsV1(), deploymentLister, c.recorder, deployment)
	return actual, err
}

func (c *TargetController) sync(ctx context.Context) error {
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

	err = c.ensureTargetNamespace(ctx)
	if err != nil {
		reconciliationErrors = append(reconciliationErrors, fmt.Errorf("ensuring namespace: %w", err))
	}

	err = c.ensurePDB(ctx)
	if err != nil {
		reconciliationErrors = append(reconciliationErrors, fmt.Errorf("ensuring pdb: %w", err))
	}

	err = c.ensureServiceAccount(ctx)
	if err != nil {
		reconciliationErrors = append(reconciliationErrors, fmt.Errorf("ensuring serviceaccount: %w", err))
	}

	err = c.ensureClusterRole(ctx)
	if err != nil {
		reconciliationErrors = append(reconciliationErrors, fmt.Errorf("ensuring cluster role: %w", err))
	}

	err = c.ensureClusterRoleBinding(ctx)
	if err != nil {
		reconciliationErrors = append(reconciliationErrors, fmt.Errorf("ensuring cluster role binding: %w", err))
	}

	err = c.ensureIssuers(ctx)
	if err != nil {
		reconciliationErrors = append(reconciliationErrors, fmt.Errorf("ensuring issuers: %w", err))
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
		klog.V(2).Info(err)
		c.queue.Add(workQueueKey)
		return nil
	} else if err != nil {
		return fmt.Errorf("can't update status: %w", err)
	}

	return nil
}

func (c *TargetController) processNextItem(ctx context.Context) bool {
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

func (c *TargetController) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

func (c *TargetController) Run(ctx context.Context) {
	defer utilruntime.HandleCrash()

	klog.Infof("Starting %s", ControllerName)
	var wg sync.WaitGroup
	defer func() {
		klog.Infof("Shutting down %s", ControllerName)
		c.queue.ShutDown()
		wg.Wait()
		klog.Infof("%s shut down", ControllerName)
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

func DecodeAssetTemplateOrDie(path string, data interface{}) runtime.Object {
	obj, err := runtime.Decode(
		scheme.Codecs.UniversalDeserializer(),
		[]byte(assetutil.MustTemplate(path, string(target_v100.MustAsset(path)), data)),
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

	return base64.StdEncoding.EncodeToString(hasher.Sum(nil))
}
