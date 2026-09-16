package server

import (
	"bytes"
	"crypto/sha256"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"nib/internal/vault"
)

// attestedProof hand-builds a minimal .ots whose only sequence is a Bitcoin attestation of the
// digest itself, so verifying it goes straight to the block explorers.
func attestedProof(digest [32]byte, height uint64) []byte {
	magic := []byte{0x00, 0x4f, 0x70, 0x65, 0x6e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x73, 0x00, 0x00, 0x50, 0x72, 0x6f, 0x6f, 0x66, 0x00, 0xbf, 0x89, 0xe2, 0xe8, 0x84, 0xe8, 0x92, 0x94}
	bitcoin := []byte{0x05, 0x88, 0x96, 0x0d, 0x73, 0xd7, 0x19, 0x01}
	leb := func(n uint64) []byte {
		var b []byte
		for {
			c := byte(n & 0x7f)
			n >>= 7
			if n != 0 {
				b = append(b, c|0x80)
				continue
			}
			return append(b, c)
		}
	}
	out := append([]byte{}, magic...)
	out = append(out, 0x01, 0x08) // version, sha256 file-hash op
	out = append(out, digest[:]...)
	out = append(out, 0x00) // attestation
	out = append(out, bitcoin...)
	h := leb(height)
	out = append(out, leb(uint64(len(h)))...)
	return append(out, h...)
}

// TestTimestampVerifyEndsWithinItsBudget — /pending 502.
//
// handleTimestampVerify passed VerifyProof the request context, which has no deadline: each GET
// inside `ots` had its own timeout and nothing bounded the sequence of them. The handler now gives
// the whole verification a budget. An explorer that never answers is the stimulus.
func TestTimestampVerifyEndsWithinItsBudget(t *testing.T) {
	s, v := unlockedServer(t)
	if err := v.UpdateSettings(func(s *vault.Settings) {
		s.Advanced = &vault.Advanced{Timestamp: true}
	}); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var asked int64
	explorer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&asked, 1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); explorer.Close() })

	saved := timestampVerifyBudget
	timestampVerifyBudget = 300 * time.Millisecond
	t.Cleanup(func() { timestampVerifyBudget = saved })

	pdf := []byte("%PDF-1.4\nnot really a document, only bytes to hash\n")
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, data := range map[string][]byte{"pdf": pdf, "ots": attestedProof(sha256.Sum256(pdf), 800000)} {
		fw, err := mw.CreateFormFile(name, name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(data)
	}
	mw.WriteField("explorer", explorer.URL)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/timestamp/verify", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() { defer close(done); s.handleTimestampVerify(rec, req) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("the verification was still running 10 s into a %v budget — nothing bounds it but the per-GET timeouts", 300*time.Millisecond)
	}
	// STIMULUS: the handler really reached the explorer, so the early return is the budget and not a
	// refusal before any network work (a disabled feature, a malformed proof).
	if atomic.LoadInt64(&asked) == 0 {
		t.Fatalf("the explorer was never asked (HTTP %d: %s) — this test is not driving a slow verification", rec.Code, rec.Body.String())
	}
	if rec.Code == http.StatusOK {
		t.Errorf("a verification that ran out of time answered 200: %s", rec.Body.String())
	}
}
