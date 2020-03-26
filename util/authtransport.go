package util

import (
	"crypto/tls"
	"github.com/docker/distribution/registry/client/auth"
	"github.com/docker/distribution/registry/client/auth/challenge"
	"github.com/docker/distribution/registry/client/transport"
	"net/http"
	"net/url"
	"time"
)

type credentials struct {
	username      string
	password      string
	refreshTokens map[string]string
}
var _ auth.CredentialStore = credentials{}

func (tcs credentials) Basic(*url.URL) (string, string) {
	return tcs.username, tcs.password
}

func (tcs credentials) RefreshToken(u *url.URL, service string) string {
	return tcs.refreshTokens[service]
}

func (tcs credentials) SetRefreshToken(u *url.URL, service string, token string) {
	if tcs.refreshTokens == nil {
		tcs.refreshTokens = make(map[string]string)
	}
		tcs.refreshTokens[service] = token
}

func NewAuthTransport(registryURL string, repo *string, username string, password string,insecure bool) (http.RoundTripper, error) {
	baseTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}

	// ping the repo to detect authentication
	client := &http.Client{
		Transport:     baseTransport,
		Timeout:       1 * time.Minute,
	}
	resp, err := client.Get(registryURL+"/v2/")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	manager := challenge.NewSimpleManager()
	if err := manager.AddResponse(resp); err != nil {
		return nil, err
	}

	credentialStore := credentials{username:username,password: password}
	var scope auth.Scope
	if repo==nil {
		scope= auth.RegistryScope{
			Name:    "catalog",
			Actions: []string{"*"},
		}
	} else {
		scope = auth.RepositoryScope{
			Repository:    *repo,
			Actions: []string{"*"},
		}
	}
	tho := auth.TokenHandlerOptions{
		Credentials: credentialStore,
		Scopes: []auth.Scope{scope},
	}
	return transport.NewTransport(baseTransport, auth.NewAuthorizer(manager, auth.NewTokenHandlerWithOptions(tho), auth.NewBasicHandler(credentialStore))),nil
}
