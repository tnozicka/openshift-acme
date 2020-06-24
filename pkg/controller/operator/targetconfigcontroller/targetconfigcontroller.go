package targetconfigcontroller

import (
	"context"
	"fmt"
	"sync"
	"time"

	kubeinformers "github.com/tnozicka/openshift-acme/pkg/machinery/informers/kube"
	corev1 "k8s.io/api/core/v1"
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

func (c *TargetConfigController) sync(ctx context.Context) error {
	klog.V(4).Infof("Started syncing %s", ControllerName)
	defer func() {
		klog.V(4).Infof("Finished syncing %s", ControllerName)
	}()

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
