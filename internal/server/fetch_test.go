package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSafeFetchRejectsNonHTTPScheme(t *testing.T) {
	for _, u := range []string{
		"file:///etc/passwd",
		"ftp://example.com/x",
		"gopher://example.com/",
		"//example.com/x", // scheme-relative parses with empty scheme
		"not-a-url",
	} {
		if _, err := safeFetch(u, 1<<20, 5*time.Second); err == nil {
			t.Errorf("safeFetch(%q) = nil error, want rejection", u)
		}
	}
}

func TestSafeFetchReadsBodyWithCap(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("A"), 100))
	}))
	defer ts.Close()

	body, err := safeFetch(ts.URL, 10, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 10 {
		t.Fatalf("body length = %d, want it capped at 10", len(body))
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

	if _, err := safeFetch(src.URL, 1<<20, 5*time.Second); err == nil {
		t.Fatal("safeFetch followed a redirect to file://; want rejection")
	}
}
