//go:build linux

package tempdir

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// leaveOrphan starts `sh -c 'sleep 300 & exit'` as the leader of its own group, so the launcher is
// gone and the child is the orphan — the shape of `soffice` exiting before `soffice.bin`.
func leaveOrphan(t *testing.T) (pgid int, child int) {
	t.Helper()
	cmd := exec.Command("sh", "-c", "sleep 300 >/dev/null 2>&1 & echo $!; exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.Output()
	if err != nil {
		t.Skip("no sh:", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("child pid %q: %v", out, err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	g, _, ok := statOf(pid)
	if !ok {
		t.Fatal("cannot read the orphan's stat")
	}
	return g, pid
}

func alive(pid int) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	i := strings.LastIndexByte(string(b), ')')
	return i >= 0 && len(b) > i+2 && b[i+2] != 'Z'
}

func waitDead(t *testing.T, pid int) bool {
	for i := 0; i < 100; i++ {
		if !alive(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func exeBase(t *testing.T, pid int) string {
	e, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Base(e)
}

func TestSweepKillsAnOrphanedConverterGroupItIsToldAbout(t *testing.T) {
	isolate(t)
	d, err := New("office")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // the converter starts after the directory is made
	pgid, child := leaveOrphan(t)
	if err := d.RecordGroup(pgid); err != nil {
		t.Fatal(err)
	}
	p := d.Path()
	d.Close() // releases the lock; the directory is removed too, so remake the crash by hand
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(p, liveFile)
	if err := os.WriteFile(live, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	_ = os.Chtimes(live, past, past) // made an hour ago: the orphan started after it
	if err := os.WriteFile(filepath.Join(p, groupFile), []byte(strconv.Itoa(pgid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Not named as a converter: left alone.
	if n, _ := Sweep(NewReaper([]string{"soffice.bin"})); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	if !alive(child) {
		t.Fatal("a process whose executable is not a known converter was killed")
	}
	// Named: killed. Remake the directory and sweep again.
	_ = os.MkdirAll(p, 0o700)
	_ = os.WriteFile(live, nil, 0o600)
	_ = os.Chtimes(live, past, past)
	_ = os.WriteFile(filepath.Join(p, groupFile), []byte(strconv.Itoa(pgid)+"\n"), 0o600)
	if n, _ := Sweep(NewReaper([]string{exeBase(t, child)})); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	if !waitDead(t, child) {
		t.Fatal("the orphaned converter group member survived the sweep")
	}
}

func TestSweepDoesNotKillAProcessOlderThanTheDirectory(t *testing.T) {
	isolate(t)
	pgid, child := leaveOrphan(t)
	time.Sleep(3200 * time.Millisecond) // past the 2 s boot-time rounding slack
	d, err := New("office")             // made AFTER the process started: it cannot be this directory's converter
	if err != nil {
		t.Fatal(err)
	}
	if err := d.RecordGroup(pgid); err != nil {
		t.Fatal(err)
	}
	filelock := d.live
	_ = filelock
	// Release as a crash would, leaving the directory.
	releaseByHand(d)
	if n, _ := Sweep(NewReaper([]string{exeBase(t, child)})); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	if !alive(child) {
		t.Fatal("a process that started before the directory was made was killed (pid-reuse guard)")
	}
}

func TestAPlantedDirectoryNamingAVictimsGroupKillsNothing(t *testing.T) {
	isolate(t)
	r, _ := Root()
	pgid, child := leaveOrphan(t)
	// A directory with the right shape but the wrong mode: acted on by nothing, so nothing is killed.
	p := filepath.Join(r, "nib-office-1-planted")
	_ = os.Mkdir(p, 0o755)
	_ = os.Chmod(p, 0o755)
	_ = os.WriteFile(filepath.Join(p, liveFile), nil, 0o600)
	_ = os.WriteFile(filepath.Join(p, groupFile), []byte(strconv.Itoa(pgid)+"\n"), 0o600)
	Sweep(NewReaper([]string{exeBase(t, child)}))
	if !alive(child) {
		t.Fatal("a planted directory got a process killed")
	}
}
