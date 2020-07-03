package targetconfigcontroller

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/tnozicka/openshift-acme/pkg/api"
	v100_00_assets "github.com/tnozicka/openshift-acme/pkg/controller/operator/v100_00_assets"
	kubeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/kube"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	workQueueKey   = "key"
	resyncPeriod   = 15 * time.Minute
)

type TargetConfigController struct {
	targetNamespace string

	kubeClient                 kubernetes.Interface
	kubeInformersForNamespaces kubeinformers.Interface

	recorder record.EventRecorder

	queue workqueue.RateLimitingInterface
}

func NewTargetConfigController(
	targetNamespace string,
	kubeClient kubernetes.Interface,
	kubeInformersForNamespaces kubeinformers.Interface,
) *TargetConfigController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})

	c := &TargetConfigController{
		targetNamespace: targetNamespace,
		kubeClient:      kubeClient,

		recorder: eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: ControllerName}),

		queue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	kubeInformersForNamespaces.InformersFor(c.targetNamespace).
		Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())
	kubeInformersForNamespaces.InformersFor(c.targetNamespace).
		Apps().V1().Deployments().Informer().AddEventHandler(c.eventHandler())

	return c
}

func (c *TargetConfigController) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.queue.Add(workQueueKey) },
		UpdateFunc: func(old, new interface{}) { c.queue.Add(workQueueKey) },
		DeleteFunc: func(obj interface{}) { c.queue.Add(workQueueKey) },
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

	var errors []error

	deployment, err := c.manageDeployment(ctx)
	if err != nil {
		errors = append(errors, fmt.Errorf("managing deployment: %v", err))
	}

	manageErr := errorutils.NewAggregate(errors)
	// Update status

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

	utilruntime.HandleError(fmt.Errorf("syncing key %v failed with : %v", key, err))
	c.queue.AddRateLimited(key)

	return true
}

func (c *TargetConfigController) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

func (c *TargetConfigController) Run(ctx context.Context) {
	defer utilruntime.HandleCrash()

	klog.Info("Starting %s", ControllerName)
	var wg sync.WaitGroup
	defer func() {
		klog.Info("Shutting down %s", ControllerName)
		c.queue.ShutDown()
		wg.Wait()
		klog.Info("%s shut down", ControllerName)
	}()

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
		scheme.Codecs.UniversalDecoder(corev1.SchemeGroupVersion),
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
