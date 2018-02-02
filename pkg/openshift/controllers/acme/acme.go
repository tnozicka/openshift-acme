package acme

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-playground/log"
	"github.com/tnozicka/openshift-acme/pkg/acme"
	"github.com/tnozicka/openshift-acme/pkg/cert"
	accountlib "github.com/tnozicka/openshift-acme/pkg/openshift/account"
	acmelib "golang.org/x/crypto/acme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	kerrors "k8s.io/client-go/pkg/api/errors"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

type AcmeObject interface {
	GetDomains() []string
	GetNamespace() string
	GetUID() string
	GetCertificate() *cert.Certificate
	UpdateCertificate(c *cert.Certificate) error
	GetExposers() map[string]acme.ChallengeExposer
}

type AcmeController struct {
	kclient              v1core.CoreV1Interface
	acmeDirectoryUrl     string
	ctx                  context.Context
	wg                   sync.WaitGroup
	Db                   *CertDB
	renewalCheckInterval time.Duration
	retryCheckInterval   time.Duration
	maxTries             int
	watchNamespaces      []string
}

func NewAcmeController(ctx context.Context, kclient v1core.CoreV1Interface, acmeDirectoryUrl string, watchNamespaces []string) (rc *AcmeController) {
	rc = &AcmeController{
		ctx:              ctx,
		kclient:          kclient,
		acmeDirectoryUrl: acmeDirectoryUrl,
		Db:               NewCertDB(ctx, kclient),
		watchNamespaces:  watchNamespaces,
	}

	if rc.renewalCheckInterval <= 0 {
		rc.renewalCheckInterval = 5 * time.Minute
	}

	if rc.retryCheckInterval <= 0 {
		rc.retryCheckInterval = 5 * time.Minute
	}

	rc.maxTries = 20

	return
}

func (ac *AcmeController) retryLoop() {
	defer ac.wg.Done()
	defer log.Info("AcmeController - retryLoop - finished")

loop:
	for {
		select {
		case <-time.After(ac.retryCheckInterval):
			log.Debug("Retry check triggered by schedule.")

			certEntries := ac.Db.GetCertEntryShallowSnapshot()
			for _, certEntry := range certEntries {
				func() {
					certEntry.mutex.Lock()
					defer certEntry.mutex.Unlock()

					if certEntry.inProgress {
						return
					}

					if certEntry.failedCounter <= 0 || certEntry.failedCounter > ac.maxTries {
						return
					}

					// TODO: do some kind of exponential backoff

					var o AcmeObject
					for _, o = range certEntry.objects {
						break // take 1st object from a map
					}
					if o == nil {
						return
					}
					log.Infof("retryLoop: start obtaining certificate for: '%s'", o.GetDomains())

					certEntry.startObtainingCertificate()
				}()
			}

		case <-ac.ctx.Done():
			break loop
		}
	}
}

func (ac *AcmeController) renewLoop() {
	defer ac.wg.Done()
	defer log.Info("AcmeController - renewLoop - finished")

loop:
	for {
		select {
		case <-time.After(ac.renewalCheckInterval):
			log.Debug("Renewal check triggered by schedule.")
			now := time.Now()

			certEntries := ac.Db.GetCertEntryShallowSnapshot()
			for _, certEntry := range certEntries {
				func() {
					certEntry.mutex.Lock()
					defer certEntry.mutex.Unlock()

					if certEntry.inProgress {
						return
					}

					if certEntry.certificate == nil {
						// Don't try to obtain certificate here. (This is a job for retryLoop which handles all the cases and does error handling.)
						return
					}

					notBefore := certEntry.certificate.Certificate.NotBefore
					notAfter := certEntry.certificate.Certificate.NotAfter

					// sanity check
					if notBefore.After(notAfter) {
						// TODO: report error in cert object's annotations
						log.Debugf("renewLoop: Invalid certificate: %#v", certEntry.certificate.Certificate)
						return
					}

					// TODO: make this configurable
					renewTime := notBefore.Add(2 * (notAfter.Sub(notBefore)) / 3)
					renew := now.After(renewTime)
					log.Debugf("notBefore=%s, notAfter=%s, renewTime=%s; renew=%t", notBefore, notAfter, renewTime, renew)
					if renew {
						log.Debugf("renewLoop: renewing certificate for: '%s'", certEntry.certificate.Domains())
						certEntry.startObtainingCertificate()
					}
				}()
			}

		case <-ac.ctx.Done():
			break loop
		}
	}
}

