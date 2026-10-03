package server

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// finalizeWith posts pdf to /api/finalize with params (a JSON object, or "" for none) and returns the response.
func finalizeWith(t *testing.T, c *http.Client, csrf, base string, pdf []byte, params string) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	if params != "" {
		mw.WriteField("params", params)
	}
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, base+"/api/finalize", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

// keptFiles lists what is in ~/nib/signed: the kept copies, and anything else (a staging temp file included).
func keptFiles(t *testing.T) (kept, other []string) {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(defaultOutputDir(), keptSubdir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if keptGrammar.MatchString(e.Name()) {
			kept = append(kept, e.Name())
		} else {
			other = append(other, e.Name())
		}
	}
	return kept, other
}

// TestFinalizeKeepsExactlyTheCopyAskedFor is P04.S01's acceptance at the route (D13, D14): ticked writes exactly one
// kept copy, byte-identical to what was returned, 0600, named in X-Nib-Kept; unticked writes nothing, and so does a
// request that does not send the field at all (an older client's, or the CLI's, shape). The ticked row is the control that makes the
// other two mean something — they run in the same HOME.
func TestFinalizeKeepsExactlyTheCopyAskedFor(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}

	resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, `{"reason":"r","keep":true,"name":"Lease 2026 (final).pdf"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ticked: status %d: %s", resp.StatusCode, body)
	}
	kept, other := keptFiles(t)
	if len(kept) != 1 || len(other) != 0 {
		t.Fatalf("ticked: ~/nib/signed holds kept %v and other %v; want exactly one kept copy", kept, other)
	}
	if got := resp.Header.Get("X-Nib-Kept"); got != kept[0] {
		t.Errorf("X-Nib-Kept %q does not name the kept copy %q", got, kept[0])
	}
	if !strings.HasPrefix(kept[0], "kept_lease-2026-final_") {
		t.Errorf("kept name %q does not carry the document's slug", kept[0])
	}
	path := filepath.Join(defaultOutputDir(), keptSubdir, kept[0])
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, body) {
		t.Fatalf("the kept copy (%d bytes) is not the signed output returned (%d bytes), byte for byte", len(onDisk), len(body))
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("kept copy mode %v, want 0600 — signed PII readable by other local users", fi.Mode().Perm())
	}
	// The join the dispute surface and the checklist will make: the kept copy ends where its signature's coverage does.
	recs, err := sign.Revisions(onDisk)
	if err != nil || len(recs) == 0 {
		t.Fatalf("the kept copy's signature records: %v (%d)", err, len(recs))
	}
	if end := recs[len(recs)-1].CoverageEnd; end != int64(len(onDisk)) {
		t.Errorf("the kept copy's last coverage end is %d, its length %d — the size join P04.S02 builds on fails", end, len(onDisk))
	}

	for _, tc := range []struct{ name, params string }{
		{"unticked", `{"reason":"r","keep":false,"name":"x.pdf"}`},
		{"no keep field (a caller that never sends it)", `{"reason":"r"}`},
		{"no params at all", ""},
	} {
		resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, tc.params)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d: %s", tc.name, resp.StatusCode, body)
		}
		if got := resp.Header.Get("X-Nib-Kept"); got != "" {
			t.Errorf("%s: X-Nib-Kept %q on a request that kept nothing", tc.name, got)
		}
		if k, _ := keptFiles(t); len(k) != 1 {
			t.Errorf("%s: ~/nib/signed now holds %d kept copies, want still the one from the ticked run", tc.name, len(k))
		}
	}
}

// TestAFailedKeepRefusesTheSigning is D14 at the route: with ~/nib/signed planted as a regular FILE (a failure that
// holds for root too, unlike a chmod), a ticked Finalize answers non-2xx with the copy-not-kept cause, returns no PDF
// bytes, and leaves nothing new in ~/nib. **What it reaches is the folder's creation failing** — `saveKept` stops at
// `MkdirAll`, before any staging file could exist. A failure mid-WRITE relies on `atomicfile.WriteDurable`'s own
// deferred removal of its temp file (`atomicfile.go:316`), which no test here drives (declared).
func TestAFailedKeepRefusesTheSigning(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(defaultOutputDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(defaultOutputDir(), keptSubdir)
	if err := os.WriteFile(blocker, []byte("not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Stimulus: the same request UNTICKED signs — the failure below is the keep, not the signing.
	if resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, `{"reason":"r"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: an unticked finalize failed too (%d: %s), so the refusal below would not be the keep's", resp.StatusCode, body)
	}
	resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, `{"reason":"r","keep":true,"name":"x.pdf"}`)
	if resp.StatusCode/100 == 2 {
		t.Fatalf("a ticked finalize whose copy could not be written answered %d — the user holds a signature they believe is kept", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		t.Errorf("422: the modal reads that as a wrong certificate passphrase")
	}
	var ref keptRefusal
	if err := json.Unmarshal(body, &ref); err != nil || ref.Cause != "copy-not-kept" {
		t.Errorf("body %q: want a keptRefusal with cause copy-not-kept (%v)", body, err)
	}
	if ref.Full {
		t.Errorf("a file where the folder should be was reported as a full disk — the user is sent to free space they do not lack")
	}
	if bytes.Contains(body, []byte("%PDF")) {
		t.Errorf("the refusal carried the signed document's bytes")
	}
	if fi, err := os.Stat(blocker); err != nil || fi.IsDir() {
		t.Fatalf("the planted file changed under the test: %v", err)
	}
	if ents, _ := os.ReadDir(defaultOutputDir()); len(ents) != 1 {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Errorf("~/nib holds %v after the failed keep, want only the planted file", names)
	}
}

