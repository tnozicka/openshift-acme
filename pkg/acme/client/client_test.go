package client

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"golang.org/x/crypto/acme"
	"golang.org/x/net/context"
)

// openssl genrsa 4096
const testKeyPEM = `
-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----
`

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

// Decodes a JWS-encoded request and unmarshals the decoded JSON into a provided
// interface.
func decodeJWSRequest(t *testing.T, v interface{}, r *http.Request) {
	// Decode request
	var req struct{ Payload string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(req.Payload)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(payload, v)
	if err != nil {
		t.Fatal(err)
	}
}

type HandlerFuncWithDiscovery func(http.ResponseWriter, *http.Request)

func (f HandlerFuncWithDiscovery) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "/" {
		baseURL := "http://" + r.Host + "/"
		var (
			reg    = baseURL + "acme/new-reg"
			authz  = baseURL + "acme/new-authz"
			cert   = baseURL + "acme/new-cert"
			revoke = baseURL + "acme/revoke-cert"
		)
		w.Header().Set("content-type", "application/json")
		fmt.Fprintf(w, `{
		"new-reg": %q,
		"new-authz": %q,
		"new-cert": %q,
		"revoke-cert": %q
		}`, reg, authz, cert, revoke)
	} else {
		f(w, r)
	}
}

func TestCreateAccount(t *testing.T) {
	contacts := []string{"mailto:admin@example.com"}

	ts := httptest.NewServer(HandlerFuncWithDiscovery(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("Method: %s; URL: %s\n", r.Method, r.URL)
		if r.Method == "HEAD" {
			w.Header().Set("replay-nonce", "test-nonce")
			return
		}
		if r.Method != "POST" {
			t.Errorf("r.Method = %q; want POST", r.Method)
		}

		var j struct {
			Resource  string
			Contact   []string
			Agreement string
		}
		decodeJWSRequest(t, &j, r)

		// Test request
		if j.Resource != "new-reg" && j.Resource != "reg" {
			t.Errorf("j.Resource = %q; want new-reg OR req", j.Resource)
		}
		if !reflect.DeepEqual(j.Contact, contacts) {
			t.Errorf("j.Contact = %#v; want %#v", j.Contact, contacts)
		}

		baseURL := "http://" + r.Host
		w.Header().Set("Location", "http://"+r.Host+"/acme/reg/1")
		w.Header().Set("Link", "<"+baseURL+`/acme/new-authz>;rel="next"`)
		w.Header().Add("Link", "<"+baseURL+`/acme/recover-reg>;rel="recover"`)
		w.Header().Add("Link", "<"+baseURL+`/acme/terms>;rel="terms-of-service"`)
		w.WriteHeader(http.StatusCreated)
		b, err := json.Marshal(contacts)
		if err != nil {
			t.Error(err)
		}
		fmt.Fprintf(w, `{"contact": %s}`, b)
	}))
	defer ts.Close()

	tests := []acme.Account{
		{
			URI:          ts.URL + "/acme/reg/1",
			Contact:      contacts,
			CurrentTerms: ts.URL + "/acme/terms",
			Authz:        ts.URL + "/acme/new-authz",
		},
	}

	for _, item := range tests {
		c := Client{
			Client:  &acme.Client{Key: testKey, DirectoryURL: ts.URL},
			Account: &item,
		}

		var err error
		prompt := func(url string) bool { return true }
		if c.Account, err = c.Client.Register(context.Background(), c.Account, prompt); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(*c.Account, item) {
			t.Errorf("c.Account = %#v; want %#v", c.Account, item)
		}
	}
}
