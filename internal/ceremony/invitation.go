package ceremony

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"

	"golang.org/x/crypto/hkdf"

	"nib/internal/addrscope"
)

// The invitation (D21): one per party, carrying the ceremony id, the roster with FULL
// 32-byte fingerprints, and a 32-byte secret. An object — a copyable string — delivered
// over whatever channel the parties already use.
//
// # What it is and is not
//
// The pin is **full strength**: the roster carries whole fingerprints, not 66 bits of
// them, so the invited path is a 256-bit pin. **It is not a signing credential** — an
// intercepted invitation reaches the rendezvous and stops at the handshake, because the
// pin is the fingerprint and the holder has no private key.
//
// **Nothing signs the invitation, and that is stated rather than fixed** (PLAN-1's Stage 6
// pin). A party learns the convener's fingerprint *from* the invitation, so it is the root
// of trust and there is nothing to verify a signature against until it has already been
// trusted. What can be done is make tampering loud: the roster it carries is compared
// against the record's when the document arrives (MatchesRecord), which is the first
// moment there is an independent copy to check it against.

// MaxRoster bounds how many parties one invitation may name (D33).
//
// **Enforced here at P04.S04; D33 documented it and no code enforced it.** D25 allocates
// signature pages from the roster length, so an unbounded roster is an unbounded page
// count — and since P04.S04 made the punch budget per HOP, the roster is now also the
// multiplier on a ceremony's total emitted packets. A 10,000-entry roster parsed cleanly
// before this, giving 9,999 hops.
//
// Thirty-two is six signature pages and is far past any real signing.
const MaxRoster = 32

// MaxSeeds bounds how many DHT bootstrap addresses one invitation may carry.
//
// Eight, for the reasons the shipped list already demonstrates: you need ONE live node and
// the traversal expands from there, and the shipped list's own measured failure rate (three
// of five dead on the day it was written) puts P(all eight dead) near 1.7%. It also bounds
// what an invitation can aim: eight datagrams at addresses the sender chose, per recipient.
//
// And it keeps the invitation pasteable. A two-party invitation is ~570-700 characters
// today; eight IPv4 seeds add ~240. The QR claim in Encode's doc has never been enforced or
// tested, and base64url cannot use QR's alphanumeric mode, so the real ceiling is byte
// mode — comfortable at ~940 characters and not comfortable if this were unbounded.
const MaxSeeds = 8

// SecretLen is 32 bytes of uniform randomness.
//
// The length is the whole of caveat 11's answer. At 32 bytes there is no dictionary to
// attack, so a PAKE — whose entire purpose is bounding an attacker to one online guess
// against a low-entropy secret — would buy a property this secret already has, at the cost
// of a dependency and extra round trips.
const SecretLen = 32

// InvitationVersion is the invitation's format version (D32), carried in the text form's
// prefix so a reader knows what it is holding before it parses anything.
const InvitationVersion = 1

const invitationPrefix = "nib-invite-v1:"

var (
	// ErrInvitationFormat: the text is not an invitation at all. Distinct from a corrupted
	// one, because they mean different things to a person: the first is "you pasted the
	// wrong thing", the second is "what you pasted arrived damaged".
	ErrInvitationFormat = errors.New("that is not a Nib invitation")
	// ErrInvitationCorrupt: it is an invitation and its checksum does not match. **The
	// refusal is whole**: a partial pairing from a damaged invitation is worse than none,
	// because it pins some parties and silently omits others.
	ErrInvitationCorrupt = errors.New("this invitation is damaged — ask for a fresh copy")
	// ErrInvitationVersion: a version this build does not know.
	ErrInvitationVersion = errors.New("this invitation was made by a newer version of Nib")
	// ErrRosterMismatch: the invitation's roster is not the record's.
	ErrRosterMismatch = errors.New("this invitation does not describe the ceremony in this document")
)

