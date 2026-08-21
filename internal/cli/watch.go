package cli

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"nib/internal/ots"
	"nib/internal/pdfops"
	"syscall"
)

// cmdWatch polls a directory and runs an operation on each PDF added to it,
// until interrupted — the "process my inbox" / "on a schedule" workflow. It
// uses polling rather than an OS file-event dependency, keeping the binary lean,
// and acts on a file only once its size and mtime have settled (so a file still
// being copied in isn't processed half-written). Unattended `sign` is
// deliberately not offered: signing on a watch would need the key passphrase
// sitting in the daemon's environment, a security posture that shouldn't be a
// silent default.
func cmdWatch(args []string) int {
	fs := flag.NewFlagSet("nib watch", flag.ContinueOnError)
	var op string
	var interval int
	fs.StringVar(&op, "do", "", "operation per PDF: timestamp | optimize | sanitize (required)")
	fs.IntVar(&interval, "interval", 2, "seconds between directory scans")
	fs.Usage = usageFunc(fs, "nib watch DIR --do timestamp|optimize|sanitize",
		"Watch DIR and run an operation on each PDF added to it, until interrupted.\ntimestamp writes a .ots sidecar; optimize/sanitize rewrite the file in place.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		errf("watch needs exactly one directory")
		return 1
	}
	dir := fs.Arg(0)
	act, ok := watchOps[op]
	if !ok {
		errf("--do must be one of: timestamp, optimize, sanitize")
		return 1
	}
	info, err := os.Stat(dir)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if !info.IsDir() {
		errf("%s is not a directory", dir)
		return 1
	}
	if interval < 1 {
		interval = 1
	}
	return watchLoop(dir, interval, op, act)
}

// watchAction processes one file and returns a short status word for the log.
type watchAction func(path string) (status string, err error)

var watchOps = map[string]watchAction{
	"timestamp": watchTimestamp,
	"optimize":  func(p string) (string, error) { return watchTransform(p, pdfops.Optimize, "optimized") },
	"sanitize":  func(p string) (string, error) { return watchTransform(p, sanitize, "sanitized") },
}

// fileState is the size+mtime fingerprint used to tell when a file has settled.
type fileState struct {
	size int64
	mod  time.Time
}

func watchLoop(dir string, interval int, opName string, act watchAction) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(os.Stderr, "nib: watching %s — %s each new PDF (scan every %ds); Ctrl-C to stop\n", dir, opName, interval)

	seen := map[string]fileState{}
	processed := map[string]bool{}
	failed := map[string]fileState{}

	// Everything already in the directory is marked processed BEFORE the first scan,
	// so the watch acts only on files that arrive after it starts.
	//
	// That is what this command has always claimed to do — "each PDF ADDED to it"
	// here, "each NEW PDF" in the command table, "dropped into DIR" in the README —
	// and not what it did: scanOnce walks every settled .pdf in the directory, so
	// pointing `nib watch ~/Documents --do sanitize` at an existing folder rewrote
	// every PDF already in it, in place, stripping metadata from files the user
	// never intended to touch. A destructive default that three separate documents
	// said would not happen.
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
				continue
			}
			processed[filepath.Join(dir, e.Name())] = true
		}
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for {
		scanOnce(dir, seen, processed, failed, act)
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "\nnib: stopped")
			return 0
		case <-ticker.C:
		}
	}
}

