package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"nib/internal/testpdf"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	"nib/internal/browserrender"
	"nib/internal/pdfops"
)

// The import door's censuses (PLAN-import-export P01.S02a, ADR-185): that every open route decides
// through ONE function, that the same file becomes the same document at every route, and that no route
// ever writes the file it was given (L1).

// ---- one decision, five doors --------------------------------------------------------------------

// forbiddenInDoors are the pdfops calls that classify or convert a file. A server file that makes one is a
// door deciding for itself — the defect this function replaced: the same Office file was converted at one
// door and refused at four.
var forbiddenInDoors = map[string]bool{
	"LooksLikeImage": true, "ImageToDocument": true, "SupportedDocExt": true, "SupportedOfficeExt": true,
	"SupportedMarkdownExt": true, "ConvertDocToPDF": true, "ConvertDocToPDFWith": true, "ConvertOfficeToPDF": true,
	"FormatForExt": true, "LooksLikePDF": true,
}

// doorUses reports, for one server source file, the forbidden pdfops calls it makes, whether it calls
// pdfops.Import itself, and which of the two server doors (importOrRefuse, openPath) it calls.
func doorUses(t *testing.T, name, src string) (forbidden []string, callsImport bool, viaServer []string) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if id, ok := fn.X.(*ast.Ident); ok && id.Name == "pdfops" {
				if forbiddenInDoors[fn.Sel.Name] {
					forbidden = append(forbidden, fn.Sel.Name)
				}
				if fn.Sel.Name == "Import" {
					callsImport = true
				}
			}
			if fn.Sel.Name == "importOrRefuse" {
				viaServer = append(viaServer, "importOrRefuse")
			}
		case *ast.Ident:
			if fn.Name == "openPath" || fn.Name == "readInstallablePDF" {
				viaServer = append(viaServer, fn.Name)
			}
		}
		return true
	})
	return
}

func TestTheDoorCensusSeesWhatItGuards(t *testing.T) {
	f, imp, _ := doorUses(t, "x.go", `package server; import "nib/internal/pdfops"
func door() { pdfops.ImageToDocument(nil, ""); pdfops.Import(nil, nil, 0, "", pdfops.ImportOptions{}) }`)
	if len(f) != 1 || f[0] != "ImageToDocument" || !imp {
		t.Fatalf("the scan missed a planted door: forbidden=%v import=%v", f, imp)
	}
	_, _, via := doorUses(t, "y.go", `package server; func d() { s.importOrRefuse(); openPath(); readInstallablePDF() }`)
	if len(via) != 3 {
		t.Fatalf("the scan missed the server doors: %v", via)
	}
}

// TestEveryOpenRouteDecidesThroughTheImportFunction — the per-caller census (S2). The five doors are named
// here with the function each reaches `pdfops.Import` through; a door added without one, or a server file
// that classifies a file itself, fails.
func TestEveryOpenRouteDecidesThroughTheImportFunction(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	// who may call pdfops.Import directly, and why
	importers := map[string]string{
		"importdoor.go": "importOrRefuse and openPath — the two server doors onto Import",
		"combine.go":    "Combine reads a LIST of files and keeps an unrecognised one as it came",
	}
	// the doors, and the server door each one calls
	doors := map[string][]string{
		"server.go":    {"importOrRefuse", "readInstallablePDF"}, // upload; path open
		"office.go":    {"importOrRefuse"},                       // Import… (with options)
		"sources.go":   {"importOrRefuse"},                       // fetch by URL
		"handoff.go":   {"readInstallablePDF"},                   // OS hand-off
		"diskstate.go": {"readInstallablePDF"},                   // reload (PDF only)
		"combine.go":   nil,                                      // calls Import itself
	}
	seenImporters, seenDoors := map[string]bool{}, map[string]map[string]bool{}
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		forb, imp, via := doorUses(t, n, string(b))
		if len(forb) > 0 {
			t.Errorf("%s calls pdfops.%v itself. A route that classifies or converts a file is a door deciding for itself: "+
				"route it through importOrRefuse / readInstallablePDF (ADR-009, D8).", n, forb)
		}
		if imp {
			if _, ok := importers[n]; !ok {
				t.Errorf("%s calls pdfops.Import directly and is not a declared importer", n)
			}
			seenImporters[n] = true
		}
		for _, v := range via {
			if seenDoors[n] == nil {
				seenDoors[n] = map[string]bool{}
			}
			seenDoors[n][v] = true
		}
	}
	for n := range importers {
		if !seenImporters[n] {
			t.Errorf("%s is declared as an importer and does not call pdfops.Import", n)
		}
	}
	for n, want := range doors {
		for _, w := range want {
			if !seenDoors[n][w] {
				t.Errorf("%s is a door and no longer calls %s", n, w)
			}
		}
	}
}

