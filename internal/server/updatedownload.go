package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nib/internal/atomicfile"
	// Aliased, because this package already declares `type browser interface` (discover.go) — the
	// discovery socket's read surface, which has its own guard reasoning and is the older claim on
	// the name.
	nibbrowser "nib/internal/browser"
	"nib/internal/safe"
)

// Downloading a release, with progress — the other half of the update pill (ADR-039).
//
// ── Why this is here at all, when the browser could do it ────────────────────
// It could not. The pill used to hand the asset URL to `location.assign`, so the BROWSER fetched
// the bytes and Nib never saw them — which is exactly why there was no progress, no destination and
// no next step: a page cannot read the browser's download folder, and cannot observe a transfer it
// does not own. The obvious fix — fetch it in the page and post the bytes to `/api/write` — was
// measured and refused: a real release asset answers with **no `Access-Control-Allow-Origin` on
// either hop** (the github.com 302 or the release-assets.githubusercontent.com 200), so a page
// fetch is blocked outright. Server-side is forced, not preferred.
//
// ── What this deliberately does NOT do ───────────────────────────────────────
// **It never executes, installs, or replaces anything**, and the file it writes is **not
// executable** (0o644). `build.sh` publishes the binaries and the .deb with no checksum, no
// signature and no manifest, so there is nothing to verify a downloaded artifact against — and a
// ~95 MB binary fetched over the network and run unverified is the supply-chain shape no dialog
// makes safe. README's promise ("Nib only notifies and downloads — it never installs or replaces
// itself") stands, and the UI offers "Show in folder" rather than "Run".
//
// ── The URL is the SERVER's, never the client's ──────────────────────────────
// The request carries a destination folder and nothing else. The asset URL is re-resolved here by
// the same `latestRelease` + `assetURL` the check uses. Accepting a URL from the client would turn
// an authenticated loopback route into a general "fetch this and write it to my disk" primitive —
// the request body would choose both the host and the path.

// maxAssetBytes bounds what this route will write. The largest published asset today is ~95 MB
// (measured: 94,658,744 bytes for a linux-amd64 build); 512 MiB is the same ceiling ADR-005 puts on
// the document pool, chosen here for the same reason — a remote `Content-Length` is a claim, and a
// stream that ignores it fills the disk.
const maxAssetBytes = 512 << 20

// downloadEvent is what a window sees on the stream. Sent as `event: download`.
//
// `Percent` is the throttle key, not a decoration: a frame per `io.Copy` chunk would push thousands
// of events into a stream every window holds open and re-announce an aria-live region continuously.
//
// **Every published field has a reader (/pending 646).** `active` and `name` were sent and never
// read, and `published.test.mjs` passed them only because `.active` is a CSS class in app.js. Both
// are gone: "active" was exactly `status == "running"` (begin is its only writer of running, and
// every exit goes through finish), so it is now that method; the name was set and read by nothing.
type downloadEvent struct {
	Status  string `json:"status"`         // running | done | failed | cancelled
	Path    string `json:"path,omitempty"` // the full destination, shown to the user
	Done    int64  `json:"done"`           // bytes so far; the only progress a length-less transfer has
	Total   int64  `json:"total"`          // 0 when the server did not state a length
	Percent int    `json:"percent"`
	Problem string `json:"problem,omitempty"` // a sentence, when Status is failed
}

// active reports whether the download slot is held — the state `begin` claims and `finish` releases.
func (e downloadEvent) active() bool { return e.Status == "running" }

// downloadState is the process's one download, broadcast to every window.
//
// The wake mechanism is `session.armedChangedLocked`'s, deliberately: close the channel and nil it,
// and every waiter holding it wakes. `/pending 464` records what the other order costs — subscribe
// AFTER reading and a change landing in the window between them is a wakeup nobody receives.
type downloadState struct {
	mu     sync.Mutex
	ev     downloadEvent
	watch  chan struct{}
	cancel context.CancelFunc
}

func (d *downloadState) changedLocked() {
	if d.watch != nil {
		close(d.watch)
		d.watch = nil
	}
}

// changes returns a channel closed on the next change. Same contract as armedChanges.
func (d *downloadState) changes() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.watch == nil {
		d.watch = make(chan struct{})
	}
	return d.watch
}

func (d *downloadState) snapshot() downloadEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ev
}