// scanOnce processes any PDF in dir that is new and has settled since the last
// scan (size+mtime unchanged). Each path is SUCCESSFULLY acted on at most once
// per run, so an in-place rewrite's own mtime change doesn't retrigger it. A
// failed action is retried — but only after the file's size or mtime changes,
// so a settled-but-broken file doesn't re-error on every scan. State is carried
// in seen/processed/failed across calls.
func scanOnce(dir string, seen map[string]fileState, processed map[string]bool, failed map[string]fileState, act watchAction) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		errf("%v", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if processed[path] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// REGULAR FILES ONLY, and a symlink is the reason.
		//
		// `DirEntry.Info()` is an Lstat, so a symlink named `x.pdf` passed the extension
		// filter; `watchTransform` then read THROUGH it and `writeAtomic` — which calls
		// `filepath.EvalSymlinks` — renamed over the TARGET. Anyone who can drop a file
		// into the watched directory (the documented "process my inbox" and shared
		// scan-drop uses) caused an unrequested in-place rewrite of any PDF elsewhere on
		// disk the user can write, outside the directory the watch was pointed at.
		// `--do sanitize` strips that document's metadata irreversibly.
		//
		// `writeAtomic`'s symlink-following is deliberate and stays — it is for `-w` on a
		// path the USER named, which is a different provenance from directory discovery.
		if !info.Mode().IsRegular() {
			continue
		}
		st := fileState{info.Size(), info.ModTime()}
		if f, ok := failed[path]; ok {
			if f == st {
				continue // same bytes that already failed — wait for a change
			}
			delete(failed, path) // changed since the failure — eligible again
		}
		prev, ok := seen[path]
		seen[path] = st
		if !ok || prev != st {
			continue // first sight or still changing — let it settle
		}
		status, err := act(path)
		if err != nil {
			failed[path] = st
			errf("%s: %v", path, err)
		} else {
			processed[path] = true
			fmt.Printf("%s: %s\n", path, status)
		}
	}
}

// readNoFollow reads path, refusing if it is a symlink — at the OPEN, not before it.
//
// scanOnce already skips a non-regular entry (see the note there, and the rewrite-through-a-
// symlink defect it closed). This closes the window that check cannot: between the Lstat and
// the action, the same actor who could plant the symlink in the first place can swap the
// file for one. The check-then-act is unavoidable when the two steps are a directory listing
// and a file operation; O_NOFOLLOW makes the OPEN itself carry the refusal, so timing stops
// mattering.
//
// The actor is the documented one — anyone who can drop a file into the watched directory,
// which is the shared scan-drop and "process my inbox" use the command exists for. The
// consequence is the same: an unrequested in-place rewrite of a PDF elsewhere on disk, and
// `--do sanitize` strips its metadata irreversibly.
//
// **O_NOFOLLOW is POSIX and is not defined on Windows at all** — the first draft of this
// comment said it was "defined and ignored" there, and `GOOS=windows go build` said
// otherwise. So the flag lives in a two-file shim (noFollow_*.go). Windows keeps the
// Lstat-only protection scanOnce already gives it, plus the regular-file check below, which
// is done on the OPEN HANDLE and so is not a second check-then-act.
func readNoFollow(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|oNoFollow, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// And a regular file: O_NOFOLLOW refuses a symlink, not a fifo or a device, either of
	// which would make the read block or return something that is not the document.
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return io.ReadAll(f)
}

// watchTimestamp writes a .ots proof beside path, skipping a file that already
// has one (so restarting the watch doesn't re-stamp).
func watchTimestamp(path string) (string, error) {
	proof := path + ".ots"
	if _, err := os.Stat(proof); err == nil {
		return "skipped (.ots exists)", nil
	}
	data, err := readNoFollow(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p, err := ots.Stamp(ctx, safeClient(), digest, ots.DefaultCalendars)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(proof, p, 0o644); err != nil {
		return "", err
	}
	return "timestamped", nil
}

func watchTransform(path string, fn func([]byte) ([]byte, error), done string) (string, error) {
	data, err := readNoFollow(path)
	if err != nil {
		return "", err
	}
	// A signed document is skipped, not failed: the rewrite would invalidate
	// every signature on it (see signedInPlace), and nothing about the file will
	// ever make it eligible — returning an error here would re-report the same
	// refusal on every scan for as long as the watch runs.
	if signedInPlace(data) {
		return "skipped (signed — a rewrite would invalidate it)", nil
	}
	res, err := fn(data)
	if err != nil {
		return "", err
	}
	if err := writeAtomic(path, res); err != nil {
		return "", err
	}
	return done, nil
}
