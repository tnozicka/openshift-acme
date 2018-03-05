package cert

import (
	"crypto/x509"
	"testing"
	"time"
)

func TestIsValid(t *testing.T) {
	now := time.Now()

	tt := []struct {
		certificate x509.Certificate
		valid       bool
	}{
		{
			certificate: x509.Certificate{
				NotBefore: now,
				NotAfter:  now,
			},
			valid: true,
		},
		{
			certificate: x509.Certificate{
				NotBefore: now.Add(-1),
				NotAfter:  now.Add(1),
			},
			valid: true,
		},
		{
			certificate: x509.Certificate{
				NotBefore: now.Add(1),
				NotAfter:  now.Add(2),
			},
			valid: false,
		},
		{
			certificate: x509.Certificate{
				NotBefore: now.Add(-2),
				NotAfter:  now.Add(-1),
			},
			valid: false,
		},
	}

	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			expected := tc.valid
			got := IsValid(&tc.certificate, now)
			if got != expected {
				t.Errorf("expected %t, got %t", expected, got)
			}
		})
	}
}
