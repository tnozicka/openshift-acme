package route

import (
	routev1 "github.com/openshift/api/route/v1"
)

func IsAdmitted(route *routev1.Route) bool {
	admittedSet := false
	admittedValue := true
	for _, ingress := range route.Status.Ingress {
		for _, condition := range ingress.Conditions {
			if condition.Type == "Admitted" {
				admittedSet = true
				if condition.Status != "True" {
					admittedValue = false
				}
			}
		}
	}
	return admittedSet && admittedValue
}
