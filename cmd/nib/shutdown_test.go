package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// A running Nib that is asked to leave — by Quit, by SIGTERM — ends its converter before it exits
// (ADR-185, D31). Neither cancels the conversion's context, so before `Shutdown` a LibreOffice that
// was converting survived, with the user's source in its temp directory. The converter here is a
// script named `soffice` first on PATH that records its pid and sleeps; the document is opened by path
// through the real `/api/open`, the window's only conversion door since ADR-215 retired `/api/office`.
type liveNib struct {
	cmd              *exec.Cmd
	base, csrf       string
	pidFile, tmpRoot string
	log              *bytes.Buffer
}

var keyedURL = regexp.MustCompile(`open Nib at (http://[^#\s]+)#k=([A-Za-z0-9_-]+)`)

func startNibWithFakeConverter(t *testing.T) *liveNib {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs a binary")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "nib")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building nib: %v\n%s", err, out)
	}
	fake := filepath.Join(dir, "bin")
	if err := os.MkdirAll(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(dir, "fakepid")
	script := "#!/bin/sh\necho $$ > " + pidFile + "\nsleep 300 >/dev/null 2>&1 &\nwait\n"
	if err := os.WriteFile(filepath.Join(fake, "soffice"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	n := &liveNib{pidFile: pidFile, tmpRoot: tmp, log: &bytes.Buffer{}}
	n.cmd = exec.Command(bin)
	n.cmd.Env = append(os.Environ(), "HOME="+filepath.Join(dir, "home"), "XDG_CONFIG_HOME="+filepath.Join(dir, "cfg"),
		"TMPDIR="+tmp, "PATH="+fake+":"+os.Getenv("PATH"), "NIB_NO_BROWSER=1", "NIB_ADDR=127.0.0.1:0")
	n.cmd.Stderr = n.log
	if err := n.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = n.cmd.Process.Kill()
		_, _ = n.cmd.Process.Wait()
		if b, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		}
	})
	var m []string
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if m = keyedURL.FindStringSubmatch(n.log.String()); m != nil {
			break
		}
	}
	if m == nil {
		t.Fatalf("Nib never logged its keyed address:\n%s", n.log.String())
	}
	n.base = m[1]
	req, _ := http.NewRequest("POST", n.base+"/api/launch", nil)
	req.Header.Set("X-Nib-Launch", m[2])
	req.Header.Set("Origin", n.base)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var lr struct {
		CSRF string `json:"csrf"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil || lr.CSRF == "" {
		t.Fatalf("launch trade: %v %+v", err, lr)
	}
	n.csrf = lr.CSRF
	// A fresh home has no vault, and `/api/open` needs one open: enrol a key as the harnesses do.
	enrol := fmt.Sprintf(`{"mode":"create","keyPath":%q}`, filepath.Join(dir, "home", ".ssh", "id_ed25519"))
	er, err := n.post(t, "/api/ssh/enroll", strings.NewReader(enrol), "application/json")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, er.Body)
	er.Body.Close()
	if er.StatusCode/100 != 2 {
		t.Fatalf("enrolling a key answered %d", er.StatusCode)
	}
	return n
}

func (n *liveNib) post(t *testing.T, path string, body io.Reader, ctype string) (*http.Response, error) {
	req, _ := http.NewRequest("POST", n.base+path, body)
	req.Header.Set("X-CSRF-Token", n.csrf)
	req.Header.Set("Origin", n.base)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	return http.DefaultClient.Do(req)
}

// startConversion opens a .docx by path and returns once the fake converter is running; the response is
// drained in the background, because a killed converter answers it with a failure.
func (n *liveNib) startConversion(t *testing.T) (converterPID int) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "report.docx")
	if err := os.WriteFile(src, testpdf.MinimalDocx("shutdown"), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"path": src})
	answer := make(chan string, 1)
	go func() {
		resp, err := n.post(t, "/api/open", bytes.NewReader(body), "application/json")
		if err != nil {
			answer <- err.Error()
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		answer <- fmt.Sprintf("%d %s", resp.StatusCode, b)
	}()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if b, err := os.ReadFile(n.pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && alive(pid) {
				return pid
			}
		}
	}
	select {
	case a := <-answer:
		t.Fatalf("the converter never started; /api/open answered %s\n%s", a, n.log.String())
	default:
		t.Fatalf("the converter never started and /api/open never answered:\n%s", n.log.String())
	}
	return 0
}

func alive(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	i := bytes.LastIndexByte(b, ')')
	return i >= 0 && len(b) > i+2 && b[i+2] != 'Z'
}

func (n *liveNib) waitExit(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() { _ = n.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("Nib did not exit:\n%s", n.log.String())
	}
}

func TestSIGTERMEndsARunningConverterBeforeNibLeaves(t *testing.T) {
	n := startNibWithFakeConverter(t)
	pid := n.startConversion(t)
	if err := n.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	n.waitExit(t)
	if alive(pid) {
		t.Fatalf("SIGTERM left the converter (pid %d) running", pid)
	}
}

func TestQuitEndsARunningConverterBeforeNibLeaves(t *testing.T) {
	n := startNibWithFakeConverter(t)
	pid := n.startConversion(t)
	resp, err := n.post(t, "/api/quit", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("/api/quit answered %d", resp.StatusCode)
	}
	n.waitExit(t)
	if alive(pid) {
		t.Fatalf("Quit left the converter (pid %d) running", pid)
	}
}

// The idle exit is the third arrival at the same teardown. It needs a window to have come and gone,
// which a headless run cannot fake cheaply, so what is held is that all three causes sit in the ONE
// select that precedes Shutdown, and that Shutdown precedes the listener's close.
func TestTheThreeExitCausesMeetAtOneShutdown(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	sel := strings.Index(s, "case <-ctx.Done():")
	shut := strings.Index(s, "\n\ts.Shutdown()\n")
	closeAt := strings.Index(s, "_ = srv.Close()")
	if sel < 0 || shut < 0 || closeAt < 0 || !(sel < shut && shut < closeAt) {
		t.Fatalf("the select (%d), s.Shutdown() (%d) and srv.Close() (%d) are not in that order", sel, shut, closeAt)
	}
	for _, cause := range []string{"case <-ctx.Done():", "case err := <-serveErr:", "case <-s.IdleExit():"} {
		i := strings.Index(s, cause)
		if i < sel-1 || i > shut {
			t.Errorf("%q is not in the select that precedes Shutdown", cause)
		}
	}
	if strings.Count(s, "\n\ts.Shutdown()\n") != 1 {
		t.Error("Shutdown is called from more than one place; the teardown is meant to be one")
	}
}
