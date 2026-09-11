package ceremony

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nib/internal/atomicfile"
)

// Close-out: what happens to a ceremony directory after the proceeding has ended (D29, P08.S06).
//
// D29 states the lifecycle once — **end state → delivery round → close-out** — and puts the pin
// drop and the prune at close-out. This file is the prune half, and the whole of it is one idea:
// **the prune MOVES, it does not delete.**
//
// The reason is not caution, it is arithmetic. On declined, expired and abandoned there is no
// delivery round, so nothing has carried this machine's copy anywhere; and on every machine but
// the convener's, `~/nib/ceremonies/<id>/document.pdf` is the ONLY place that party's own signed
// contribution exists. `RemoveMirror` — whose doc comment says *"D29's close-out prune is
// P08.S06's, and this function is what it will call"* — is an `os.RemoveAll`, and calling it as
// that comment invites is a user's signature destroyed by their own software's tidying.
//
// **The mirror holds no key material, which is what makes a move sufficient.** D29's design has
// the invitation secret in the vault and the mirror as ordinary files; `WriteMirror`'s own doc
// says so. So there is nothing in the directory that has to be destroyed, and the correct prune
// is `os.Rename` — atomic, one syscall, same filesystem, and it preserves the record and the
// termination alongside the document so the preserved contribution stays *verifiable* rather than
// becoming an orphan PDF. The vault teardown that runs beside this one still deletes: pins,
// secrets and stored invitations are key material and D29 says they go.
//
// **`ListStored` reads only `~/nib/ceremonies`**, so a closed-out ceremony leaves the listing by
// construction. That is deliberate — a filter would be a second place the rule lived.

// endedDir is the sibling directory closed-out ceremonies move into.
//
// A sibling of `ceremonies/` rather than a subdirectory of it, because `ListStored` enumerates
// `ceremonies/`'s entries and skips anything `ValidID` rejects — a subdirectory would be silently
// skipped today and would become a listing entry the moment that filter was relaxed. And not
// `~/nib/` itself, which is where the user's own saved documents land: a close-out must not put
// files in the folder a user browses for their own work.
const endedDir = "ended"

var (
	// ErrRootNotAbsolute refuses a destructive operation under a relative root.
	//
	// **`defaultOutputDir` returns a bare `"nib"` when `os.UserHomeDir()` fails**, and that string
	// reaches twenty-four production call sites. Everywhere else it is merely wrong — a document
	// saved beside the binary. Here it would make the rename relative to whatever the process's
	// working directory happens to be, which for a desktop launcher is not a thing the user chose.
	//
	// **The refusal lives at this one door and not at the resolver** (ADR-009: a rule gets one
	// door, and the guard checks the door). Changing `defaultOutputDir` to return an error would
	// touch all twenty-four sites to guard the one that is destructive.
	ErrRootNotAbsolute = errors.New("this operation needs an absolute output directory")

	// ErrAlreadyClosedOut reports a destination that already exists.
	//
	// Refused rather than merged or overwritten. A second close-out of the same id means either a
	// re-run over a ceremony already dealt with — where the first move's contents are the ones to
	// keep — or an id collision, where overwriting destroys the earlier party's contribution. The
	// two are indistinguishable from here and both want the same answer.
	ErrAlreadyClosedOut = errors.New("this ceremony has already been closed out")

	// ErrReceiptConflict reports a second receipt naming a different end state. See WriteReceipt.
	ErrReceiptConflict = errors.New("this ceremony is already recorded locally as having ended " +
		"in a different state")
)

// EndedDir is where a closed-out ceremony's directory lives after CloseOutMirror.
//
// Exported so a caller — a test, or a surface telling the user where their copy went — can name
// the path without rebuilding the join and drifting from it.
func EndedDir(root, id string) (string, error) {
	if err := ValidID(id); err != nil {
		return "", err
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("%w: %q", ErrRootNotAbsolute, root)
	}
	return filepath.Join(root, endedDir, id), nil
}

