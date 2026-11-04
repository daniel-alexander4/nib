package tempdir

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"nib/internal/filelock"
)

// isolate points the temp root at a directory of the test's own and forgets the cached one.
func isolate(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	resetRoot()
	t.Cleanup(resetRoot)
	return tmp
}

func TestNewIsPrivateLockedAndRemovedOnClose(t *testing.T) {
	isolate(t)
	d, err := New("office")
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(d.Path())
	if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf("directory %v mode %v err %v: want a 0700 directory", d.Path(), fi, err)
	}
	if !strings.Contains(filepath.Base(d.Path()), "nib-office-") {
		t.Fatalf("name %q does not carry its kind", d.Path())
	}
	if r, _ := Root(); filepath.Dir(d.Path()) != r {
		t.Fatalf("directory %q is not under the per-user root %q", d.Path(), r)
	}
	// Held: a second open file description cannot take the liveness lock.
	f, err := os.OpenFile(d.Join(liveFile), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := filelock.Try(f); got {
		t.Fatal("the liveness lock was takeable while the directory was open")
	}
	f.Close()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory survived Close: %v", err)
	}
	if err := d.Close(); err != nil { // idempotent
		t.Fatal(err)
	}
}

func TestRootRefusesAPlantedLinkAndAForeignShape(t *testing.T) {
	tmp := isolate(t)
	name := "nib"
	if uid := os.Getuid(); uid >= 0 {
		name = "nib-" + itoa(uid)
	}
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(tmp, name)); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := Root(); !errors.Is(err, ErrUnsafeRoot) {
		t.Fatalf("Root over a symlink = %v, want ErrUnsafeRoot", err)
	}
	if _, err := New("x"); !errors.Is(err, ErrUnsafeRoot) {
		t.Fatalf("New over a symlinked root = %v, want ErrUnsafeRoot", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestRootTightensALooseModeWeOwn(t *testing.T) {
	tmp := isolate(t)
	name := "nib"
	if uid := os.Getuid(); uid >= 0 {
		name = "nib-" + itoa(uid)
	}
	p := filepath.Join(tmp, name)
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Root(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o700 {
		t.Fatalf("root mode %v, want 0700", fi.Mode().Perm())
	}
}

func TestClaimPointsTheProcessTempAtASweptSpillDirectory(t *testing.T) {
	isolate(t)
	t.Cleanup(Release)
	if err := Claim(); err != nil {
		t.Fatal(err)
	}
	r, _ := Root()
	spill := os.TempDir()
	if filepath.Dir(spill) != r || !strings.Contains(filepath.Base(spill), "nib-spill-") {
		t.Fatalf("os.TempDir() = %q, want a nib-spill directory under %q", spill, r)
	}
	// What the standard library spills (a multipart upload) lands there, not in the shared temp dir.
	f, err := os.CreateTemp("", "multipart-")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if filepath.Dir(f.Name()) != spill {
		t.Fatalf("a stdlib temp file went to %q, not the spill directory %q", f.Name(), spill)
	}
	// Claim is idempotent, and Release removes the directory with whatever was spilled.
	if err := Claim(); err != nil || os.TempDir() != spill {
		t.Fatalf("a second Claim moved the claim: %v %q", err, os.TempDir())
	}
	Release()
	if _, err := os.Stat(spill); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Release left the spill directory: %v", err)
	}
}

// dead makes a directory exactly as a crashed Nib leaves it: a .live file nobody holds.
func dead(t *testing.T, kind string) string {
	t.Helper()
	d, err := New(kind)
	if err != nil {
		t.Fatal(err)
	}
	p := d.Path()
	if err := os.WriteFile(filepath.Join(p, "in.docx"), []byte("the user's source"), 0o600); err != nil {
		t.Fatal(err)
	}
	filelock.Unlock(d.live) // the kernel's release on exit, done by hand
	_ = d.live.Close()
	return p
}

func TestSweepRemovesWhatNobodyHoldsAndLeavesWhatIsHeld(t *testing.T) {
	isolate(t)
	orphan := dead(t, "office")
	held, err := New("office")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	n, err := Sweep(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("Sweep removed %d, want 1", n)
	}
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the orphan survived the sweep: %v", err)
	}
	if _, err := os.Stat(held.Path()); err != nil {
		t.Fatalf("a directory a live holder owns was removed: %v", err)
	}
}

func TestSweepNeverActsOnALinkAWrongModeOrAForeignName(t *testing.T) {
	isolate(t)
	r, _ := Root()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(r, "nib-evil-1-x")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	loose := filepath.Join(r, "nib-office-1-loose")
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(r, "not-ours")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if n, _ := Sweep(nil); n != 0 {
		t.Fatalf("Sweep removed %d entries it should not touch", n)
	}
	for _, p := range []string{victim, loose, other} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s was removed: %v", p, err)
		}
	}
}

func TestADirectoryMidCreationIsNotSwept(t *testing.T) {
	isolate(t)
	r, _ := Root()
	p := filepath.Join(r, "nib-office-1-young")
	if err := os.Mkdir(p, 0o700); err != nil {
		t.Fatal(err)
	}
	if n, _ := Sweep(nil); n != 0 {
		t.Fatal("a young directory with no .live yet was swept (the creation window)")
	}
	old := time.Now().Add(-2 * creationGrace)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if n, _ := Sweep(nil); n != 1 {
		t.Fatal("an old directory with no .live was not swept")
	}
}

