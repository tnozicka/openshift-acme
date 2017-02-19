package acme

import (
	"context"
	"sync"

	"github.com/go-playground/log"
	"github.com/tnozicka/openshift-acme/pkg/acme"
	"github.com/tnozicka/openshift-acme/pkg/cert"
)

type DbCertEntry struct {
	mutex         sync.Mutex
	accountEntry  *DbAccountEntry
	ctx           context.Context
	ctxCancel     context.CancelFunc
	inProgress    bool
	certificate   *cert.Certificate
	objects       map[string]AcmeObject
	failedCounter int
}

func NewDbCertEntry(ctx context.Context, accountEntry *DbAccountEntry) *DbCertEntry {
	ctx, cancel := context.WithCancel(ctx)
	d := &DbCertEntry{
		objects:      make(map[string]AcmeObject),
		accountEntry: accountEntry,
		ctx:          ctx,
		ctxCancel:    cancel,
	}

	return d
}

func (e *DbCertEntry) updateCertificate() {
	// update certificate on all objects
	for _, o := range e.objects {
		go func(o AcmeObject) {
			log.Debugf("Updating certificate for %s", o.GetUID())
			if err := o.UpdateCertificate(e.certificate); err != nil {
				log.Error(err)
				// FIXME: write error into object's status/annotation
				return
			}
		}(o)
	}
}

func (e *DbCertEntry) UpdateCertificate(certificate *cert.Certificate) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.certificate = certificate
	e.failedCounter = 0
	e.updateCertificate()
}

func (e *DbCertEntry) obtainCertificate() {
	log.Info("Obtaining certificate start")
	defer func() {
		e.inProgress = false
	}()
	e.inProgress = true

	if len(e.objects) < 1 {
		// at this point we can only log an error; this code should be never reached
		log.Error("obtainCertificate: there are no objects")
		return
	}
	var o AcmeObject
	for _, o = range e.objects {
		break // take 1st object from a map
	}

	log.Info("Obtaining certificate")
	certificate, err := e.accountEntry.account.Client.ObtainCertificate(e.ctx, o.GetDomains(), o.GetExposers(), false)
	switch err.(type) {
	case acme.DomainsAuthorizationError:
		log.Error(err)
		e.failedCounter = e.failedCounter + 1
		// FIXME: write error into object's status/annotation
		return
	default:
		log.Error(err)
		e.failedCounter = e.failedCounter + 1
		// FIXME: write error into object's status/annotation
		return
	case nil:
	}

	log.Debugf("updating cert %p", certificate)
	e.certificate = certificate
	e.failedCounter = 0
	e.updateCertificate()
	go e.accountEntry.AddCertificates(certificate)
}

func (e *DbCertEntry) ObtainCertificate() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.obtainCertificate()
}

func (e *DbCertEntry) startObtainingCertificate() {
	if e.inProgress {
		return
	}

	go e.ObtainCertificate()
}

func (e *DbCertEntry) StartObtainingCertificate() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.startObtainingCertificate()
}

func (e *DbCertEntry) cancelObtainingCertificate() {
	if !e.inProgress {
		return
	}

	e.ctxCancel()
}

func (e *DbCertEntry) AddObject(o AcmeObject) {
	log.Debug("Adding object")
	e.mutex.Lock()
	defer e.mutex.Unlock()

	key := o.GetUID()
	// we want to create the object or update it if it was caused by MODIFIED event
	e.objects[key] = o

	if e.certificate == nil {
		log.Debug("AddObject starting new certificate request")
		e.startObtainingCertificate()
	} else {
		currentCert := o.GetCertificate()
		// check if object isn't already using this certificate
		if !currentCert.Equal(e.certificate) {
			log.Debug("AddObject using existing certificate")
			o.UpdateCertificate(e.certificate)
		}
	}
}

func (e *DbCertEntry) RemoveObject(o AcmeObject) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	key := o.GetUID()
	delete(e.objects, key)

	if len(e.objects) < 1 {
		e.cancelObtainingCertificate()
	}
}
