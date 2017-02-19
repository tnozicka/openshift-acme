package account

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-playground/log"
	"github.com/tnozicka/openshift-acme/pkg/acme"
	"github.com/tnozicka/openshift-acme/pkg/cert"
	acmelib "golang.org/x/crypto/acme"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

const (
	AnnotationAcmeAccountContactsKey = "kubernetes.io/acme.account-contacts"
	DataAcmeAccountCertificatesKey   = "kubernetes.io-acme.account-certificates"
	DataAcmeAccountUrlKey            = "acme.account-url"
	DataTlslKey                      = "tls.key"
	LabelAcmeTypeKey                 = "kubernetes.io/acme.type"
	LabelAcmeAccountType             = "account"
)

var (
	LabelSelectorAcmeAccount = fmt.Sprintf("%s=%s", LabelAcmeTypeKey, LabelAcmeAccountType)
)

type Authorization struct {
	acmelib.Authorization
}

type Account struct {
	Client         acme.Client
	Certificates   []*cert.Certificate
	authorizations []*Authorization
	Secret         *api_v1.Secret
}

func NewAccountFromSecret(secret *api_v1.Secret, acmeUrl string) (a *Account, err error) {
	if secret.Data == nil {
		err = errors.New("malformed acme account: missing Data")
		return
	}

	keyPem, ok := secret.Data[DataTlslKey]
	if !ok {
		err = fmt.Errorf("malformed acme account: missing Data.'%s'", DataTlslKey)
		return
	}
	urlBytes, ok := secret.Data[DataAcmeAccountUrlKey]
	if !ok {
		err = fmt.Errorf("malformed acme account: missing Data.'%s'", DataAcmeAccountUrlKey)
		return
	}
	url := string(urlBytes)

	block, _ := pem.Decode(keyPem)
	if block == nil {
		err = errors.New("existing account has invalid PEM encoded private key")
		return
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return
	}

	a = &Account{
		Client: acme.Client{
			Client: &acmelib.Client{
				Key:          key,
				DirectoryURL: acmeUrl,
			},
			Account: &acmelib.Account{
				URI: url,
			},
		},
		Secret: secret,
	}

	if secret.Annotations != nil {
		contact, found := secret.Annotations[AnnotationAcmeAccountContactsKey]
		if found {
			err = json.Unmarshal([]byte(contact), &a.Client.Account.Contact)
			if err != nil {
				log.Debugf("unable to unmarshal contact '%s': %s", contact, err)
				// ignore this error because it was caused by invalid value put in there by user
				// TODO: update secret.status to tell it to the user
			}
		}
	}

	if secret.Data != nil {
		certificates, found := secret.Data[DataAcmeAccountCertificatesKey]
		if found {
			var err = json.Unmarshal(certificates, &a.Certificates)
			if err != nil {
				log.Debugf("unable to unmarshal certificates '%s': %s", certificates, err)
				// ignore this error because it was caused by invalid value put in there by user
				// TODO: update secret.status to tell it to the user
			}
			for _, c := range a.Certificates {
				c.UpdateTargetCertificate()
			}
		}
	}

	return
}

func (a *Account) ToSecret() (*api_v1.Secret, error) {
	if a.Secret == nil {
		// we are creating new secret
		a.Secret = &api_v1.Secret{
			ObjectMeta: api_v1.ObjectMeta{
				Labels: map[string]string{
					LabelAcmeTypeKey: LabelAcmeAccountType,
				},
			},
		}
	}

	// update all items that could have been changed

	key, ok := a.Client.Client.Key.(*rsa.PrivateKey)
	if !ok {
		err := fmt.Errorf("unsupported key type '%s'", reflect.TypeOf(a.Client.Client.Key))
		return nil, err
	}
	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if a.Secret.Data == nil {
		a.Secret.Data = make(map[string][]byte)
	}
	a.Secret.Data[DataAcmeAccountUrlKey] = []byte(a.Client.Account.URI)
	a.Secret.Data[DataTlslKey] = keyPem

	certificates, err := json.Marshal(a.Certificates)
	if err != nil {
		err = fmt.Errorf("unable to marshal certificates '%s': %s", certificates, err)
		return nil, err
	}
	a.Secret.Data[DataAcmeAccountCertificatesKey] = certificates

	if a.Secret.Annotations == nil {
		a.Secret.Annotations = make(map[string]string)
	}

	contact, err := json.Marshal(a.Client.Account.Contact)
	if err != nil {
		err = fmt.Errorf("unable to marshal contact '%s': %s", contact, err)
		return nil, err
	}
	a.Secret.Annotations[AnnotationAcmeAccountContactsKey] = string(contact)

	return a.Secret, nil
}

func (a *Account) UpdateRemote(ctx context.Context) (err error) {
	a.Client.Account, err = a.Client.Client.UpdateReg(ctx, a.Client.Account)
	if err != nil {
		return
	}

	a.Client.Account, err = a.Client.Client.GetReg(ctx, a.Client.Account.URI)
	if err != nil {
		return
	}

	return
}

func (a *Account) FetchAuthorizations() error {
	// this is not supported in the library not even by letsencrypt.org

	return nil
}

func (a *Account) FetchCertificates() error {
	// this is not supported in the library not even by letsencrypt.org

	return nil
}
