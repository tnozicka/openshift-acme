package challengeexposers

import (
	"golang.org/x/crypto/acme"
)

// Has to support concurrent calls
type Interface interface {
	// Exposes challenge
	Expose(c *acme.Client, domain string, token string) error

	// Removes challenge
	Remove(domain string) error
}
