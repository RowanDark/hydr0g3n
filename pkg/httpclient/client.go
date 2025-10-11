package httpclient

import (
	"context"
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
func New(timeout time.Duration) *Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// Head issues an HTTP HEAD request using the shared client.
func (c *Client) Head(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
