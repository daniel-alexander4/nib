package main

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nib/internal/instance"
)

// captureLog redirects the standard logger for one test — handedOff's only output is the log,
// which is exactly what a stranger's machine leaves behind to read.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// TestABusyInstanceKeepsItsRecord — /pending 783, 630. A probe that timed out was read as "nobody
// there": the launch removed the record of a Nib that was merely busy, started a second one, and
// the first deleted the second's record on its way out. Not answering in time now leaves the record
// alone, and the second Nib it costs is announced where whoever reads the log will see why.
func TestABusyInstanceKeepsItsRecord(t *testing.T) {
	release := make(chan struct{})
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // longer than the probe's limit
	}))
	defer busy.Close()
	defer close(release)

	dir := t.TempDir()
	rec := instance.Record{Addr: strings.TrimPrefix(busy.URL, "http://"), Token: "busy-token", Handoff: "h"}
	if err := instance.Create(dir, rec); err != nil {
		t.Fatal(err)
	}
	logged := captureLog(t)
	if handedOff(dir, "") {
		t.Fatal("the launch reported a hand-off to an instance that never answered")
	}
	got, err := instance.Read(dir)
	if err != nil || got.Token != rec.Token {
		t.Errorf("the busy instance's record was taken (%+v, %v) — the next launch finds this one instead, and the busy one deletes it on exit", got, err)
	}
	if !strings.Contains(logged.String(), "starting a second Nib") {
		t.Errorf("two Nibs are about to run and the log does not say why:\n%s", logged)
	}
}

// TestAnUnreadableRecordIsClearedNotRunBeside — /pending 813. An unreadable record was "treated as
// absent" and left there, so the launch's own exclusive Create failed and it ran without a record —
// and so did every launch after it, for good.
func TestAnUnreadableRecordIsClearedNotRunBeside(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(instance.Path(dir), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	logged := captureLog(t)
	if handedOff(dir, "") {
		t.Fatal("the launch reported a hand-off with no instance to hand to")
	}
	if err := instance.Create(dir, instance.Record{Addr: "127.0.0.1:4001", Token: "next"}); err != nil {
		t.Errorf("after the launch, publishing its own record still fails (%v) — it runs alongside with no record, as will every launch after it", err)
	}
	if !strings.Contains(logged.String(), "cleared an unreadable instance record") {
		t.Errorf("the record was cleared without a word in the log:\n%s", logged)
	}
}
