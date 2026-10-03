// Package instance is the rendezvous a second launch needs to find the first one.
//
// Nib binds `127.0.0.1:0` — a random loopback port, deliberately, so the app is never
// network-exposed and never collides with anything. The cost of that choice is that a
// second process has **no way at all** to find the running one: there is no fixed port
// to knock on and no name to look up. Everything P07 does rests on the record this
// package writes.
//
// **It replaced a Linux-only mechanism rather than extending one.** `internal/singleton`
// (deleted in P07.S03) found siblings by walking `/proc` and comparing `exe` symlinks,
// SIGTERMed them, and its non-Linux build was `func ReplaceOthers() int { return 0 }` —
// so on Windows, where `nib register` makes double-click the ordinary path, nothing had
// ever handled a second launch. A file plus a port works the same everywhere, which is
// the point, and it hands the document over instead of killing the process holding it.
package instance

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"nib/internal/addrscope"
	"nib/internal/atomicfile"
)

// Name is the record's filename inside the config directory — the same directory the
// vault lives in, which is already the private, per-user place this app keeps state.
const Name = "instance.json"

// HeaderToken carries the probe token on GET /api/instance, and HeaderHandoff the
// hand-off secret on POST /api/handoff. Named here, beside the record that holds the
// values, so the client and the server cannot disagree about them.
const (
	HeaderToken   = "X-Nib-Instance"
	HeaderHandoff = "X-Nib-Handoff"
)

// Record is what a running Nib publishes about itself: where to reach it, and a secret
// that proves the thing answering there is a Nib and not something else that happens to
// have taken the port.
//
// **There is no pid here, and its absence is deliberate.** A pid invites pid-based
// decisions — signal it, stat it, compare its executable — and pids are RECYCLED, so
// such a check passes against whatever process inherited the number. That is ADR-001's
// never-reuse hazard one level out: the comparison still succeeds, which is exactly what
// makes it dangerous. An address you can call and a token it must return is
// self-authenticating, and it answers the question that actually matters ("is a Nib
// listening there, and is it mine?") rather than a proxy for it ("does a process with
// this number exist?").
type Record struct {
	// Addr is the loopback host:port the running instance serves on.
	Addr string `json:"addr"`
	// Token authenticates a probe. It is not a capability: it proves identity to
	// GET /api/instance and grants nothing else.
	Token string `json:"token"`
	// Handoff authorises POST /api/handoff, and nothing else (D20).
	//
	// **Separate from Token deliberately, and the separation IS the decision.** The
	// probe token is presented to anything that asks whether this instance is alive;
	// if it also authorised "open this file", every read of the record would become a
	// capability grant, and the sentence above about Token would be false. Two fields
	// in one 0600 file cost nothing and keep a leak of the cheap, widely-presented
	// secret from handing over the expensive one.
	Handoff string `json:"handoff"`
	// Version is the running build. It is read by HandOff, and ONLY on the failure
	// path: when the running instance answers and refuses, the error names its version
	// and calls out a mismatch with the launching build.
	//
	// It deliberately does not gate the hand-off. This comment used to say the field
	// existed "so a launch can report a mismatch rather than hand a path to an instance
	// that may not understand it" — a promise nothing implemented, and one that would
	// have been wrong to implement as written: refusing on mismatch strands a
	// double-click with no window at all, on Windows, where the launch has no terminal
	// and the refusal would go nowhere a user looks. The route already answers the
	// question exactly — a shape it does not understand comes back as a non-200 — so a
	// version comparison guessing at the same thing in advance would be a second,
	// weaker mechanism that can refuse a hand-off which would have worked.
	Version string `json:"version"`
}

// ErrExists means a record is already there — another instance may be running. The
// caller probes it and decides: hand off to it, or take over from a stale one.
var ErrExists = errors.New("an instance record already exists")

// Path returns the record's location inside dir.
func Path(dir string) string { return filepath.Join(dir, Name) }

