package auth

import (
	"errors"
	"io"
	"net/http"
	"sync/atomic"
)

const oidcMaxResponse = 1 << 20

type oidcBoundedTransport struct {
	next     http.RoundTripper
	failures *atomic.Uint64
}

func (t oidcBoundedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := validateOIDCURL(r.URL.String(), false); err != nil {
		return nil, err
	}
	res, err := t.next.RoundTrip(r)
	if err != nil {
		t.failures.Add(1)
		return nil, err
	}
	if res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests {
		t.failures.Add(1)
	}
	if res.ContentLength > oidcMaxResponse {
		t.failures.Add(1)
		_ = res.Body.Close()
		return nil, errors.New("OIDC response is too large")
	}
	res.Body = &oidcBody{ReadCloser: res.Body, remaining: oidcMaxResponse, failures: t.failures}
	return res, nil
}

type oidcBody struct {
	io.ReadCloser
	remaining int64
	failures  *atomic.Uint64
}

func (b *oidcBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		var probe [1]byte
		n, err := b.ReadCloser.Read(probe[:])
		if n > 0 {
			b.failures.Add(1)
			return 0, errors.New("OIDC response is too large")
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, err
}
