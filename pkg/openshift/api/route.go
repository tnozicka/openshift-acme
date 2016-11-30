package api

import (
	"time"

	"k8s.io/client-go/pkg/api/unversioned"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

type RouteTargetReference struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Weight int32  `json:"weight,omitempty"`
}

type RoutePort struct {
	TargetPort string `json:"targetPort"`
}

type TlsConfig struct {
	Termination                   string `json:"termination,omitempty"`
	Certificate                   string `json:"certificate,omitempty"`
	Key                           string `json:"key,omitempty"`
	CaCertificate                 string `json:"caCertificate,omitempty"`
	DestinationCACertificate      string `json:"destinationCACertificate,omitempty"`
	InsecureEdgeTerminationPolicy string `json:"insecureEdgeTerminationPolicy,omitempty"`
}

type RouteSpec struct {
	Host string               `json:"host"`
	Path string               `json:"path,omitempty"`
	To   RouteTargetReference `json:"to,omitempty"`
	Port *RoutePort           `json:"port,omitempty"`
	Tls  *TlsConfig           `json:"tls,omitempty"`
}

type RouteIngressCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty"`
}

type RouteIngress struct {
	Host           string                  `json:"host,omitempty"`
	RouteName      string                  `json:"routeName,omitempty"`
	Conditions     []RouteIngressCondition `json:"conditions,omitempty"`
	WildcardPolicy string                  `json:"wildcardPolicy,omitempty"`
}

type RouteStatus struct {
	Ingress []RouteIngress `json:"ingress,"`
}

type Route struct {
	unversioned.TypeMeta `json:",inline"`
	apiv1.ObjectMeta     `json:"metadata,omitempty"`
	Spec                 RouteSpec   `json:"spec"`
	Status               RouteStatus `json:"status"`
}
