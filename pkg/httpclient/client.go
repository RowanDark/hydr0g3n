package httpclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Client provides an HTTP client that can be shared between workers.
type Client struct {
	client *http.Client
}

// New creates a Client configured with the provided timeout. It reuses a
// single http.Transport to allow connection pooling across concurrent
// requests.
func New(timeout time.Duration, followRedirects bool) *Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	const maxRedirects = 5

	if followRedirects {
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		}
	} else {
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return &Client{client: httpClient}
}

// Head issues an HTTP HEAD request using the shared client.
func (c *Client) Head(ctx context.Context, url string) (*http.Response, error) {
	return c.Request(ctx, http.MethodHead, url)
}

// Request issues an HTTP request using the provided method.
func (c *Client) Request(ctx context.Context, method, url string) (*http.Response, error) {
	if method == "" {
		method = http.MethodHead
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
