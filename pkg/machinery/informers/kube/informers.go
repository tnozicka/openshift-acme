package kube

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	Start(stopCh <-chan struct{})
	InformersFor(namespace string) informers.SharedInformerFactory
	InformersForOrGlobal(namespace string) informers.SharedInformerFactory
	Namespaces() []string
}

type kubeInformersForNamespaces map[string]informers.SharedInformerFactory

var _ Interface = kubeInformersForNamespaces{}

func NewKubeInformersForNamespaces(kubeClient kubernetes.Interface, namespaces []string) kubeInformersForNamespaces {
	res := kubeInformersForNamespaces{}

	for _, namespace := range namespaces {
		res[namespace] = informers.NewSharedInformerFactoryWithOptions(kubeClient, 0, informers.WithNamespace(namespace))
	}

	return res
}

func (i kubeInformersForNamespaces) Start(stopCh <-chan struct{}) {
	for _, informer := range i {
		informer.Start(stopCh)
	}
}

func (i kubeInformersForNamespaces) Namespaces() []string {
	var ns []string
	for n := range i {
		ns = append(ns, n)
	}
	return ns
}

func (i kubeInformersForNamespaces) InformersFor(namespace string) informers.SharedInformerFactory {
	return i[namespace]
}

func (i kubeInformersForNamespaces) InformersForOrGlobal(namespace string) informers.SharedInformerFactory {
	informer, ok := i[namespace]
	if !ok {
		return i[metav1.NamespaceAll]
	}
	return informer
}

func (i kubeInformersForNamespaces) HasInformersFor(namespace string) bool {
	return i.InformersFor(namespace) != nil
}
