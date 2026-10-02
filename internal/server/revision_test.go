package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// revisionSigner is an identity that signs returned-document fixtures, with the fingerprint the route takes.
type revisionSigner struct {
	cert, key []byte
	fp        string
}

func newRevisionSigner(t *testing.T, cn string) revisionSigner {
	t.Helper()
	c, k, err := sign.GenerateIdentity(cn)
	if err != nil {
		t.Fatal(err)
	}
	fp, err := sign.Fingerprint(c)
	if err != nil {
		t.Fatal(err)
	}
	return revisionSigner{c, k, hex.EncodeToString(fp)}
}

func (r revisionSigner) sign(t *testing.T, doc []byte, reason string) []byte {
	t.Helper()
	out, err := sign.SignApproval(doc, r.cert, r.key, sign.Options{Name: reason, Reason: reason, When: time.Now()})
	if err != nil {
		t.Fatalf("sign %s: %v", reason, err)
	}
	return out
}

func appendRevision(t *testing.T, prev []byte, objs map[int]string) []byte {
	t.Helper()
	out, err := testpdf.AppendRevision(prev, objs)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var reRevisionByteRange = regexp.MustCompile(`/ByteRange\s*\[[^\]]*\]`)

// getRevision asks the route for fp's version of the document id holds, pinned as the client pins it (ADR-004).
func getRevision(t *testing.T, c *http.Client, base string, id docID, fp string) (int, []byte, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/api/document/revision?signer="+fp, nil)
	req.Header.Set("X-Nib-Doc", id.String())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, resp.Header
}

// TestTheRouteReturnsTheSignedVersionOrNamesWhyNot is P02's exit criterion at the route: a verified signer's version
// comes back byte-identical with its facts in X-Nib-Revision, and each of the five causes is a distinct 422.
func TestTheRouteReturnsTheSignedVersionOrNamesWhyNot(t *testing.T) {
	ts, s := startServerWith(t)
	c, _ := authedClient(t, ts)
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a, b := newRevisionSigner(t, "Alice"), newRevisionSigner(t, "Bob")
	sA := a.sign(t, base, "original")
	spoof := b.sign(t, sA, "stranger")
	vnum, _, err := testpdf.SignatureDictionary(sA)
	if err != nil {
		t.Fatal(err)
	}
	redefined := appendRevision(t, sA, map[int]string{vnum: "<< /Foo 2 >>"})
	var resaved bytes.Buffer
	if err := api.Optimize(bytes.NewReader(sA), &resaved, nil); err != nil {
		t.Fatal(err)
	}
	unreadable := append(append([]byte(nil), sA...), "\nstartxref\n99999999\n%%EOF\n"...)
	// I3: a negative-length copy inside B's signed version, which the latest revision then replaces.
	sA2 := a.sign(t, base, "original")
	_, vb2, err := testpdf.SignatureDictionary(sA2)
	if err != nil {
		t.Fatal(err)
	}
	hostile := appendRevision(t, sA2, map[int]string{900: reRevisionByteRange.ReplaceAllString(vb2, "/ByteRange [0 -5 10 10]")})
	hB := b.sign(t, hostile, "bob-over-hostile")
	prefixFails := appendRevision(t, hB, map[int]string{900: "<< /Foo 3 >>"})

	type facts struct {
		Obj             uint32 `json:"obj"`
		End             int64  `json:"end"`
		EarlierRevision bool   `json:"earlierRevision"`
		RedefinedObj    uint32 `json:"redefinedObj"`
		History         string `json:"history"`
	}
	for _, tc := range []struct {
		name  string
		doc   []byte
		fp    string
		want  []byte
		cause sign.RevisionCause
		check func(t *testing.T, f facts, refusal revisionRefusal)
	}{
		{name: "returned untouched", doc: sA, fp: a.fp, want: sA},
		{name: "a stranger co-signed to EOF", doc: spoof, fp: a.fp, want: sA},
		{name: "the signer's dictionary replaced later", doc: redefined, fp: a.fp, want: sA,
			check: func(t *testing.T, f facts, _ revisionRefusal) {
				if !f.EarlierRevision || f.RedefinedObj != uint32(vnum) {
					t.Errorf("facts %+v: want earlierRevision and redefinedObj %d", f, vnum)
				}
			}},
		{name: "upper-case fingerprint", doc: sA, fp: strings.ToUpper(a.fp), want: sA},
		{name: "unsigned", doc: base, fp: a.fp, cause: sign.RevisionNoSignature},
		{name: "signed by someone else", doc: sA, fp: b.fp, cause: sign.RevisionNotYours},
		{name: "re-saved wholesale", doc: resaved.Bytes(), fp: a.fp, cause: sign.RevisionResaved,
			check: func(t *testing.T, _ facts, r revisionRefusal) {
				if !r.Attributed {
					t.Error("a re-save keeps the SignerInfo; the 422 should say the name checks against the key")
				}
			}},
		{name: "an erroring record inside the signed version", doc: prefixFails, fp: b.fp, cause: sign.RevisionPrefixFailed},
		{name: "the whole file unreadable, asked as a stranger", doc: unreadable, fp: b.fp, cause: sign.RevisionCouldNotCheck},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := addDocument(s, tc.doc)
			code, body, hdr := getRevision(t, c, ts.URL, id, tc.fp)
			var f facts
			var refusal revisionRefusal
			if tc.want != nil {
				if code != http.StatusOK {
					t.Fatalf("status %d (%s); want 200 and the signed version", code, body)
				}
				if !bytes.Equal(body, tc.want) {
					t.Fatalf("returned %d bytes, want the %d-byte signed version byte for byte", len(body), len(tc.want))
				}
				if ct := hdr.Get("Content-Type"); ct != "application/pdf" || hdr.Get("Cache-Control") != "no-store" {
					t.Errorf("Content-Type %q Cache-Control %q", ct, hdr.Get("Cache-Control"))
				}
				if err := json.Unmarshal([]byte(hdr.Get("X-Nib-Revision")), &f); err != nil {
					t.Fatalf("X-Nib-Revision %q: %v", hdr.Get("X-Nib-Revision"), err)
				}
				if f.End != int64(len(tc.want)) || f.Obj == 0 || f.History != "none" {
					t.Errorf("facts %+v: want end %d, the holder's object, history none", f, len(tc.want))
				}
			} else {
				if code != http.StatusUnprocessableEntity {
					t.Fatalf("status %d (%s); want 422 %q", code, body, tc.cause)
				}
				if err := json.Unmarshal(body, &refusal); err != nil {
					t.Fatal(err)
				}
				if refusal.Cause != tc.cause {
					t.Fatalf("cause %q, want %q", refusal.Cause, tc.cause)
				}
			}
			if tc.check != nil {
				tc.check(t, f, refusal)
			}
		})
	}
}

