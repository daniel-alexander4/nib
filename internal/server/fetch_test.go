package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSafeFetchRejectsNonHTTPScheme asserts the RULE'S OWN TOKEN, not merely an error.
//
// Its first version checked `err == nil` and was green with `requireHTTPScheme` deleted:
// `http.Transport.RoundTrip` refuses any non-http(s) scheme itself, so every case still
// errored — from the stdlib, not from the guard. The token is the only thing that
// distinguishes our refusal from the transport's, and the difference matters because the
// transport's refusal happens after a DNS lookup and ours does not.
func TestSafeFetchRejectsNonHTTPScheme(t *testing.T) {
	for _, u := range []string{
		"file:///etc/passwd",
		"ftp://example.com/x",
		"gopher://example.com/",
		"//example.com/x", // scheme-relative parses with empty scheme
		"not-a-url",
	} {
		_, err := safeFetch(u, 1<<20, 5*time.Second)
		if err == nil {
			t.Errorf("safeFetch(%q) = nil error, want rejection", u)
			continue
		}
		if !strings.Contains(err.Error(), "unsupported URL scheme") {
			t.Errorf("safeFetch(%q) refused with %q — that is the stdlib transport's "+
				"refusal, not requireHTTPScheme's, and this test passes with the guard "+
				"deleted unless it names the token", u, err)
		}
	}
}

// TestSafeFetchRefusesAnOverSizeBodyRatherThanTruncating.
//
// This test used to CODIFY the defect: it asserted `len(body) == 10` for a 100-byte
// response, i.e. that a fetch silently returns the first 10 bytes of a larger document. On
// `/api/open-url` that made a 250 MiB URL open as a 200 MiB corrupt PDF with canSave set —
// `LooksLikePDF` only reads the header. `openHandedOff` refuses on size for the same class
// of input, so the tree carried two policies for one question and one of them was lossy.
func TestSafeFetchRefusesAnOverSizeBodyRatherThanTruncating(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("A"), 100))
	}))
	defer ts.Close()

	if _, err := safeFetch(ts.URL, 10, 5*time.Second); err == nil {
		t.Error("safeFetch returned a truncated document and no error — the caller cannot " +
			"tell a 10-byte document from the first 10 bytes of a 100-byte one")
	}

	// The boundary, both sides. A body EXACTLY at the cap is not over it, and a fetch that
	// refuses everything is an outage rather than a fix.
	body, err := safeFetch(ts.URL, 100, 5*time.Second)
	if err != nil {
		t.Fatalf("a body exactly at the cap was refused: %v", err)
	}
	if len(body) != 100 {
		t.Fatalf("body length = %d, want the whole 100", len(body))
	}
}

func TestSafeFetchFollowsHTTPRedirect(t *testing.T) {
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("redirected-body"))
	}))
	defer dest.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusFound)
	}))
	defer src.Close()

	body, err := safeFetch(src.URL, 1<<20, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "redirected-body" {
		t.Fatalf("body = %q, want the redirect target's body", body)
	}
}

func TestSafeFetchRefusesRedirectToNonHTTP(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer src.Close()

	_, err := safeFetch(src.URL, 1<<20, 5*time.Second)
	if err == nil {
		t.Fatal("safeFetch followed a redirect to file://; want rejection")
	}
	// Same reason as the scheme test above: with CheckRedirect nil, net/http follows the
	// redirect and the transport refuses file:// on its own, so `err != nil` passes with
	// the hop guard deleted.
	if !strings.Contains(err.Error(), "unsupported URL scheme") {
		t.Errorf("the redirect was refused with %q — that is the transport's refusal, not "+
			"CheckRedirect's", err)
	}
}