// set replaces the state and wakes the windows. Callers hold nothing.
func (d *downloadState) set(ev downloadEvent) {
	d.mu.Lock()
	d.ev = ev
	d.changedLocked()
	d.mu.Unlock()
}

// begin claims the single download slot. It refuses a second one rather than racing two writers
// onto one path — a refusal a user can act on, where two interleaved streams is a corrupt file.
func (d *downloadState) begin(cancel context.CancelFunc, path string, total int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ev.active() {
		return false
	}
	d.ev = downloadEvent{Status: "running", Path: path, Total: total}
	d.cancel = cancel
	d.changedLocked()
	return true
}

// finish leaves a terminal state on the record. Every exit lands here — success, failure, cancel —
// because a dialog that never receives a terminal event spins forever (the gap-down this closes).
func (d *downloadState) finish(status, problem string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ev.Status = status
	d.ev.Problem = problem
	d.cancel = nil
	d.changedLocked()
}

// progress records bytes and wakes the windows ONLY when the whole percent moves — or, with no
// stated length, when a whole MiB is crossed.
//
// **The length-less half (/pending 646).** With `Total` 0 the percent is 0 forever, so the throttle
// returned on every chunk and a transfer whose server sent no `Content-Length` emitted no progress
// at all: "Downloading…" for ninety-odd megabytes. A MiB is the unit the dialog shows, so it is the
// unit that moves the line.
func (d *downloadState) progress(done int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.ev.active() {
		return
	}
	prev := d.ev.Done
	d.ev.Done = done
	if d.ev.Total <= 0 {
		if done>>20 == prev>>20 {
			return
		}
		d.changedLocked()
		return
	}
	pct := 0
	if d.ev.Total > 0 {
		pct = int(done * 100 / d.ev.Total)
	}
	if pct == d.ev.Percent {
		return // the throttle: no frame, no aria-live churn
	}
	d.ev.Percent = pct
	d.changedLocked()
}

// setTotal records the length the server stated, once the headers are in. The slot is claimed before
// the request goes out, so the length is not known at `begin`.
func (d *downloadState) setTotal(total int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.ev.active() {
		return
	}
	d.ev.Total = total
	d.changedLocked()
}

// downloadHeaderTimeout bounds the wait for the release server's response HEADERS — never the body.
// A var so a test can shorten it.
var downloadHeaderTimeout = 30 * time.Second

// stop cancels a running download, if there is one.
func (d *downloadState) stop() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.ev.active() || d.cancel == nil {
		return false
	}
	d.cancel()
	return true
}

