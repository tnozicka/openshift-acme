package acme

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/go-playground/log"
	"github.com/tnozicka/openshift-acme/pkg/cert"
	accountlib "github.com/tnozicka/openshift-acme/pkg/openshift/account"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	kerrors "k8s.io/client-go/pkg/api/errors"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func hashDomains(domains ...string) string {
	var buffer bytes.Buffer
	for _, domain := range domains {
		buffer.WriteString(domain)
		buffer.WriteString(";")
	}
	return buffer.String()
}

func accountKeyString(account *accountlib.Account) string {
	keyBytes := x509.MarshalPKCS1PrivateKey(account.Client.Client.Key.(*rsa.PrivateKey))
	return base64.StdEncoding.EncodeToString(keyBytes)
}

type DbAccountEntry struct {
	account                 *accountlib.Account
	kclient                 v1core.CoreV1Interface
	certificatesMutex       sync.Mutex
	syncCertificatesChannel chan []*cert.Certificate
	syncCertificatesWg      sync.WaitGroup
	ctx                     context.Context
	ctxCancel               context.CancelFunc
	db                      map[string]*DbCertEntry
}

func NewDbAccountEntry(ctx context.Context, account *accountlib.Account, kclient v1core.CoreV1Interface) *DbAccountEntry {
	ctx, cancel := context.WithCancel(ctx)
	e := &DbAccountEntry{
		account:                 account,
		kclient:                 kclient,
		db:                      make(map[string]*DbCertEntry),
		ctx:                     ctx,
		ctxCancel:               cancel,
		syncCertificatesChannel: make(chan []*cert.Certificate, 100),
	}

	e.syncCertificatesWg.Add(1)
	go e.syncCertificatesLoop()

	return e
}

func (e *DbAccountEntry) GetCertEntry(domainHash string) (entry *DbCertEntry) {
	entry, present := e.db[domainHash]
	if !present {
		entry = NewDbCertEntry(e.ctx, e)
		e.db[domainHash] = entry
	}

	return
}

func (e *DbAccountEntry) syncCertificatesLoop() {
	defer e.syncCertificatesWg.Done()
	defer log.Debug("DbAccountEntry - SyncCertificates - finished")

loop:
	for {
		select {
		case <-e.syncCertificatesChannel:
			cachedSecret, err := e.GetAccountSecret()
			if err != nil {
				log.Errorf("SyncCertificates: %s", err)
			}

			syncTries := 3
			for i := 0; i < syncTries; i++ {
				secret, err := e.kclient.Secrets(cachedSecret.Namespace).Get(cachedSecret.Name)
				if err != nil {
					if kerrors.IsNotFound(err) {
						log.Errorf("SyncCertificates: %s", err)
						break
					} else {
						log.Errorf("SyncCertificates attempt %d/%d failed: %s", i, syncTries, err)
						continue
					}
				}

				if secret.Data == nil {
					secret.Data = make(map[string][]byte)
				}
				// FIXME: merge instead of rewrite
				secret.Data[accountlib.DataAcmeAccountCertificatesKey] = cachedSecret.Data[accountlib.DataAcmeAccountCertificatesKey]

				secret, err = e.kclient.Secrets(secret.Namespace).Update(secret)
				if err != nil {
					log.Errorf("SyncCertificates attempt %d/%d failed: %s", i, syncTries, err)
					continue
				}

				log.Debugf("Synced certificate for %s/%s", cachedSecret.Namespace, cachedSecret.Name)

				break
			}
		case <-e.ctx.Done():
			break loop
		}
	}
}

func (e *DbAccountEntry) GetAccountSecret() (*api_v1.Secret, error) {
	e.certificatesMutex.Lock()
	defer e.certificatesMutex.Unlock()

	return e.account.ToSecret()
}

func (e *DbAccountEntry) AddCertificates(c ...*cert.Certificate) {
	e.certificatesMutex.Lock()
	defer e.certificatesMutex.Unlock()

	e.account.Certificates = append(e.account.Certificates, c...)

	e.syncCertificatesChannel <- c

	return
}

type CertDB struct {
	kclient   v1core.CoreV1Interface
	db        map[string]*DbAccountEntry
	dbMutex   sync.Mutex
	ctx       context.Context
	ctxCancel context.CancelFunc
}

func NewCertDB(ctx context.Context, kclient v1core.CoreV1Interface) *CertDB {
	ctx, cancel := context.WithCancel(ctx)
	return &CertDB{
		db:        make(map[string]*DbAccountEntry),
		ctx:       ctx,
		ctxCancel: cancel,
		kclient:   kclient,
	}
}

