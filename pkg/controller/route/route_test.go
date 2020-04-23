package route

import (
	"flag"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrors "k8s.io/apimachinery/pkg/util/errors"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog"
)

func init() {
	// Enable klog which is used in dependencies
	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("v", "9")
}

func TestGetTemporaryName(t *testing.T) {
	tt := []struct {
		name string
		key  string
	}{
		{
			name: "empty key",
			key:  "",
		},
		{
			name: "simple key",
			key:  "my_route",
		},
		{
			name: "combined key",
			key:  "my_route:a.com/b/c/42",
		},
		{
			name: "long key",
			key:  utilrand.String(utilvalidation.DNS1035LabelMaxLength * 2),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r := getTemporaryName(tc.key)

			errs := validation.NameIsDNSSubdomain(r, false)
			if len(errs) != 0 {
				t.Errorf("name %q isn't DNS subdomain: %v", r, errs)
			}
		})
	}
}

func TestAdjustContainerResourceRequirements(t *testing.T) {
	tt := []struct {
		name                         string
		resourceRequirements         *corev1.ResourceRequirements
		limitRanges                  []*corev1.LimitRange
		expectedResourceRequirements *corev1.ResourceRequirements
		expectedErr                  error
	}{
		{
			name: "doesn't change with no LimitRange",
			resourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
			limitRanges: nil,
			expectedResourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
			expectedErr: nil,
		},
		{
			name: "doesn't change with unrelated LimitRange",
			resourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("150Mi"),
				},
			},
			limitRanges: []*corev1.LimitRange{
				{
					Spec: corev1.LimitRangeSpec{
						Limits: []corev1.LimitRangeItem{
							{
								Type: corev1.LimitTypePod,
								Min: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("25Mi"),
								},
								Max: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("300m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
								MaxLimitRequestRatio: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("3"),
								},
							},
						},
					},
				},
			},
			expectedResourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("150Mi"),
				},
			},
			expectedErr: nil,
		},
		{
			name: "adjusts min to the LimitRange",
			resourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
			limitRanges: []*corev1.LimitRange{
				{
					Spec: corev1.LimitRangeSpec{
						Limits: []corev1.LimitRangeItem{
							{
								Type: corev1.LimitTypePod,
								Min: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("300m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
								Max: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("500Mi"),
								},
							},
						},
					},
				},
			},
			expectedResourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("300m"),
					corev1.ResourceMemory: resource.MustParse("200Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("300m"),
					corev1.ResourceMemory: resource.MustParse("200Mi"),
				},
			},
			expectedErr: nil,
		},
		{
			name: "fails on higher resources then LimitRange max",
			resourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
			limitRanges: []*corev1.LimitRange{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "namespace",
					},
					Spec: corev1.LimitRangeSpec{
						Limits: []corev1.LimitRangeItem{
							{
								Type: corev1.LimitTypeContainer,
								Min: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("10Mi"),
								},
								Max: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("20m"),
									corev1.ResourceMemory: resource.MustParse("20Mi"),
								},
							},
						},
					},
				},
			},
			expectedResourceRequirements: nil,
			expectedErr: apierrors.NewAggregate([]error{
				fmt.Errorf("memory ask for 50Mi is higher then maximum memory from limitrange namespace/name"),
				fmt.Errorf("memory ask for 100Mi is higher then maximum memory from limitrange namespace/name"),
				fmt.Errorf("cpu ask for 100m is higher then maximum cpu from limitrange namespace/name"),
				fmt.Errorf("cpu ask for 200m is higher then maximum cpu from limitrange namespace/name"),
			}),
		},
		{
			name: "adjusts request to the LimitRange request ratio",
			resourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("1000Mi"),
				},
			},
			limitRanges: []*corev1.LimitRange{
				{
					Spec: corev1.LimitRangeSpec{
						Limits: []corev1.LimitRangeItem{
							{
								Type: corev1.LimitTypePod,
								MaxLimitRequestRatio: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("5"),
									corev1.ResourceMemory: resource.MustParse("4"),
								},
							},
						},
					},
				},
			},
			expectedResourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("250Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("1000Mi"),
				},
			},
			expectedErr: nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := adjustContainerResourceRequirements(tc.resourceRequirements, tc.limitRanges)

			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("expected error %v, got %v", tc.expectedErr, err)
			}
			if err != nil {
				return
			}

			if !apiequality.Semantic.DeepEqual(tc.resourceRequirements, tc.expectedResourceRequirements) {
				t.Errorf("actual ResourceRequirements expected ones, diff: %s", cmp.Diff(tc.expectedResourceRequirements, tc.resourceRequirements))
			}
		})
	}
}

func TestFilterOutAnnotations(t *testing.T) {
	tt := []struct {
		name                string
		annotations         map[string]string
		expectedAnnotations map[string]string
	}{
		{
			name:                "nil annotations",
			annotations:         nil,
			expectedAnnotations: nil,
		},
		{
			name: "filters correctly",
			annotations: map[string]string{
				"http.exposer.acme.openshift.io/filter-out-annotations": "^matc[h]ing$",
				"foo": "bar",
				"haproxy.router.openshift.io/ip_whitelist": "10.0.0.0/16",
				"matching": "42",
			},
			expectedAnnotations: map[string]string{
				"http.exposer.acme.openshift.io/filter-out-annotations": "^matc[h]ing$",
				"foo": "bar",
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			filterOutAnnotations(tc.annotations)

			if !apiequality.Semantic.DeepEqual(tc.annotations, tc.expectedAnnotations) {
				t.Errorf("expected annotations differ: %s", cmp.Diff(tc.expectedAnnotations, tc.annotations))
			}
		})
	}
}

func TestFilterOutLabels(t *testing.T) {
	tt := []struct {
		name           string
		labels         map[string]string
		annotations    map[string]string
		expectedLabels map[string]string
	}{
		{
			name:           "nil annotations",
			labels:         nil,
			annotations:    nil,
			expectedLabels: nil,
		},
		{
			name: "filters correctly",
			annotations: map[string]string{
				"http.exposer.acme.openshift.io/filter-out-labels": "^matc[h]ing$",
			},
			labels: map[string]string{
				"foo":      "bar",
				"matching": "42",
			},
			expectedLabels: map[string]string{
				"foo": "bar",
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var annotationsCopy map[string]string
			if tc.annotations != nil {
				annotationsCopy = map[string]string{}
				for k, v := range tc.annotations {
					annotationsCopy[k] = v
				}
			}

			filterOutLabels(tc.labels, tc.annotations)

			if !reflect.DeepEqual(tc.annotations, annotationsCopy) {
				t.Errorf("annotations were changed: %s", cmp.Diff(annotationsCopy, tc.annotations))
			}

			if !apiequality.Semantic.DeepEqual(tc.labels, tc.expectedLabels) {
				t.Errorf("expected labels differ: %s", cmp.Diff(tc.expectedLabels, tc.labels))
			}
		})
	}
}
