package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	legacyGroupVersion            = schema.GroupVersion{Group: "operator.openshift.io", Version: "v1"}
	legacySchemeBuilder           = runtime.NewSchemeBuilder(addLegacyKnownTypes)
	DeprecatedInstallWithoutGroup = legacySchemeBuilder.AddToScheme
)

func addLegacyKnownTypes(scheme *runtime.Scheme) error {
	types := []runtime.Object{
		&ACMEController{},
		&ACMEControllerList{},
	}
	scheme.AddKnownTypes(legacyGroupVersion, types...)
	return nil
}