func (ac *AcmeController) Start() {
	ac.Wait() // make sure it can't be started twice at the same time

	ac.wg.Add(1)
	go ac.retryLoop()
	ac.wg.Add(1)
	go ac.renewLoop()
}

func (rc *AcmeController) Wait() {
	rc.wg.Wait()
}

func (ac *AcmeController) AcmeAccount(namespace string) (a *accountlib.Account, err error) {
	secretList, err := ac.kclient.Secrets(namespace).List(api_v1.ListOptions{
		LabelSelector: accountlib.LabelSelectorAcmeAccount,
	})
	if err != nil {
		return
	}

	if len(secretList.Items) < 1 {
		// there is no ACME account present => create new one
		a = &accountlib.Account{
			Client: acme.Client{
				Client: &acmelib.Client{
					DirectoryURL: ac.acmeDirectoryUrl,
				},
				Account: &acmelib.Account{},
			},
		}

		log.Infof("Creating new account in namespace %s", namespace)
		defer log.Tracef("Creating new account in namespace %s finished", namespace).End()
		if err = a.Client.CreateAccount(ac.ctx, a.Client.Account, acmelib.AcceptTOS); err != nil {
			return nil, err
		}

		newSecret, err := a.ToSecret()
		if err != nil {
			return nil, err
		}
		newSecret.Name = "acme-account"
		secret, err := ac.kclient.Secrets(namespace).Create(newSecret)
		if err != nil {
			return nil, err
		}
		a.Secret = secret
	} else {
		// there is at least 1 account, but there could be more
		// TODO: we should probably pick up the one with highest registration URL to be consistent
		a, err = accountlib.NewAccountFromSecret(&secretList.Items[0], ac.acmeDirectoryUrl)
		if err != nil {
			err = fmt.Errorf("acmeClient: '%s'", err)
			return
		}
	}

	return a, nil
}

func (ac *AcmeController) UpdateAcmeAccount(a *accountlib.Account) (err error) {
	maxAttempts := 10
	for i := 0; i < 10; i++ {
		secret, err := a.ToSecret()
		if err != nil {
			return fmt.Errorf("UpdateAcmeAccount: %s", err)
		}

		secret, err = ac.kclient.Secrets(secret.Namespace).Update(secret)
		if err != nil {
			kerr, ok := err.(*kerrors.StatusError)
			if ok && kerr.Status().Code == 409 {
				// there is a conflict for the update, someone is trying to change it as well
				log.Debugf("UpdateAcmeAccount failed because of conflict: '%#v'", kerr.ErrStatus)
				continue
			} else {
				return fmt.Errorf("UpdateAcmeAccount: %#v", secret)
			}
		}
		a.Secret = secret
		return nil
	}
	return fmt.Errorf("UpdateAcmeAccount: all %d attempt(s) failed with resource conflict (409)", maxAttempts)
}

func (ac *AcmeController) Manage(o AcmeObject) (err error) {
	account, err := ac.AcmeAccount(o.GetNamespace())
	if err != nil {
		return err
	}
	ac.Db.AddObject(account, o)
	return
}

func (ac *AcmeController) Done(o AcmeObject) (err error) {
	account, err := ac.AcmeAccount(o.GetNamespace())
	if err != nil {
		return err
	}
	ac.Db.RemoveObject(account, o)
	return
}

func (ac *AcmeController) BootstrapDB(updateAccounts bool, updateStatus bool) error {
	for _, namespace := range ac.watchNamespaces {
		log.Debugf("AcmeController: Bootstrapping namespace '%s'", namespace)
		err := ac.Db.Bootstrap(ac.ctx, namespace, ac.acmeDirectoryUrl, updateAccounts, updateStatus)
		if err != nil {
			return err
		}
	}
	return nil
}