// NewToken mints a probe token.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Create publishes the record, refusing if one is already present.
//
// **Exclusive, not a plain write**, because the refusal is the useful outcome: two nibs
// starting at once must not both believe they are the primary, and last-writer-wins
// would leave the loser serving on a port nothing points at. The caller that gets
// ErrExists probes the incumbent and either hands off or, finding it stale, removes the
// record and tries again — which is a decision the caller must make explicitly rather
// than one this function should make for it.
//
// **The record lands WHOLE, through `atomicfile.CreateFrom`** (/pending 606, 813): written to a
// temp file and hard-linked into place, which the kernel refuses while a record is there. It used
// to be an O_EXCL create followed by a write, so a launch reading in between saw an empty file —
// and since an unreadable record is now cleared rather than run beside (RemoveDamaged), a reader
// must never be able to mistake a record being written for a damaged one. Deliberately not
// durable: a power loss that eats the record also ate the process it described, and whatever it
// leaves — nothing, or an empty file — is exactly what the next launch now clears.
//
// 0600 for the same reason the vault directory is private: the token is a secret, and a
// world-readable one is not.
func Create(dir string, rec Record) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := atomicfile.CreateFrom(Path(dir), bytes.NewReader(data), 0o600, 0, nil); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrExists
		}
		return err
	}
	return nil
}

// ErrDamaged means a record is on disk and cannot be used: not JSON, or missing the address or
// the token a probe needs. It is the caller's cue for RemoveDamaged — never for "absent".
var ErrDamaged = errors.New("the instance record is damaged")

// Read returns the published record, or an error when there is none. A record that is there and
// cannot be used is ErrDamaged.
func Read(dir string) (Record, error) {
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		return Record{}, err
	}
	rec, err := parse(data)
	if err != nil {
		return rec, fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	// Handoff may be empty on a record written by an older build. Probing still works;
	// handing off does not, and the caller finds that out when it tries — which is
	// better than refusing to read a record that is valid for what it was written for.
	return rec, nil
}

// parse is the one reading of a record's bytes, shared by Read and RemoveDamaged so "damaged"
// means the same thing to the launch that finds it and the door that clears it.
func parse(data []byte) (Record, error) {
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, err
	}
	if rec.Addr == "" || rec.Token == "" {
		return rec, errors.New("instance record is incomplete")
	}
	return rec, nil
}

// Remove deletes the record ONLY if it still carries token, and reports whether it did.
//
// **It used to delete whatever record was on disk (/pending 630, 606)**, while each of its two
// callers is removing ONE PARTICULAR record: an exiting instance its own, a launch the stale one
// it just probed. Neither may take a record somebody else has published since. The live failure:
// a launch's probe finds an exiting instance's listener already closed (`srv.Close()` runs before
// the deferred removal), clears that record and publishes its own — and the exiting instance's
// deferred Remove then deleted the NEW one, so the next launch found nothing and started a third
// primary beside a running Nib that no longer owned the rendezvous.
//
// Absent is (false, nil): a clean exit and a crash-then-cleanup should not be distinguishable to
// the caller, and neither is an error.
func Remove(dir, token string) (bool, error) {
	return removeIf(dir, func(data []byte) bool {
		rec, err := parse(data)
		return err == nil && token != "" && TokenMatches(rec.Token, token)
	})
}

// RemoveDamaged deletes the record only if it is STILL unreadable, and reports whether it did
// (/pending 813).
//
// **An unreadable record used to be "treated as absent" and left in place**, which is not absent
// at all: Create is exclusive, so every later launch found it, skipped the hand-off, failed to
// publish, and ran alongside whatever else was running — for good. The comment said a damaged
// record must never become "delete this file to start Nib", and leaving it there made it exactly
// that, minus the instruction. Clearing it is safe only because Create lands a record whole: an
// unreadable file is never one being written.
func RemoveDamaged(dir string) (bool, error) {
	return removeIf(dir, func(data []byte) bool {
		_, err := parse(data)
		return err != nil
	})
}

