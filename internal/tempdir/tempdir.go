// Package tempdir is the one door a Nib temporary directory comes through (ADR-185, ADR-009).
//
// Every import and export writes its input, its converter's profile and its output somewhere, and
// until this door existed that was four unrelated `os.MkdirTemp` calls in the system temp directory,
// one of which held the user's source file under a name that told nobody which process owned it.
// A crash, a SIGKILL or a Quit that beat the deferred removal left it there for good.
//
// # Liveness is a kernel lock, never a name or an age
//
// `New` creates `nib-<kind>-<pid>-<rand>` under a per-user 0700 root and takes an exclusive
// `filelock` on `<dir>/.live`. The kernel drops that lock when the holder exits or crashes, so
// `Sweep` — run at every start — removes exactly the directories whose lock it can take. Nothing is
// parsed out of the name (the pid in it is for a person reading `ls`), and nothing is removed for
// being old: a long import in another Nib process is untouched.
//
// # What it does not cover, declared
//
// The window between a crash and the next start of Nib leaves the directory (and a confidential
// source in it) on disk — the README says so. SIGKILL cannot be handled.
package tempdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"nib/internal/filelock"
)

const (
	livePrefix = "nib-"
	liveFile   = ".live"
	groupFile  = ".pgid"
)

// creationGrace is how long a directory with no `.live` file is left alone: `New` makes the
// directory and then the file, and a sweep from a second process must not remove one in between. It
// is an interlock on that window only; it is never how a directory is judged dead.
const creationGrace = time.Minute

// ErrNoSpace is a temporary write that the disk refused. A named refusal (the import says "the disk
// is full"), not a generic failure.
var ErrNoSpace = errors.New("there is not enough free disk space to hold the temporary files")

// ErrUnsafeRoot: the per-user root exists and is not ours alone (another user's, a link, or open to
// others), so nothing is created in it.
var ErrUnsafeRoot = errors.New("the temporary directory root is not private to this user")

var (
	rootMu sync.Mutex
	root   string
)

// Root is the per-user 0700 directory every Nib temporary directory lives in — never the bare
// system temp directory, where another user could plant a name.
func Root() (string, error) {
	rootMu.Lock()
	defer rootMu.Unlock()
	if root != "" {
		return root, nil
	}
	name := "nib"
	if uid := os.Getuid(); uid >= 0 {
		name = "nib-" + strconv.Itoa(uid)
	}
	p := filepath.Join(os.TempDir(), name)
	if err := os.Mkdir(p, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", wrap(err)
	}
	if err := checkPrivate(p); err != nil {
		return "", err
	}
	root = p
	return root, nil
}

var (
	claimMu sync.Mutex
	claimed *Dir
)

// Claim makes a `nib-spill-…` directory the process's temporary directory (TMPDIR / TEMP), once, at
// start and before any goroutine runs. It covers what Nib does not create itself — a multipart
// upload's overflow (`multipart-*`, which holds the user's source and carries no pid), pdfcpu's,
// `x/image`'s and Ghostscript's temp writes — so they land in a directory that has a liveness lock
// and is swept like any other. The process-wide effect is declared (ADR-185). Idempotent; Release
// removes the directory.
func Claim() error {
	claimMu.Lock()
	defer claimMu.Unlock()
	if claimed != nil {
		return nil
	}
	d, err := New("spill")
	if err != nil {
		return err
	}
	keys := []string{"TMPDIR"}
	if runtime.GOOS == "windows" {
		keys = []string{"TMP", "TEMP"}
	}
	for _, k := range keys {
		if err := os.Setenv(k, d.Path()); err != nil {
			_ = d.Close()
			return err
		}
	}
	claimed = d
	return nil
}

// Release removes the claimed spill directory. Called once on the way out of `run()`.
func Release() {
	claimMu.Lock()
	defer claimMu.Unlock()
	if claimed != nil {
		_ = claimed.Close()
		claimed = nil
	}
}

// Dir is one live temporary directory. Close removes it and drops its lock; both are idempotent.
type Dir struct {
	path string
	live *os.File
	once sync.Once
	err  error
}

