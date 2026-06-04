package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Nib fetches remote bytes for three user-triggered features: open-by-URL,
// add-image-by-URL, and (validation only) the finalize timestamp authority. Every
// such URL is user-supplied, so it goes through safeFetch / requireHTTPScheme:
// only http and https are ever dialed, including across redirects. These paths sit
// behind the CSRF + loopback-origin gate, so the threat is narrow; the scheme guard
// keeps a stray file:// (or a future custom protocol) from reaching the transport.

// httpFetchClient follows redirects but rejects any hop that isn't http(s), and
// caps the chain. Setting CheckRedirect overrides net/http's default 10-hop cap,
// so the cap is reimposed here. Per-call deadlines come from the request context.
var httpFetchClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		return requireHTTPScheme(req.URL)
	},
}

// requireHTTPScheme rejects any URL that isn't http or https.
func requireHTTPScheme(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	return nil
}

// safeFetch GETs rawURL with a size cap and timeout, refusing any non-http(s)
// scheme on the initial request and on every redirect.
func safeFetch(rawURL string, maxBytes int64, timeout time.Duration) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if err := requireHTTPScheme(u); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpFetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errStatus(resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

// errStatus is a non-200 HTTP status from a fetch.
type errStatus int

func (e errStatus) Error() string { return "unexpected status " + http.StatusText(int(e)) }