// removeIf is the one door that deletes the record, and it judges the bytes it actually REMOVED
// rather than a read taken beforehand (ADR-009: Remove and RemoveDamaged both route here).
//
// A read-then-delete leaves a window in which another process replaces the record and the delete
// takes the replacement. So the record is first RENAMED aside — atomic, and afterwards this call
// holds the only name for those bytes — and judged there. A record that fails the judgement is
// put back by hard link, which refuses if a new record has landed meanwhile (that newer one is the
// live one, and the moved-aside bytes are dropped). Where the filesystem has no hard links it is
// put back by rename when the name is still free, which narrows the window to two calls rather
// than closing it — the same fallback `atomicfile.CreateFrom` declares.
func removeIf(dir string, remove func(data []byte) bool) (bool, error) {
	path := Path(dir)
	suffix, err := NewToken()
	if err != nil {
		return false, err
	}
	aside := filepath.Join(dir, ".instance-"+suffix[:16]+".tmp")
	if err := os.Rename(path, aside); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	data, rerr := os.ReadFile(aside)
	if rerr == nil && remove(data) {
		return true, os.Remove(aside)
	}
	if lerr := os.Link(aside, path); lerr == nil || errors.Is(lerr, fs.ErrExist) {
		return false, os.Remove(aside)
	}
	if _, serr := os.Lstat(path); os.IsNotExist(serr) {
		return false, os.Rename(aside, path)
	}
	return false, os.Remove(aside)
}

// TokenMatches compares a presented token against the expected one in constant time.
//
// The same primitive `requireUnlocked` uses for CSRF, and for the same reason. Anyone
// who can read a 0600 file in the config directory already has the token and has won —
// but a byte-by-byte compare answering 200 or 403 is a token ORACLE for someone who
// cannot, and the cost of not offering one is a single call.
func TokenMatches(presented, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) == 1
}

// probeTimeout bounds a probe. It is a loopback request to a process that is either
// answering immediately or not there, so the only thing this really bounds is the
// pathological case — a port taken by something that accepts and then says nothing. A
// launch must not hang behind that: the user double-clicked a file.
//
// **That premise needs the route to take no lock, and it used to take the global one**
// (/pending 783): `handleInstance` read its token under `s.mu`, so any request holding `s.mu`
// for two seconds made a live Nib answer like a dead one. The token is now read lock-free. And
// running out of time is reported as Unknown, never as Gone, so the bound costs a launch at most
// a second Nib beside the first — announced in the log — and never the first one's record.
//
// A var only so a test can shorten it.
var probeTimeout = 2 * time.Second

// ErrNotLoopback refuses a record naming anything but a loopback host:port.
var ErrNotLoopback = errors.New("the instance record does not name a loopback address")

// checkLoopback is the one address rule for every request made to a recorded instance.
//
// A record must name a loopback address. Nothing writes anything else today, but a tampered or
// hand-edited record must not turn a probe or a hand-off into an outbound request to somewhere
// else entirely — nib is never network-exposed, and that includes as a client.
func checkLoopback(addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("%w: %v", ErrNotLoopback, err)
	}
	if !addrscope.Loopback(addr) {
		return ErrNotLoopback
	}
	return nil
}

// Liveness is Probe's answer. There are three because "not alive" was two different facts and
// one of them must never remove a record.
type Liveness int

const (
	// Alive: a Nib answered at the recorded address with the recorded token.
	Alive Liveness = iota
	// Gone: nothing is there (refused, reset, closed), or something that is not this record's
	// Nib answered. The record is stale and may be cleared.
	Gone
	// Unknown: the address did not answer within probeTimeout. Something may be there and busy,
	// so the record is NOT stale — clearing it is how a slow Nib lost its rendezvous to a
	// second one (/pending 783, 630).
	Unknown
)

func (l Liveness) String() string {
	switch l {
	case Alive:
		return "alive"
	case Gone:
		return "gone"
	}
	return "unknown"
}

// Probe asks whether the instance the record names is alive AND is a Nib holding this
// record's token.
//
// **Both halves matter and they fail differently.** "Something is listening on that
// port" is not the question — a random port freed by a dead Nib is reassigned by the
// kernel to whatever asks next, so a bare connect would say yes to an unrelated service
// and a launch would hand it a document path. The token is what makes the answer mean
// "this is my Nib". And "the port is refused" is the ordinary stale case: the Nib exited,
// the record outlived it, and the caller should take over.
//
// It works against a LOCKED instance, because /api/instance is public. A probe that
// needed an unlocked vault would report a locked Nib as dead, and the taking-over launch
// would replace the user's session with a fresh locked one.
//
// **It answers one of THREE things, not alive-or-not** (/pending 783): "did not answer in time"
// used to fold into "not alive", and the caller removes a record that is not alive.
func Probe(rec Record) Liveness {
	if rec.Addr == "" || rec.Token == "" {
		return Gone
	}
	if checkLoopback(rec.Addr) != nil {
		return Gone
	}
	req, err := http.NewRequest(http.MethodGet, "http://"+rec.Addr+"/api/instance", nil)
	if err != nil {
		return Gone
	}
	req.Header.Set(HeaderToken, rec.Token)
	c := &http.Client{Timeout: probeTimeout}
	resp, err := c.Do(req)
	if err != nil {
		if isTimeout(err) {
			return Unknown
		}
		return Gone
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return Alive
	}
	return Gone
}