// TestTheRouteRefusesWhatIsNotAFingerprint — a malformed signer would match nothing and read "not your signature".
func TestTheRouteRefusesWhatIsNotAFingerprint(t *testing.T) {
	ts, s := startServerWith(t)
	c, _ := authedClient(t, ts)
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	id := addDocument(s, base)
	for _, fp := range []string{"", "abc", strings.Repeat("g", 64), strings.Repeat("a", 63), strings.Repeat("a", 65)} {
		if code, body, _ := getRevision(t, c, ts.URL, id, fp); code != http.StatusBadRequest {
			t.Errorf("signer %q: status %d (%s), want 400", fp, code, body)
		}
	}
	if code, _, _ := getRevision(t, c, ts.URL, id, strings.Repeat("a", 64)); code != http.StatusUnprocessableEntity {
		t.Fatalf("stimulus: a well-formed fingerprint over an unsigned document reads %d, not the 422 the refusals are measured against", code)
	}
	gone := docID{Epoch: id.Epoch, Seq: id.Seq + 1000}
	if code, _, _ := getRevision(t, c, ts.URL, gone, strings.Repeat("a", 64)); code != http.StatusConflict {
		t.Errorf("a document no longer open: %d, want 409 (ADR-004)", code)
	}
}

// TestTheRouteReportsRecordedHistory — W9: the working copy's recorded history rides in the facts.
func TestTheRouteReportsRecordedHistory(t *testing.T) {
	ts, s := startServerWith(t)
	c, _ := authedClient(t, ts)
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newRevisionSigner(t, "Alice")
	sA := a.sign(t, base, "original")
	for _, tc := range []struct {
		name    string
		mutate  func(d *document)
		history string
	}{
		{"none", func(*document) {}, "none"},
		{"undo", func(d *document) { d.undo = [][]byte{base} }, "undo"},
		{"evicted", func(d *document) { d.historyEvicted = true }, "evicted"},
	} {
		id := addDocument(s, sA)
		s.mu.Lock()
		for _, d := range s.docs {
			if d.id == id {
				tc.mutate(d)
			}
		}
		s.mu.Unlock()
		_, _, hdr := getRevision(t, c, ts.URL, id, a.fp)
		var f struct {
			History string `json:"history"`
		}
		_ = json.Unmarshal([]byte(hdr.Get("X-Nib-Revision")), &f)
		if f.History != tc.history {
			t.Errorf("%s: history %q, want %q", tc.name, f.History, tc.history)
		}
	}
}

