package agent

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func outboundProxyURL(authorization *service.AuthorizeSubsiteResponse) string {
	if authorization == nil || authorization.Credential.Proxy == nil {
		return ""
	}
	return strings.TrimSpace(authorization.Credential.Proxy.URL)
}

func outboundHTTPClientForAuthorization(authorization *service.AuthorizeSubsiteResponse) (*http.Client, error) {
	proxyURL := outboundProxyURL(authorization)
	if proxyURL == "" {
		return http.DefaultClient, nil
	}
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:            proxyURL,
		AllowPrivateHosts:   true,
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
	})
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("subsite relay proxy client is nil")
	}
	return client, nil
}
