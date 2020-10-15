package resourceapply

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
)

// GuessObjectGroupVersionKind returns a human readable for the passed runtime object.
func GuessObjectGroupVersionKind(object runtime.Object) schema.GroupVersionKind {
	gvk := object.GetObjectKind().GroupVersionKind()
	if len(gvk.Kind) > 0 {
		return gvk
	}

	kinds, _, _ := scheme.Scheme.ObjectKinds(object)
	if len(kinds) > 0 {
		return kinds[0]
	}

	return schema.GroupVersionKind{Kind: "<unknown>"}
}

func FormatResourceForCLIWithNamespace(obj runtime.Object) string {
	gvk := GuessObjectGroupVersionKind(obj)
	kind := gvk.Kind
	group := gvk.Group

	var name, namespace string
	accessor, err := meta.Accessor(obj)
	if err != nil {
		name = "<unknown>"
		namespace = "<unknown>"
	} else {
		name = accessor.GetName()
		namespace = accessor.GetNamespace()
	}
	if len(group) > 0 {
		group = "." + group
	}
	if len(namespace) > 0 {
		namespace = " -n " + namespace
	}
	return kind + group + "/" + name + namespace
}

func reportCreateEvent(recorder record.EventRecorder, obj runtime.Object, originalErr error) {
	gvk := GuessObjectGroupVersionKind(obj)

	if originalErr == nil {
		recorder.Eventf(
			obj,
			corev1.EventTypeNormal,
			fmt.Sprintf("%sCreated", gvk.Kind),
			"Created %s because it was missing.", FormatResourceForCLIWithNamespace(obj),
		)
		return
	}

	recorder.Eventf(
		obj,
		corev1.EventTypeWarning,
		fmt.Sprintf("%ssCreateFailed", gvk.Kind),
		"Failed to create %s: %v",
		FormatResourceForCLIWithNamespace(obj),
		originalErr,
	)
}

func reportUpdateEvent(recorder record.EventRecorder, obj runtime.Object, originalErr error) {
	gvk := GuessObjectGroupVersionKind(obj)

	if originalErr == nil {
		recorder.Eventf(
			obj,
			corev1.EventTypeNormal,
			fmt.Sprintf("%sUpdated", gvk.Kind),
			"Updated %s because it changed.", FormatResourceForCLIWithNamespace(obj),
		)
		return
	}

	recorder.Eventf(
		obj,
		corev1.EventTypeWarning,
		fmt.Sprintf("%ssUpdateFailed", gvk.Kind),
		"Failed to update %s: %v",
		FormatResourceForCLIWithNamespace(obj),
		originalErr,
	)
}
