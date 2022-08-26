package util

import (
	"crypto/tls"
	"net/http"
	"strings"
)

type BasicAuthTransport struct {
	transport http.RoundTripper
	url       string
	uname     string
	passwd    string
}

func (t *BasicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), t.url) {
		if t.uname != "" || t.passwd != "" {
			req.SetBasicAuth(t.uname, t.passwd)
		}
	}

	return t.transport.RoundTrip(req)
}

func NewBasicAuthTransport(URL string, uname string, passwd string, insecure bool) *BasicAuthTransport {
	baseTransport := &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}

	return &BasicAuthTransport{
		transport: baseTransport,
		url:       URL,
		uname:     uname,
		passwd:    passwd,
	}
}
