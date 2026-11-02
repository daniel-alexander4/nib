package server

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nib/internal/gate"
	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// The gate, driven through the real routes with a real LibreOffice (ADR-185, D9, R6, Y4). Present-path
// tests: skipped by name where LibreOffice is absent, never silently.

func needLibreOffice(t *testing.T) {
	t.Helper()
	if !pdfops.LibreOfficeAvailable() {
		t.Skip("NOT RUN (not a pass): LibreOffice is absent, so the gate was not driven through a real conversion")
	}
}

// useGate swaps the process's import gate for one test.
func useGate(t *testing.T, g *gate.Gate) {
	t.Helper()
	was := pdfops.ImportGate
	pdfops.ImportGate = g
	t.Cleanup(func() { pdfops.ImportGate = was })
}

// uploadAs posts one file to /api/upload under ctx and returns the status (0 for a transport error).
func uploadAs(ctx context.Context, c *http.Client, csrf, base, name string, body []byte) int {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(body)
	mw.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	res, err := c.Do(req)
	if err != nil {
		return 0
	}
	defer res.Body.Close()
	return res.StatusCode
}

func officeDirs(t *testing.T) int {
	t.Helper()
	root := filepath.Join(os.TempDir(), "nib-"+strconv.Itoa(os.Getuid()))
	ents, _ := os.ReadDir(root)
	n := 0
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "nib-office-") {
			n++
		}
	}
	return n
}

// With one slot and one place in the queue, three simultaneous conversions: one runs, one waits, and
// the third is refused BY NAME — and after all of it nothing is held, queued or left on disk.
func TestAFullQueueRefusesTheNextConversionByName(t *testing.T) {
	needLibreOffice(t)
	g := gate.New(1, 1)
	useGate(t, g)
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	doc := testpdf.MinimalDocx("queued")
	before := officeDirs(t)

	codes := make(chan int, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- uploadAs(context.Background(), c, csrf, ts.URL, "q.docx", doc)
		}()
	}
	wg.Wait()
	close(codes)
	got := map[int]int{}
	for code := range codes {
		got[code]++
	}
	if got[http.StatusOK] != 2 || got[http.StatusServiceUnavailable] != 1 {
		t.Fatalf("statuses = %v, want two 200 and exactly one 503 (the queue's bound)", got)
	}
	if g.Held() != 0 || g.Queued() != 0 {
		t.Errorf("after the run the gate holds %d and queues %d", g.Held(), g.Queued())
	}
	srv.mu.Lock()
	n := len(srv.docs)
	srv.mu.Unlock()
	if n != 2 {
		t.Errorf("%d documents installed, want 2 (the refused one installs nothing)", n)
	}
	if after := officeDirs(t); after != before {
		t.Errorf("%d conversion directories left behind", after-before)
	}
}

// A request that leaves while queued installs nothing, holds no slot and makes no directory; the third
// then gets the slot.
func TestACancelledQueuedConversionLeavesNothingAndALaterOneProceeds(t *testing.T) {
	needLibreOffice(t)
	g := gate.New(1, 4)
	useGate(t, g)
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	doc := testpdf.MinimalDocx("one")
	before := officeDirs(t)

	first := make(chan int, 1)
	go func() { first <- uploadAs(context.Background(), c, csrf, ts.URL, "a.docx", doc) }()
	for g.Held() != 1 {
		time.Sleep(5 * time.Millisecond)
	}
	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan int, 1)
	go func() { second <- uploadAs(ctx, c, csrf, ts.URL, "b.docx", doc) }()
	for g.Queued() != 1 {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if code := <-second; code == http.StatusOK {
		t.Fatal("a cancelled queued upload reported success")
	}
	if code := <-first; code != http.StatusOK {
		t.Fatalf("the running conversion = %d", code)
	}
	if code := uploadAs(context.Background(), c, csrf, ts.URL, "c.docx", doc); code != http.StatusOK {
		t.Fatalf("a conversion after the cancel = %d: the cancelled one kept the slot", code)
	}
	srv.mu.Lock()
	n := len(srv.docs)
	srv.mu.Unlock()
	if n != 2 {
		t.Errorf("%d documents installed, want 2 (the cancelled one installs nothing)", n)
	}
	if g.Held() != 0 || g.Queued() != 0 || officeDirs(t) != before {
		t.Errorf("held %d queued %d, leftover directories %d", g.Held(), g.Queued(), officeDirs(t)-before)
	}
}

// Four hand-offs against a gate that would refuse the second: none is refused — the OS has already told
// the person their files are opening.
func TestAHandoffBacklogIsNeverRefusedForALongQueue(t *testing.T) {
	needLibreOffice(t)
	useGate(t, gate.New(2, 0))
	_, srv := startServerWith(t)
	dir := t.TempDir()
	const n = 4
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, "h"+strconv.Itoa(i)+".docx")
		if err := os.WriteFile(p, testpdf.MinimalDocx("handoff "+strconv.Itoa(i)), 0o600); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := srv.openHandedOff(context.Background(), p)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("a hand-off was refused: %v", err)
		}
	}
	srv.mu.Lock()
	got := len(srv.docs)
	srv.mu.Unlock()
	if got != n {
		t.Errorf("%d of %d handed-off documents opened", got, n)
	}
}

// A conversion runs on the request's context: a client that goes away mid-conversion kills the converter,
// so no LibreOffice outlives the request that wanted it.
func TestALeavingClientEndsItsConversion(t *testing.T) {
	needLibreOffice(t)
	g := gate.New(1, 1)
	useGate(t, g)
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- uploadAs(ctx, c, csrf, ts.URL, "x.docx", testpdf.MinimalDocx("leaving")) }()
	for g.Held() != 1 {
		time.Sleep(2 * time.Millisecond)
	}
	for pdfops.LiveConverters() == 0 {
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	<-done
	deadline := time.Now().Add(10 * time.Second)
	for g.Held() != 0 || pdfops.LiveConverters() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("after the client left: gate held %d, converters %d", g.Held(), pdfops.LiveConverters())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