// ---- the same file, the same bytes, at every door ------------------------------------------------

func tinyWebP(t *testing.T) []byte {
	b, err := base64.StdEncoding.DecodeString("UklGRh4AAABXRUJQVlA4TBEAAAAvAUAAAAdQniKXp/+BiOh/AAA=")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func nativeFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 9), G: uint8(y * 15), B: 80, A: 255})
		}
	}
	var j, tf bytes.Buffer
	if err := jpeg.Encode(&j, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := tiff.Encode(&tf, img, nil); err != nil {
		t.Fatal(err)
	}
	var gf, bm, cz bytes.Buffer
	if err := gif.Encode(&gf, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := bmp.Encode(&bm, img); err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(&cz)
	for _, n := range []string{"p1.png", "p2.png"} {
		w, _ := zw.Create(n)
		w.Write(testPNG(t, 24, 16))
	}
	zw.Close()
	return map[string][]byte{
		"gif": gf.Bytes(), "bmp": bm.Bytes(), "cbz": cz.Bytes(),
		"png": testPNG(t, 24, 16), "jpeg": j.Bytes(), "tiff": tf.Bytes(), "webp": tinyWebP(t),
		"markdown": []byte("# A title\n\nSome prose, with **bold** words.\n"),
		"txt":      []byte("A plain line\n\tand an indented one\n"),
		"csv":      []byte("name,qty\nbolt,4\nnut,9\n"),
	}
}

var fixtureExt = map[string]string{"png": ".png", "jpeg": ".jpg", "tiff": ".tif", "webp": ".webp", "markdown": ".md", "gif": ".gif", "bmp": ".bmp", "cbz": ".cbz", "txt": ".txt", "csv": ".csv"}

// docData finds the open document by name and returns its bytes.
func docData(srv *Server, name string) ([]byte, bool) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, d := range srv.docs {
		if d.name == name {
			return d.data, true
		}
	}
	return nil, false
}