// Invitation is what a party receives.
type Invitation struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	// Roster carries FULL fingerprints — this is what makes the invited path a 256-bit
	// pin rather than the six-word name's 66 bits.
	Roster []Party `json:"roster"`
	// Secret keys the rendezvous, the record encryption and the channel binding. It is
	// never written to the ceremony mirror (D29) — that directory is ordinary files under
	// the user's home, and this belongs in the vault.
	Secret []byte `json:"secret"`
	// ConvenerFingerprint identifies who convened, so a party can check the record's
	// signer against what the invitation told them to expect.
	ConvenerFingerprint string `json:"convener"`
	// Seeds are DHT bootstrap addresses (D6's second half), so the common case does not
	// depend on a shipped list that rots — three of the five Nib ships were dead the day
	// they were written.
	//
	// **This is the only field that will never have a signed counterpart.** MatchesRecord
	// compares the id, the roster and the signs flags against the convener-signed Record,
	// and a Record has no seeds to compare against — so unlike every other field here,
	// tampering with these can never be made loud. They are therefore treated throughout as
	// what they are: attacker-controllable input to a bootstrap path, validated at both
	// doors, capped, and consulted only when nothing else works.
	//
	// `omitempty`, and NO version bump: an invitation without seeds encodes byte-identically
	// to today's, and Go drops unknown fields silently, so a build that predates this field
	// ignores it and falls back to the shipped list. That is exactly the degradation wanted.
	// Bumping InvitationVersion would instead make every older build refuse the whole
	// invitation — "made by a newer version of Nib" — over an optional bootstrap hint it
	// does not need. Recorded against D32 rather than assumed: D32's rule is that a skew
	// produces a sentence, and here there is no skew to produce one for.
	Seeds []netip.AddrPort `json:"seeds,omitempty"`
	// SeedsDropped is how many of the seeds offered were not addresses Nib will dial.
	//
	// Never serialized — it is an observation about the invitation just parsed, not a field
	// of it. The acceptance says a bad entry is "dropped and counted", and a count nobody
	// can read is not counted; this is where a caller reads it.
	SeedsDropped int `json:"-"`
}

// ErrInvitationSeeds: the seed list is unusable as a whole.
var ErrInvitationSeeds = errors.New("this invitation's bootstrap addresses are unusable")

// validateSeeds refuses an unusable list and drops unusable entries, returning the kept
// seeds and how many were dropped.
//
// # Called at BOTH doors, and that is the whole point
//
// P04.S04 shipped MaxRoster in ParseInvitation alone, and its review found that this bound
// the RECIPIENTS and left the convener — the party that dials every hop and emits every
// packet — entirely unbounded, because a convener never parses its own invitation. Seed
// validation in one door only would repeat that defect verbatim.
//
// # What is dropped and what is refused, stated as the code actually behaves
//
// The split is **routable-versus-not**, not malformed-versus-not, and an earlier version of
// this comment claimed the opposite — naming as "dropped" the one case that is in fact
// refused wholesale. A syntactically malformed entry never reaches here: `netip.AddrPort`
// unmarshals through `UnmarshalText`, so one bad character aborts the whole `json.Unmarshal`
// and the invitation dies at `ErrInvitationCorrupt`. `TestAHostnameCannotBeASeed` asserts
// exactly that.
//
// So: an address that PARSES but is not somewhere Nib will send UDP — loopback, private,
// CGNAT, a low port, a duplicate — is dropped and counted, because a list of eight from a
// stranger is expected to contain some rubbish and refusing the invitation over it would
// hand an attacker a denial of honest invitations. An oversized list cannot be a mistake,
// so it is loud.
func validateSeeds(in []netip.AddrPort) (kept []netip.AddrPort, dropped int, err error) {
	if len(in) > MaxSeeds {
		return nil, 0, fmt.Errorf("%w: %d addresses, the limit is %d",
			ErrInvitationSeeds, len(in), MaxSeeds)
	}
	seen := make(map[netip.AddrPort]bool, len(in))
	for _, ap := range in {
		// IsValid explicitly, though the port floor would also catch it. Measured:
		// `{"seeds":[""]}` and `{"seeds":[null]}` both unmarshal with a NIL error into a
		// zero AddrPort, so the refusal exists either way — but resting it on "port 0 is
		// below MinPort" makes it a property inherited from another rule rather than one
		// this function states, and `Seed()` takes whatever a caller hands it.
		if !ap.IsValid() || seen[ap] || !addrscope.Seed(ap) {
			dropped++
			continue
		}
		seen[ap] = true
		kept = append(kept, ap)
	}
	return kept, dropped, nil
}

