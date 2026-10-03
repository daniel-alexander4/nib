package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/ceremony"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// getCeremonyCopy asks the route for this machine's copy of the ceremony document id belongs to, pinned (ADR-004).
func getCeremonyCopy(t *testing.T, c *http.Client, base string, id docID) (int, []byte, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/api/document/ceremony-copy", nil)
	req.Header.Set("X-Nib-Doc", id.String())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, resp.Header
}

func extractRecord(t *testing.T, pdf []byte) ceremony.Record {
	t.Helper()
	r, err := ceremony.Extract(pdf)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestTheCeremonyCopyIsTheStoredOneOrNamesWhyNot is P03.S03's route: the copy this machine stored for the ceremony the
// document's own record names — live or ended — byte for byte with its facts, or a 422 naming each of the six causes.
func TestTheCeremonyCopyIsTheStoredOneOrNamesWhyNot(t *testing.T) {
	ts, s := startServerWith(t)
	c, _ := authedClient(t, ts)
	root := defaultOutputDir()

	// The ceremony this machine took part in, stored, and the document as it came back: a stranger signed after it.
	docA, _, _ := convenedDocument(t)
	recA := extractRecord(t, docA)
	if _, err := ceremony.WriteMirror(root, recA, docA); err != nil {
		t.Fatal(err)
	}
	returned := newRevisionSigner(t, "Stranger").sign(t, docA, "after")

	// An ENDED ceremony (ADR-012 moved its folder), asked about long after.
	docE, _, _ := convenedDocument(t)
	recE := extractRecord(t, docE)
	if _, err := ceremony.WriteMirror(root, recE, docE); err != nil {
		t.Fatal(err)
	}
	if err := ceremony.CloseOutMirror(root, recE.ID); err != nil {
		t.Fatal(err)
	}

	// A planted id: the stored ceremony under docP's id is ANOTHER proceeding — its own convener re-signed it under
	// that id, so it verifies; only the roster commitment tells them apart.
	docP, _, _ := convenedDocument(t)
	recP := extractRecord(t, docP)
	docQ, certQ, keyQ := convenedDocument(t)
	recQ := extractRecord(t, docQ)
	recQ.ID = recP.ID
	if err := recQ.Sign(certQ, keyQ); err != nil {
		t.Fatal(err)
	}
	if _, err := ceremony.WriteMirror(root, recQ, docQ); err != nil {
		t.Fatal(err)
	}

	// A record that does not verify: a convened record with its intent altered after signing.
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	bad := extractRecord(t, docA)
	bad.Intent = "We agree to other terms"
	badDoc, err := ceremony.Embed(base, bad)
	if err != nil {
		t.Fatal(err)
	}

	// A damaged copy: stored, then truncated on disk under its own checksum.
	docD, _, _ := convenedDocument(t)
	recD := extractRecord(t, docD)
	dirD, err := ceremony.WriteMirror(root, recD, docD)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirD, "document.pdf"), docD[:len(docD)/2], 0o600); err != nil {
		t.Fatal(err)
	}

	// A record with no document beside it: a folder torn before its document landed.
	docT, _, _ := convenedDocument(t)
	if _, err := ceremony.WriteMirror(root, extractRecord(t, docT), nil); err != nil {
		t.Fatal(err)
	}

	// Convened elsewhere: nothing stored here.
	docN, _, _ := convenedDocument(t)

	// Stored but unreadable: the record on disk is not JSON.
	docU, _, _ := convenedDocument(t)
	recU := extractRecord(t, docU)
	dirU, err := ceremony.WriteMirror(root, recU, docU)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirU, "record.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		doc   []byte
		want  []byte
		facts ceremonyCopyFacts
		cause ceremonyCopyCause
	}{
		{name: "came back co-signed by a stranger", doc: returned, want: docA,
			facts: ceremonyCopyFacts{Extends: true}},
		{name: "the ceremony has ended", doc: docE, want: docE,
			facts: ceremonyCopyFacts{Ended: true, Same: true}},
		{name: "a planted id names another proceeding", doc: docP, cause: copyDifferentProceeding},
		{name: "no ceremony record", doc: base, cause: copyNoRecord},
		{name: "a record that does not verify", doc: badDoc, cause: copyRecordInvalid},
		{name: "convened elsewhere", doc: docN, cause: copyNotOnThisMachine},
		{name: "the stored copy is damaged", doc: docD, cause: copyDamaged},
		{name: "the stored record has no document", doc: docT, cause: copyDamaged},
		{name: "the stored record is unreadable", doc: docU, cause: copyUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := addDocument(s, tc.doc)
			code, body, hdr := getCeremonyCopy(t, c, ts.URL, id)
			if tc.want == nil {
				if code != http.StatusUnprocessableEntity {
					t.Fatalf("status %d (%s); want 422 %q", code, body, tc.cause)
				}
				var got ceremonyCopyRefusal
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				if got.Cause != tc.cause {
					t.Fatalf("cause %q, want %q", got.Cause, tc.cause)
				}
				return
			}
			if code != http.StatusOK {
				t.Fatalf("status %d (%s); want 200 and the stored copy", code, body)
			}
			if !bytes.Equal(body, tc.want) {
				t.Fatalf("returned %d bytes, want the %d-byte stored copy byte for byte", len(body), len(tc.want))
			}
			if hdr.Get("Content-Type") != "application/pdf" || hdr.Get("Cache-Control") != "no-store" {
				t.Errorf("Content-Type %q Cache-Control %q", hdr.Get("Content-Type"), hdr.Get("Cache-Control"))
			}
			var f ceremonyCopyFacts
			if err := json.Unmarshal([]byte(hdr.Get("X-Nib-Ceremony-Copy")), &f); err != nil {
				t.Fatalf("X-Nib-Ceremony-Copy %q: %v", hdr.Get("X-Nib-Ceremony-Copy"), err)
			}
			if f != tc.facts {
				t.Errorf("facts %+v, want %+v", f, tc.facts)
			}
		})
	}
}