func TestEveryNativeFormatIsTheSameDocumentAtEveryDoor(t *testing.T) {
	fixtures := nativeFixtures(t)
	// A native row (no outside tool) must have a fixture: this is how a producer added by a later slice
	// fails the census on arrival (R9).
	for _, f := range pdfops.Formats() {
		if f.Tool == pdfops.ToolNone {
			if _, ok := fixtures[f.Key]; !ok {
				t.Fatalf("the format table has a native row %q and this census has no fixture for it — add one", f.Key)
			}
		}
	}
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	var body []byte
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer src.Close()

	keys := make([]string, 0, len(fixtures))
	for k := range fixtures {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	before := pdfops.ImportCounts()
	for _, key := range keys {
		body = fixtures[key]
		name := "pic" + fixtureExt[key]
		want := "pic.pdf"
		dir := t.TempDir()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}

		// path
		if code, out := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: p}); code != http.StatusOK {
			t.Fatalf("%s: path door = %d %s", key, code, out)
		}
		viaPath, ok := docData(srv, want)
		if !ok {
			t.Fatalf("%s: the path door installed no document named %s", key, want)
		}
		srv.setDoc(nil)
		// upload
		if out := uploadForBody(t, c, csrf, ts.URL, name, body); !strings.Contains(out, want) {
			t.Fatalf("%s: upload door answered %s", key, out)
		}
		viaUpload, _ := docData(srv, want)
		srv.setDoc(nil)
		// url
		if code, out := postForCode(t, c, csrf, ts.URL+"/api/open-url", map[string]string{"url": src.URL + "/" + name}); code != http.StatusOK {
			t.Fatalf("%s: url door = %d %s", key, code, out)
		}
		viaURL, _ := docData(srv, want)
		srv.setDoc(nil)
		// hand-off
		if _, _, err := srv.openHandedOff(context.Background(), p); err != nil {
			t.Fatalf("%s: hand-off door: %v", key, err)
		}
		viaHandoff, _ := docData(srv, want)
		srv.setDoc(nil)

		// "The same bytes" is the same CONTENT: a PDF carries its creation time and a fresh /ID, so two
		// conversions of one file differ in the file's bytes and never in what is on the page, which is
		// what `ContentDigest` (ADR-013) hashes.
		h := func(b []byte) string {
			d, err := pdfops.ContentDigest(b)
			if err != nil {
				t.Fatalf("%s: %v", key, err)
			}
			return d
		}
		if len(viaPath) == 0 || h(viaPath) != h(viaUpload) || h(viaPath) != h(viaURL) || h(viaPath) != h(viaHandoff) {
			t.Errorf("%s: the four doors made different documents: path %s upload %s url %s hand-off %s",
				key, h(viaPath)[:12], h(viaUpload)[:12], h(viaURL)[:12], h(viaHandoff)[:12])
		}
	}
	// P2: each door AND each format moved its counter.
	after := pdfops.ImportCounts()
	for _, key := range keys {
		for _, door := range []string{"path", "upload", "url", "handoff"} {
			k := fmt.Sprintf("import.ok{door=%s,format=%s}", door, key)
			if after[k] < before[k]+1 {
				t.Errorf("%s did not move (%d → %d)", k, before[k], after[k])
			}
		}
	}
}

// ---- L1: the source is never written -------------------------------------------------------------

type fileState struct {
	sum        [32]byte
	size       int64
	mtime      int64
	ino        uint64
	listing    string
	parentTime int64
}

func stateOf(t *testing.T, p string) fileState {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	st := fi.Sys().(*syscall.Stat_t)
	ents, _ := os.ReadDir(filepath.Dir(p))
	var names []string
	for _, e := range ents {
		i, _ := e.Info()
		names = append(names, fmt.Sprintf("%s:%d:%d", e.Name(), i.Size(), i.ModTime().UnixNano()))
	}
	sort.Strings(names)
	pi, _ := os.Stat(filepath.Dir(p))
	return fileState{sha256.Sum256(b), fi.Size(), fi.ModTime().UnixNano(), st.Ino, strings.Join(names, "|"), pi.ModTime().UnixNano()}
}

// TestNoRouteWritesTheFileItWasGiven — L1's census (R9, D35). After every path-taking route, on success AND
// on every kind of failure, the source's hash, size, mtime and inode and its parent directory's listing
// and mtime are what they were. The refusal fixtures are generated from the format table, so a row added
// later is covered on arrival; a native row with no success fixture fails the other census above.
func TestNoRouteWritesTheFileItWasGiven(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	png := testPNG(t, 8, 8)
	type fx struct {
		why  string
		name string
		data []byte
	}
	var cases []fx
	for k, b := range nativeFixtures(t) {
		cases = append(cases, fx{"success: " + k, "pic" + fixtureExt[k], b})
	}
	for _, f := range pdfops.Formats() {
		// every row under a name that contradicts its contents: always a refusal, never a tool run
		bytesOf := png
		if f.Family == pdfops.FamilyImage {
			bytesOf = []byte("plain text, not that image")
		}
		cases = append(cases, fx{"refusal: " + f.Key, "wrong." + f.Exts[0], bytesOf})
	}
	cases = append(cases,
		fx{"refusal: not a document", "mystery.xyz", []byte("hello")},
		fx{"refusal: a corrupt package", "broken.docx", []byte("PK\x03\x04broken")},
		fx{"refusal: binary named text", "bin.md", []byte("a\x00b")},
		fx{"failure: an undecodable image", "bad.png", []byte("\x89PNG\r\n\x1a\nnot an image")},
	)
	for _, tc := range cases {
		t.Run(tc.why, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, tc.name)
			if err := os.WriteFile(p, tc.data, 0o644); err != nil {
				t.Fatal(err)
			}
			before := stateOf(t, p)
			postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: p})
			srv.setDoc(nil)
			srv.openHandedOff(context.Background(), p)
			srv.setDoc(nil)
			if after := stateOf(t, p); after != before {
				t.Fatalf("a route wrote or touched the source:\nbefore %+v\nafter  %+v", before, after)
			}
		})
	}
}

