package builder

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/golang/glog"
	acmelib "golang.org/x/crypto/acme"
	corev1 "k8s.io/api/core/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kcorelistersv1 "k8s.io/client-go/listers/core/v1"

	acmeclient "github.com/tnozicka/openshift-acme/pkg/acme/client"
)

// TODO: consider moving constants into common package
const (
	DataAcmeAccountUrlKey          = "acme.account-url"
	DataAcmeAccountDirectoryUrlKey = "acme.account-created-at-directory-url"
	DataTlslKey                    = "tls.key"
	LabelAcmeTypeKey               = "kubernetes.io/acme.type"
	LabelAcmeAccountType           = "account"
)

func BuildClientFromSecret(secret *corev1.Secret) (*acmeclient.Client, error) {
	if secret.Data == nil {
		return nil, errors.New("malformed acme account: missing Data")
	}

	keyPem, ok := secret.Data[DataTlslKey]
	if !ok {
		return nil, fmt.Errorf("malformed acme account: missing Data.'%s'", DataTlslKey)
	}
	urlBytes, ok := secret.Data[DataAcmeAccountUrlKey]
	if !ok {
		return nil, fmt.Errorf("malformed acme account: missing Data.'%s'", DataAcmeAccountUrlKey)
	}
	url := string(urlBytes)

	directoryUrlBytes, ok := secret.Data[DataAcmeAccountDirectoryUrlKey]
	if !ok {
		return nil, fmt.Errorf("malformed acme account: missing Data.'%s'", DataAcmeAccountDirectoryUrlKey)
	}
	directoryUrl := string(directoryUrlBytes)

	block, _ := pem.Decode(keyPem)
	if block == nil {
		return nil, errors.New("existing account has invalid PEM encoded private key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return &acmeclient.Client{
		Account: &acmelib.Account{
			URI: url,
		},
		Client: &acmelib.Client{
			Key:          key,
			DirectoryURL: directoryUrl,
		},
	}, nil
}

func SecretFromClient(client *acmeclient.Client) (*corev1.Secret, error) {
	key, ok := client.Client.Key.(*rsa.PrivateKey)
	if !ok {
		err := fmt.Errorf("unsupported key type '%T'", client.Client.Key)
		return nil, err
	}

	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				LabelAcmeTypeKey: LabelAcmeAccountType,
			},
		},
		Data: map[string][]byte{
			DataAcmeAccountUrlKey: []byte(client.Account.URI),
			DataTlslKey:           keyPem,
		},
	}

	return secret, nil
}

func SetSpecificAnnotationsForNewAccount(secret *corev1.Secret, acmeUrl string) {
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[DataAcmeAccountDirectoryUrlKey] = []byte(acmeUrl)
}

type SharedClientFactory struct {
	acmeUrl         string
	secretName      string
	secretNamespace string
	kubeClientset   kubernetes.Interface
	secretGetter    kcorelistersv1.SecretLister
}

func NewSharedClientFactory(acmeUrl, secretName, secretNamespace string, kubeClientset kubernetes.Interface, secretGetter kcorelistersv1.SecretLister) *SharedClientFactory {
	return &SharedClientFactory{
		acmeUrl:         acmeUrl,
		secretName:      secretName,
		secretNamespace: secretNamespace,
		kubeClientset:   kubeClientset,
		secretGetter:    secretGetter,
	}
}

func (f *SharedClientFactory) clientByRegisteringNewAccount(ctx context.Context, account *acmelib.Account) (*acmeclient.Client, error) {
	client := &acmeclient.Client{
		Client: &acmelib.Client{
			DirectoryURL: f.acmeUrl,
		},
		Account: &acmelib.Account{},
	}

	err := client.CreateAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	glog.Infof("Registered new ACME account %q", client.Account.URI)

	return client, nil
}

func (f *SharedClientFactory) GetClient(ctx context.Context) (*acmeclient.Client, error) {
	secret, err := f.secretGetter.Secrets(f.secretNamespace).Get(f.secretName)
	if err != nil {
		if !kapierrors.IsNotFound(err) {
			return nil, err
		}

		// Register new ACME account
		client, err := f.clientByRegisteringNewAccount(ctx, &acmelib.Account{})
		if err != nil {
			return nil, err
		}
		secret, err = SecretFromClient(client)
		if err != nil {
			return nil, err
		}
		secret.Name = f.secretName
		SetSpecificAnnotationsForNewAccount(secret, f.acmeUrl)

		secret, err = f.kubeClientset.CoreV1().Secrets(f.secretNamespace).Create(secret)
		if err == nil {
			glog.Infof("Saved new ACME account %s/%s", secret.Namespace, secret.Name)
			return client, nil
		}

		if !kapierrors.IsAlreadyExists(err) {
			return nil, err
		}

		// Someone created new account just in the interim but that's ok
	}

	return BuildClientFromSecret(secret)
}