// TestTheCeremonyCopySaysWhenItIsSignedAndWhenTheFileDoesNotBeginWithIt — the two facts that decide how the surface
// words the copy: a signed mirror (this machine's own hop) and an open file that is NOT an extension of it.
func TestTheCeremonyCopySaysWhenItIsSignedAndWhenTheFileDoesNotBeginWithIt(t *testing.T) {
	ts, s := startServerWith(t)
	c, _ := authedClient(t, ts)
	root := defaultOutputDir()
	doc, _, _ := convenedDocument(t)
	rec := extractRecord(t, doc)
	mine := newRevisionSigner(t, "Me").sign(t, doc, "mine")
	if _, err := ceremony.WriteMirror(root, rec, mine); err != nil {
		t.Fatal(err)
	}
	if !sign.HasSignatureBlob(mine) {
		t.Fatal("setup: the stored copy carries no signature, so \"signed\" below is asked of the wrong bytes")
	}
	for _, tc := range []struct {
		name string
		open []byte
		want ceremonyCopyFacts
	}{
		{name: "the file is the stored copy", open: mine,
			want: ceremonyCopyFacts{Signed: true, Same: true}},
		{name: "the file is the convened original, before this machine's signature", open: doc,
			want: ceremonyCopyFacts{Signed: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body, hdr := getCeremonyCopy(t, c, ts.URL, addDocument(s, tc.open))
			if code != http.StatusOK || !bytes.Equal(body, mine) {
				t.Fatalf("status %d, %d bytes; want 200 and the %d-byte signed copy", code, len(body), len(mine))
			}
			var f ceremonyCopyFacts
			if err := json.Unmarshal([]byte(hdr.Get("X-Nib-Ceremony-Copy")), &f); err != nil {
				t.Fatal(err)
			}
			if f != tc.want {
				t.Errorf("facts %+v, want %+v", f, tc.want)
			}
		})
	}
}
