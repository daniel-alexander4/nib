# PLAN — pdfcpu v0.13.0 → v0.16.2

**Dateline.** Seeded 2026-10-14 on Dan's instruction ("we need to upgrade"), reactivating `/pending 898`. The
factual base is the deepdive `memory/deepdives/2026-10-14-pdfcpu-v0130-to-v0162.md` and its saved evidence
`memory/instruments/pdfcpu-upgrade/` (the port's 48 failing tests, the 24 digest moves, the 287-file edit log,
`popscan.sh`). Every number below is from those unless marked otherwise.

**Status: unbuilt. P01.S01 is next.** Not yet through `/plan-review`.

**Target: v0.16.2**, the newest release (`go list -m -versions`, 2026-10-14; `v1.0.0-test` is a test tag, not a
release). If a newer release lands before P02 starts, P01.S03 re-runs the comparison against it and the target moves
with Dan's word, not silently.

---

## What the upgrade is

**A port of nib's v0.13 workarounds, not a dependency change.** Nib restates pdfcpu's behaviour in ~66 places across
42 files (`grep -c v0.13.0`, outside `third_party`) — the optimize-cost estimate, the reference door, the free-
reference pass, the page walks, the set-asides of ADR-115/116/129/136/145/164/168 — and every one is written against
v0.13's source by line number. The API change itself is mechanical: ~740 edits, mostly a `context.Context` threaded
into every call, and go ≥ 1.26.

**What it buys, measured:** upstream's own fixes from here on (nib is on a fork since ADR-173 — one line, but a fork);
the Recology hang and the disclosure's crypt filter are already handled on v0.13 (v1.218.9, v1.218.10), as is the one
document v0.16 newly accepts (`/pending 899`). So the gain is **being current**, not a document that reads today
only on v0.16. That is the reason to do it carefully rather than quickly.

**What it costs, measured on the deepdive's port tree (v1.218.8 + v0.16.2):**

| What | Size |
|---|---|
| `ContentDigest` moves on unchanged bytes | 24 of 2,326 real files; six validator/optimizer repairs |
| Documents nib reads today that v0.16 refuses | ~25 kinds in the test fixtures; **0** in the 2,326-file population |
| Failing Go tests on the raw port | 48 (6 pins by design, ~25 refusals, cost floors, port artifacts) |
| Form reading | radio vs check box by the `Ff` flag: 133 multi-widget check boxes, 11 documents' Yes/No pairs |
| Privacy | catalog `/AF` is now WRITTEN — remove-attachments, extract and redaction can leave a file reachable |
| Cost | embedded-face `TextWidth` 6.7–317 ms per call; a 600-word stamp 207 MB |
| Toolchain | go ≥ 1.26 (this machine: go1.25.10, `GOTOOLCHAIN=auto`) |

---

## Decisions

### D1 — The digest moves to rule 7 at the port *(Dan, 2026-10-14, /discuss on /pending 892)*
`ContentDigestVersion` 6 → 7. Rules 5 and 6 are RETIRED, through ADR-143's existing path: a record under them is
answered `digestSkew` "older-nib" (wire code 18, `ErrCeremonyRuleRetired`), never reported as a mismatch. The digest is
not undone repair by repair. **Consequence, declared:** a ceremony convened on a pre-port build cannot be continued on
a post-port one; it finishes on the build it began on. New ADR.

### D2 — Go 1.26 lands first, on v0.13, by itself *(rung 1)*
The toolchain is the one change that can be separated from the port, so it is: `go 1.26.0` in `go.mod` with pdfcpu
still v0.13, the whole gate green, released or not as Dan chooses. A red after that is pdfcpu's, not Go's.
`GOTOOLCHAIN=auto` fetches the toolchain into the module cache with no system change; `build.sh`, the `.deb` and
`gen-notices.sh` are checked under it. A contributor on go < 1.26 is told by the `go` directive itself.

