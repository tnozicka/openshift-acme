package challengeexposers

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"reflect"
	"testing"

	"golang.org/x/crypto/acme"
)

// openssl genrsa 2048
const (
	testKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAq9hvyTwL8KO4b1UGJqMp2cLM8Dc6waTyuGaNWcCiEaK0dR6h
e/lnSJB7oUJn9pnh6K6aOFadO82tgl1F0RkZghlgqsGVj3gag3WzYAl6jZHwjtEZ
Mt3BCfUqS8+913TpTJ4UOeg+vrFmdaQBrXvD9M48Wc9/xdjM4uWTuqPizm5qhPeL
OGYDC2WpqG28h/Zjul90tgVgFSOLAR6u71Cia5AIC6zt2cgWiZX2eTBwCpdIrlVl
sEHRTecxc84ametfiepeclfQRJsIQzOXRjCdrtm+Q3Mpq3fFQFd9wnzSMoXOoz+w
lg3CDZ28o4aBH+kvU/qMaU1WKmC4Md+SLfynqwIDAQABAoIBAGFZ6OIgoUb/CPIR
Qb3Lu17A66a+MwwWrOZoOnV5GpdRDFg5NRRtkuRZ7Z/KZeke/gr6NyXpc4O4ni33
NyEgzgGU7N1vc5HmYWbN3ANw+zuOTLArptHZLI2+lOqD0rFnau9bsmuntTwEdd35
PnYQYk+iMjBSy7jCfDeiBVG7nDaDClV0DscBdnC/5Ef82gIUv3xVvZFRm7L3rUAY
njwP7UhkjpWxLU22Cn5cV7wjzSPiiTa3/gyrp4+cQhKTb6RsZNBwcN4XLCJsueKp
3zK58cmuWnJzliAGUwCl4h56ioiHBuxQLtJlGy865VJypKi4g75x9CG2VGMOocFF
7qXkE+ECgYEA5GGSuzsjZ5t2m7OxtLo0RLokqjprgYA3xDfNU3KAWMaSYfmKn/7g
byF/Cf4I36c3OTkzfIonPdZE/r+fiOve8p7JdDPPHRvzYPW9poFu2gEmKBbFJgPs
JNQg5b5cAAYm733/f5pDViPD5qlDsNPSOvw8V/IknnA9LyMI5/vcq9ECgYEAwKCU
zwfJnrmNV3lX0mL+b9VW8ecOMTXfz86UbNoctlbK+cVEcw5Fq1WJGgAMx+r1c8SV
xC+Losln1r42A7Geui5Qa9K0rRuIwPbKAGAUDj8Q9E9SPcxENJhPJj+2YQBHSy9S
8EgLuPOpjS6aY7cWx2STvzUGBSTyNUK4dtPQRrsCgYEAt3ylVQRIh79h5erTha5s
vCMJvjK9mQgYxe9Hahn+gFTZ2xmQhMdULjUtSivtmTNRrQoGEbM1n/r85+exF2La
dveYR2IwruR7/5SwUIyBMWnm7CKPNuHD4jsES1FLvUE0GwqSMkUQgK6vgCzSE8m7
iGSLXuVPAnSO08ZEK44xV2ECgYADdl92YTN0kO1Dd0Dm3TSpmfIYIwkURV2ihJoS
YtFFTcYUO0GBt+30qHLwbrPMHCMRU6VFg31FDc26BG1AH780pYR4i68HtYj9vvHe
k9uIbgXF/m8CAVvwfhReIaMLl0+wwEcDXqgnSOnxSbcot6/HIb2uICvdh856uppK
OIBz5QKBgETJu+0uGxkT64l3+YwSBj/vODBpK//bymqW+AcerhXj7pIfD73hH2rv
hsZKIsvz8ywzMwdtn0tGWayY8Dt8CFTkIaWBwVtkxF/CPXVnuGbDv5c089AviOeK
JwCh2MUGL0HzoyMNDUDxGAYc2xei5qjMiq8xgNrPs6cYwpMvSTcd
-----END RSA PRIVATE KEY-----`
)

var (
	testKey *rsa.PrivateKey
)

func init() {
	d, _ := pem.Decode([]byte(testKeyPEM))
	if d == nil {
		panic("no block found in testKeyPEM")
	}
	var err error
	if testKey, err = x509.ParsePKCS1PrivateKey(d.Bytes); err != nil {
		panic(err.Error())
	}
}

func TestNewHttp01(t *testing.T) {
	tt := []struct {
		address     string
		expectedErr error
	}{
		{
			address:     "127.0.0.1:0",
			expectedErr: nil,
		},
		{
			address: "666.0.0.0:0",
			expectedErr: &net.OpError{Op: "listen", Net: "tcp", Source: nil, Addr: nil, Err: &net.DNSError{
				Err: "no such host", Name: "666.0.0.0", Server: "", IsTimeout: false, IsTemporary: false,
			}},
		},
	}

	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			_, err := NewHttp01(context.Background(), tc.address)
			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("expected error '%#v', got '%#v'", tc.expectedErr, err)
				return
			}
		})
	}
}

func urlCountForDomain(h *Http01, domain string) int {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return len(h.domainToUrls[domain])
}

func tokenCountForDomain(h *Http01, domain string) int {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	count := 0
	for _, url := range h.domainToUrls[domain] {
		_, found := h.urlToToken[url]
		if !found {
			panic("invalid domain->url reference: no token found")
		}

		count += 1
	}

	return count
}

func TestHttp01_ExposeAndRemove(t *testing.T) {
	tt := []struct {
		domain      string
		token       string
		expectedErr error
	}{
		{
			domain:      "com",
			token:       "IlirfxKKXAsHtmzK29Pj8A",
			expectedErr: nil,
		},
		{
			domain:      "example.com",
			token:       "example.com-key",
			expectedErr: nil,
		},
		{
			domain:      "example.com",
			token:       "example-key",
			expectedErr: nil,
		},
		{
			domain:      "alfa.example.com",
			token:       "aaaaa",
			expectedErr: nil,
		},
		{
			domain:      "beta.example.com",
			token:       "bbbbb",
			expectedErr: nil,
		},
		{
			domain:      "",
			token:       "any",
			expectedErr: errors.New("domain can't be empty"),
		},
		{
			domain:      "any.com",
			token:       "",
			expectedErr: nil,
		},
		{
			domain:      "",
			token:       "",
			expectedErr: errors.New("domain can't be empty"),
		},
	}

	a := &acme.Client{
		Key: testKey,
	}

	h, err := NewHttp01(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{}

	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			// Run this twice to make sure is works continuously
			for i := 0; i < 2; i++ {
				t.Run("", func(t *testing.T) {
					if urlCountForDomain(h, tc.domain) != 0 {
						t.Errorf("there should be no entry for domain %q", tc.domain)
					}
					if tokenCountForDomain(h, tc.domain) != 0 {
						t.Errorf("there should be no token for domain %q", tc.domain)
					}

					url := fmt.Sprintf("http://%s%s", h.Addr, a.HTTP01ChallengePath(tc.token))

					// test that the url did not exist before and returns correct status
					req, err := http.NewRequest("GET", url, nil)
					if err != nil {
						t.Error(err)
						return
					}
					req.Host = tc.domain
					resp, err := client.Do(req)
					if err != nil {
						t.Error(err)
						return
					}

					if resp.StatusCode != http.StatusNotFound {
						t.Errorf("unexisting path for domain '%s' and token '%s' returned status code '%d' instead of '%d'", tc.domain, tc.token, resp.StatusCode, http.StatusNotFound)
					}

					// Expose
					err = h.Expose(a, tc.domain, tc.token)
					if !reflect.DeepEqual(err, tc.expectedErr) {
						t.Error(err)
						return
					}
					if err != nil {
						return
					}

					if urlCountForDomain(h, tc.domain) != 1 {
						t.Errorf("there should be no entry for domain %q", tc.domain)
					}
					if tokenCountForDomain(h, tc.domain) != 1 {
						t.Errorf("there should be no token for domain %q", tc.domain)
					}

					// Check that it is exposed
					req, err = http.NewRequest("GET", url, nil)
					if err != nil {
						t.Error(err)
						return
					}
					req.Host = tc.domain
					resp, err = client.Do(req)
					if err != nil {
						t.Error(err)
						return
					}
					defer resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						t.Errorf("couldn't expose domain '%s' and token '%s' - returned status code '%d' instead of '%d'", tc.domain, tc.token, resp.StatusCode, http.StatusOK)
						return
					}

					body, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						t.Error(err)
						return
					}
					receivedKey := string(body)
					correctKey, err := a.HTTP01ChallengeResponse(tc.token)
					if err != nil {
						t.Error(err)
						return
					}

					if receivedKey != correctKey {
						t.Errorf("expected key '%s', got '%s'", receivedKey, correctKey)
					}

					err = h.Remove(tc.domain)
					if err != nil {
						t.Error(err)
						return
					}

					if urlCountForDomain(h, tc.domain) != 0 {
						t.Errorf("there should be no entry for domain %q", tc.domain)
					}
					if tokenCountForDomain(h, tc.domain) != 0 {
						t.Errorf("there should be no token for domain %q", tc.domain)
					}
				})
			}
		})
	}
}
