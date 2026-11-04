//go:build !windows

package pdfops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// useGS makes gsCache answer with a stand-in Ghostscript for the test.
func useGS(t *testing.T, path string) {
	t.Helper()
	gsCache.mu.Lock()
	old := gsCache.path
	gsCache.path = path
	gsCache.mu.Unlock()
	t.Cleanup(func() {
		gsCache.mu.Lock()
		gsCache.path = old
		gsCache.mu.Unlock()
	})
}

// The PDF/A route is an outside tool, so it waits at ImportGate like every other conversion (ADR-185, ADR-009):
// with both slots held it starts nothing, and a cancel ends the wait.
func TestThePDFAGhostscriptRouteWaitsAtTheImportGate(t *testing.T) {
	ran := filepath.Join(t.TempDir(), "ran")
	useGS(t, fakeGS(t, `echo started > `+ran+`; exit 1`))
	r1, err := ImportGate.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer r1()
	r2, err := ImportGate.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer r2()
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := ConvertPDFAGhostscript(ctx, []byte("%PDF-1.4"))
		errc <- err
	}()
	select {
	case err := <-errc:
		t.Fatalf("returned %v while both slots were held", err)
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("a cancelled wait answered %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled wait never returned")
	}
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("Ghostscript started without a slot")
	}
}

// A cancel from the caller (a closed request, Ctrl-C at the command line) kills the Ghostscript group and the
// conversion's directory goes with it, with nothing to wait for the next start's sweep.
func TestCancellingThePDFAGhostscriptRouteKillsTheGroupAndRemovesItsDirectory(t *testing.T) {
	scratch := t.TempDir()
	pidFile, outFile := filepath.Join(scratch, "pid"), filepath.Join(scratch, "out")
	useGS(t, fakeGS(t, `for a in "$@"; do case "$a" in -sOutputFile=*) out="${a#-sOutputFile=}";; esac; done
echo $$ > `+pidFile+`
echo "$out" > `+outFile+`
exec sleep 300`))
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := ConvertPDFAGhostscript(ctx, []byte("%PDF-1.4"))
		errc <- err
	}()
	var pid int
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		b, _ := os.ReadFile(pidFile)
		o, _ := os.ReadFile(outFile)
		if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && len(o) > 0 && LiveConverters() > 0 {
			pid = p
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the stand-in Ghostscript never registered")
		}
	}
	if ImportGate.Held() < 1 {
		t.Fatal("a running conversion holds no slot at ImportGate")
	}
	o, _ := os.ReadFile(outFile)
	dir := filepath.Dir(strings.TrimSpace(string(o)))
	cancel()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("a cancelled conversion reported success")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled conversion never returned")
	}
	if pidAlive(pid) {
		t.Fatal("the Ghostscript process survived the cancel")
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the conversion's directory survived the cancel: %v", err)
	}
	if LiveConverters() != 0 {
		t.Fatal("a cancelled converter is still registered")
	}
}