// New creates a directory of the given kind ("office", "gs", "import", …) and holds its liveness
// lock until Close or process exit.
func New(kind string) (*Dir, error) {
	if kind == "" || strings.ContainsAny(kind, `/\`+"\x00") {
		return nil, fmt.Errorf("tempdir: bad kind %q", kind)
	}
	r, err := Root()
	if err != nil {
		return nil, err
	}
	p, err := os.MkdirTemp(r, livePrefix+kind+"-"+strconv.Itoa(os.Getpid())+"-")
	if err != nil {
		return nil, wrap(err)
	}
	// MkdirTemp makes it 0700; asserted rather than assumed, because the sweep refuses anything else.
	if err := os.Chmod(p, 0o700); err != nil {
		_ = os.RemoveAll(p)
		return nil, wrap(err)
	}
	f, err := os.OpenFile(filepath.Join(p, liveFile), os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o600)
	if err != nil {
		_ = os.RemoveAll(p)
		return nil, wrap(err)
	}
	if got, err := filelock.Try(f); err != nil || !got {
		_ = f.Close()
		_ = os.RemoveAll(p)
		if err == nil {
			err = errors.New("tempdir: a new directory was already locked")
		}
		return nil, err
	}
	return &Dir{path: p, live: f}, nil
}

// Path is the directory.
func (d *Dir) Path() string { return d.path }

// Join is a path inside the directory.
func (d *Dir) Join(elem ...string) string { return filepath.Join(append([]string{d.path}, elem...)...) }

// WriteFile writes a file inside the directory at 0600, mapping a full disk to ErrNoSpace.
func (d *Dir) WriteFile(name string, data []byte) error {
	return wrapIf(os.WriteFile(d.Join(name), data, 0o600))
}

// RecordGroup writes the process group of a converter started in this directory. It is read ONLY by
// the sweep, to kill the group if Nib died first; it is never trusted as a name.
func (d *Dir) RecordGroup(pgid int) error {
	return wrapIf(os.WriteFile(d.Join(groupFile), []byte(strconv.Itoa(pgid)+"\n"), 0o600))
}

// Close removes the directory and releases its lock.
func (d *Dir) Close() error {
	d.once.Do(func() {
		// The handle closes FIRST: Windows cannot delete a directory holding a file this process has open. A sweep
		// that takes the released lock in the gap removes the same directory, and removal is idempotent.
		filelock.Unlock(d.live)
		_ = d.live.Close()
		d.err = os.RemoveAll(d.path)
	})
	return d.err
}

// Reaper kills a converter group an earlier Nib left running. since is when the directory was made:
// a process older than it cannot be the converter that was started in it.
type Reaper func(pgid int, since time.Time)

// Sweep removes every directory under the root that no live process holds, killing a recorded
// orphaned converter group first when reap is non-nil. It acts only on real directories (lstat),
// owned by this user, mode 0700, named `nib-…`; a link, a foreign owner or anything else is left
// alone and nothing is killed on its account. Returns how many it removed.
func Sweep(reap Reaper) (removed int, err error) {
	r, err := Root()
	if err != nil {
		return 0, err
	}
	ents, err := os.ReadDir(r)
	if err != nil {
		return 0, err
	}
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), livePrefix) {
			continue
		}
		p := filepath.Join(r, e.Name())
		if sweepOne(p, reap) {
			removed++
		}
	}
	return removed, nil
}

func sweepOne(p string, reap Reaper) bool {
	fi, err := os.Lstat(p)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || !modeOK(fi) || !ownedByUs(fi) {
		return false
	}
	lf, err := os.OpenFile(filepath.Join(p, liveFile), os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && time.Since(fi.ModTime()) > creationGrace {
			return os.RemoveAll(p) == nil
		}
		return false
	}
	defer lf.Close()
	got, err := filelock.Try(lf)
	if err != nil || !got {
		return false // held by a live process — possibly this one
	}
	defer filelock.Unlock(lf)
	if reap != nil {
		if b, err := os.ReadFile(filepath.Join(p, groupFile)); err == nil {
			if pgid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pgid > 1 {
				since := fi.ModTime()
				if lfi, err := lf.Stat(); err == nil {
					since = lfi.ModTime()
				}
				reap(pgid, since)
			}
		}
	}
	return os.RemoveAll(p) == nil
}

func wrap(err error) error {
	if errors.Is(err, syscall.ENOSPC) {
		return fmt.Errorf("%w: %v", ErrNoSpace, err)
	}
	return err
}

func wrapIf(err error) error {
	if err == nil {
		return nil
	}
	return wrap(err)
}