### D3 — The port is built on a branch and lands as one merge *(rung 1 — departs from "work on main", recorded)*
`memory/work-on-main-not-a-branch.md` says build on main. It is departed from here, for a reason that note does not
cover: from the first edit of P02 the tree cannot be green until the last — the bump flips every caller at once, and
this repo never commits a red suite. So P02 lives on `pdfcpu-v016` in a worktree, each slice committed there with its
own per-commit tier, and merges to main only at P02's close with the full gate green on the MERGED tree. P01 and P03
are on main. **Dan can reverse this in a word**; the alternative is P02 as one very large commit.

### D4 — No document nib reads today becomes unreadable without a named cause *(rung 1)*
Each of v0.16's new refusals gets one of three dispositions, recorded per kind in the seam inventory:
**carry** (set it aside for the validator and put it back, ADR-115/168's pattern — the default where nib reads
nothing from it), **refuse with a named 422** (where the document is genuinely malformed and nib would only be
guessing), or **adopt** (upstream is right and nib's old reading was the defect). "pdfcpu now says no" is not a
disposition. The population scan must show **0 real documents newly refused** at P02's close, as the raw port did.

### D5 — Forms take the spec's radio rule *(rung 1)*
v0.16 classifies a button as a radio by the `Ff` Radio flag (ISO 32000-1 12.7.4.2.4); v0.13 by widget count. The flag
is the spec and the window (pdf.js) reads the flag too, so nib adopts it; ADR-172's pair handling, `heldOptionValues`
and `build/formrt.sh`'s classification are re-keyed to it. If ADR-172's patch is then inert it is dropped, with its pin.

### D6 — A file nib removes is removed from `/AF` too *(rung 1)*
v0.16 writes the catalog's `/AF`. Remove-attachments, extract and redaction strip every `/AF` entry naming a file they
take away, at ONE door (ADR-009); ADR-136's refusal is re-read against the new writer and superseded where it now
over-refuses. Privacy residue is a defect, not a declared gap.

### D7 — A restatement is re-read, not re-pinned *(rung 1)*
Every place nib restates pdfcpu (P01.S02's inventory) is re-read against v0.16.2's source and gets one of: **delete**
(upstream fixed what it guards — the test then asserts upstream's behaviour, so a regression upstream is caught),
**re-cite** (same behaviour, new line numbers), **re-derive** (behaviour changed; re-measured). A pin test that
re-pins a moved number without a re-read is the failure this decision exists to prevent.

### D8 — Cost regressions are fixed before the merge, not after *(rung 1)*
The tests' own floors (stamp memory, `SplitRegions`, `TextWidth` per call) are the budget. Where v0.16 is slower, nib
caches or works around it as it did for v0.13's quadratic walks; a floor is never raised to pass.

### D9 — ADR-173's fork goes *(already decided, ADR-173)*
`third_party/pdfcpu`, its `replace`, `TestThePdfcpuPatchIsTheCryptFilterLineOnly` and the sixteen walkers' skips that
exist only for it. The crypt-filter test stays, asserting upstream's behaviour.

---

## Build order

### P01 — Before the port, on main

- **P01.S01 — The comparison instrument, committed.** `build/pdfcpudiff.sh <listfile> <treeA> <treeB>`: per document,
  each build's read verdict (refusal text), `ContentDigest` under the current rule, and the formrt counts; the output
  is the DIFF only. Generalises the deepdive's `popscan.sh` (CLI-level) to nib's own reads. Corpus: a local list, never
  committed (as `accuracy.sh`). **Exit:** run HEAD against itself → empty diff; run against the saved v0.16.2 port tree
  → the 24 digest moves and 1 newly accepted file the deepdive measured.
- **P01.S02 — The restatement inventory.** Every comment or code site citing pdfcpu source (`v0.13.0`, or a pdfcpu
  file:line), one row each: site, what it restates, the v0.13 line, the v0.16.2 line (or "gone"), and D7's verdict
  to be filled in P02.S03. Written to `memory/instruments/pdfcpu-upgrade/restatements.md`. A sweep — delegate the
  reading, keep the table. **Exit:** the table's row count equals the grep's hit count.
- **P01.S03 — Go 1.26 (D2).** `go.mod` `go 1.26.0`, pdfcpu still v0.13. **Exit:** full Go suite through `suiterun`,
  tiers 2–3, `build.sh` (all artifacts) and `gen-notices.sh` green; `go version -m` on the built binary says 1.26.

### P02 — The port, on branch `pdfcpu-v016` (D3)

- **P02.S01 — Mechanical.** go.mod to v0.16.2, drop the fork (D9), thread `context.Context` (the deepdive's edit log
  is the map; it is redone on the current tree, not copied). **Exit:** `go build ./...` and `go vet ./...` clean; the
  suite RUNS — its reds are the list below, not panics.
- **P02.S02 — Port artifacts.** The user-font reload (`registerMetrics` → `ReloadUserFonts`) breaking OCR and a second
  stamp; `freerefs_test.go`'s `go:linkname` signature; the structure-tree panic after a refusal. **Exit:** no test
  panics; OCR and stamping tests green.
- **P02.S03 — Restatements (D7).** Work P01.S02's table. Expected heaviest: `pdfread/optimize.go` (10 citations),
  `contentcost.go`, `pdfread.go`, the set-asides, `pagesim.go`, the reference door. **Exit:** every row has a verdict;
  the `...NamesThePdfcpuItRestates` guards green against v0.16's source.
- **P02.S04 — New refusals (D4).** The ~25 kinds: the 100-level `traverse` cap (deep form chains, field DAGs), an
  embedded-file entry with no `/EF` (external file specifications), ADR-164's deep/unread name trees (now fatal even
  relaxed), structure-tree defects nib reports as cannot-check (cycle, self-listing, unreadable kid), duplicate
  number-tree nodes, Rendition/media actions, an output of nib's own page-unlink. **Exit:** each kind dispositioned in
  the seam inventory; uacheck's veraPDF-agreement tests at their v0.13 counts; population scan 0 newly refused.
- **P02.S05 — Forms (D5).** Radio by `Ff`; re-key ADR-172, `heldOptionValues`, formrt; the document (#164) v0.16 can
  no longer export. **Exit:** `build/formrt.sh` on the 203-document list: every value read and written back as on
  v0.13, with the reclassification as the only declared move.
- **P02.S06 — `/AF` (D6).** **Exit:** a test per door — remove-attachments, extract, redaction — that the removed
  file is reachable from nothing, red on the unstripped writer.
- **P02.S07 — Cost (D8).** **Exit:** every cost floor at its v0.13 value or better, measured both builds.
- **P02.S08 — Digest rule 7 (D1).** `ContentDigestVersion` 7; rules 5/6 retired via `digestSkew`; goldens regenerated
  under 7 only (`-v5`/`-v6` goldens never regenerated — they become the retired rules' record); new ADR.
  **Exit:** a rule-6 record on the new build answers "older-nib", never a mismatch; the comparison instrument's digest
  column moves on exactly the deepdive's 24 files and nothing else.
- **P02.S09 — Pins and docs.** `contentcostpin`, notices, the red-proof table rows that staled, every ADR citing v0.13
  behaviour (superseded, not edited), `CLAUDE.md`, `CONTRIBUTING.md`.

**P02 close, before the merge:** `/code-review` of the branch; the full gate on the MERGED tree — Go suite through
`suiterun`, tiers 2–3, and **tiers 4 and 6** (the digest is what a ceremony checks across machines); `accuracy.sh`
and `formrt.sh` against their v0.13 baselines; `pdfcpudiff.sh` showing only declared moves. Then merge.

### P03 — After the merge, on main
Release is Dan's call (`./build.sh --publish`). Re-run the population scan on the released binary; close `/pending
898`; record the comparison instrument's last output beside the deepdive.

---

## Out of scope
pdfcpu features nib does not use yet. Any change to what nib *offers*; this plan only keeps what nib does while the
library under it changes.

## Standing caveats
- The 24 digest moves and 0 population refusals were measured on a v1.218.8 tree; P01.S01 re-measures on the tree
  the port starts from.
- Field impact of D1 is unknown: no live ceremonies on this machine (`~/nib/ceremonies` empty, 2026-10-14).
- go 1.26 is a floor for anyone building nib from source.