func TestADiskFullIsANamedRefusal(t *testing.T) {
	if err := wrap(&os.PathError{Op: "write", Path: "x", Err: syscall.ENOSPC}); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("ENOSPC = %v, want ErrNoSpace", err)
	}
}

func TestANewDirectoryIsRefusedABadKind(t *testing.T) {
	isolate(t)
	for _, k := range []string{"", "a/b", `a\b`} {
		if _, err := New(k); err == nil {
			t.Fatalf("kind %q was accepted", k)
		}
	}
}

// A second PROCESS holding a directory leaves it untouched — the kernel lock, not a name, says live.
func TestASecondProcessHoldingADirectoryIsUntouched(t *testing.T) {
	if os.Getenv("TEMPDIR_HOLD") != "" {
		d, err := New("office")
		if err != nil {
			os.Exit(2)
		}
		os.Stdout.WriteString(d.Path() + "\n")
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	tmp := isolate(t)
	cmd := exec.Command(os.Args[0], "-test.run=TestASecondProcessHoldingADirectoryIsUntouched")
	cmd.Env = append(os.Environ(), "TEMPDIR_HOLD=1", "TMPDIR="+tmp)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	buf := make([]byte, 512)
	n, _ := out.Read(buf)
	held := strings.TrimSpace(string(buf[:n]))
	if held == "" {
		t.Fatal("the holder process printed no directory")
	}
	if cnt, _ := Sweep(nil); cnt != 0 {
		t.Fatalf("Sweep removed %d with a live holder", cnt)
	}
	if _, err := os.Stat(held); err != nil {
		t.Fatalf("the live holder's directory was removed: %v", err)
	}
	// Killed, the kernel drops its lock and the next sweep takes the directory.
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if cnt, _ := Sweep(nil); cnt != 1 {
		t.Fatalf("after the holder died Sweep removed %d, want 1", cnt)
	}
}

func TestChildEnvRestoresTheTempVariableClaimRedirected(t *testing.T) {
	orig := t.TempDir()
	t.Setenv("TMPDIR", orig)
	resetRoot()
	t.Cleanup(resetRoot)
	old, oldOrig := claimed, origTemp
	claimed, origTemp = nil, nil
	t.Cleanup(func() { claimed, origTemp = old, oldOrig })
	if err := Claim(); err != nil {
		t.Fatal(err)
	}
	defer Release()
	if os.Getenv("TMPDIR") == orig {
		t.Fatal("Claim did not redirect TMPDIR: the test has no stimulus")
	}
	var got []string
	for _, kv := range ChildEnv() {
		if strings.HasPrefix(kv, "TMPDIR=") {
			got = append(got, kv)
		}
	}
	if len(got) != 1 || got[0] != "TMPDIR="+orig {
		t.Fatalf("a detached child would start with %v, want exactly TMPDIR=%s", got, orig)
	}
}

// A teardown that arrives while an import still holds its directory removes it, and the owner's own Close
// afterwards is a no-op that returns the same answer — the registry is what Server.Shutdown reaches.
func TestCloseAllEndsEveryLiveDirectoryAndTheOwnersCloseStaysSafe(t *testing.T) {
	isolate(t)
	CloseAll() // other tests in this package leave directories open (a held-by-hand lock, a sweep fixture)
	a, err := New("import")
	if err != nil {
		t.Fatal(err)
	}
	b, err := New("office")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil { // already closed by its owner: not counted, not touched
		t.Fatal(err)
	}
	c, err := New("gs")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteFile("in.pdf", []byte("x")); err != nil {
		t.Fatal(err)
	}
	// The owner closes at the same moment the teardown does.
	done := make(chan error, 1)
	go func() { done <- a.Close() }()
	n := CloseAll()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n < 1 || n > 2 {
		t.Fatalf("CloseAll closed %d directories, want 1 or 2 (c, and a unless its owner won)", n)
	}
	for _, d := range []*Dir{a, b, c} {
		if _, err := os.Stat(d.Path()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s survived the teardown: %v", d.Path(), err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatalf("the owner's Close after CloseAll: %v", err)
	}
	if n := CloseAll(); n != 0 {
		t.Fatalf("a second CloseAll closed %d directories, want 0", n)
	}
}

// The claimed spill directory is Release's, and the process is still taking multipart overflow into it
// while Shutdown runs.
func TestCloseAllLeavesTheClaimedSpillDirectory(t *testing.T) {
	isolate(t)
	d, err := New("spill")
	if err != nil {
		t.Fatal(err)
	}
	claimMu.Lock()
	claimed = d
	claimMu.Unlock()
	t.Cleanup(func() {
		claimMu.Lock()
		claimed = nil
		claimMu.Unlock()
		_ = d.Close()
	})
	CloseAll()
	if _, err := os.Stat(d.Path()); err != nil {
		t.Fatalf("CloseAll removed the claimed spill directory: %v", err)
	}
}