// CloseOutMirror moves a ceremony's directory out of the live set, preserving everything in it.
//
// The two refusals come FIRST and neither is advisory: a relative root and an id `ValidID` rejects
// are both ways for a path join to end up somewhere nobody named, and this function's next act is
// destructive to the source path. An absent source is not an error — a ceremony this machine never
// mirrored, or one a previous run already closed out, is the ordinary case for a sweep that runs
// on every unlock, and reporting it as damage would make a clean second pass look like a fault.
func CloseOutMirror(root, id string) error {
	dst, err := EndedDir(root, id)
	if err != nil {
		return err
	}
	src, err := MirrorDir(root, id)
	if err != nil {
		return err
	}
	if _, serr := os.Stat(src); serr != nil {
		if os.IsNotExist(serr) {
			return nil
		}
		return serr
	}
	if _, derr := os.Stat(dst); derr == nil {
		return fmt.Errorf("%w: %s already exists", ErrAlreadyClosedOut, dst)
	} else if !os.IsNotExist(derr) {
		return derr
	}
	if merr := os.MkdirAll(filepath.Dir(dst), 0o700); merr != nil {
		return merr
	}
	return os.Rename(src, dst)
}

// A Receipt is what this machine observed about how a ceremony ended, and when.
//
// **Unattested, deliberately, and it is the only unattested artifact in the ceremony set.**
// Everything else here carries a signature because it crosses a machine boundary and has to
// survive a hostile reader. This one never leaves the machine that wrote it: it records what THIS
// Nib saw, on THIS Nib's clock, for THIS Nib's own retention arithmetic. A signature over it would
// attest to nothing a reader could not already trust, and would imply to a future reader that it
// carried weight elsewhere.
//
// **It exists because `Termination` deliberately has no `When`.** That exclusion was S04b's
// finding — a convener-chosen timestamp driving other machines' retention hands the convener
// control of when they prune — and `Termination`'s own doc records the consequence: *"Retention
// starts from the local receipt's observed-at time."* That sentence named this type four days
// before it existed, which is the reason it is written here rather than derived again later.
type Receipt struct {
	// Ceremony is the id, so a receipt found alone is still attributable.
	Ceremony string `json:"ceremony"`
	// State is what this machine believes the end state was: `StateDeclined` or `StateCompleted`
	// where a termination reached it, and the derived `StateExpired` or `StateAbandoned` where
	// nothing ever did. The vocabulary is wider than `Termination`'s on purpose — see the
	// derived-states block below.
	State string `json:"state"`
	// ObservedAt is this machine's clock at the moment it decided the proceeding had ended. It
	// is the retention clock's zero, and it is local by design: see the type's doc.
	ObservedAt time.Time `json:"observed_at"`
	// Name is this machine's label for the proceeding, filled in by `ReadReceipt` and **never
	// written into `receipt.json`**. The name is editable and the receipt is write-once, so a copy
	// stored here would be a snapshot that disagrees with the `name` file the moment either
	// changes — and the receipt is the artifact whose whole value is that it does not change.
	// `WriteReceipt`'s one caller builds its own literal, so nothing sets this on a write path.
	Name string `json:"name,omitempty"`
}

// receiptFile is the receipt's name inside the closed-out directory.
const receiptFile = "receipt.json"

// The two DERIVED end states, which exist here and deliberately not in `Termination`.
//
// `Termination`'s own doc closes its set at two and says why: *"A third would need a convener able
// to observe it, which is the whole reason expired and abandoned are derived rather than
// attested."* Nobody can sign *"nothing happened"* — the party who would attest to it is precisely
// the one who stopped answering. So these are conclusions a machine reaches from its own clock and
// its own record, and the receipt is the only artifact in the ceremony set that can honestly carry
// one, because it is the only one that never leaves the machine that drew the conclusion.
//
// **Adding either to `Termination`'s set would be the defect, not the fix.** A signed *"expired"*
// is a convener's opinion about a deadline dressed as a fact, and D28 gives the deadline to the
// record rather than to a party for exactly that reason.
const (
	// StateExpired is a proceeding whose record's own `Expires` has passed.
	StateExpired = "expired"
	// StateAbandoned is a proceeding that ended without reaching this machine at all — the
	// deadline and the grace both passed and nothing ever said what happened.
	StateAbandoned = "abandoned"
	// StateLeft is this machine deciding it is no longer taking part (D17, `/pending 378`).
	//
	// **The third derived state, and it belongs here for a reason the other two do not share.**
	// Expired and abandoned are derived because nobody CAN attest them. This one is derived
	// because nobody SHOULD: leaving is a local decision that reaches no other party, and an
	// attested "left" would be a withdrawal the roster is entitled to act on — which is a decline,
	// and a decline is a different thing the user did not choose. `Termination`'s set stays closed
	// at two.
	//
	// **It is not a decline, and the distinction is the whole of D17.** A decline is an attested
	// refusal the convener learns about; leaving is this machine stopping. Conflating them would
	// either mint an attestation the user did not intend or leave a decline nobody can see.
	StateLeft = "left"
)

