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

	"nib/internal/atomicfile"
	"nib/internal/ots"
	"nib/internal/pdfops"
	"nib/internal/uacheck"
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
	fs.StringVar(&op, "do", "", "operation per PDF: timestamp | optimize | sanitize | ua (required)")
	fs.IntVar(&interval, "interval", 2, "seconds between directory scans")
	fs.Usage = usageFunc(fs, "nib watch DIR --do timestamp|optimize|sanitize|ua",
		"Watch DIR and run an operation on each PDF added to it, until interrupted.\ntimestamp writes a .ots sidecar; optimize/sanitize rewrite the file in place;\nua writes the accessibility report \"nib ua\" prints to FILE.ua.txt.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		errf("watch needs exactly one directory")
		return 1
	}
	dir := fs.Arg(0)
	// Tagging is the one batch operation refused by name (`PLAN-accessibility.md` P10): inferred structure is
	// a proposal a person reviews before it is written, and a watch has no one to review it.
	if op == "tag" {
		errf("--do tag is refused: inferred structure is a proposal, never an assertion (accessibility law 3), " +
			"and a watch has no one to review it — run \"nib tag propose\" and \"nib tag commit\" instead, " +
			"or --do ua for each file's report")
		return 1
	}
	act, ok := watchOps[op]
	if !ok {
		errf("--do must be one of: timestamp, optimize, sanitize, ua")
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
	"ua":        watchUA,
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
// in seen/processed/failed across calls, and a path the scan no longer sees is
// forgotten from all three.
func scanOnce(dir string, seen map[string]fileState, processed map[string]bool, failed map[string]fileState, act watchAction) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		errf("%v", err)
		return
	}
	present := make(map[string]bool, len(entries))
	// **A name that leaves the directory is forgotten** (`/pending 504`). Everything below is keyed by
	// path, and `processed` was never cleared, so a file deleted and later replaced by a NEW file of the
	// same name — the next scan from the same scanner, `invoice.pdf` again — was ignored for the rest
	// of the run. Absence is the one signal that cannot come from the watch's own in-place rewrite: that
	// is a rename over the entry, which is never missing from a listing.
	//
	// Deferred, so every return path prunes; the ReadDir failure above returns before this exists, so a
	// transient listing error never forgets the whole directory.
	//
	// **Absence, and deliberately not a changed fingerprint** (`/pending 508`). Treating changed bytes
	// as a new file is the only thing that would also catch a file COPIED over the old one in place,
	// and it cannot be told apart from the user editing a document that happens to be sitting in the
	// watched folder — the unrequested rewrite the startup rule above exists to prevent, which `--do
	// sanitize` makes irreversible. The two errors are not symmetric: a file the watch misses costs one
	// command the user can run by hand and can see was not run; a rewrite it should not have done is
	// silent and costs them the document.
	defer func() {
		for _, m := range []map[string]fileState{seen, failed} {
			for p := range m {
				if !present[p] {
					delete(m, p)
				}
			}
		}
		for p := range processed {
			if !present[p] {
				delete(processed, p)
			}
		}
	}()
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		present[path] = true
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
		// path the USER named, which is a different provenance from directory discovery, and
		// is why no watch action writes through it (watchTransform, `/pending 639`).
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
			// The name is whatever was dropped in the folder, not something the user typed (/pending 727).
			fmt.Printf("%s: %s\n", termText(path), termText(status))
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
	data, _, err := readNoFollowMode(path)
	return data, err
}

// readNoFollowMode is readNoFollow that also returns the permission bits of the file it READ —
// taken from the open handle, so they belong to the same inode as the bytes and not to whatever
// the entry names by the time anyone asks again. watchTransform carries them onto its rewrite.
func readNoFollowMode(path string) ([]byte, os.FileMode, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|oNoFollow, 0)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	// And a regular file: O_NOFOLLOW refuses a symlink, not a fifo or a device, either of
	// which would make the read block or return something that is not the document.
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := io.ReadAll(f)
	return data, info.Mode().Perm(), err
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
	// Through the atomic door, as watchUA's sidecar is and for its reason (`/pending 504`): this path is
	// the directory's, and `os.WriteFile` followed a symlink planted at FILE.pdf.ots to a file outside
	// it. A rename replaces the entry instead. Durable, because a proof is not re-derivable — it
	// anchors the moment it was made.
	if err := atomicfile.WriteDurable(proof, p, 0o644); err != nil {
		return "", err
	}
	return "timestamped", nil
}

// watchUA writes `nib ua`'s report beside path as FILE.ua.txt — `PLAN-accessibility.md` P10.S03. It
// writes only the sidecar, so a signed document is reported like any other. A document that fails a
// checked clause is a report, not a failed action: the status says which.
func watchUA(path string) (string, error) {
	data, err := readNoFollow(path)
	if err != nil {
		return "", err
	}
	table, notes, standing, err := uaReport(data)
	if err != nil {
		return "", err
	}
	body := strings.Join(table, "\n") + "\n\n" + strings.Join(notes, "\n") + "\n"
	// Through the atomic door — durable, as every write in this package is (atomicdurable_test.go) — and
	// NOT through writeAtomic: that one resolves a symlink first, deliberately, for a path the user named.
	// This path is the directory's, and a rename replaces the entry rather than following it, so a symlink
	// planted at FILE.ua.txt by the actor scanOnce's note describes is replaced instead of written through
	// to a file outside the directory.
	if err := atomicfile.WriteDurable(path+".ua.txt", []byte(body), 0o644); err != nil {
		return "", err
	}
	return watchUAStatus(standing, path), nil
}

// watchUAStatus is the watch's status line for a report's standing. "not PDF/UA" only over a failing clause: a clause
// nib could not settle is not a breach (/pending 698).
func watchUAStatus(standing uacheck.Standing, path string) string {
	switch standing {
	case uacheck.StandingAllCheckedPass:
		return "checked (every clause nib checks passes)"
	case uacheck.StandingFails:
		return "checked (not PDF/UA — see " + filepath.Base(path) + ".ua.txt)"
	}
	return "checked (PDF/UA not established — see " + filepath.Base(path) + ".ua.txt)"
}

func watchTransform(path string, fn func([]byte) ([]byte, error), done string) (string, error) {
	data, perm, err := readNoFollowMode(path)
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
	// **`atomicfile.WriteDurable`, NOT `writeAtomic`** (`/pending 639`) — the write half of the rule
	// readNoFollow is the read half of. `writeAtomic` ends in `ReplaceDurable`, which resolves a symlink
	// and writes to its TARGET: right for a path the user named, wrong here, where the path is the
	// directory's. The read refuses a link at the open, but `fn` runs for as long as a rewrite takes, and
	// the actor scanOnce's note describes can swap the entry for a symlink inside that window — the
	// rewrite then landed on a file outside the watched directory. A rename replaces the ENTRY, so a link
	// planted there is replaced rather than written through, as watchTimestamp's and watchUA's sidecars
	// already are. Wider on Windows, where oNoFollow is 0 and this was the only half that could hold.
	//
	// The mode is the one the READ handle saw, so a rewrite keeps the document's own bits — what
	// `ReplaceDurable` would have given it — without asking the entry, which by now may name something
	// else. Durable for `writeNamedMode`'s reason: this rename lands over the user's only copy.
	if err := atomicfile.WriteDurable(path, res, perm); err != nil {
		return "", err
	}
	return done, nil
}
