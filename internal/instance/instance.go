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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
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
// **O_EXCL, not a plain write**, because the refusal is the useful outcome: two nibs
// starting at once must not both believe they are the primary, and last-writer-wins
// would leave the loser serving on a port nothing points at. The caller that gets
// ErrExists probes the incumbent and either hands off or, finding it stale, removes the
// record and tries again — which is a decision the caller must make explicitly rather
// than one this function should make for it.
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
	f, err := os.OpenFile(Path(dir), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return ErrExists
		}
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(Path(dir))
		return err
	}
	return f.Close()
}

// Read returns the published record, or an error when there is none.
func Read(dir string) (Record, error) {
	var rec Record
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, err
	}
	if rec.Addr == "" || rec.Token == "" {
		return rec, errors.New("instance record is incomplete")
	}
	// Handoff may be empty on a record written by an older build. Probing still works;
	// handing off does not, and the caller finds that out when it tries — which is
	// better than refusing to read a record that is valid for what it was written for.
	return rec, nil
}

// Remove deletes the record. Absent is success: a clean exit and a crash-then-cleanup
// should not be distinguishable to the caller, and neither is an error.
func Remove(dir string) error {
	err := os.Remove(Path(dir))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
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
const probeTimeout = 2 * time.Second

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
func Probe(rec Record) bool {
	if rec.Addr == "" || rec.Token == "" {
		return false
	}
	// A record must name a loopback address. Nothing writes anything else today, but a
	// tampered or hand-edited record must not turn a probe into an outbound request to
	// somewhere else entirely — nib is never network-exposed, and that includes as a
	// client.
	host, _, err := net.SplitHostPort(rec.Addr)
	if err != nil {
		return false
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		if host != "localhost" {
			return false
		}
	}
	req, err := http.NewRequest(http.MethodGet, "http://"+rec.Addr+"/api/instance", nil)
	if err != nil {
		return false
	}
	req.Header.Set(HeaderToken, rec.Token)
	c := &http.Client{Timeout: probeTimeout}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// HandOff asks the instance the record names to open path (empty means "just surface
// yourself"), and reports what it did.
//
// The four results are distinct because the caller has a decision to make and "it
// worked" does not answer it: `opened` and `focused` mean exit; `queued` means exit, the
// user will see it when they unlock; `refused` means exit but say so. Only an error
// means "that instance is not usable — become the primary".
// HandOff posts path to the running instance. `myVersion` is the LAUNCHING build, used
// only to annotate a refusal — see Record.Version.
func HandOff(rec Record, path, myVersion string) (result string, reason string, err error) {
	if rec.Handoff == "" {
		return "", "", errors.New("the instance record carries no hand-off secret")
	}
	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+rec.Addr+"/api/handoff", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderHandoff, rec.Handoff)
	c := &http.Client{Timeout: probeTimeout}
	resp, err := c.Do(req)
	if err != nil {
		return "", "", err
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
			return "", "", fmt.Errorf("the running instance (version %s) refused the hand-off from this build (version %s) with HTTP %d — the two builds differ, which is the likeliest cause", running, myVersion, resp.StatusCode)
		}
		return "", "", fmt.Errorf("the running instance (version %s) refused the hand-off with HTTP %d", running, resp.StatusCode)
	}
	var out struct {
		Result string `json:"result"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return "", "", err
	}
	return out.Result, out.Reason, nil
}
