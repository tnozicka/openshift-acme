package acme

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-playground/log"
	"github.com/tnozicka/openshift-acme/pkg/cert"
	"golang.org/x/crypto/acme"
)

//func(tosURL string) bool {
//	c.Logger.Infof("By continuing running this program you aggree to the CA's Terms of Service (%s). If you do not agree exit the program immediately!", tosURL)
//	return true
//}

// Has to support concurrent calls
type ChallengeExposer interface {
	// Exposes challenge
	Expose(c *acme.Client, domain string, token string) error

	// Removes challenge
	Remove(c *acme.Client, domain string, token string) error
}

type Client struct {
	//Logger *log.Entry
	Client  *acme.Client
	Account *acme.Account
}

func (c *Client) CreateAccount(ctx context.Context, a *acme.Account, prompt func(tosURL string) bool) (err error) {
	c.Client.Key, err = rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return
	}

	c.Account, err = c.Client.Register(ctx, c.Account, prompt)
	if err != nil {
		return
	}

	return
}

func (c *Client) DeactivateAccount(ctx context.Context, a *acme.Account) error {
	return c.Client.RevokeAuthorization(ctx, a.URI)
}

// Consider if it needs to return acme.Authorization
func (c *Client) ValidateDomain(ctx context.Context, domain string, exposers map[string]ChallengeExposer) (authorization *acme.Authorization, err error) {
	authorization, err = c.Client.Authorize(ctx, domain)
	if err != nil {
		return
	}
	defer func() {
		if err != nil && authorization != nil {
			log.Debugf("Revoking authorization '%s' for domain '%s'", authorization, domain)
			// We can't use the default context because this call has to be done even if ctx is done (canceling)
			shortCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if e := c.Client.RevokeAuthorization(shortCtx, domain); e != nil {
				err = fmt.Errorf("%v (+Revoking failed authorization crashed because: %v)", err, e)
			}
		}
	}()

	if authorization.Status == acme.StatusValid {
		return
	}

	// TODO: prefer faster combinations like http-01 before dns-01 with cost based estimation
	log.Debugf("Authorization: %+v", authorization)

	found := false
	for _, combination := range authorization.Combinations {
		// We have to check if we support all the challenges in this combination otherwise it's pointless to start
		// validating some of them and then find out later than some can't be satisfied
		satisfiable := true
		for _, challengeId := range combination {
			if challengeId >= len(authorization.Challenges) {
				err = errors.New("ACME authorization has returned an invalid combination")
				return
			}

			if _, ok := exposers[authorization.Challenges[challengeId].Type]; !ok {
				satisfiable = false
			}
		}
		if !satisfiable {
			continue
		}

		combLength := len(combination)
		if combLength > 10 {
			combLength = 10
		}
		errCh := make(chan error, combLength)
		for _, challengeId := range combination {
			// challengeId is already verified to be in range from previous cycle

			go func(chal *acme.Challenge) {
				var err error
				defer func() { errCh <- err }()
				exposer, ok := exposers[chal.Type]
				if !ok {
					err = errors.New("internal error: unavailable exposer")
					return
				}

				err = exposer.Expose(c.Client, domain, chal.Token)
				if err != nil {
					return
				}

				chal, err = c.Client.Accept(ctx, chal)
				if err != nil {
					return
				}
			}(authorization.Challenges[challengeId])
		}

		err = nil
		for _, challengeId := range combination {
			e := <-errCh
			if e != nil {
				if err == nil {
					err = e
				} else {
					err = fmt.Errorf("%v: %v", err, e)
				}
			} else {
				chal := authorization.Challenges[challengeId]
				// we already checked above if we have exposer available
				defer exposers[chal.Type].Remove(c.Client, domain, chal.Token)
			}
		}
		if err != nil {
			log.Error(err)
			return
		}

		found = true
		break
	}
	if !found {
		err = errors.New("unable to satisfy all challenge combinations for ACME authorization")
		return
	}

	// TODO: consider implementing a timeout in case something went wrong
	authorization, err = c.Client.WaitAuthorization(ctx, authorization.URI)
	if err != nil {
		log.Errorf("Authorization failed: %+v", err)
		return
	}

	return
}

type FailedDomain struct {
	Domain string
	Err    error
}

func (d FailedDomain) String() string {
	return fmt.Sprintf("domain: %s, error: %s", d.Domain, d.Err)
}

type DomainsAuthorizationError struct {
	FailedDomains []FailedDomain
}

func (e DomainsAuthorizationError) Error() (res string) {
	return fmt.Sprint(e.FailedDomains)
}

func (c *Client) ObtainCertificate(ctx context.Context, domains []string, exposers map[string]ChallengeExposer, onlyForAllDomains bool) (certificate *cert.Certificate, err error) {
	defer log.Trace("acme.Client ObtainCertificate").End()
	var wg sync.WaitGroup
	results := make([]error, len(domains))
	for i, domain := range domains {
		wg.Add(1)
		go func(i int, domain string) {
			defer wg.Done()
			_, err := c.ValidateDomain(ctx, domain, exposers)
			results[i] = err
		}(i, domain)
	}
	wg.Wait()
	log.Info("finished validating domains")

	validatedDomains := []string{}
	var domainsError DomainsAuthorizationError
	for i, err := range results {
		if err == nil {
			validatedDomains = append(validatedDomains, domains[i])
		} else {
			domainsError.FailedDomains = append(domainsError.FailedDomains, FailedDomain{Domain: domains[i], Err: err})
		}
	}

	if len(validatedDomains) == 0 {
		return nil, domainsError
	}

	if onlyForAllDomains && len(domainsError.FailedDomains) != 0 {
		return nil, domainsError
	}

	domains = validatedDomains

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: domains[0],
		},
	}
	if len(domains) > 1 {
		template.DNSNames = domains
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, &template, privateKey)
	if err != nil {
		return
	}

	der, _, err := c.Client.CreateCert(ctx, csr, 0, true)
	if err != nil {
		return
	}

	certificate, err = cert.NewCertificateFromDER(der, privateKey)
	if err != nil {
		return
	}

	return
}
