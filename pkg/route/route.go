package route

import (
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
)

func IsAdmitted(route *routev1.Route) bool {
	c := FindMostRecentIngressAdmittedCondition(route)
	return c != nil && c.Status == corev1.ConditionTrue
}

func FindMostRecentIngressAdmittedCondition(route *routev1.Route) *routev1.RouteIngressCondition {
	var condition *routev1.RouteIngressCondition
	for ingressIdx := range route.Status.Ingress {
		ingress := &route.Status.Ingress[ingressIdx]

		for conditionIdx := range ingress.Conditions {
			c := &ingress.Conditions[conditionIdx]

			if c.Type != routev1.RouteAdmitted {
				continue
			}

			if condition == nil {
				condition = c
				continue
			}

			if c.LastTransitionTime.Time.After(condition.LastTransitionTime.Time) {
				condition = c
			}
		}
	}

	return condition
}

func FindMostRecentIngressAdmittedConditionOrUnknown(route *routev1.Route) *routev1.RouteIngressCondition {
	c := FindMostRecentIngressAdmittedCondition(route)
	if c == nil {
		return &routev1.RouteIngressCondition{
			Type:   routev1.RouteAdmitted,
			Status: corev1.ConditionUnknown,
			Reason: "SyntheticUnknownCondition",
		}
	}
	return c
}
