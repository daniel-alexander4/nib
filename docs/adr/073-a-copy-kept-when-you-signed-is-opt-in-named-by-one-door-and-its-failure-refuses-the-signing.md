# ADR-073 — a copy kept when you signed is opt-in, named by one door, and its failure refuses the signing

**Status:** accepted. `PLAN-returned-document.md` P04.S01 (2026-10-02), decisions D13-D17 as amended by P04's
phase-open PIN and plan-review pins. Extends ADR-009 (one door per rule) and ADR-027/029 (a step is ticked only where
Nib observes it).

## Context

A signed document comes back changed, and the file that came back cannot always answer what was signed: a wholesale
re-save leaves no prefix to recover (P01-P03's "the version you signed is not inside this file"). The one thing that
can answer then is a copy the signer kept. Until this change Nib persisted nothing on the solo signing path: Finalize
signed in one request and handed the bytes to Save As (`internal/server/finalize.go`, `web/app.js`).

Two facts constrained where a kept copy can live and how it is named:

- **`~/nib/signed/` already has two writers.** Every signed arrival from a peer (`saveReceived`, named by
  `receivedName`) and every delivered ceremony copy (`saveDelivered`, named by `deliveredName`) land there, and
  `alreadyDelivered` re-arms a ceremony on a stat of the delivered name. A kept copy that a list-and-delete surface can
  reach must therefore carry a name neither writer can produce.
- **Nib handles PII by design** (the redaction presets are SSN, email, phone and card), so a new persisted artifact is
  a posture change on a local-first tool, and the copy sits OUTSIDE the vault, unencrypted.

## Decision

1. **Opt-in, off at every opening, never remembered** (D13). Finalize & sign's modal carries "Keep a copy for my
   records", with a hint that says the copy is saved unencrypted in `~/nib/signed` where anyone who can read the user's
   files, and their backups, can read it — the opt-in is informed only if it says so. Complete & sign and the CLI offer
   no tick (`/pending 814`; the CLI already writes where the user names).
2. **The copy is the exact bytes returned, written before they are sent** — `atomicfile.WriteDurable(path, signed,
   0o600)` after `MkdirAll(…, 0o755)`, the pair the folder's other writers use. **A failed write refuses the signing**
   (D14): `500 {error, cause: "copy-not-kept", reason}` and no PDF bytes, so a user who ticked the box never holds a
   signature they believe is kept and is not; the modal stays open and says the document was not signed, why, and that
   unticking signs without a copy. 500, never 422 — the modal reads 422 as a wrong certificate passphrase.
3. **The name grammar, version 1: `kept_<slug>_<YYYYmmdd-HHMMSS>-<first 8 hex digits of sha256>.pdf`.** `<slug>` is
   `labelSlug` of the document's name (untrusted client text, so it only ever narrows; 48 characters at most;
   `document` when empty). `labelSlug` never emits `_`, so the name parses one way and no peer label or ceremony intent
   produces it. A later grammar takes a NEW prefix; a reader ignores what it does not parse.
4. **One door names a kept copy** (ADR-009): `keptPathFor` (`internal/server/kept.go`) accepts only the grammar,
   anchored, and REBUILDS the path from the parsed parts, so no client string reaches a path join. The listing and the
   removal (P04.S02) reach a kept copy only through it.
5. **The name's digest is an index, never evidence.** Save As can overwrite a kept file, and 32 bits are minutes of
   work to collide; every reader that decides "this kept copy is this document's" compares bytes (P04.S02's
   `keptCopyFor`).

## Consequences

- **Declared residue.** An interrupted write leaves `atomicfile`'s hidden `.nib-*.tmp` in `~/nib/signed/` holding a
  full signed document. The listing does not show it and the removal cannot reach it; nothing sweeps it, because a
  sweep could race a live `saveReceived` write.
- **What a kept copy proves.** It proves what the signer HAD at signing, not what they sent (D17): it is written before
  the response, so a cancelled Save As or a lost response still leaves a copy of a signature that went nowhere, and in
  a counterparts signing it is this party's counterpart, not the executed original. Every surface calls it "a copy
  kept when you signed", never "what you sent" or "the original". It is not timestamped (D17, `/pending 388`).
- **"Not signed" is true of the bytes, not of every effect.** A refused signing returns nothing and keeps nothing, but
  the signature WAS computed first — and with a timestamp server named, the document's hash has already been sent to
  that third party, which holds a stamp for a signing the user is told did not happen. Harmless as evidence; declared.
- **Right after Finalize the checklist row cannot tick**: the open view is still the unsigned document (the signed
  bytes went to Save As), and no byte relation joins them. The row answers for an opened signed document (P04.S03);
  no flag from the Finalize response may tick it (ADR-027).
- **The list is reached from the Sign & Timestamp card**, which the menu strip shows once a document is open (ADR-037)
  and the user has not cut from the menu (ADR-036) — so reaching your copies means opening some document first. D15's
  "listable and deletable in the UI" holds; a document-free entry point is not built (P04.S02, declared).
- **The removal widens nothing a session could not already do** — `/api/write` and `/api/open` reach the folder — and
  its only property is that it cannot reach a file that is not a kept copy.