// A Verification is what this machine observed about the spoken check on a ceremony hop (D5).
//
// **Unattested and local, on the Receipt's exact footing** — see that type's doc for the argument,
// which holds here unchanged: it records what THIS Nib put in front of THIS user, and a signature
// over it would attest to nothing a reader could not already trust.
//
// # Why it cannot be a field on the signature instead
//
// D5 asks that *"a later reader can tell a confirmed ceremony from one where the modal never
// appeared"*, and the acceptance clause asks for three states. Two of the three leave **no
// signature at all**: `p2p.Receive` runs the spoken check before it reads a single document byte,
// so a refused or unanswered check produces nothing to carry a field. A signed token could express
// *confirmed* and *not presented*; it could never express *presented and not answered*, because it
// would have to ride on the signature whose absence defines that state.
//
// # The three states, and why none of them is an absence
//
// **Absence means UNKNOWN, never "not presented."** A build older than this, a failed write, or a
// plain two-party co-sign all leave no file, and a reader that read that as "the modal never
// appeared" would accuse a party of skipping a check they performed. So *not presented* is written
// POSITIVELY, by the one path that can produce it: `ConfirmVerification` refusing with
// `errVerifyBusy`, which returns before anything reaches the screen.
//
// **`autoVerifier` deliberately writes nothing**, and that is not a gap. It is confined to
// `deliverOneLeg` by a structural guard, and a delivery leg is not a signing event — its own header
// argues that the two identities already completed a spoken check at the hop. The population this
// type describes is HOPS.
//
// **Last-write-wins**, on `noteFailure`'s stated doctrine: the useful thing is the most recent
// answer, and a queue nobody prunes becomes its own problem. `At` is what lets a reader see which.
type Verification struct {
	// Presented is whether the four words actually reached the screen.
	Presented bool `json:"presented"`
	// Confirmed is whether the user said they matched. Meaningless unless Presented.
	Confirmed bool `json:"confirmed"`
	// At is this machine's clock when the outcome was known.
	At time.Time `json:"at"`
}

// verificationFile is the note's name inside the ceremony directory.
const verificationFile = "verification.json"

// WriteVerification records the spoken check's outcome for one ceremony.
//
// **It does not MkdirAll**, unlike `WriteMe`. The directory exists by the time any spoken check
// runs — the invitee's is created at accept and the convener's at convene — and creating one here
// would mean a ceremony this machine is not party to could be conjured into the listing by a
// stray session. A missing directory is therefore an error the caller logs, not a directory.
func WriteVerification(root, id string, v Verification) error {
	dir, err := MirrorDir(root, id)
	if err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return atomicfile.WriteDurable(filepath.Join(dir, verificationFile), b, 0o600)
}

// readVerification returns the note, or nil when this machine has none.
//
// Nil is UNKNOWN and never "not presented" — see the type's doc.
func readVerification(dir string) *Verification {
	b, err := os.ReadFile(filepath.Join(dir, verificationFile))
	if err != nil {
		return nil
	}
	var v Verification
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	return &v
}

