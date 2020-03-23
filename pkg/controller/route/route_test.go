package route

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/validation"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
)

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