// mutex is held by calling method
func (d *CertDB) getAccountEntry(account *accountlib.Account) (entry *DbAccountEntry) {
	key := accountKeyString(account)
	entry, present := d.db[key]
	if !present {
		entry = NewDbAccountEntry(d.ctx, account, d.kclient)
		d.db[key] = entry
	}

	return
}

// mutex is held by calling method
func (d *CertDB) getCertEntry(account *accountlib.Account, domainsKey string) *DbCertEntry {
	return d.getAccountEntry(account).GetCertEntry(domainsKey)
}

func (d *CertDB) AddObject(account *accountlib.Account, o AcmeObject) {
	// FIXME: do this properly with account key for case of cross-namespace accounts
	domainsKey := hashDomains(o.GetDomains()...)

	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	entry := d.getCertEntry(account, domainsKey)
	entry.AddObject(o)
}

func (d *CertDB) RemoveObject(account *accountlib.Account, o AcmeObject) {
	// FIXME: do this properly with account key for case of cross-namespace accounts
	domainsKey := hashDomains(o.GetDomains()...)

	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	entry := d.getCertEntry(account, domainsKey)
	entry.RemoveObject(o)
}

func (d *CertDB) AddCertificate(account *accountlib.Account, certificate *cert.Certificate) {
	log.Debug("Adding object")
	domainsKey := hashDomains(certificate.Domains()...)

	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	entry := d.getCertEntry(account, domainsKey)
	entry.UpdateCertificate(certificate)

	return
}

func (d *CertDB) GetCertEntryShallowSnapshot() []*DbCertEntry {
	log.Debug("GetCertEntryShallowSnapshot")

	d.dbMutex.Lock()
	defer d.dbMutex.Unlock()

	r := make([]*DbCertEntry, 0, len(d.db))

	for _, accountEntry := range d.db {
		for _, certEntry := range accountEntry.db {
			r = append(r, certEntry)
		}
	}

	return r
}

func (db *CertDB) LoadAccount(ctx context.Context, secret *api_v1.Secret, acmeurl string, updateAccounts bool, updateStatus bool) (err error) {
	account, err := accountlib.NewAccountFromSecret(secret, acmeurl)
	if err != nil {
		err = fmt.Errorf("failed to create an account from secret '%s/%s': %s", secret.Namespace, secret.Name, err)
		return
	}

	if updateAccounts {
		log.Debugf("Updating account '%s/%s' (%s)", secret.Namespace, secret.Name, account.Client.Account.URI)
		err = account.UpdateRemote(ctx)
		if err != nil {
			err = fmt.Errorf("failed to update account from acme server: %s", err)
			return err
		}
	}

	if updateStatus {
		log.Debugf("Loading certificates and authorizations for account '%s/%s' (%s)", secret.Namespace, secret.Name, account.Client.Account.URI)
		err = account.FetchAuthorizations()
		if err != nil {
			err = fmt.Errorf("failed to fetch authorizations: %s", err)
			return err
		}
		err = account.FetchCertificates()
		if err != nil {
			err = fmt.Errorf("failed to fetch certificates: %s", err)
			return err
		}
	}

	// we need to choose only one certificate per domains if there are more
	certificatesByDomain := make(map[string]*cert.Certificate)
	t := time.Now()
	for _, c := range account.Certificates {
		h := hashDomains(c.Domains()...)
		existingCert, found := certificatesByDomain[h]
		if found {
			certificatesByDomain[h] = cert.FresherCertificate(existingCert, c, t)
		} else {
			certificatesByDomain[h] = c
		}
	}

	for _, value := range certificatesByDomain {
		log.Debugf("Loading certificate for account %s; domains=%#v", account.Client.Account.URI, value.Domains())
		db.AddCertificate(account, value)
	}

	return
}

func (db *CertDB) Bootstrap(ctx context.Context, namespace string, acmeUrl string, updateAccounts bool, updateStatus bool) (err error) {
	secretList, err := db.kclient.Secrets(namespace).List(api_v1.ListOptions{
		LabelSelector: accountlib.LabelSelectorAcmeAccount,
	})
	if err != nil {
		return
	}

	for _, secret := range secretList.Items {
		err = db.LoadAccount(ctx, &secret, acmeUrl, updateAccounts, updateStatus)
		if err != nil {
			log.Warn(err)
			continue
		}
	}

	return
}
