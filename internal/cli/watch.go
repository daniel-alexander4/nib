package cli

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"nib/internal/ots"
	"nib/internal/pdfops"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
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

// watchTimestamp writes a .ots proof beside path, skipping a file that already
// has one (so restarting the watch doesn't re-stamp).
func watchTimestamp(path string) (string, error) {
	proof := path + ".ots"
	if _, err := os.Stat(proof); err == nil {
		return "skipped (.ots exists)", nil
	}
	data, err := os.ReadFile(path)
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
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
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
