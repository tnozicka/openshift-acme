package cert

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"reflect"
	"time"
)

type Certificate struct {
	Crt         []byte            // PEM encoded
	Key         []byte            // PEM encoded
	Certificate *x509.Certificate `json:"-"`
}

func NewCertificateFromDER(der [][]byte, privateKey *rsa.PrivateKey) (certificate *Certificate, err error) {
	if len(der) < 1 {
		err = errors.New("can't create certificate from empty DER array")
		return
	}

	certificate = &Certificate{}

	certBuffer := bytes.NewBuffer([]byte{})
	for i, cert := range der {
		var c *x509.Certificate
		c, err = x509.ParseCertificate(cert)
		if err != nil {
			return
		}

		if i == 0 {
			certificate.Certificate = c
		}

		pem.Encode(certBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: cert})
	}
	certificate.Crt = certBuffer.Bytes()

	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	certificate.Key = keyPem

	return
}

func (c *Certificate) UpdateTargetCertificate() (err error) {
	block, _ := pem.Decode(c.Crt)
	if block == nil {
		return errors.New("UpdateTargetCertificate: no data found in Crt")
	}

	c.Certificate, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}

	return
}

func (lhs *Certificate) Equal(rhs *Certificate) bool {
	return reflect.DeepEqual(lhs.Key, rhs.Key) && reflect.DeepEqual(lhs.Crt, rhs.Crt)
}

func (c *Certificate) Domains() (domains []string) {
	domains = append(domains, c.Certificate.DNSNames...)
	// Add common name only if it was not in  DNSNames to avoid duplicate domains
	found := false
	for _, domain := range domains {
		if domain == c.Certificate.Subject.CommonName {
			found = true
		}
	}

	if !found {
		domains = append(domains, c.Certificate.Subject.CommonName)
	}

	return
}

func (c *Certificate) IsValid(t time.Time) bool {
	return IsValid(c, t)
}

func IsValid(c *Certificate, t time.Time) bool {
	return !(t.Before(c.Certificate.NotBefore) || t.After(c.Certificate.NotAfter))
}

func FresherCertificate(c1, c2 *Certificate, t time.Time) *Certificate {
	c1Valid := c1.IsValid(t)
	c2Valid := c1.IsValid(t)
	if c1Valid {
		if c2Valid {
			if c2.Certificate.NotAfter.After(c1.Certificate.NotAfter) {
				return c2
			} else {
				return c1
			}
		} else {
			return c1
		}
	} else {
		if c2Valid {
			return c2
		} else {
			return c1
		}
	}
}