// WriteReceipt records the end state in the closed-out directory, after the move.
//
// **After, not before**, and the order is the whole contract: written into `ceremonies/<id>/` it
// would be moved along with everything else and would still be correct, but a close-out that
// failed at the rename would have left a receipt claiming an end state beside a live ceremony —
// and the sweep reads receipts to decide what it has already dealt with. Writing into the
// destination means a receipt exists only where the move succeeded.
//
// **Write-once on the STATE, and the first observation wins** — `WriteTermination`'s rule one file
// over, for a sharper reason. The states are not equal in standing: `declined` and `completed` come
// from a verified termination somebody signed, while `expired` and `abandoned` are conclusions this
// machine drew from a clock. The sweep can reach a ceremony more than once (an interrupted
// close-out, a second unlock), and past the grace `closeOutReason` returns `abandoned` for anything
// it cannot find an end state for — so without this rule a re-sweep would overwrite *"they declined
// on the 2nd"* with *"nothing ever said"*, destroying the better answer with the worse one and
// leaving no trace that it had been there.
//
// A repeat with the SAME state is a no-op rather than an error: that is what an interrupted
// close-out finishing looks like, and it must not read as a fault.
func WriteReceipt(root, id string, r Receipt) error {
	dir, err := EndedDir(root, id)
	if err != nil {
		return err
	}
	// **The directory is created here, because a close-out with NOTHING to move still happened.**
	// The party that declines refuses at its consent gate, which returns before `rd.Store` — so it
	// never mirrored the ceremony at all, `CloseOutMirror` correctly finds no source and does
	// nothing, and the receipt then had no directory to land in. Measured at tier 4: the decline
	// leg logged *"could not write the local receipt: … no such file or directory"* and the
	// decliner was left with no local record that it had ever refused.
	//
	// That is the one machine the record matters most on. The receipt is not an annotation on a
	// moved folder; it is this machine's statement that it observed a proceeding end, and it has
	// to survive the case where there was nothing of the user's to preserve.
	if merr := os.MkdirAll(dir, 0o700); merr != nil {
		return merr
	}
	if prev, rerr := ReadReceipt(root, id); rerr == nil {
		if prev.State == r.State {
			return nil
		}
		return fmt.Errorf("%w: this ceremony is already recorded locally as %q and cannot also "+
			"be recorded as %q — the first thing this machine observed is the one that stands",
			ErrReceiptConflict, prev.State, r.State)
	}
	b, merr := json.MarshalIndent(r, "", "  ")
	if merr != nil {
		return merr
	}
	return atomicfile.WriteDurable(filepath.Join(dir, receiptFile), b, 0o600)
}

// ReadReceipt loads a closed-out ceremony's local receipt.
//
// Returns `os.ErrNotExist`-wrapping errors unchanged so a caller can tell "closed out before
// receipts existed" from "unreadable" with `errors.Is` rather than by matching on a string.
func ReadReceipt(root, id string) (Receipt, error) {
	dir, err := EndedDir(root, id)
	if err != nil {
		return Receipt{}, err
	}
	b, rerr := os.ReadFile(filepath.Join(dir, receiptFile))
	if rerr != nil {
		return Receipt{}, rerr
	}
	var r Receipt
	if uerr := json.Unmarshal(b, &r); uerr != nil {
		return Receipt{}, fmt.Errorf("this ceremony's local receipt could not be read: %w", uerr)
	}
	// **Read from the folder, never from the file — see `Receipt.Name`.** ADR-012 MOVES a ceremony's
	// directory, so a name given while the proceeding was running travels with it and is the only
	// identifying text a finished row can carry: the receipt holds a state and a date and nothing
	// a person would recognise.
	r.Name = readName(dir)
	return r, nil
}

// ListEnded enumerates the ceremonies this machine has closed out.
//
// **This is why the prune is a move rather than a delete, made visible.** A ceremony that ended
// leaves the live listing by construction — `ListStored` reads only `ceremonies/` — and without
// this a user whose proceeding finished would watch it vanish with no trace and no way to find the
// signed contribution that is the whole reason the directory was preserved. The receipt names the
// state and the date; `EndedDir` names where to look.
//
// **Sorted by `ObservedAt`, newest first**, because this is a history and a history is read from
// the end. `ListStored` sorts by id, which is right for a set of live things a user picks from and
// wrong for a set of finished ones.
//
// A directory with no readable receipt is skipped rather than reported as damage: close-out writes
// the receipt after the move, so a run interrupted between the two leaves exactly this, and the
// contribution is still there for a user told the path. Absence here is incompleteness, never
// corruption — the same reading `ErrNoTermination` gets one file over.
func ListEnded(root string) ([]Receipt, error) {
	ents, err := os.ReadDir(filepath.Join(root, endedDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // nothing has ended yet is not a failure
		}
		return nil, err
	}
	var out []Receipt
	for _, e := range ents {
		if !e.IsDir() || ValidID(e.Name()) != nil {
			continue
		}
		r, rerr := ReadReceipt(root, e.Name())
		if rerr != nil {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ObservedAt.After(out[j].ObservedAt) })
	return out, nil
}