// TestTheKeptGrammarIsTheOnlyDoor — keptPathFor accepts only names keptName makes, rebuilds the path inside
// ~/nib/signed, and refuses every name another writer in that folder makes, and every escape.
func TestTheKeptGrammarIsTheOnlyDoor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Date(2026, 10, 2, 14, 15, 2, 0, time.UTC)
	if got := keptName("Contract v1.2", nil, now); !strings.HasPrefix(got, "kept_contract-v12_") {
		t.Errorf("keptName(\"Contract v1.2\") = %q: only a .pdf extension may be dropped", got)
	}
	for _, doc := range []string{"Lease.pdf", "", "___", strings.Repeat("ab_", 40) + ".pdf", "../../etc/passwd", "über straße"} {
		name := keptName(doc, []byte("x"), now)
		path, err := keptPathFor(name)
		if err != nil {
			t.Errorf("keptPathFor refused keptName(%q) = %q: %v", doc, name, err)
			continue
		}
		if filepath.Dir(path) != filepath.Join(defaultOutputDir(), keptSubdir) || filepath.Base(path) != name {
			t.Errorf("keptName(%q) = %q resolved to %q, outside ~/nib/signed or renamed", doc, name, path)
		}
	}
	peer := receivedName("Alice_Smith", []byte{1, 2, 3, 4, 5}, []byte("doc"))
	for _, name := range []string{
		peer, // a signed arrival from a peer
		"lease-0123456789abcdef0123456789abcdef.pdf", // a delivered ceremony copy
		"kept_x_20261002-141502-0011aabb.pdf/../../a", "../kept_x_20261002-141502-0011aabb.pdf",
		"/abs/kept_x_20261002-141502-0011aabb.pdf", "kept__20261002-141502-0011aabb.pdf",
		"kept_x_20261002-141502-0011AABB.pdf", "kept_x_20261002-141502-0011aabb.PDF", "kept_x_20261002-141502-0011aabb.pdf ",
		"kept_a_b_20261002-141502-0011aabb.pdf",
	} {
		if _, err := keptPathFor(name); err == nil {
			t.Errorf("keptPathFor accepted %q", name)
		}
	}
}

