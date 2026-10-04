package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"nib/internal/vault"
)

// TestTimestampRoutesReadTheRequestsPinnedVault — /pending 727, /pending 500's pattern. Both
// timestamp routes sit behind requireUnlocked, which pins the vault it authorised to the request;
// re-reading `s.vault` instead answered "this machine has timestamping switched off" (403) to a
// request authorised against a vault that had it ON, whenever a lock or import landed mid-request.
func TestTimestampRoutesReadTheRequestsPinnedVault(t *testing.T) {
	ts, srv := startServerWith(t)
	authedClient(t, ts)
	v := srv.unlockedVault()
	if v == nil {
		t.Fatal("setup: no vault to pin")
	}
	setAdvanced(t, v, &vault.Advanced{Timestamp: true})
	srv.mu.Lock()
	srv.vault = nil // a lock landing after requireUnlocked took its snapshot
	srv.mu.Unlock()

	for name, h := range map[string]http.HandlerFunc{"timestamp": srv.handleTimestamp, "timestamp/verify": srv.handleTimestampVerify} {
		req := httptest.NewRequest(http.MethodPost, "/api/"+name, strings.NewReader(""))
		req = req.WithContext(context.WithValue(req.Context(), vaultCtxKey{}, v))
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code == http.StatusForbidden {
			t.Errorf("/api/%s refused as switched off (%s) a request authorised against a vault with "+
				"timestamping ON — it read the server's current vault, not the request's", name, strings.TrimSpace(rec.Body.String()))
		}
	}
}

// TestACustomExplorerWithAQueryIsRefused — /pending 727. `ots.NewEsplora` appends
// `/block-height/N` to the base, so a `?` or `#` in a user's explorer URL turned that fixed path into
// a query or fragment and the lookup asked the wrong thing. Nothing is fetched: the explorer here
// counts every request it gets.
func TestACustomExplorerWithAQueryIsRefused(t *testing.T) {
	s, v := unlockedServer(t)
	setAdvanced(t, v, &vault.Advanced{Timestamp: true})
	var asked int64
	explorer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&asked, 1)
		http.NotFound(w, r)
	}))
	t.Cleanup(explorer.Close)

	pdf := []byte("%PDF-1.4\nbytes to hash\n")
	for _, base := range []string{explorer.URL + "/api?key=1", explorer.URL + "/api#frag", explorer.URL + "/api?"} {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		for name, data := range map[string][]byte{"pdf": pdf, "ots": attestedProof(sha256.Sum256(pdf), 800000)} {
			fw, err := mw.CreateFormFile(name, name)
			if err != nil {
				t.Fatal(err)
			}
			fw.Write(data)
		}
		mw.WriteField("explorer", base)
		mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/timestamp/verify", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req = req.WithContext(context.WithValue(req.Context(), vaultCtxKey{}, v))
		rec := httptest.NewRecorder()
		s.handleTimestampVerify(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("explorer %q answered %d, want 400 — the appended block path becomes part of its query or fragment", base, rec.Code)
		}
	}
	if n := atomic.LoadInt64(&asked); n != 0 {
		t.Errorf("the explorer was asked %d time(s) for a base URL that cannot address its API", n)
	}
}
