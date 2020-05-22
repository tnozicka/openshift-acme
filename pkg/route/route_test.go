package route

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func timePointer(t metav1.Time) *metav1.Time {
	return &t
}

func TestFindMostRecentIngressAdmittedCondition(t *testing.T) {
	tt := []struct {
		name              string
		route             *routev1.Route
		expectedCondition *routev1.RouteIngressCondition
	}{
		{
			name: "empty status",
			route: &routev1.Route{
				Status: routev1.RouteStatus{},
			},
			expectedCondition: nil,
		},
		{
			name: "no conditions present",
			route: &routev1.Route{
				Status: routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{
							Conditions: []routev1.RouteIngressCondition{},
						},
					},
				},
			},
			expectedCondition: nil,
		},
		{
			name: "single ingress",
			route: &routev1.Route{
				Status: routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{
							Conditions: []routev1.RouteIngressCondition{
								{
									Type:               "foo",
									LastTransitionTime: timePointer(metav1.Unix(10, 0)),
								},
								{
									Type:               "foo-no-time",
									LastTransitionTime: nil,
								},
								{
									Type:               routev1.RouteAdmitted,
									LastTransitionTime: timePointer(metav1.Unix(30, 0)),
									Message:            "winner",
								},
								{
									Type:               routev1.RouteAdmitted,
									LastTransitionTime: timePointer(metav1.Unix(20, 0)),
								},
								{
									Type:               "bar",
									LastTransitionTime: timePointer(metav1.Unix(40, 0)),
								},
							},
						},
					},
				},
			},
			expectedCondition: &routev1.RouteIngressCondition{
				Type:               routev1.RouteAdmitted,
				LastTransitionTime: timePointer(metav1.Unix(30, 0)),
				Message:            "winner",
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			condition := FindMostRecentIngressAdmittedCondition(tc.route)

			if !equality.Semantic.DeepEqual(condition, tc.expectedCondition) {
				t.Errorf("expected condition differs: %s", cmp.Diff(tc.expectedCondition, condition))
			}
		})
	}
}

func TestIsAdmitted(t *testing.T) {
	tt := []struct {
		name     string
		route    *routev1.Route
		expected bool
	}{
		{
			name: "admitted condition missing",
			route: &routev1.Route{
				Status: routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{
							Conditions: []routev1.RouteIngressCondition{},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "admitted unknown",
			route: &routev1.Route{
				Status: routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{
							Conditions: []routev1.RouteIngressCondition{
								{
									Type:   routev1.RouteAdmitted,
									Status: corev1.ConditionUnknown,
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "admitted false",
			route: &routev1.Route{
				Status: routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{
							Conditions: []routev1.RouteIngressCondition{
								{
									Type:   routev1.RouteAdmitted,
									Status: corev1.ConditionFalse,
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "admitted true",
			route: &routev1.Route{
				Status: routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{
							Conditions: []routev1.RouteIngressCondition{
								{
									Type:   routev1.RouteAdmitted,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := IsAdmitted(tc.route)

			if got != tc.expected {
				t.Errorf("expected %t, got %t", tc.expected, got)
			}
		})
	}
}