// ---- after shutdown, nothing installs --------------------------------------------------------------

func TestNothingInstallsOnceShutdownBegan(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	// The flag Shutdown sets; the converter half of Shutdown is held by pdfops and cmd/nib's tests, and
	// cannot run here — it closes the converter registry for the life of the process.
	srv.shuttingDown.Store(true)
	defer srv.shuttingDown.Store(false)
	if out := uploadForBody(t, c, csrf, ts.URL, "p.png", testPNG(t, 8, 8)); !strings.Contains(out, "closing") {
		t.Fatalf("an upload after Shutdown answered %q, want the closing refusal", out)
	}
	srv.mu.Lock()
	n := len(srv.docs)
	srv.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d documents were installed after Shutdown", n)
	}
}

// ---- a large upload spills to disk, and leaves nothing ----------------------------------------------

func TestALargeUploadSpillsToTheClaimedDirectoryAndLeavesNothing(t *testing.T) {
	spill := t.TempDir()
	t.Setenv("TMPDIR", spill)
	big := bytes.Repeat([]byte("%"), multipartMemory+(2<<20))
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "big.pdf")
	fw.Write(big)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	cleanup, ok := parseMultipart(rec, req, maxPDFBytes)
	if !ok {
		t.Fatalf("parseMultipart refused: %s", rec.Body)
	}
	ents, _ := os.ReadDir(spill)
	if len(ents) == 0 {
		t.Fatal("a body larger than the in-memory limit left nothing on disk: it was held in memory whole")
	}
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), "multipart-") {
			t.Errorf("unexpected spill file %s", e.Name())
		}
	}
	cleanup()
	if ents, _ := os.ReadDir(spill); len(ents) != 0 {
		t.Fatalf("%d spill file(s) survive the request", len(ents))
	}
	_ = io.Discard
}

// ---- admission is by declared length, before the body is read ---------------------------------------

func TestAnUploadDeclaringMoreThanTheCapIsRefusedBeforeItsBodyIsRead(t *testing.T) {
	srv := New(nil, nil, t.TempDir(), "test")
	read := false
	body := io.NopCloser(readerFunc(func(p []byte) (int, error) { read = true; return 0, io.EOF }))
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.ContentLength = pdfops.MaxImportBytes + 8<<20
	rec := httptest.NewRecorder()
	srv.handleUpload(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", rec.Code)
	}
	if read {
		t.Fatal("the body was read before the declared length was refused")
	}
	if srv.queuedUpload.Load() != 0 {
		t.Fatal("a refused upload kept its declared bytes counted")
	}
}

type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestQueuedUploadBytesAreBoundedAndGivenBack(t *testing.T) {
	srv := New(nil, nil, t.TempDir(), "test")
	mk := func(n int64) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/upload", nil)
		r.ContentLength = n
		return r
	}
	var releases []func()
	for i := 0; i < 5; i++ { // 5 × 200 MiB = the 1 GiB bound exactly
		rel, ok := srv.admitUpload(httptest.NewRecorder(), mk(200<<20))
		if !ok {
			t.Fatalf("upload %d of 5 was refused under the bound", i+1)
		}
		releases = append(releases, rel)
	}
	rec := httptest.NewRecorder()
	if _, ok := srv.admitUpload(rec, mk(200<<20)); ok || rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("the sixth 200 MiB upload = admitted %v status %d, want refused 503", ok, rec.Code)
	}
	if got := srv.queuedUpload.Load(); got != 1000<<20 {
		t.Fatalf("a refused upload changed the count: %d MiB", got>>20)
	}
	releases[0]()
	if _, ok := srv.admitUpload(httptest.NewRecorder(), mk(200<<20)); !ok {
		t.Fatal("a finished upload did not give its bytes back")
	}
}

// ---- the window is told once, by the route that imported ------------------------------------------