// TestConcurrentRequestsShareOneWalk — I5: requests for one document and signer while a walk is running share it.
func TestConcurrentRequestsShareOneWalk(t *testing.T) {
	ts, s := startServerWith(t)
	c, _ := authedClient(t, ts)
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	var walks atomic.Int32
	s.revisionFor = func(pdf []byte, fp string) sign.SignedRevision {
		walks.Add(1)
		time.Sleep(300 * time.Millisecond) // long enough that every request below arrives while it runs
		return sign.SignedRevision{Prefix: pdf, End: int64(len(pdf)), Obj: 1}
	}
	id := addDocument(s, base)
	fp := strings.Repeat("a", 64)
	var wg sync.WaitGroup
	const n = 8
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, _, _ := getRevision(t, c, ts.URL, id, fp); code != http.StatusOK {
				t.Errorf("status %d", code)
			}
		}()
	}
	wg.Wait()
	if got := walks.Load(); got >= n {
		t.Errorf("%d concurrent requests ran %d walks; they should share one", n, got)
	}
}

// TestAnEditMidWalkIsNeverAnsweredWithTheStaleBytes — the single-flight key carries the bytes' identity, so a request
// made after an edit gets a walk over the new bytes, not the answer the walk over the old ones is about to give.
func TestAnEditMidWalkIsNeverAnsweredWithTheStaleBytes(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	// Two edits: one that grows the file, and one that keeps its LENGTH (a form value (a) → (b)), which only the
	// bytes' address in the key tells apart — the length alone would share the stale walk (the P02 phase-close review).
	sameLength := bytes.Clone(base)
	sameLength[len(sameLength)-2] ^= 1
	for name, edited := range map[string][]byte{
		"an edit that grows the file":   append(append([]byte(nil), base...), "\n% an edit\n"...),
		"an edit that keeps its length": sameLength,
	} {
		t.Run(name, func(t *testing.T) {
			ts, s := startServerWith(t)
			c, _ := authedClient(t, ts)
			release := make(chan struct{})
			entered := make(chan struct{}, 2)
			s.revisionFor = func(pdf []byte, fp string) sign.SignedRevision {
				entered <- struct{}{}
				if bytes.Equal(pdf, base) {
					<-release // the walk over the old bytes is held open
				}
				return sign.SignedRevision{Prefix: pdf, End: int64(len(pdf)), Obj: 1}
			}
			id := addDocument(s, base)
			fp := strings.Repeat("a", 64)
			done := make(chan []byte, 1)
			go func() { _, body, _ := getRevision(t, c, ts.URL, id, fp); done <- body }()
			<-entered
			s.mu.Lock()
			for _, d := range s.docs {
				if d.id == id {
					d.data = edited
				}
			}
			s.mu.Unlock()
			second := make(chan []byte, 1)
			go func() { _, body, _ := getRevision(t, c, ts.URL, id, fp); second <- body }()
			select {
			case body := <-second:
				if !bytes.Equal(body, edited) {
					t.Errorf("a request after the edit got %d bytes, want the %d edited bytes", len(body), len(edited))
				}
			case <-time.After(3 * time.Second):
				t.Error("a request after the edit is waiting on the walk over the OLD bytes — it shares that walk's key")
			}
			close(release)
			<-done
		})
	}
}