// NewInvitation builds an invitation for a record, generating the secret.
func NewInvitation(r Record) (Invitation, error) {
	sec := make([]byte, SecretLen)
	if _, err := rand.Read(sec); err != nil {
		return Invitation{}, err
	}
	conv, ok := r.Convener()
	if !ok {
		return Invitation{}, errors.New("the record's convener is not in its own roster")
	}
	return Invitation{
		Version:             InvitationVersion,
		ID:                  r.ID,
		Roster:              append([]Party(nil), r.Roster...),
		Secret:              sec,
		ConvenerFingerprint: conv.Fingerprint,
	}, nil
}

// Encode renders the invitation as one pasteable line.
//
// A prefix, a base64url payload and a truncated checksum. The checksum is what turns a
// mangled paste — a line break inserted by a mail client, a character lost at a boundary —
// into a refusal rather than a partial parse. base64url so it survives being put in a URL
// or a QR code without escaping.
func (i Invitation) Encode() (string, error) {
	// The producing door. See validateSeeds: a convener never parses its own invitation, so
	// a rule enforced only at the parse binds everyone except the party that acts on it.
	//
	// **A producer REFUSES rather than drops.** At the parse, dropping is right — the list
	// came from a stranger and some rubbish is expected. Here the list is the convener's
	// own, so silently shipping fewer addresses than they chose is a producer that lies
	// about what it sent. If a sampler ever hands this an address Nib would itself refuse,
	// that is a bug upstream and it should be loud.
	kept, dropped, err := validateSeeds(i.Seeds)
	if err != nil {
		return "", err
	}
	if dropped > 0 {
		return "", fmt.Errorf("%w: %d of %d are not addresses Nib would dial",
			ErrInvitationSeeds, dropped, len(i.Seeds))
	}
	i.Seeds = kept
	b, err := json.Marshal(i)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	payload := base64.RawURLEncoding.EncodeToString(b)
	return invitationPrefix + payload + "." + hex.EncodeToString(sum[:4]), nil
}

// ParseInvitation reads the text form.
//
// Whitespace anywhere is removed first: an invitation travels through email, chat and
// paste buffers, and a line break in the middle is the ordinary case rather than the
// exception. A checksum failure refuses the WHOLE thing — a partially-applied invitation
// would pin some parties and silently omit others, which is worse than pinning none.
func ParseInvitation(text string) (Invitation, error) {
	t := strings.Join(strings.Fields(text), "")
	if !strings.HasPrefix(t, invitationPrefix) {
		// A prefix from a future version parses as "not an invitation" without this, which
		// sends the user to the wrong problem.
		if strings.HasPrefix(t, "nib-invite-v") {
			return Invitation{}, ErrInvitationVersion
		}
		return Invitation{}, ErrInvitationFormat
	}
	body := strings.TrimPrefix(t, invitationPrefix)
	dot := strings.LastIndex(body, ".")
	if dot < 0 {
		return Invitation{}, ErrInvitationCorrupt
	}
	payload, want := body[:dot], body[dot+1:]
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Invitation{}, ErrInvitationCorrupt
	}
	sum := sha256.Sum256(b)
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:4])), []byte(want)) != 1 {
		return Invitation{}, ErrInvitationCorrupt
	}
	var inv Invitation
	if err := json.Unmarshal(b, &inv); err != nil {
		return Invitation{}, ErrInvitationCorrupt
	}
	if inv.Version != InvitationVersion {
		return Invitation{}, fmt.Errorf("%w: version %d", ErrInvitationVersion, inv.Version)
	}
	if len(inv.Secret) != SecretLen {
		return Invitation{}, ErrInvitationCorrupt
	}
	if len(inv.Roster) == 0 || len(inv.Roster) > MaxRoster {
		return Invitation{}, fmt.Errorf("%w: it names %d parties (the limit is %d)",
			ErrInvitationCorrupt, len(inv.Roster), MaxRoster)
	}
	for i, p := range inv.Roster {
		b, err := hex.DecodeString(p.Fingerprint)
		if err != nil || len(b) != sha256.Size {
			// A roster entry that is not a full fingerprint is the one corruption that
			// must never be tolerated: the whole point of the invited path is that the pin
			// is 256 bits.
			return Invitation{}, ErrInvitationCorrupt
		}
		// Normalise the case HERE, once, because hex.DecodeString accepts both and every
		// consumer downstream compares the STRING.
		//
		// Left alone, an uppercase fingerprint is a legitimate roster entry that breaks
		// the ceremony two different ways. The two parties derive different DHT salts and
		// never find each other — silently, indistinguishable from an offline peer. And if
		// they did meet, `Fingerprint()` always emits lowercase, so the record would be
		// refused with ErrCandidateAuthor: *"signed by someone other than the party it
		// claims"* — an accusation of impersonation over a difference in letter case.
		inv.Roster[i].Fingerprint = strings.ToLower(p.Fingerprint)
	}
	// Seeds last, and the whole-refusal discipline applies: an over-cap list returns the
	// ZERO invitation, never a partly-filtered one. A caller that ignores the error is the
	// ordinary mistake, and a half-applied invitation is worse than none.
	kept, dropped, err := validateSeeds(inv.Seeds)
	if err != nil {
		return Invitation{}, err
	}
	inv.Seeds = kept
	inv.SeedsDropped = dropped
	return inv, nil
}