// assetName is the file name to write, derived from the asset URL's last segment.
//
// **It is remote-controlled** — it comes from GitHub's JSON — so it is reduced to a base name and
// refused if anything but a plain name survives. `containedJoin` is the second guard, not the first:
// a name is rejected here rather than merely contained, because a download that silently landed
// somewhere other than the folder the dialog named would be worse than a refusal.
func assetName(rawURL string) (string, bool) {
	// Query and fragment first: they are not part of the name.
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		rawURL = rawURL[:i]
	}
	// **A trailing slash is refused, not trimmed.** Trimming it walks UP: ".../d/" then yields "d",
	// so a URL naming no file at all would have produced a file named after a path component.
	// Caught by this function's own test.
	if rawURL == "" || strings.HasSuffix(rawURL, "/") {
		return "", false
	}
	last := rawURL[strings.LastIndex(rawURL, "/")+1:]
	// **Decoded BEFORE it is judged.** `%2e%2e` is `..` and reached `containedJoin` as a literal
	// name otherwise — the escape survives every check that reads the raw segment. A segment that
	// does not decode is refused rather than used as-is.
	decoded, err := url.PathUnescape(last)
	if err != nil {
		return "", false
	}
	last = strings.TrimSpace(decoded)
	if last == "" || last == "." || last == ".." || last != filepath.Base(last) {
		return "", false
	}
	if strings.ContainsAny(last, `/\`) {
		return "", false
	}
	return last, true
}

// handleUpdateDownload fetches the release asset for this machine into a folder the user chose.
//
// **`requireUnlocked` + CSRF, and NOT the check route's `requirePublicLoopback`.** That route is
// public because it only queries out; this one writes bytes into the user's filesystem, which is the
// same act `POST /api/write` performs and takes the same guard.
func (s *Server) handleUpdateDownload(w http.ResponseWriter, r *http.Request) {
	dir, ok := destDir(w, r)
	if !ok {
		return
	}
	// Resolved HERE, never taken from the request — see the file header.
	rel, err := latestRelease()
	if err != nil {
		httpError(w, http.StatusBadGateway, "could not reach the update server")
		return
	}
	if rel == nil {
		httpError(w, http.StatusNotFound, "no release has been published")
		return
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	if !versionLess(s.version, latest) {
		httpError(w, http.StatusConflict, "this is already the latest version")
		return
	}
	url := assetURL(runtime.GOOS, runtime.GOARCH, managedInstall(), rel.Assets)
	if url == "" {
		httpError(w, http.StatusNotFound, "no download matches this system")
		return
	}
	name, ok := assetName(url)
	if !ok {
		httpError(w, http.StatusBadGateway, "the release names a file Nib will not write")
		return
	}
	target, ok := containedJoin(dir, name)
	if !ok {
		httpError(w, http.StatusBadRequest, "unsafe file name")
		return
	}
	// Refused before any bytes move, so a collision is not discovered at 94% — and 412 rather than
	// 409, the same split `handleWriteFile` draws: 409 means "the server no longer holds this
	// document" and the client hooks it to reconcile.
	if _, err := os.Stat(target); err == nil {
		httpError(w, http.StatusPreconditionFailed, "a file of that name is already there")
		return
	}
	// SELF-OVERWRITE EXEMPT: this route replaces NOTHING, and there is no source to protect.
	//
	// /pending 569's rule stops a derived name landing on the document it was derived from. The
	// bytes here come off the network rather than out of a document, so there is no such file — and
	// the 412 above refuses every existing name, not merely the one that would be a source, which
	// is strictly stronger than the check the rule asks for.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		httpError(w, http.StatusInternalServerError, "could not create folder")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		httpError(w, http.StatusInternalServerError, "could not start the download")
		return
	}
	// **The slot is claimed, and `cancel` registered, BEFORE the request goes out (/pending 589,
	// 805, 821).** It used to be claimed after the response headers, so the header wait had no
	// deadline and nothing Cancel could reach: a host that accepted the connection and never
	// answered held this handler for as long as the kernel kept the socket, and Cancel answered
	// "nothing running" over it. Now a Cancel in that phase cancels the request, and a host that
	// sends no headers within `downloadHeaderTimeout` is given up on. Only the HEADERS are timed —
	// a ~95 MB body on a slow line is legitimate, and Cancel reaches it.
	if !s.dl.begin(cancel, target, 0) {
		cancel()
		httpError(w, http.StatusConflict, "a download is already running")
		return
	}
	fail := func(code int, status, msg string) {
		cancel()
		s.dl.finish(status, msg)
		httpError(w, code, msg)
	}
	var timedOut atomic.Bool
	headerTimer := time.AfterFunc(downloadHeaderTimeout, func() {
		defer safe.Recover("release download header deadline")
		timedOut.Store(true)
		cancel()
	})
	resp, err := http.DefaultClient.Do(req)
	headerTimer.Stop()
	if err != nil {
		switch {
		case timedOut.Load():
			fail(http.StatusGatewayTimeout, "failed", "the download server did not answer")
		case ctx.Err() != nil:
			fail(http.StatusConflict, "cancelled", "the download was cancelled")
		default:
			fail(http.StatusBadGateway, "failed", "could not reach the download server")
		}
		return
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		// The asset URL is signed and time-limited (~1 hour), so an expired one is the ordinary
		// case rather than an outage, and the message says which to do about it.
		fail(http.StatusBadGateway, "failed", "the download link was refused — click the pill again")
		return
	}
	// A timer that fired after the headers arrived has cancelled the body too; the stream then
	// fails as a cancel and says so, which is honest about a transfer that is no longer running.
	s.dl.setTotal(resp.ContentLength)
	// The request returns as soon as the transfer is under way; progress and the outcome ride the
	// window stream. A synchronous response would hold a request open for ~95 MB and tell the user
	// nothing until it ended.
	go s.runDownload(resp, target)
	writeJSON(w, map[string]any{"status": "started", "name": name, "path": target})
}

// runDownload streams the body to disk, reporting progress, and leaves exactly one terminal state.
func (s *Server) runDownload(resp *http.Response, target string) {
	// **`safe.Recover` FIRST, and the repo's own rather than a local one.** The goroutine law wants
	// it as the first statement so every other defer still runs as the stack unwinds — and the
	// first draft of this file wrote its own `safeRecoverDownload` helper, which is the
	// second-implementation shape the guard exists to catch. It caught it.
	defer safe.Recover("release download")
	// Registered SECOND so it runs FIRST on the way out: a panic must still leave a terminal state,
	// or the dialog spins forever on a download that is no longer happening.
	//
	// It removes no file. The door's temp is removed by the door's own defer, which a panic runs; and
	// what is at `target` may be a file the door REFUSED to replace, so deleting it here would be the
	// overwrite the door exists to prevent, done by other means.
	defer func() {
		if s.dl.snapshot().active() {
			s.dl.finish("failed", "the download stopped unexpectedly")
		}
	}()
	defer resp.Body.Close()

	// Written beside the target and linked into place on success, so a failure or a cancel never leaves a
	// truncated file wearing the name of a real release — the gap-down that matters most here,
	// because the artifact a user finds in that folder is one they might run.
	// **Through the atomic door, not a hand-rolled temp-plus-rename.** The first draft wrote
	// `<name>.part` and renamed it here, and both `atomicdoor_test.go` and `atomicroute_test.go`
	// refused it as a second implementation of the rule `internal/atomicfile` owns. The door had no
	// streaming form because every existing caller holds its bytes already; `CreateFrom` is that
	// form, so the rename lives in the door and this package calls `os.Rename` nowhere.
	//
	// Non-durable, matching `Write`: a release artifact is re-downloadable, so it is not the only
	// copy of anything and does not earn an fsync.
	//
	// 0o644 — NOT executable. Nothing published verifies these bytes, so Nib does not hand back a
	// ready-to-run artifact (ADR-039).
	//
	// And it REFUSES a name that appeared while the bytes were moving: the 412 in the handler is a
	// stat taken before the transfer, and only the door can answer at the moment the file lands.
	written, err := atomicfile.CreateFrom(target, resp.Body, 0o644, maxAssetBytes, s.dl.progress)
	if err != nil {
		// The door removes its own temp on every failure path, so there is nothing to clean up
		// here and nothing half-written wearing a release's name.
		switch {
		case errors.Is(err, context.Canceled):
			s.dl.finish("cancelled", "")
		case errors.Is(err, fs.ErrExist):
			s.dl.finish("failed", "a file of that name appeared there during the download, and Nib will not replace it")
		case errors.Is(err, atomicfile.ErrTooLarge):
			s.dl.finish("failed", "the download was larger than Nib will write")
		default:
			s.dl.finish("failed", "the download stopped before it finished")
		}
		return
	}
	// A short read against a stated length is a truncated file, not a success. The door has already
	// renamed it into place, so this removes it: a file that is present and wrong is worse than
	// absent, because the user might run it.
	if total := s.dl.snapshot().Total; total > 0 && written != total {
		os.Remove(target)
		s.dl.finish("failed", "the download ended early")
		return
	}
	s.dl.finish("done", "")
}

// handleUpdateDownloadCancel stops a running download. Idempotent: cancelling nothing is 200, so a
// dialog closing twice is not an error.
func (s *Server) handleUpdateDownloadCancel(w http.ResponseWriter, r *http.Request) {
	stopped := s.dl.stop()
	writeJSON(w, map[string]any{"cancelled": stopped})
}

// handleUpdateReveal shows the finished download's FOLDER in the desktop's file manager.
//
// **It takes no path from the request**, for the reason the download takes no URL: a route that
// opened whatever folder a caller named would be a general "show me this directory" primitive
// reachable from any page in the user's browser that got past the guard. The only folder it will
// open is the one holding the download this process just wrote.
//
// **And it opens the folder, never the file.** `browser.OpenFolder` cannot express the latter; a
// desktop asked to open a binary or a .deb may run it or hand it to an installer, which is the act
// this feature refuses while nothing verifies the bytes.
func (s *Server) handleUpdateReveal(w http.ResponseWriter, r *http.Request) {
	ev := s.dl.snapshot()
	if ev.Status != "done" || ev.Path == "" {
		httpError(w, http.StatusConflict, "there is no finished download to show")
		return
	}
	if err := nibbrowser.OpenFolder(filepath.Dir(ev.Path)); err != nil {
		httpError(w, http.StatusInternalServerError, "could not open that folder")
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

// marshalDownload is the stream's payload for this state.
func marshalDownload(ev downloadEvent) ([]byte, error) { return json.Marshal(ev) }
