package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"nib/internal/addrscope"
	"syscall"
	"time"
)

// Nib fetches remote bytes for three user-triggered features: open-by-URL,
// add-image-by-URL, and (validation only) the finalize timestamp authority. Every
// such URL is user-supplied, so it goes through safeFetch / requireHTTPScheme:
// only http and https are ever dialed, including across redirects. These paths sit
// behind the CSRF + loopback-origin gate, so the threat is narrow; the scheme guard
// keeps a stray file:// (or a future custom protocol) from reaching the transport.
//
// TRIPWIRE: private/LAN/loopback targets are intentionally NOT blocked. Only the
// local user can reach these endpoints (loopback bind + CSRF + origin, and
// NIB_ADDR is loopback-enforced so nib is never network-exposed), and self-host
// users legitimately fetch from their own network. Add connect-time IP filtering
// (a net.Dialer.Control hook on httpFetchClient, applied across redirects) ONLY if
// that model changes — a multi-user mode or a non-loopback bind. Don't add it as a
// generic "SSRF fix": with no remote caller it guards nothing and breaks homelab use.

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

// blockPrivateIP is a net.Dialer.Control hook that refuses to connect to any address
// `addrscope.Routable` refuses — loopback, private, link-local (incl. cloud-metadata
// 169.254.169.254), unspecified, multicast, shared address space, and the reserved
// prefixes. It runs at connect time on the actually-resolved IP, so it also defeats DNS
// rebinding. Unlike the user-typed-URL paths (see the TRIPWIRE above, where LAN targets
// are deliberately allowed), this guards URLs taken from untrusted FILE content — there is
// no user choosing the host, so a hostile file is exactly the "remote caller" that
// TRIPWIRE assumed didn't exist.
//
// **The predicate moved to `internal/addrscope` at P04's phase close**, where the address
// table already lived. Written out here it had drifted to the stdlib's vocabulary and let
// nine classes through, 100.64/10 among them — see `RefuseNonPublicDialAddress`.
func blockPrivateIP(network, address string, _ syscall.RawConn) error {
	return addrscope.RefuseNonPublicDialAddress(address)
}

// untrustedFetchClient dials only public http(s) hosts, for URLs that come from
// untrusted file content rather than the local user (e.g. the calendar URL
// embedded in an .ots proof). The Control hook blocks non-public IPs on every
// dial, including redirect hops; CheckRedirect re-imposes the scheme guard and
// hop cap.
var untrustedFetchClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   blockPrivateIP,
		}).DialContext,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		return requireHTTPScheme(req.URL)
	},
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
	// REFUSE an over-size body, never truncate it.
	//
	// `io.LimitReader(body, maxBytes)` returned exactly maxBytes of a larger document and
	// no error, so a 250 MiB URL opened as a 200 MiB corrupt PDF — `LooksLikePDF` passes
	// on the header — with `canSave` set. `openHandedOff` already refuses on size for the
	// same class of input; two size policies for one class, one of them silently lossy.
	//
	// Read one byte past the cap to tell "exactly at the cap" from "over it".
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxBytes {
		return nil, fmt.Errorf("that document is larger than %d bytes", maxBytes)
	}
	return b, nil
}

// errStatus is a non-200 HTTP status from a fetch.
type errStatus int

func (e errStatus) Error() string { return "unexpected status " + http.StatusText(int(e)) }
