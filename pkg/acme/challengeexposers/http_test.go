package challengeexposers

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/go-playground/log"
	"github.com/go-playground/log/handlers/console"
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
	cLog := console.New()
	log.RegisterHandler(cLog, log.AllLevels...)

	d, _ := pem.Decode([]byte(testKeyPEM))
	if d == nil {
		panic("no block found in testKeyPEM")
	}
	var err error
	if testKey, err = x509.ParsePKCS1PrivateKey(d.Bytes); err != nil {
		panic(err.Error())
	}
}

type FakeKey struct{}

func (FakeKey) Public() crypto.PublicKey {
	return nil
}
func (FakeKey) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) {
	return []byte{}, nil
}

func TestHttp01_Expose(t *testing.T) {
	// test failure with nil key
	a := &acme.Client{
		Key: FakeKey{},
	}

	h, err := NewHttp01(context.Background(), "127.0.0.1:0", log.Logger)
	if err != nil {
		t.Fatal(err)
	}

	err = h.Expose(a, "aa", "ss")
	expectedErr := errors.New("acme: unknown key type; only RSA and ECDSA are supported")
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("using nil key for acme should have ended up with an error '%v', got '%v'", expectedErr, err)
	}
}

func TestHttp01NonExistingAddr(t *testing.T) {
	_, err := NewHttp01(context.Background(), "666.0.0.0:0", log.Logger)

	if err == nil {
		t.Fatalf("setting invalid ip should have ended up with an error")
	}

}

func TestHttp01(t *testing.T) {
	// TODO: consider adding context

	h, err := NewHttp01(context.Background(), "127.0.0.1:0", log.Logger)
	if err != nil {
		t.Fatal(err)
	}

	testTable := []struct {
		domain string
		token  string
		err    error
	}{
		{"com", "IlirfxKKXAsHtmzK29Pj8A", nil},
		{"example.com", "example.com-key", nil},
		{"example.com", "example-key", nil},
		{"alfa.example.com", "aaaaa", nil},
		{"beta.example.com", "bbbbb", nil},
		{"", "any", errors.New("domain can't be empty")},
		{"any.com", "", nil},
		{"", "", errors.New("domain can't be empty")},
	}

	a := &acme.Client{
		Key: testKey,
	}

	client := &http.Client{}

	for _, item := range testTable {
		url := fmt.Sprintf("http://%s%s", h.Addr, a.HTTP01ChallengePath(item.token))

		// test that the url did not exist before and returns correct status
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = item.domain
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("unexisting path for domain '%s' and token '%s' returned status code '%d' instead of '%d'", item.domain, item.token, resp.StatusCode, http.StatusNotFound)
		}

		expectedErr := func() (expectedErr error) {
			expectedErr = h.Expose(a, item.domain, item.token)
			if expectedErr != nil {
				return
			}
			defer h.Remove(a, item.domain, item.token)

			req, err = http.NewRequest("GET", url, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Host = item.domain
			resp, err = client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("couldn't expose domain '%s' and token '%s' - returned status code '%d' instead of '%d'", item.domain, item.token, resp.StatusCode, http.StatusOK)
				return
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			receivedKey := string(body)
			correctKey, err := a.HTTP01ChallengeResponse(item.token)
			if err != nil {
				t.Error(err)
			}

			if receivedKey != correctKey {
				t.Errorf("received key '%s' is not correct ('%s')", receivedKey, correctKey)
			}

			return nil
		}()
		if !reflect.DeepEqual(item.err, expectedErr) {
			t.Errorf("expecting error '%v', got '%v'", item.err, expectedErr)
		}
	}

}