// --- key derivation (D6's amendment, D30) ------------------------------------

// derive is the one HKDF call. Every key the secret produces is domain-separated by its
// info string, so a value used for one purpose can never be the value used for another —
// the failure that turns a rendezvous key into a decryption key.
func (i Invitation) derive(info string, n int) ([]byte, error) {
	if len(i.Secret) != SecretLen {
		return nil, errors.New("invitation has no secret")
	}
	out := make([]byte, n)
	r := hkdf.New(sha256.New, i.Secret, []byte(i.ID), []byte(info))
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}

// HopSeed is the ed25519 seed the hop publishes under (D6's amendment, D30).
//
// **Renamed from RendezvousKey at P04.S03**, and the rename is the finding. The old name
// said "the DHT key for one hop", which reads as an *address* — something publishable,
// loggable, usable as a cache directory name. It is not. Seeded into
// `ed25519.NewKeyFromSeed`, these 32 bytes ARE the BEP-44 private key for the hop, i.e.
// the write authority for both parties' records under it. This repo already writes
// ceremony state to plain files under `~/nib/ceremonies/<id>/` and P04 is a phase full of
// diagnostics; a future slice that named a cache directory or emitted a counter by "the
// rendezvous key" would hand any local reader the ability to overwrite both records. The
// value that must stay secret and the value that may be logged should not be the same
// bytes, and after this rename there is no address-shaped name to reach for.
//
// **Per hop, not per ceremony**: D30 requires two hops of one ceremony to publish under
// different keys, so an observer who learns one hop cannot follow the rest. The hop index
// is in the info string rather than mixed into the salt, so the derivation is one call
// with one shape.
//
// What the resulting BEP-44 signature proves, stated precisely so it is not over-read:
// *whoever wrote this held the invitation*. Every roster member derives this seed from
// the same secret, so it is a GROUP credential — it authorises a write, it does not name
// a speaker. Naming the speaker is the inner ECDSA signature's job (see candidate.go).
func (i Invitation) HopSeed(hop int) ([]byte, error) {
	return i.derive(fmt.Sprintf("nib-bep44-seed-v1/hop-%d", hop), 32)
}

// RecordKey encrypts the published candidate record (D6's amendment).
//
// Keyed on the SECRET rather than on the names: names are the public identifier this plan
// exists to have people say aloud, so a key derived from them is obfuscation against a
// scraper and not confidentiality against the party the pin is actually worried about.
//
// **Per hop as of P04.S03, and it was not before.** Per-hop *addressing* shipped at
// P01.S07; per-hop *encryption* did not — this was `derive("nib-record-v1", 32)` with no
// hop, so one key decrypted every hop of the ceremony. That made D30's own P05 acceptance
// clause — "a party cannot read the candidates of a hop it is not in" — unsatisfiable by
// construction, since every roster member holds the secret and therefore that one key.
// With the hop in the info string the clause is buildable, and cross-hop replay becomes an
// AEAD failure as well as a signature failure: two independent defences rather than one.
//
// What it still does not buy: any roster member can derive ANY hop's key from the secret.
// The boundary is the roster, not below it.
func (i Invitation) RecordKey(hop int) ([]byte, error) {
	return i.derive(fmt.Sprintf("nib-record-v1/hop-%d", hop), 32)
}