// TestAnExternallySignedKeptCopyEndsAtItsCoverage — the size join P04.S02 builds on, for the imported-certificate
// signer too: Finalize with signAs "external" keeps a copy whose last signature covers it to its last byte.
func TestAnExternallySignedKeptCopyEndsAtItsCoverage(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	if resp := importP12(t, c, csrf, ts.URL, testP12(t, "pw"), "pw"); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: importing the external signer = %d", resp.StatusCode)
	}
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	resp, body := finalizeWith(t, c, csrf, ts.URL, pdf, `{"reason":"r","keep":true,"name":"x.pdf","signAs":"external","passphrase":"pw"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	kept, _ := keptFiles(t)
	if len(kept) != 1 {
		t.Fatalf("kept %v, want one", kept)
	}
	onDisk, _ := os.ReadFile(filepath.Join(defaultOutputDir(), keptSubdir, kept[0]))
	recs, err := sign.Revisions(onDisk)
	if err != nil || len(recs) == 0 {
		t.Fatalf("records: %v (%d)", err, len(recs))
	}
	if end := recs[len(recs)-1].CoverageEnd; end != int64(len(onDisk)) || !bytes.Equal(onDisk, body) {
		t.Errorf("external: last coverage end %d, length %d, identical to the response %v", end, len(onDisk), bytes.Equal(onDisk, body))
	}
}

// plantKept writes data under name in ~/nib/signed, as Finalize would have.
func plantKept(t *testing.T, name string, data []byte) string {
	t.Helper()
	dir := filepath.Join(defaultOutputDir(), keptSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestTheListShowsOnlyKeptCopies — the folder is shared, so the list is the grammar AND a regular file: a peer's signed
// arrival, a delivered ceremony copy, a staging file, a kept-named symlink and a kept-named directory are all left out.
func TestTheListShowsOnlyKeptCopies(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)
	older := keptName("Lease", []byte("a"), time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local))
	newer := keptName("Deed", []byte("bb"), time.Date(2026, 10, 2, 9, 0, 0, 0, time.Local))
	plantKept(t, older, []byte("a"))
	plantKept(t, newer, []byte("bb"))
	plantKept(t, receivedName("Alice", []byte{1, 2, 3, 4}, []byte("x")), []byte("x"))
	plantKept(t, "lease-0123456789abcdef0123456789abcdef.pdf", []byte("x"))
	plantKept(t, ".nib-123.tmp", []byte("x"))
	dir := filepath.Join(defaultOutputDir(), keptSubdir)
	if err := os.Symlink(filepath.Join(dir, older), filepath.Join(dir, "kept_link_20261001-090000-00000000.pdf")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "kept_dir_20261001-090000-00000000.pdf"), 0o755); err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(ts.URL + "/api/kept")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out keptListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Kept) != 2 || out.Kept[0].Name != newer || out.Kept[1].Name != older {
		t.Fatalf("listed %+v; want exactly the two kept copies, newest first", out.Kept)
	}
	if out.Kept[0].Document != "deed" || out.Kept[0].KeptAt != "2026-10-02 09:00:00" || out.Kept[0].Size != 2 || out.TotalBytes != 3 {
		t.Errorf("entry %+v total %d: want document deed, kept at 2026-10-02 09:00:00, size 2, total 3", out.Kept[0], out.TotalBytes)
	}
}

// TestTheRemovalReachesOnlyAKeptCopy — the repo's first route that deletes a user's file takes a bare name, refuses
// anything the grammar does not accept, refuses a kept-named symlink, removes a real kept copy, and answers a second
// removal of it as "already gone" rather than a failure.
func TestTheRemovalReachesOnlyAKeptCopy(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	remove := func(name string) (int, keptRemoveResponse) {
		b, _ := json.Marshal(keptRemoveRequest{Name: name})
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/kept/remove", "application/json", bytes.NewReader(b))
		defer resp.Body.Close()
		var out keptRemoveResponse
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	kept := keptName("Lease", []byte("a"), time.Now())
	keptPath := plantKept(t, kept, []byte("a"))
	others := []string{
		plantKept(t, receivedName("Alice", []byte{1, 2, 3, 4}, []byte("x")), []byte("x")),
		plantKept(t, "lease-0123456789abcdef0123456789abcdef.pdf", []byte("x")),
	}
	for _, name := range []string{filepath.Base(others[0]), filepath.Base(others[1]), "../" + kept, keptPath, "", "kept_x_1-2-3.pdf"} {
		if code, _ := remove(name); code != http.StatusBadRequest {
			t.Errorf("removing %q answered %d, want 400", name, code)
		}
	}
	target := plantKept(t, "victim.pdf", []byte("v"))
	link := keptName("Link", []byte("l"), time.Now().Add(time.Hour))
	if err := os.Symlink(target, filepath.Join(filepath.Dir(keptPath), link)); err != nil {
		t.Fatal(err)
	}
	if code, out := remove(link); code != http.StatusBadRequest || out.Removed {
		t.Errorf("a kept-named symlink answered %d (%+v), want 400 — a refusal, not a server fault", code, out)
	}
	for _, p := range append(others, target, keptPath) {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s is gone after the refusals: %v", p, err)
		}
	}
	if code, out := remove(kept); code != http.StatusOK || !out.Removed {
		t.Fatalf("removing the kept copy: %d %+v, want 200 removed", code, out)
	}
	if _, err := os.Stat(keptPath); !os.IsNotExist(err) {
		t.Fatalf("the kept copy is still there: %v", err)
	}
	if code, out := remove(kept); code != http.StatusOK || out.Removed {
		t.Errorf("a second removal answered %d %+v, want 200 and removed=false (already gone)", code, out)
	}
}

// TestKeptCopyForMatchesByBytesNotByName — the match door: a kept copy is found when the document IS it or BEGINS with
// it at one of the given coverage ends; a file carrying the right size AND the right name digest but other bytes is
// never a match, and nothing matches at an end the document's signatures do not have.
func TestKeptCopyForMatchesByBytesNotByName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	k := []byte("%PDF-1.7 the signed lease %%EOF\n")
	name := keptName("Lease", k, time.Now())
	plantKept(t, name, k)
	end := int64(len(k))
	grown := append(append([]byte{}, k...), "% a later signature\n"...)
	for _, tc := range []struct {
		what   string
		data   []byte
		ends   []int64
		placed bool
		want   bool
	}{
		{"the kept copy itself", k, []int64{end}, true, true},
		{"the kept copy with bytes appended", grown, []int64{end, int64(len(grown))}, true, true},
		{"the right bytes at an end the signatures do not have", grown, []int64{int64(len(grown))}, true, false},
		{"no ends at all", k, nil, true, false},
		// The verdict could not place the signatures (the phase-close review): matched by size, tail and bytes.
		{"unplaced, the kept copy with bytes appended", grown, nil, false, true},
		{"unplaced, a different document of the same size", []byte("%PDF-1.7 the signed lease %%EOF\r"), nil, false, false},
	} {
		got, b, err := keptCopyFor(tc.data, tc.ends, tc.placed)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		if (b != nil) != tc.want || (tc.want && (got.Name != name || !bytes.Equal(b, k))) {
			t.Errorf("%s: matched %q (%d bytes), want match=%v", tc.what, got.Name, len(b), tc.want)
		}
	}
	// An altered copy: same size, same name (so the same digest), other bytes — Save As over a kept file does this.
	altered := append([]byte{}, k...)
	altered[10] ^= 1
	if err := os.WriteFile(filepath.Join(defaultOutputDir(), keptSubdir, name), altered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, b, _ := keptCopyFor(k, []int64{end}, true); b != nil {
		t.Errorf("an altered kept file matched by its name's digest — the digest was treated as evidence")
	}
}

// TestTheOpenDocumentsKeptCopyIsFoundByTheServer — GET /api/document/kept-copy end to end: a document signed with the
// tick and returned with bytes appended finds its kept copy (extends); the copy itself is found (same); a document no
// copy was kept for answers none-kept; an unsigned one no-signature.
func TestTheOpenDocumentsKeptCopyIsFoundByTheServer(t *testing.T) {
	ts, s := startServerWith(t)
	c, csrf := authedClient(t, ts)
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	_, keptBody := finalizeWith(t, c, csrf, ts.URL, base, `{"reason":"r","keep":true,"name":"Lease"}`)
	_, otherBody := finalizeWith(t, c, csrf, ts.URL, base, `{"reason":"r"}`)
	if !bytes.HasPrefix(keptBody, []byte("%PDF")) || !bytes.HasPrefix(otherBody, []byte("%PDF")) {
		t.Fatal("setup: the finalizes did not return documents")
	}
	open := func(data []byte) docID {
		id := addDocument(s, data)
		s.mu.Lock()
		for _, d := range s.docs {
			if d.id == id {
				d.sig = sign.Verify(data) // what opening computes
			}
		}
		s.mu.Unlock()
		return id
	}
	ask := func(method string, id docID) (int, []byte, http.Header) {
		req, _ := http.NewRequest(method, ts.URL+"/api/document/kept-copy", nil)
		req.Header.Set("X-Nib-Doc", id.String())
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, resp.Header
	}
	for _, tc := range []struct {
		what  string
		data  []byte
		facts keptCopyFacts
		cause string
	}{
		{what: "came back with bytes appended", data: append(append([]byte{}, keptBody...), "\n% appended\n"...), facts: keptCopyFacts{Extends: true}},
		{what: "the kept copy itself", data: keptBody, facts: keptCopyFacts{Same: true}},
		{what: "signed without the tick", data: otherBody, cause: "none-kept"},
		{what: "unsigned", data: base, cause: "no-signature"},
	} {
		id := open(tc.data)
		code, body, hdr := ask(http.MethodGet, id)
		// HEAD is the Simple Sign checklist's probe (P04.S03): the same verdict, the cause in a header, and no bytes.
		hcode, hbody, hhdr := ask(http.MethodHead, id)
		// That no body travels is net/http's own HEAD rule (the client never reads one), so it is not asserted here.
		_ = hbody
		if hcode != code || hhdr.Get("X-Nib-Kept-Copy-Cause") != tc.cause {
			t.Errorf("%s: HEAD answered %d, cause %q; want %d, %q", tc.what, hcode, hhdr.Get("X-Nib-Kept-Copy-Cause"), code, tc.cause)
		}
		if tc.cause != "" {
			var ref keptCopyRefusal
			json.Unmarshal(body, &ref)
			if code != http.StatusUnprocessableEntity || ref.Cause != tc.cause {
				t.Errorf("%s: %d %+v, want 422 %s", tc.what, code, ref, tc.cause)
			}
			continue
		}
		if code != http.StatusOK || !bytes.Equal(body, keptBody) {
			t.Fatalf("%s: %d, %d bytes; want 200 and the kept copy", tc.what, code, len(body))
		}
		var f keptCopyFacts
		if err := json.Unmarshal([]byte(hdr.Get("X-Nib-Kept-Copy")), &f); err != nil {
			t.Fatal(err)
		}
		if f.Same != tc.facts.Same || f.Extends != tc.facts.Extends || f.Document != "lease" || f.KeptAt == "" {
			t.Errorf("%s: facts %+v", tc.what, f)
		}
	}
}

// TestOnlyTheKeptDoorNamesAKeptCopyOrRemovesAFile — the census behind ADR-073's one door (ADR-009), read from the AST so a
// comment cannot satisfy or trip it: no string literal outside kept.go spells `kept_`; `os.Remove`/`os.RemoveAll` is
// called only in kept.go (once, on the path `keptPathFor` returned, inside `removeKept`) and in the update download's two
// named temp-file cleanups. What it cannot see, declared: a removal through another package's helper or a syscall.
func TestOnlyTheKeptDoorNamesAKeptCopyOrRemovesAFile(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	removes := map[string]int{}
	sawKept, doorRemoves := false, false
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fd, _ := d.(*ast.FuncDecl)
			pathFromDoor := false
			ast.Inspect(d, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.BasicLit:
					if n.Kind == token.STRING && strings.Contains(n.Value, "kept_") {
						if f == "kept.go" {
							sawKept = true
						} else {
							t.Errorf("%s: %s spells a kept copy's name — only kept.go's grammar may", f, fset.Position(n.Pos()))
						}
					}
				case *ast.AssignStmt:
					if len(n.Lhs) > 0 && len(n.Rhs) == 1 {
						if call, ok := n.Rhs[0].(*ast.CallExpr); ok {
							if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "keptPathFor" {
								if lhs, ok := n.Lhs[0].(*ast.Ident); ok && lhs.Name == "path" {
									pathFromDoor = true
								}
							}
						}
					}
				case *ast.CallExpr:
					sel, ok := n.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					if x, ok := sel.X.(*ast.Ident); ok && x.Name == "os" && (sel.Sel.Name == "Remove" || sel.Sel.Name == "RemoveAll") {
						removes[f]++
						if f == "kept.go" && fd != nil && fd.Name.Name == "removeKept" && sel.Sel.Name == "Remove" && pathFromDoor {
							if arg, ok := n.Args[0].(*ast.Ident); ok && arg.Name == "path" {
								doorRemoves = true
							}
						}
					}
				}
				return true
			})
		}
	}
	if !sawKept {
		t.Fatal("stimulus: kept.go spells no `kept_` literal — this census reads the wrong files")
	}
	if !doorRemoves {
		t.Error("kept.go's one removal is not os.Remove(path) on keptPathFor's output inside removeKept")
	}
	for f, n := range removes {
		switch {
		case f == "kept.go" && n == 1, f == "updatedownload.go" && n == 1: // the short-read removal; the dead `.part` remove went with /pending 821
		default:
			t.Errorf("%s calls os.Remove/os.RemoveAll %d time(s): a file removal outside kept.go's door (or a new update-download cleanup) needs naming here", f, n)
		}
	}
}

// TestTheLongestKeptCopyWins — two copies were kept at two signings of one document; it comes back grown. The match is
// the later, longer copy: the shorter one would call your own later signing "appended" (the slice review).
func TestTheLongestKeptCopyWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := []byte("%PDF-1.7 first signing %%EOF\n")
	b := append(append([]byte{}, a...), "% second signing %%EOF\n"...)
	plantKept(t, keptName("Lease", a, time.Now()), a)
	plantKept(t, keptName("Lease", b, time.Now().Add(time.Second)), b)
	data := append(append([]byte{}, b...), "% came back with this\n"...)
	_, got, err := keptCopyFor(data, []int64{int64(len(a)), int64(len(b))}, true)
	if err != nil || !bytes.Equal(got, b) {
		t.Fatalf("matched %d bytes (%v), want the longer %d-byte copy", len(got), err, len(b))
	}
}

// TestAnUnreadableKeptFolderSaysSo — the route's third cause: ~/nib/signed is a file, so the folder cannot be listed.
func TestAnUnreadableKeptFolderSaysSo(t *testing.T) {
	ts, s := startServerWith(t)
	c, csrf := authedClient(t, ts)
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	_, signed := finalizeWith(t, c, csrf, ts.URL, base, `{"reason":"r"}`)
	if err := os.MkdirAll(defaultOutputDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultOutputDir(), keptSubdir), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	id := addDocument(s, signed)
	s.mu.Lock()
	for _, d := range s.docs {
		if d.id == id {
			d.sig = sign.Verify(signed)
		}
	}
	s.mu.Unlock()
	head, _ := http.NewRequest(http.MethodHead, ts.URL+"/api/document/kept-copy", nil)
	head.Header.Set("X-Nib-Doc", id.String())
	hresp, err := c.Do(head)
	if err != nil {
		t.Fatal(err)
	}
	hresp.Body.Close()
	if hresp.StatusCode != http.StatusUnprocessableEntity || hresp.Header.Get("X-Nib-Kept-Copy-Cause") != "unreadable" {
		t.Errorf("HEAD: %d, cause %q; want 422 unreadable", hresp.StatusCode, hresp.Header.Get("X-Nib-Kept-Copy-Cause"))
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/document/kept-copy", nil)
	req.Header.Set("X-Nib-Doc", id.String())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var ref keptCopyRefusal
	json.NewDecoder(resp.Body).Decode(&ref)
	if resp.StatusCode != http.StatusUnprocessableEntity || ref.Cause != "unreadable" || resp.Header.Get("X-Nib-Kept-Copy-Cause") != "unreadable" {
		t.Errorf("%d %+v (header %q), want 422 unreadable", resp.StatusCode, ref, resp.Header.Get("X-Nib-Kept-Copy-Cause"))
	}
}

// TestASignedDocumentWhoseSignaturesCouldNotBePlacedStillFindsItsKeptCopy — the phase-close review: a document that came
// back so changed the verdict could not place its signatures (no Signers, or no coverage ends) is Invalid, not unsigned;
// the route must still match its kept copy by size, tail and bytes, and never tell the user it carries no signature.
// Signedness is `sign.HasSignatureBlob`'s answer about the bytes, so the injected verdict below is only the "unplaced" half.
func TestASignedDocumentWhoseSignaturesCouldNotBePlacedStillFindsItsKeptCopy(t *testing.T) {
	ts, s := startServerWith(t)
	c, csrf := authedClient(t, ts)
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	_, keptBody := finalizeWith(t, c, csrf, ts.URL, base, `{"reason":"r","keep":true,"name":"Lease"}`)
	_, otherBody := finalizeWith(t, c, csrf, ts.URL, base, `{"reason":"r"}`)
	ask := func(data []byte, sig sign.Status) (int, []byte, string) {
		id := addDocument(s, data)
		s.mu.Lock()
		for _, d := range s.docs {
			if d.id == id {
				d.sig = sig
			}
		}
		s.mu.Unlock()
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/document/kept-copy", nil)
		req.Header.Set("X-Nib-Doc", id.String())
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, resp.Header.Get("X-Nib-Kept-Copy-Cause")
	}
	unplaced := sign.Status{State: sign.Invalid} // the library refused it: no Signers, so no coverage ends
	returned := append(append([]byte{}, keptBody...), "\n% came back changed\n"...)
	if code, b, cause := ask(returned, unplaced); code != http.StatusOK || !bytes.Equal(b, keptBody) {
		t.Errorf("an unplaced signed document: %d (%s), %d bytes; want 200 and its kept copy", code, cause, len(b))
	}
	if code, _, cause := ask(otherBody, unplaced); code != http.StatusUnprocessableEntity || cause != "none-kept" {
		t.Errorf("an unplaced document no copy was kept for: %d %q, want 422 none-kept — never no-signature", code, cause)
	}
	if code, _, cause := ask(base, sign.Status{State: sign.Unsigned}); cause != "no-signature" {
		t.Errorf("an unsigned document: %d %q, want 422 no-signature", code, cause)
	}
}
