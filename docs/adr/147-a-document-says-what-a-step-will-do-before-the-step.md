# ADR-147 — a document says what a step will do before the step, from facts on its own answer

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-032 (an addendum: the label's fate is said, not only applied), ADR-047 (a crop keeps the tree),
ADR-051/058 (who signed), ADR-087 (the bar), ADR-145 (facts read by the answer's one parse), ADR-031 (notices).
**Supersedes:** ADR-135 decision 4's "one press signs" and its declared "there is no block preview" only.
**Context:** /pending 520, 577, 831, 869 (its preview), 582, 846 — six of the owner's decisions of 2026-10-12,
built as one slice. Four of them were a step that did something the user was not told about first; the facts
needed to say so were not on the document's answer. Read from the code before building:

- No signing path runs a conformance check. Finalize, `nib sign` and the co-sign's prepare each DROP a PDF/UA or
  PDF/A label from an unsigned document before the signature (`pdfops.DropUAIdentificationUnlessSigned`), and keep
  it on a signed one, with nothing said. So the sentence is about what signing does, never "the check failed".
- A signer's row led with `/Name`, text the signer typed. The certificate's subject was parsed
  (`p7.GetOnlySigner`) and thrown away.
- The convener's Sign now signed on one press; the receiving side has always been shown where its block goes.
- The loss notice counted the form's top-level `/Fields` array. A form appended to itself measured 2 → 3 while
  holding all four fields: pdfcpu puts the incoming fields under ONE new parent (`2.fullName`, `2.agree`).
  Nothing merges.

**Applies:** `internal/pdfops/flags.go` (`ReadAnswerFacts`), `uaid.go`, `tagfate.go`; `internal/server/server.go`
(`docResponse`), `undo.go`, `pages.go`, `ceremonyhop.go`; `internal/sign/revisions.go`, `verify.go`;
`internal/cli/commands.go`; `web/app.js`, `web/index.html`.

## Decision

**1. The document's answer carries two more facts, read by the parse its flags come from.**
`pdfops.ReadAnswerFacts` is the one read (`FlagsAndXFA` is a view of it). `Labelled` is asked through
`dropUAIdentification` itself, on a context that is then discarded — so "has a label" is exactly "the next change
would drop one". `Tagged` is `taggingClaimOf`, the reading `inspectTags` uses. They are cached with the flags, by
the same identity rule. `docResponse.ConformanceLabel` is `removed` (unsigned — every signing path drops it) or
`kept` (`sign.HasSignatureBlob`, the predicate the doors use — asked only of a labelled document), else absent.

**2. Every signing surface says what signing does to the label, before the user signs.** One painter
(`reflectSigningLabel`) fills every `.signlabelnote`: Finalize, Co-sign, Simple Sign, and the rail's Sign now.
`nib sign` prints the same sentence (`pdfops.LabelRemovedBySigning` / `LabelKeptBySigning`; a jsdom test holds
the window's copies equal). Never a refusal, and not dismissible: it is part of the step. No fourth corner
notice was added — `#tagNotice`, the lost-content notice and `#xfaNotice` stay three.

**3. A crop on a tagged document says the hidden part is still read aloud, and names Redact** — on the crop's
confirm step, and again in its result (shown for 12 s, not the toast's 2.5). Never in `#tagNotice`, which says
the tags were lost. ADR-047 stands: nothing is re-marked as an artifact.

**4. A signer is named by what Nib knows, in a pinned order.** The pinned peer's label; then the certificate's
subject, labelled "Certificate says:"; then the fingerprint's six words. The first there is leads `signerRow`;
the rest follow in that order; `/Name` is last, as "Calls themselves:". `Revision.Subject` /
`SignerInfo.Subject` are set exactly when the fingerprint is (verified, well-formed) from the same certificate,
as one line of at most 200 characters with control and format characters replaced. `docResponse.SignerNamed`
carries the pinned label (vault unlocked) and the words (always), per signer.

**5. Sign now is two presses.** The own-turn quote carries `Block` — page and rectangle from
`p2p.PlacementFor` on the document, the roster and this machine's fingerprint, which is `buildCoSigned`'s own
call. The first press quotes and draws that page with the place outlined by `markBlock`, the receiving
consent's outline. The second signs exactly what was quoted — the same rendered block and pinned time — and
only on the document and view the preview was drawn for. *Not now* signs nothing. The page loop is not shared
with `loadPendingPreview`: that one renders a parked document in its own pdf.js instance, this one the open view.

**6. The loss notice compares form fields by fully qualified name.** `pdfops.Inspect` names every terminal
field (`terminalFieldNames`: a node none of whose kids has a `/T`; bounded at depth 32 and 100,000 nodes). A
name the document had and the result has not is lost; fields another document brought in are counted against
the new names the result holds. The decided "merged with a field of the same name" sentence is NOT built: it
would be false of every case measured.

**7. The hand tool is a toggle in the View group.** On, a left drag pans from text too and no selection
starts; a field, a link and an overlay keep their drags; an armed tool still owns the pointer; the middle
button is unchanged. It is window state, not an editing tool, and is in `DOC_REQUIRED`.

## Consequences

- Declared: the receiving consent (a document that arrives from a peer) does not show the label sentence —
  that document is not an open one and has no answer to carry the fact.
- Declared: `foldThresholds` rung 1 is DERIVED (1165 + 40 for one `.tbicon` and its gap), not re-measured.
  ADR-087 asks for a measurement; `test/ui/toolbaricons.test.mjs` prints every group's width.
- Declared: annotations are still compared by count (with what was brought in). Their names do not survive a
  rewrite and their rectangles move under a split or a crop, so an identity for them is a design of its own.
- Declared: whether pdfcpu renames incoming fields only on a collision was measured on one form appended and
  inserted into itself, not on two different forms.
- A certificate's subject is a claim: a self-signed certificate says what its maker typed. Do not show it
  without its label, and do not lead with `/Name` again.
- Do not make "labelled" a second reading of the packet: ask the drop.
- Do not move the crop sentence into `#tagNotice`, and do not add a corner notice for the label.
- Not pressed in a real window: Sign now's preview in the rail (its width is the rail's), the hand tool, the
  crop sentence. No multi-process tier presses Sign now.