// RecordSalt is the BEP-44 salt separating the two parties publishing under one hop.
//
// A hop has two parties and both publish. Sharing one target would mean the higher-seq
// write silently clobbered the other, so the two need distinct targets — that is what the
// salt is for, and it is addressing, not authentication (both parties hold the same
// private key, so either can write at either salt; only the inner signature separates
// them).
//
// **It is KEYED, and that is the security-relevant part.** The salt travels in cleartext
// in every put and is held by every storing node, and the target is a plain
// `sha1(pubkey ‖ salt)`. A salt that was the party's fingerprint — or any unkeyed function
// of it — would turn the public DHT into a searchable index of Nib identities: a
// fingerprint is the permanent, never-rotated pin people hand out on a card or a QR code,
// so anyone holding one could watch for that salt and learn every ceremony that person
// runs and when. Deriving it through HKDF means both parties compute both salts by
// construction while nobody else can recognise either.
//
// 32 bytes, comfortably inside BEP-44's 64-byte salt cap (bep44/item.go:140).
func (i Invitation) RecordSalt(hop int, fingerprint string) ([]byte, error) {
	raw, err := hex.DecodeString(fingerprint)
	if err != nil || len(raw) != sha256.Size {
		return nil, errors.New("record salt needs a full hex fingerprint")
	}
	// Keyed on the DECODED bytes, not the string. hex.DecodeString accepts either case, so
	// interpolating the text would make `AB12…` and `ab12…` derive different salts for the
	// same peer — two parties publishing to two targets and never meeting. Case is
	// normalised at ParseInvitation as well; this is the same fact enforced structurally,
	// where it cannot be undone by a caller that built an Invitation some other way.
	return i.derive(fmt.Sprintf("nib-rendezvous-salt-v1/hop-%d/party-%x", hop, raw), 32)
}

// --- caveat 11: the channel binding -------------------------------------------

// BindingMAC is the confirmation that used to be spoken (D21), computed by the machines.
//
// **The mechanism, and why it is not a PAKE.** A PAKE bounds an attacker to one online
// guess against a secret small enough to enumerate. This secret is 32 uniform bytes; there
// is nothing to enumerate, and a PAKE would charge a dependency and round trips for a
// property the secret already has. So: HKDF over the secret keyed to THIS channel's
// exporter, and each side proves possession with a MAC over its role.
//
// exporter is RFC 5705 keying material from the live TLS connection — the same value the
// spoken verification string binds to (P01.S04). That is what makes a recording of one
// connection useless on another: the binding key changes with the channel, so a MAC
// captured from one session verifies on no other.
//
// role distinguishes the two directions so the initiator's MAC cannot be replayed back as
// the responder's — a reflection that would otherwise let an attacker who can echo bytes
// look like a peer holding the secret.
// bindingDomain separates the MAC preimage from every other length-prefixed structure this
// package builds. The MAC input had no tag at all.
const bindingDomain = "nib-invitation-binding-preimage-v1"

func (i Invitation) BindingMAC(exporter []byte, role string) ([]byte, error) {
	if len(exporter) == 0 {
		return nil, errors.New("no channel binding material")
	}
	k, err := i.derive("nib-invitation-binding-v1", 32)
	if err != nil {
		return nil, err
	}
	// Through preimageBuilder, like every other signed input in this package — its own
	// doc calls itself "the one length-prefix encoder every signed preimage in this
	// package uses", and this was the exception that made the sentence false.
	//
	// Bare concatenation is ambiguous: ("a","bc") and ("ab","c") write identical bytes and
	// produce one MAC. Two mitigations existed and neither was written down or asserted —
	// the key is purpose-derived, so confusion needs the same key, and the only roles ever
	// passed happen to be the same length. `role` is a free string parameter on an exported
	// method, so neither is a property of the code.
	var pb preimageBuilder
	pb.addString(bindingDomain)
	pb.addString(role)
	pb.add(exporter)
	m := hmac.New(sha256.New, k)
	m.Write(pb.bytes())
	return m.Sum(nil), nil
}

// CheckBindingMAC verifies a peer's MAC in constant time.
func (i Invitation) CheckBindingMAC(exporter []byte, role string, got []byte) error {
	want, err := i.BindingMAC(exporter, role)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(want, got) != 1 {
		return errors.New("the peer does not hold this ceremony's invitation secret, or is on a different channel")
	}
	return nil
}

