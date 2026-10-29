# ADR-174 — nib carries pdfcpu v0.16.2 as published, and the digest moves to rule 7

**Status:** accepted
**Date:** 2026-10-29
**Supersedes:** ADR-173 (the fork is gone: `third_party/pdfcpu`, its `replace` and `TestThePdfcpuPatchIsTheCryptFilterLineOnly`
are removed; the require line is v0.16.2, which carries the crypt-filter fix upstream), ADR-143 in its readable set only
(the rules this build computes are 7; 5 and 6 are retired as 4 was), and ADR-136 in its scope only (the write refusal
narrows from "an associated file only `/AF` reaches" to a **page's** `/AF`; the catalog's is written and is removed with
its file). Extends ADR-009, ADR-115, ADR-129, ADR-164, ADR-166, ADR-168.
**Context:** /pending 898, PLAN-pdfcpu-upgrade.md (D1–D9, P02.S01–S09). Nib restated pdfcpu's v0.13.0 behaviour in ~66
places, each written against v0.13's source by line number (`internal/pdfread/restatements.tsv` is the inventory); the
upgrade was a port of those, not a dependency change. Decided by Dan on 2026-10-14 (/discuss on /pending 892).
**Applies:** `go.mod` (pdfcpu v0.16.2, go 1.26), `internal/pdfread`, `internal/pdfops`, `internal/ceremony`,
`internal/server` (`checkArrival`), `internal/uacheck`.

## Decision