// TestOnlyTheRouteThatImportedSaysSoAndDocsNever — Y1. `importedFrom` rides on the answer of the open,
// upload, URL and Import… routes; `/api/docs` (which a second window reads) never carries it, or every
// window that joined later would announce every import again.
func TestOnlyTheRouteThatImportedSaysSoAndDocsNever(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	_ = srv
	body := testPNG(t, 24, 16)
	out := uploadForBody(t, c, csrf, ts.URL, "photo.png", body)
	var got struct {
		ImportedFrom string `json:"importedFrom"`
		Name         string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.ImportedFrom != "photo.png" || got.Name != "photo.pdf" {
		t.Fatalf("upload answered %s (%v), want importedFrom photo.png and name photo.pdf", out, err)
	}
	// A PDF is not an import.
	pdf, _ := testpdf.Text("plain")
	if out := uploadForBody(t, c, csrf, ts.URL, "plain.pdf", pdf); strings.Contains(out, "importedFrom") {
		t.Fatalf("a PDF upload claims to have been imported: %s", out)
	}
	res, err := c.Get(ts.URL + "/api/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	docs, _ := io.ReadAll(res.Body)
	if strings.Contains(string(docs), "importedFrom") {
		t.Fatalf("/api/docs carries importedFrom: %s", docs)
	}
}

// A hand-off that converted tells the open windows what it converted; one that was refused tells them why,
// in the import function's own words (Y12), not "Nib may be full".
func TestAHandoffEventCarriesWhatWasImportedOrWhyItWasRefused(t *testing.T) {
	ts, srv := startServerWith(t)
	c, _ := authedClient(t, ts)
	srv.SetHandoffSecret("secret")
	srv.windows.n.Store(1) // a window is open, so the hand-off is announced to it
	dir := t.TempDir()
	good := filepath.Join(dir, "scan.png")
	os.WriteFile(good, testPNG(t, 8, 8), 0o600)
	bad := filepath.Join(dir, "broken.docx")
	os.WriteFile(bad, []byte("PK\x03\x04broken"), 0o600)
	if code, out := handoffTo(t, ts, c, "secret", good); code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("hand-off of an image = %d %+v", code, out)
	}
	if code, out := handoffTo(t, ts, c, "secret", bad); code != http.StatusOK || out.Result != "refused" {
		t.Fatalf("hand-off of a broken docx = %d %+v", code, out)
	}
	evs := srv.push.since(0)
	if len(evs) != 2 {
		t.Fatalf("%d hand-off events, want 2", len(evs))
	}
	if evs[0].result != "opened" || evs[0].importedFrom != "scan.png" {
		t.Errorf("first event %+v, want opened from scan.png", evs[0])
	}
	if evs[1].result != "refused" || !strings.Contains(evs[1].reason, "package") {
		t.Errorf("second event %+v, want a refusal that says why", evs[1])
	}
}

func TestThePostScriptRefusalsAreAnsweredAsRefusals(t *testing.T) {
	for _, err := range []error{pdfops.ErrGhostscriptTooOld, pdfops.ErrGhostscriptNotConfined, pdfops.ErrPostScriptUnreadable} {
		status, msg := importRefusal(fmt.Errorf("wrapped: %w", err))
		if status != http.StatusUnprocessableEntity || msg == "" {
			t.Errorf("%v: answered %d %q", err, status, msg)
		}
		if !isImportRefusal(err) {
			t.Errorf("%v is not an import refusal, so a path open would report it as a disk error", err)
		}
	}
}

func TestTheBrowserRefusalsAreAnsweredAsRefusals(t *testing.T) {
	for _, err := range []error{pdfops.ErrBrowserCannotRender, pdfops.ErrHTMLUnreadable, browserrender.ErrNotServed, pdfops.ErrPackageTooBig, pdfops.ErrPackageUnreadable} {
		status, msg := importRefusal(fmt.Errorf("wrapped: %w", err))
		if status != http.StatusUnprocessableEntity || msg == "" {
			t.Errorf("%v: answered %d %q", err, status, msg)
		}
		if !isImportRefusal(err) {
			t.Errorf("%v is not an import refusal, so a path open would report it as a disk error", err)
		}
	}
}