// --- the record comparison ----------------------------------------------------

// Hop returns the hop index joining two parties, read off the invitation's roster.
//
// The same rule as Record.Hop and the same code — an invitation's roster is checked against
// the record's by MatchesRecord, so the two cannot disagree about order without that check
// already having failed.
func (i Invitation) Hop(a, b string) (int, error) {
	return hopBetween(i.Roster, i.ConvenerFingerprint, a, b)
}

// Hops is how many hops this ceremony has: one fewer than its roster.
func (i Invitation) Hops() int {
	if len(i.Roster) < 2 {
		return 0
	}
	return len(i.Roster) - 1
}

// MatchesRecord checks the invitation's roster against the record's.
//
// This is the only check that can catch a tampered invitation, and it can only happen once
// the document arrives — nothing signs the invitation (PLAN-1), so until there is a second,
// independently-signed copy of the roster there is nothing to compare against. The record
// is that copy: it is signed by the convener, and its rosterHash commits to every
// fingerprint and every `signs` flag.
//
// A one-byte change to a fingerprint in the invitation therefore surfaces HERE, by name,
// rather than as a handshake that mysteriously fails later.
func (i Invitation) MatchesRecord(r Record) error {
	if i.ID != r.ID {
		return fmt.Errorf("%w: the invitation is for ceremony %s and the document carries %s",
			ErrRosterMismatch, short(i.ID), short(r.ID))
	}
	if len(i.Roster) != len(r.Roster) {
		return fmt.Errorf("%w: the invitation lists %d parties and the document's record lists %d",
			ErrRosterMismatch, len(i.Roster), len(r.Roster))
	}
	for n := range i.Roster {
		a, b := i.Roster[n], r.Roster[n]
		if a.Fingerprint != b.Fingerprint {
			return fmt.Errorf("%w: party %d is %s in the invitation and %s in the document's record",
				ErrRosterMismatch, n+1, short(a.Fingerprint), short(b.Fingerprint))
		}
		if a.Signs != b.Signs {
			return fmt.Errorf("%w: party %d is %s in the invitation and %s in the document's record",
				ErrRosterMismatch, n+1, signsWord(a.Signs), signsWord(b.Signs))
		}
	}
	// The convener, which this check skipped entirely — while ConvenerFingerprint's own doc
	// said it exists "so a party can check the record's signer against what the invitation
	// told them to expect". That check was the one thing the field was for, and it did not
	// exist; the field was written at two sites and read nowhere.
	//
	// It matters because the roster hash does not bind who convened: any roster member can
	// re-sign an unchanged roster and `Record.Verify` still passes, since it only asks that
	// the signer appear SOMEWHERE in the roster. The invitation is the second, independent
	// statement of who it should have been.
	// **Not opt-in.** The check was `if i.ConvenerFingerprint != ""`, so an invitation with
	// the field blank skipped the one comparison it exists for — and the field is plain JSON
	// on a document a party receives, so blanking it is a one-byte edit an attacker makes
	// rather than an old build's oversight. NewInvitation has always set it, and this
	// project has no compatibility population to carry, so an absent convener is a malformed
	// invitation and says so.
	if i.ConvenerFingerprint == "" {
		return fmt.Errorf("%w: the invitation does not say who convened, so there is nothing "+
			"to check the document's record against", ErrRosterMismatch)
	}
	{
		conv, ok := r.Convener()
		if !ok {
			return fmt.Errorf("%w: the document's record is signed by nobody in its own roster",
				ErrRosterMismatch)
		}
		// Case-insensitively: both sides are hex and nothing normalises them. The invitation
		// is parsed from JSON a party received, so its case is not this build's to assume,
		// and a mismatch here reads as "someone else convened" — the loudest wrong sentence
		// this function can produce.
		if !strings.EqualFold(conv.Fingerprint, i.ConvenerFingerprint) {
			return fmt.Errorf("%w: the invitation says %s convened and the document's record "+
				"is signed by %s", ErrRosterMismatch,
				short(i.ConvenerFingerprint), short(conv.Fingerprint))
		}
	}
	return nil
}

func short(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12] + "…"
}

func signsWord(b bool) string {
	if b {
		return "a signer"
	}
	return "not signing"
}
