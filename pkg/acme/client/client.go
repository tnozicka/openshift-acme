package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/golang/glog"
	"golang.org/x/crypto/acme"

	"github.com/tnozicka/openshift-acme/pkg/acme/challengeexposers"
)

var once sync.Once

func acceptTerms(tosURL string) bool {
	once.Do(func() {
		glog.Infof("By continuing running this program you agree to the CA's Terms of Service (%s). If you do not agree exit the program immediately!", tosURL)
	})

	return true
}

type Client struct {
	Client  *acme.Client
	Account *acme.Account
}

func (c *Client) CreateAccount(ctx context.Context, a *acme.Account) error {
	var err error
	c.Client.Key, err = rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	c.Account, err = c.Client.Register(ctx, a, acceptTerms)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) DeactivateAccount(ctx context.Context, a *acme.Account) error {
	return c.Client.RevokeAuthorization(ctx, a.URI)
}

func getStatisfiableCombinations(authorization *acme.Authorization, exposers map[string]challengeexposers.Interface) [][]int {
	var combinations [][]int
	for _, combination := range authorization.Combinations {
		satisfiable := true
		for _, challengeId := range combination {
			if challengeId >= len(authorization.Challenges) {
				glog.Warning("ACME authorization has contains challengeId %d out of range; %#v", challengeId, authorization)
				satisfiable = false
				continue
			}

			if _, ok := exposers[authorization.Challenges[challengeId].Type]; !ok {
				satisfiable = false
				continue
			}
		}

		if satisfiable {
			combinations = append(combinations, combination)
		}
	}

	return combinations
}

func (c *Client) AcceptAuthorization(
	ctx context.Context,
	authorization *acme.Authorization,
	domain string,
	exposers map[string]challengeexposers.Interface,
	labels map[string]string,
) (*acme.Authorization, error) {
	glog.V(4).Infof("Found %d possible combinations for authorization", len(authorization.Combinations))

	combinations := getStatisfiableCombinations(authorization, exposers)
	if len(combinations) == 0 {
		return nil, fmt.Errorf("none of %d combination could be satified", len(authorization.Combinations))
	}

	glog.V(4).Infof("Found %d valid combinations for authorization", len(combinations))

	// TODO: sort combinations by preference

	// TODO: consider using the remaining combinations if this one fails
	combination := combinations[0]
	for _, challengeId := range combination {
		challenge := authorization.Challenges[challengeId]

		exposer, ok := exposers[challenge.Type]
		if !ok {
			return nil, errors.New("internal error: unavailable exposer")
		}

		err := exposer.Expose(c.Client, domain, challenge.Token)
		if err != nil {
			return nil, err
		}

		challenge, err = c.Client.Accept(ctx, challenge)
		if err != nil {
			return nil, err
		}
	}

	authorization, err := c.Client.GetAuthorization(ctx, authorization.URI)
	if err != nil {
		return nil, err
	}

	return authorization, nil
}

func GetAuthorizationErrors(authorization *acme.Authorization) string {
	var res []string
	for _, challenge := range authorization.Challenges {
		if challenge.Status != "invalid" {
			continue
		}

		res = append(res, fmt.Sprintf("%q challenge is %q: %v", challenge.Type, challenge.Status, challenge.Error))
	}

	return fmt.Sprintf("[%s]", strings.Join(res, ", "))
}
