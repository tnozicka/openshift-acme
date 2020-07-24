package v1

import (
	operatorv1 "github.com/openshift/api/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ACMEController provides information to configure an operator to manage openshift-acme controller.
// +kubebuilder:resource:scope="Cluster"
type ACMEController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	// spec is the specification of the desired behavior of the openshift-acme controller
	// +kubebuilder:validation:Required
	// +required
	Spec ACMEControllerSpec `json:"spec"`

	// status is the most recently observed status of the openshift-acme controller
	// +optional
	Status ACMEControllerStatus `json:"status"`
}

type ACMEControllerSpec struct {
	// managementState indicates whether and how the operator should manage the component
	ManagementState operatorv1.ManagementState `json:"managementState"`

	// logLevel is an intent based logging for an overall component.  It does not give fine grained control, but it is a
	// simple way to manage coarse grained logging choices that operators have to interpret for their operands.
	// +optional
	LogLevel operatorv1.LogLevel `json:"logLevel"`

	// operatorLogLevel is an intent based logging for the operator itself.  It does not give fine grained control, but it is a
	// simple way to manage coarse grained logging choices that operators have to interpret for themselves.
	// +optional
	OperatorLogLevel operatorv1.LogLevel `json:"operatorLogLevel"`
}

type ACMEControllerStatus struct {
	// observedGeneration is the last generation change you've dealt with
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions is a list of conditions and their status
	// +optional
	Conditions []operatorv1.OperatorCondition `json:"conditions,omitempty"`

	// availableReplicas indicates how many replicas are available and at the desired state
	AvailableReplicas int32 `json:"availableReplicas"`

	// version is the level this availability applies to
	// +optional
	Version string `json:"version,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ACMEControllerList is a collection of items
type ACMEControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items contains the items
	Items []ACMEController `json:"items"`
}