**1. Rule 7 is rule 6's axes, read by pdfcpu v0.16.x** (D1). The digest is taken over pdfcpu's post-validation,
post-optimize graph (`ReadOptimized`), and v0.16.2 reads 24 of 2,326 real documents to a different graph (six
validator/optimizer repairs), so `ContentDigestVersion` is 7. The digest is not undone repair by repair. Rules 5 and 6
are RETIRED through ADR-080/143's path: a record under either is **accepted by every record-level question**
(`CheckRecord`, delivery, `ReadMirrorToEnd` for stop, close-out and an ended folder's read-back) and **refused with the
older-nib skew (wire 18) wherever a digest is computed** (`DocumentHashFor`) **or a ceremony is continued**
(`ReadMirror`, a live folder, `checkArrival` through `RequireComputableRule`, which also covers hop 2 and later where
no hash is asked for). A FINISHED rule-5/6 ceremony document therefore keeps its roster verdict, and an unsigned
mirror can still be stopped and moved to `~/nib/ended/`. The goldens for the retired rules are deleted and only `-v7`
is written (ADR-143 §3: a golden for a rule nothing computes pins nothing); the contract is held by
`TestARecordUnderARetiredDigestRuleIsARecordButNotAHash`.
**The flag day, declared:** a ceremony convened on a pre-port build cannot be continued on a post-port one, and a
pre-port party and a post-port party cannot finish one together; it finishes on the build it began on. The
release notes name that build (P03).

**2. A pdfcpu bump re-measures the rule.** `digestRulePdfcpu` (`internal/pdfops/digestpdfcpu.go`) names the pdfcpu each
rule was measured against, and `TestTheDigestRuleNamesThePdfcpuItWasMeasuredWith` fails when go.mod, the table or
`ContentDigestVersion` is edited alone (each arm planted in the test). Do not move the table without running
`build/pdfcpudiff.sh` and reading the digest column.

**3. Every new refusal has a disposition, and nothing is refused without a cause** (D4; per kind in
`memory/instruments/pdfcpu-upgrade/S04-refusals.md`). Carry — set aside for the validator and put back, ADR-115/168's
pattern: name trees past the walk budget, external-file filespecs, the checker's mistyped `/Widths`, an ExtGState `/Font`
and a MediaClip `/Alt`, the structure tree (checker only). Refuse with a named cause — where the document is malformed
and nib would only be guessing: nesting past 100 (`ErrNestedTooDeep`), reference cycles (`ErrReferenceCycle`) and
duplicate nodes (`ErrNodeNamedTwice`), each over pdfcpu's own words (`pdfread/refusal.go`). Adopt — upstream is right
and nib's workaround was the defect: the Bookmark kid, the media rendition `/P`. Population scan: 0 real documents newly
refused. A refusal that bounds a recursion, a cycle or a size is never carried without a test that the hostile shape
runs through write, optimize and the checker within bounded stack and memory.

**4. A form button is a radio by its `/Ff` flag** (D5; ISO 32000-1 12.7.4.2.4), the spec's rule and the one pdf.js
applies. `buttonKind(ff)` is the one answer (ADR-009); `widgetKind` and `formField.pushButton` route through it. The
**declared** move: 133 multi-widget check boxes in 9 of 206 corpus documents were fillable as radio groups on v0.13 and
are now check boxes. ADR-172's pdf.js patch is kept (it is on the window side). `pushButtonsAside` is kept as belt and
braces: with its push-button half off the form round-trip rows are identical on all 206 documents (v0.16.2 skips
push buttons itself), and retiring it is a later change that supersedes this clause. A widget whose `/Parent` reaches
nothing is carried by `unwalkableWidgets`, because v0.16.2 answers the whole export "corrupt field" where v0.13 skipped it.

**5. A file nib removes is removed from `/AF` too** (D6). v0.16.2 writes the catalog's `/AF` and the files it names.
`removeAllAttachments` — the one door remove-files and sanitize reach — strips `/AF` from every dictionary
(`dropAssociatedFileKeys`) and clears the read mark (`pdfread.ForgetAssociatedFiles`, one named caller); extract and
redaction were already clean. The write refusal stays for a **page's** `/AF` (`writePages.go` writes none).
Privacy residue is a defect, not a declared gap. **Declared:** rule 7 does not hash `/AF` (nothing in
`internal/pdfops/attachments.go` reads the key, as under rule 6), so a file reachable only through `/AF` is not covered by
`DocHash` in the window between convening and the first signature that ADR-013 names; the signatures, not `DocHash`,
bind a party to bytes. Hashing it entry by entry would be a rule 8.

**6. A cost regression is fixed before the merge, and the fix is a cache scoped to one call** (D8). The tests' own
floors are absolute figures measured on both builds, never ratios: a 600-word stamp 409 ms / 575 MB on v0.13.0 against
767 ms / 208 MB on v0.16.2 before the fix; `SplitRegions` 22 ms / 81 MB against 46 ms / 73 MB. `runeWidths`
(`ocrfit.go`) holds `font.CharWidth` per rune and `font.Descent` per size for ONE `StampTextLayer` call and is dropped
with it; nil asks pdfcpu as before. No state outlives the call, so no invalidation exists to get wrong.

**7. Nothing in nib turns on pdfcpu's network code.** v0.16.2 carries `validate/link_http.go` and
`pkg/pdfcpu/sign/revocation_http.go`, off by default (`ValidateLinks` false). `TestNibNeverTurnsOnPdfcpuNetworkCode` fails
if any nib Go file sets `ValidateLinks` to anything but false or imports pdfcpu's `sign` package. This is the egress
decision for the library: a document must not be able to make this machine fetch a URL it names.

**8. A restatement is re-read, not re-pinned** (D7). `restatements.tsv` is the inventory, its census test fails on a
citation of the old version without a row, and `TestTheContentCostGateNamesThePdfcpuItRestates` and
`TestTheReferenceDoorNamesThePdfcpuItRestates` name the pdfcpu they restate. The reference door is KEPT: three
annotation-action loops (`GoTo3DView /TA` through a Link `/A`, `Screen /A`, `Screen /AA`) still overflow the Go
stack in v0.16.2.

## Consequences

- Go 1.26 is a floor for anyone building nib from source (D2, v2.2.5).
- Superseded in part by this ADR, and not edited: ADR-055, 056, 064, 069, 079, 082, 116, 129, 136, 143, 164, 168, 173.
  Where one of them cites v0.13.0 behaviour it is the record of the premise at the time; read the current behaviour in
  the code comment, which cites v0.16.2.
- Declared: a rule-5/6 document cannot be hashed on this build by design (`ContentDigestRuleReadable` is rule 7 only),
  but rule 5's arm (`everyKind` false in `hashEffectivePage`, `attachments.go`) is still compiled and is dead code; removing
  it is a later cleanup.