// Which party this machine is — the one thing about a ceremony the vault holds (P06.S02).
//
// **Everything else in a ceremony directory is answerable without the vault**, which is what let
// P06.S01 take the listing off the lock: `ReadStored` reads `record.json` and nothing else. The one
// question it cannot answer is *"which of these parties am I"*, because that needs this machine's
// own fingerprint and `identity(v)` is sealed.
//
// **So it is recorded when it IS known.** Both write paths already hold the value at the moment
// they write the mirror — `handleCeremonyAccept` computes `me` and passes it to
// `pinCeremonyRoster`, and `handleCeremonyConvene` passes the convener's fingerprint for the same
// parameter — and neither recorded it. This is one line at each of two sites that already have it.
//
// **It discloses nothing new.** `record.json`'s roster already carries this machine's fingerprint,
// among the others, in the same directory, unsealed by D29's own design. `me` is a pointer into a
// file that is already there, not a second copy of a secret — which is why this needs no new global
// identity file and nothing appears on a machine that is party to no ceremony.

// meFile names the marker inside a ceremony directory.
const meFile = "me"

// nameFile is this machine's own LABEL for a proceeding.
//
// **Local, unsigned, and never on the wire — and that is the decision, not a shortcut.** The
// obvious alternative is a name the convener chooses and every party sees, and it is refused for
// two reasons that pull the same way. A shared name carried in the record but OUTSIDE
// `rosterPreimage` would be a claim on the wire that nothing anchors — the shape /pending 390, 437
// and 440 each found and this repo keeps refusing. Inside the preimage it commits, which makes a
// convenience label a signature-format flag day: `Record.Verify` refuses a newer `Version`, so
// every ceremony in flight the day it shipped would stop verifying.
//
// A name is how THIS person finds a proceeding in a list. It is not a fact about the agreement —
// that is the Intent, and D20 makes the Intent the recital's only home. Two parties naming the
// same ceremony differently is honest rather than a defect, and it is what a local label already
// means everywhere else it appears.
//
// Beside the mirror like `me` and `ended-by`, so it is readable with the vault locked and survives
// the close-out's move.
const nameFile = "name"

// WriteMe records which roster entry this machine is, for a reader that has no vault.
//
// Best-effort at its call sites and deliberately so: a ceremony whose `me` failed to write is
// readable, resumable and signable exactly as before — the panel simply cannot say "you are party 3
// of 4" until the next write. Refusing an accept over it would trade a whole ceremony for a label.
func WriteMe(root, id, fingerprint string) error {
	dir, err := MirrorDir(root, id)
	if err != nil {
		return err
	}
	if fingerprint == "" {
		return errors.New("a roster position needs a fingerprint to point at")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return atomicfile.WriteDurable(filepath.Join(dir, meFile),
		[]byte(strings.ToLower(fingerprint)+"\n"), 0o600)
}

// readMe returns this machine's fingerprint for a ceremony, or "" when it was never recorded.
//
// **Absence is UNKNOWN and never "not a party."** A ceremony mirrored before this shipped has no
// marker and its user is still very much a party; a panel that read the empty string as "you are
// not in this" would tell every one of them they are looking at somebody else's proceeding.
func readMe(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, meFile))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(string(b)))
}

// MaxCeremonyNameLen bounds a label. It is a handle in a list, not a second recital — the Intent is
// that — and the file is line-based, so a name is one line.
const MaxCeremonyNameLen = 120

// WriteName records this machine's label for a ceremony, or REMOVES it when name is empty.
//
// Empty deletes rather than storing a blank, so "clear the name" and "never named" are the same
// state on disk and the panel has one thing to render rather than two.
func WriteName(root, id, name string) error {
	dir, err := MirrorDir(root, id)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' ' // one line, because readName reads one
		}
		return r
	}, name))
	if len([]rune(name)) > MaxCeremonyNameLen {
		name = string([]rune(name)[:MaxCeremonyNameLen])
	}
	if name == "" {
		if rerr := os.Remove(filepath.Join(dir, nameFile)); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			return rerr
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return atomicfile.WriteDurable(filepath.Join(dir, nameFile), []byte(name+"\n"), 0o600)
}

// readName reports this machine's label for a proceeding, or "" when it has none.
func readName(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, nameFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