// isTimeout reports a deadline, as distinct from a refusal. Read off net.Error rather than a
// platform errno, because "connection refused" is ECONNREFUSED on one OS and WSAECONNREFUSED on
// another, while a timeout is the same interface everywhere.
func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// ErrHandOffUnanswered means the hand-off request was sent to a Nib that had just answered a probe,
// and no reply came within handoffTimeout. The request is still being served there — the route
// does not stop when its caller does — so the document opens in that Nib, and the caller must NOT
// become a second primary and open it again (/pending 783).
var ErrHandOffUnanswered = errors.New("the running Nib took the hand-off but did not answer in time")

// handoffTimeout bounds the wait for a hand-off's reply. Longer than a probe's on purpose: the
// route reads, converts, verifies and installs the file inside the request, so its reply is as
// slow as the document is large, and the reply is only needed for the launch key of the window
// the caller opens next. A var only so a test can shorten it.
var handoffTimeout = 30 * time.Second

// HandOff asks the instance the record names to open path (empty means "just surface
// yourself"), and reports what it did.
//
// The results are distinct because the caller has a decision to make and "it
// worked" does not answer it: `opened` and `focused` mean exit; `queued` means exit, the
// user will see it when they unlock; `refused` means exit but say so; `window` (an empty
// path) means exit, having been given a window. Only an error means "that instance is not
// usable — become the primary".
//
// `launch` is the single-use key for the window the caller opens next (ADR-053): the running
// instance serves its API only to a page that traded one, so a window opened without it
// could do nothing.
// HandOff posts path to the running instance. `myVersion` is the LAUNCHING build, used
// only to annotate a refusal — see Record.Version.
func HandOff(rec Record, path, myVersion string) (result, reason, launch string, err error) {
	if rec.Handoff == "" {
		return "", "", "", errors.New("the instance record carries no hand-off secret")
	}
	// The same door as Probe's (/pending 502). The one production caller probes first, so this
	// is not reachable from it today — but HandOff sends the hand-off SECRET, the more sensitive
	// of the two requests, and was the one that did not check where it was sending it.
	if err := checkLoopback(rec.Addr); err != nil {
		return "", "", "", err
	}
	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return "", "", "", err
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+rec.Addr+"/api/handoff", bytes.NewReader(body))
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderHandoff, rec.Handoff)
	c := &http.Client{Timeout: handoffTimeout}
	resp, err := c.Do(req)
	if err != nil {
		if isTimeout(err) {
			return "", "", "", fmt.Errorf("%w: %v", ErrHandOffUnanswered, err)
		}
		return "", "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// The running instance ANSWERED and refused, which is the one failure where its
		// version is evidence: a build that does not know this route answers 404 here.
		// A connection error above is deliberately left unannotated — nothing answered,
		// so there is no version involved and naming one would invent a cause.
		//
		// Its version is reported either way, because "which instance refused" is a fact
		// worth having; the MISMATCH is called out only when there is one, so an
		// identical-version failure does not read as version skew and send the reader
		// off in the wrong direction.
		running := rec.Version
		if running == "" {
			running = "unknown"
		}
		if myVersion != "" && running != myVersion {
			return "", "", "", fmt.Errorf("the running instance (version %s) refused the hand-off from this build (version %s) with HTTP %d — the two builds differ, which is the likeliest cause", running, myVersion, resp.StatusCode)
		}
		return "", "", "", fmt.Errorf("the running instance (version %s) refused the hand-off with HTTP %d", running, resp.StatusCode)
	}
	var out struct {
		Result string `json:"result"`
		Reason string `json:"reason"`
		Launch string `json:"launch"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return "", "", "", err
	}
	return out.Result, out.Reason, out.Launch, nil
}
