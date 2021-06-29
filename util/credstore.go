package util

import (
	"net/url"
)

type CStore struct {
	user  string
	pass  string
	token string
}

func (cs *CStore) Basic(*url.URL) (string, string) {
	return cs.user, cs.pass
}

func (cs *CStore) SetBasic(user string, pass string) {
	cs.user = user
	cs.pass = pass
}

func (cs *CStore) RefreshToken(*url.URL, string) string {
	return cs.token
}

func (cs *CStore) SetRefreshToken(realm *url.URL, service, token string) {
	cs.token = token
}
