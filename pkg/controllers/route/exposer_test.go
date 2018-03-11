package route

import (
	"reflect"
	"testing"

	kvalidation "k8s.io/apimachinery/pkg/util/validation"
)

func TestCreateTemporaryExposerName(t *testing.T) {
	tt := []struct {
		routeName    string
		expectErrors []string
	}{
		{
			routeName:    "dns-label",
			expectErrors: nil,
		},
		{
			routeName:    "dns.subdomain",
			expectErrors: nil,
		},
	}

	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			// Sanity - test if it's a valid Route name
			err := kvalidation.IsDNS1123Subdomain(tc.routeName)
			if err != nil {
				t.Errorf("invalid Route name %q: %v", tc.routeName, err)
			}

			tmpExposerName := createTemporaryExposerName(tc.routeName)

			gotErrors := kvalidation.IsDNS1035Label(tmpExposerName)
			if !reflect.DeepEqual(gotErrors, tc.expectErrors) {
				t.Errorf("expected %v, got %v", tc.expectErrors, gotErrors)
			}
		})
	}
}
