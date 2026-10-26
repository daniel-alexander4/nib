# PLAN — pdfcpu v0.13.0 → v0.16.2

**Dateline.** Seeded 2026-10-14 on Dan's instruction ("we need to upgrade"), reactivating `/pending 898`. The
factual base is the deepdive `memory/deepdives/2026-10-14-pdfcpu-v0130-to-v0162.md` and its saved evidence
`memory/instruments/pdfcpu-upgrade/` (the port's 48 failing tests, the 24 digest moves, the 287-file edit log,
`popscan.sh`). Every number below is from those unless marked otherwise.

**Status: unbuilt. P01.S01 is next.** The build order was converted to the planning format 2026-10-25.
**Passes:** `/plan-review` 2026-10-25 (13 seats; 8 critical, 19 warning, all adopted; report `memory/plan-reviews/2026-10-25.md`). The
pins below are tagged `plan-review pin: <seat>, 2026-10-25`. No decision was rewritten. The numbers in this plan are v1.218.8's;
HEAD at the review was v2.2.1 (P01.S01 re-measures).
**(plan-review pin: editor, 2026-10-25)** line 11 below reads "P01.S01" where it says "P01.S03": the re-run of the comparison is
P01.S01's instrument.


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
**(plan-review pin: forensic/SRE/security/data/crypto, 2026-10-25 — the gate, not the decision)** D1 retires TWO rules at once
(ADR-143/080 retired one), and `digestRuleSkew` today refuses far more than "continuing a ceremony": `ceremony.CheckRecord` skews
any document, signed or not (so `nib verify` of a FINISHED rule-5/6 ceremony PDF loses its roster verdict, `ceremonyReportOf`
→ `unreadable`), and `ReadMirror` skews every unsigned mirror (the stop route answers 409, `endCeremony` returns silently, the
close-out skips it and leaves the secret on disk, an ended copy cannot be read back). The skew is therefore refused only where a
digest would be computed (`DocumentHashFor`, `readMirrorAt`'s unsigned branch); record-level questions — identity, roster,
signatures, completeness, stop, close-out, read-back of an ended copy — accept a retired rule. P02.S08 owns the exits below. P03
owns the release-notes sentence and the pre-update warning (an un-ended ceremony in `~/nib/ceremonies`).
**(plan-review pin: crypto/PDF-format, 2026-10-25)** The new ADR states rule 7 as "rule 6's axes, read by pdfcpu v0.16.x" (the digest
is taken over pdfcpu's post-validation, post-optimize graph, `ReadOptimized`), and a guard fails when go.mod's pdfcpu version differs
from a constant beside `ContentDigestVersion`, so a later pdfcpu bump re-measures and bumps the rule instead of moving digests
silently. Observation that discharges it: the guard is red when either side is edited alone.


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
**(plan-review pin: architect/SRE/performance, 2026-10-25)** Branch rules. (1) Branch commits take NO `VERSION` bump and no ADR number;
the merge commit carries ONE bump (a minor — the digest flag day is user-visible) and the ADR numbers. (2) The branch is rebased onto
main, with `VERSION` chosen above main's, BEFORE the P02-close gate and never after a green gate. (3) Main is frozen for pdfcpu-touching
changes during P02 (`/pending 897`, `899` touched restating code), or each such change is mirrored into the branch the same day.
(4) `CLAUDE.md`, memory files and the seam inventories are edited in the main checkout; only tracked files are committed on the
branch (a worktree does not materialise untracked files). (5) Revert: record main's tip SHA before the merge; merge `--no-ff` so one
`git revert -m1` undoes it; before publish a red result after the merge reverts the merge, never patches it in place (a patch is code
no review saw). A stalled P02 leaves the branch unmerged and main at v0.13.
**(plan-review pin: architect, 2026-10-25)** P02 slices close on a NAMED red→green set from `port-failing-tests-v0162.txt` (48), not the
full suite: each slice exit lists the failing tests it turns green and the remaining red count, falling monotonically to 0; the full
gate runs once, at the P02 close. A P02 slice's done marker is written on that set, never on a full-suite claim.


### D4 — No document nib reads today becomes unreadable without a named cause *(rung 1)*
Each of v0.16's new refusals gets one of three dispositions, recorded per kind in the seam inventory:
**carry** (set it aside for the validator and put it back, ADR-115/168's pattern — the default where nib reads
nothing from it), **refuse with a named 422** (where the document is genuinely malformed and nib would only be
guessing), or **adopt** (upstream is right and nib's old reading was the defect). "pdfcpu now says no" is not a
disposition. The population scan must show **0 real documents newly refused** at P02's close, as the raw port did.
**(plan-review pin: security/privacy/editor/PDF-format, 2026-10-25)** (1) "~25 kinds" is ~25 failing tests in ~8 kinds. (2) **adopt** means
"accept upstream's refusal and delete nib's workaround"; a fourth disposition, **fix nib's writer**, covers nib's own page-unlink
output. (3) A refusal that exists to bound recursion, a cycle or a size (the 100-level traverse cap, deep name trees, structure-tree
cycles) defaults to **refuse with a named 422**; a carry for one needs a test that the carried hostile shape runs through write,
optimize and the checker with bounded stack and memory (a Go stack overflow is not recoverable). (4) The kinds also include v0.16's
`ErrNameTreeDuplicate`, `ErrPageTreeDuplicate`, `ErrStructureTreeDuplicate` and cycle errors, which refuse before ADR-164's marker and
ADR-171's removal can apply; a deep tree the walk cannot read whole is carried as ADR-168 does. (5) Nothing newly refuses AND nothing a
removal door meant to drop newly survives its carry: each carry kind names the removal door that reaches the carried object.
**(plan-review pin: security, 2026-10-25)** v0.16.2 carries network code (`validate/link_http.go`, `sign/revocation_http.go`), off by
default (`ValidateLinks` false, `model/configuration.go:833`, read). A source scan fails if any nib file sets `ValidateLinks` or imports the
revocation path; the new ADR names it as the egress decision.


### D5 — Forms take the spec's radio rule *(rung 1)*
v0.16 classifies a button as a radio by the `Ff` Radio flag (ISO 32000-1 12.7.4.2.4); v0.13 by widget count. The flag
is the spec and the window (pdf.js) reads the flag too, so nib adopts it; ADR-172's pair handling, `heldOptionValues`
and `build/formrt.sh`'s classification are re-keyed to it. If ADR-172's patch is then inert it is dropped, with its pin.
**(plan-review pin: PDF-format/data/editor, 2026-10-25)** The 133 multi-widget check boxes (11 documents) were fillable as radio groups on
v0.13 and are not on v0.16: a counted, DECLARED regression, not a silent one (#164 and #13 are named declared moves by description, not
by corpus index). `/Ff` is read in more places than listed — `formfields.go:97`, `pagemap.go:457-461` and `test/formrt/roundtrip.test.mjs:285`
(`f.kids !== 1`) — and route through ONE `kindOfButton()` over the flag (ADR-166's one answer); D7 decides whether `pushButton()` /
`pushButtonsAside` duplicate v0.16's `FieldPushbutton`.


### D6 — A file nib removes is removed from `/AF` too *(rung 1)*
v0.16 writes the catalog's `/AF`. Remove-attachments, extract and redaction strip every `/AF` entry naming a file they
take away, at ONE door (ADR-009); ADR-136's refusal is re-read against the new writer and superseded where it now
over-refuses. Privacy residue is a defect, not a declared gap.
**(plan-review pin: privacy/PDF-format/data/crypto/security, 2026-10-25)** The doors are wider than three: sanitize
(`StripActive` → `removeAllAttachments`, `scan.go:1294`, which drops only /Names /EmbeddedFiles), `pruneNames` (`pageselect.go:1020`:
extract, split, redaction), `RemovePages`, `Collect`, `NUp`, `Booklet`, `CarryAttachments` (merge), and `structcarry.go`'s element strip
(the named exemption). A catalog /AF can name a file that is NOT in the name tree, and v0.16 writes the catalog /AF (`write.go:382`,
read) but still writes no page /AF (no `AF` entry in `writePages.go`, grepped), so ADR-136's refusal STAYS for page /AF and
is superseded for the catalog only. The ONE door is a prune in `pdfread.Write`: drop /AF entries whose filespec is not reached by
/EmbeddedFiles, a kept page's /FS or a kept structure element. The refusal is deleted in the SAME commit that adds the strip.
P02.S06 owns the exits below. Rule 7 either hashes catalog /AF entry by entry (the flag day makes it free now) or the new ADR records
the exclusion — a surviving /AF-only file is a swap channel into the convene-to-first-signature window (ADR-013).


### D7 — A restatement is re-read, not re-pinned *(rung 1)*
Every place nib restates pdfcpu (P01.S02's inventory) is re-read against v0.16.2's source and gets one of: **delete**
(upstream fixed what it guards — the test then asserts upstream's behaviour, so a regression upstream is caught),
**re-cite** (same behaviour, new line numbers), **re-derive** (behaviour changed; re-measured). A pin test that
re-pins a moved number without a re-read is the failure this decision exists to prevent.
**(plan-review pin: verification/data/QA/architect/editor/PDF-format, 2026-10-25)** `TestTheContentCostGateNamesThePdfcpuItRestates` and
`TestTheReferenceDoorNamesThePdfcpuItRestates` (`contentcostpin_test.go:13-29`) compare go.mod to `"v0.13.0"` only — editing the literal
turns them green with no re-read, which is the failure this decision forbids. The inventory is DATA IN THE REPO
(`internal/pdfread/restatements.tsv`: site, v0.13 cite, v0.16.2 cite, verdict, version re-read), with one census test that fails on a
citation without a row, and each `re-cite` row's v0.16.2 `file:line` is checked against `$GOMODCACHE/github.com/pdfcpu/pdfcpu@v0.16.2`
(present). The two tripwires become behavioural pins, red against a planted v0.13-shaped fixture. A `delete` verdict is red-proofed
with the pre-fix reproducer run against `pdfcpu@v0.13.0` from the module cache (mutating nib code cannot turn a test of upstream's
behaviour red). Observation that discharges "re-read": the census reads the vendored source text at the cited line.


### D8 — Cost regressions are fixed before the merge, not after *(rung 1)*
The tests' own floors (stamp memory, `SplitRegions`, `TextWidth` per call) are the budget. Where v0.16 is slower, nib
caches or works around it as it did for v0.13's quadratic walks; a floor is never raised to pass.
**(plan-review pin: performance, 2026-10-25)** `TextWidth` has no floor (grep: only a comment, `stampfit_test.go:346`); v0.16.2 clones the
metrics per call (`font/repository.go:230-243`, read) and the OCR stamp reaches it per word (`server/ocr.go:67` → `StampTextLayer`). Before
P02.S07 a 600-word OCR-stamp wall-clock/CPU test is written, and absolute v0.13 numbers (MB, ms) are recorded for every floor on a quiet
machine, median of ≥5 on each build — absolute figures, not ratios. The floors are also run once on go1.25 and once on go1.26 on v0.13
(GreenTea GC is on by default in 1.26; it must not be attributed to pdfcpu). **A cache or workaround is logic on a request path:
the mitigation is shown to Dan before it is built** (global CLAUDE.md); if it needs a vendor patch to `UserFont` it re-opens D9.


### D9 — ADR-173's fork goes *(already decided, ADR-173)*
`third_party/pdfcpu`, its `replace`, `TestThePdfcpuPatchIsTheCryptFilterLineOnly` and the sixteen walkers' skips that
exist only for it. The crypt-filter test stays, asserting upstream's behaviour.
**(plan-review pin: editor, 2026-10-25)** "The sixteen walkers' skips" is unsupported (24 test files skip `"third_party"`, and the digitorus forks
stay, ADR-066/142). The slice greps the skips and removes only those that name pdfcpu.


---

## Build order
**(plan-review pin: architect/editor, 2026-10-25 — structure)** Before `/createcode` walks this: slices become `####` headings
(`#### P01.S01 — …`) so a done/blocked marker has a heading to sit on, each with Scope / Refs / Acceptance, and each phase carries a
Goal and Exit criteria line (PLANNING.md "The build-order format"). A mechanical conversion; no coordinate changes except the S03
split pinned below. **Done 2026-10-25** (headings, Refs, phase Goal/Exit criteria, S03a–c).
**(plan-review pin: Go, 2026-10-25 — decision to take before P02.S01)** Which `context.Context` each of the ~740 sites gets. Install,
commit, hand-off and ceremony/mirror-write doors take `context.WithoutCancel(r.Context())` or `Background()` (handoff.go:150: the request
context is deliberately NOT honoured, `/pending 783`); only read-only previews (map, tags, ocr/pages) take `r.Context()`;
`Canceled` / `DeadlineExceeded` map to "no answer, no state change" in ONE place (`httpError` classifies by sentence, so a wrapped cancel
must not read as a named 422); a source scan fails on `Background()` in a handler file outside the named doors. The Go context is named
`c` (pdfcpu's own) and no existing `ctx *model.Context` is renamed (273 nib functions take one).


### P01 — Before the port, on main
Goal: everything that can be separated from the port is, and the instrument that judges the port exists first.
Exit criteria: the comparison instrument is committed and non-vacuous; the restatement inventory is in the repo; Go 1.26 is on main with pdfcpu still v0.13 and the gate green.

#### P01.S01 — The comparison instrument, committed *(done 2026-10-25, v2.2.4)*
Refs: D7, D1.
`build/pdfcpudiff.sh <listfile> <treeA> <treeB>`: per document,
each build's read verdict (refusal text), `ContentDigest` under the current rule, and the formrt counts; the output
is the DIFF only. Generalises the deepdive's `popscan.sh` (CLI-level) to nib's own reads. Corpus: a local list, never
committed (as `accuracy.sh`). **Exit:** run HEAD against itself → empty diff; run against the saved v0.16.2 port tree
→ the 24 digest moves and 1 newly accepted file the deepdive measured.
**(plan-review pin: QA/verification/privacy/editor/forensic/crypto/PDF-format, 2026-10-25 — own criterion, own evidence)**
Exits that stand on their own: (a) **corpora are committed as path-list + per-file sha256 (paths stay local), with the v0.13
baseline table**; every instrument (`pdfcpudiff.sh`, `formrt.sh`, `accuracy.sh` use) FAILS — never skips — on an absent, empty or
changed list, and prints "N read of N listed" even when the diff is empty (`~/nib/formrt-corpus.txt` is absent today, so `formrt.sh`
skips and reads green). (b) **non-vacuity:** a diff against a tree carrying a known one-line digest-affecting mutation names that file.
(c) the digest column is computed under a FIXED rule with the version number excluded (the rule number is hashed, `attachments.go:549`,
so 6→7 changes every digest) and runs while rule 6 still exists. (d) the 24-file list is regenerated on the starting tree and committed
(the saved `digest-moved-v0162.txt` holds 29 paths; five are ancestry reports the deepdive excludes); Recology and the 4 newly
digestible files are named expected moves. (e) "1 newly accepted file" becomes **0** (`/pending 899` is fixed). (f) the "HEAD vs saved
port tree" half moves to P02.S08 — no such tree exists and building one needs P02.S01. (g) columns added: `sign.Verify` verdict (State,
signer fingerprints, AddedAfter + cause, Unchecked, ceremony report), a hash of nib's main reads (page text, map, field list), and a
count of live objects unreachable from the catalog. (h) **output is an index or short hash per document, never a path; refusal text is
reduced to its class** (a pdfcpu refusal can quote a field, font or document string); a fixture test greps the output for the fixture's
path and name. P03 records counts only. (i) the population scan reports timeouts and unreadable apart and is valid only with
scanned == listed.
**Built (v2.2.4), with its declared limits.** `build/pdfcpudiff.sh` (`<list> <treeA> <treeB>`, `--rows`, `--manifest`, `--selftest`) lays `build/pdfcpudiff.go.src` into a tree through `go test -overlay`; columns: read class, digest under rule 6, `sign.Verify` verdict, answer facts, scan kinds, field counts by type and page, unreachable-object count, a hash of the page maps of pages 1–3. The corpus rule is `tiergate.sh`'s `nib_corpus_check` (empty list always fails; a manifest is verified; with `NIB_REQUIRE_TIERS=1` a missing manifest fails), called by `formrt.sh` and `accuracy.sh` too. `--selftest` proves exits (b), (h) and the refusal of an empty, changed or manifest-less list. Baseline: `memory/instruments/pdfcpu-upgrade/baseline-v2.2.3.tsv`, 2,023 read of 2,023 listed (list sha256 `611be0a4…`, local `~/nib/population-corpus.txt` regenerated by `find` — the original 2,326-file list was never saved; `/home/dan/ancestry` is excluded because another session rewrites it), 0 timeouts, 0 unreadable, 0 panics, 2,013 read ok and 10 refused in 4 classes. **Limits, not discharged here:** (d) the 24-file list and (e) the newly-accepted count need a port tree and move to P02.S08 with (f); (g) page TEXT is not a column (the map hash covers pages 1–3 only) and field names are deliberately not hashed; `unreachable` counts object-stream containers as live-but-unreferenced, equal on both trees by construction, so only its delta means anything; a trailing-whitespace path differs between bash and Go (`TrimSpace`); a document that outlasts `NIB_PDFCPUDIFF_TIMEOUT` (default 90 s; the baseline ran at 30 m) fails the run; `formrt-corpus.txt` is still absent, so the form round-trip baseline is the P02.S05 pre-step; the row writer has been compiled against v0.13 only — its pdfcpu identifiers were read against v0.16.2's declarations but not built.

#### P01.S02 — The restatement inventory *(done 2026-10-25, v2.2.4)*
Refs: D7.
Every comment or code site citing pdfcpu source (`v0.13.0`, or a pdfcpu
file:line), one row each: site, what it restates, the v0.13 line, the v0.16.2 line (or "gone"), and D7's verdict
to be filled in P02.S03. Written to `memory/instruments/pdfcpu-upgrade/restatements.md`. A sweep — delegate the
reading, keep the table. **Exit:** the table's row count equals the grep's hit count.
**(plan-review pin: editor/PDF-format/data/architect, 2026-10-25)** The table is `internal/pdfread/restatements.tsv` (D7's pin), not a
memory file. Three reconciliations, each its own exit: `grep -rn 'v0\.13\.0' --include=*.go` outside `third_party` (67 hits / 43 files
at the review — the dateline's 66/42 is `.go` only; ≈100/62 with docs), a second grep for bare pdfcpu `file:line` citations, and a
sweep for unmarked behaviour claims (`formfields.go:157` states the radio-by-widget-count rule D5 retires, no version token; ~71 unvalidated
"pdfcpu reads / writes" comment hits). A sweep reader keeps the genuine restatements.
  **Built (v2.2.4), with its declared limits.** `internal/pdfread/restatements.tsv` (168 rows: 62 version-token, 71 file-line, 35 unmarked-behaviour) and two guards: `TestEveryV013CitationHasARestatementsRow` (every `.go` line outside `third_party`, `web`, `test`, `dist` naming `v0.13` has a row within 12 lines — the first draft matched only `v0.13.0` and missed three) and `TestRestatementRowsPointAtRealLines` (the site exists, a version-token row's line still names `v0.13`, the verdict is one of pending/delete/re-cite/re-derive). Each probed red (row removed; site moved). **Limits:** 71 rows carry `unlocated` for the v0.16.2 cite and 2 `gone`/2 `none-found` — P02.S03a–c locate them; kind (c) is a PARTIAL pass (`internal/sign` not swept line by line); the bare `file:line` reconciliation has no census (hand-filtered, ≈71); docs, scripts and `.mjs` citations are outside the census; the ±12 window absorbs a new citation next to an existing row of the same file. Behaviour changes the sweep found in v0.16.2 that P02.S03 reads first: `optimizeResourceDicts` is now only `ConsolidatePageResources` (`optimize.go:1788`); `passDecodesPageContent`/`passInheritsResources` became `shouldOptimizeResourceDicts` (`optimize.go:1841-1847`); `EqualObjects` no longer treats null as equal to anything (`model/equal.go:76-78`); `isRadioButtonGroup` is flag-based (`form/form.go:462-475`); `write.go:382` writes `AF`; `lookupEI` takes an inline-image length; the validator has `PageTreeVisit`/`treeVisit`/`CheckRecursionDepth`.

#### P01.S03 — Go 1.26 (D2)
Refs: D2.
`go.mod` `go 1.26.0`, pdfcpu still v0.13. **Exit:** full Go suite through `suiterun`,
tiers 2–3, `build.sh` (all artifacts) and `gen-notices.sh` green; `go version -m` on the built binary says 1.26.
**(plan-review pin: Go/architect/SRE, 2026-10-25)** Also recorded, not only green: the cost floors on go1.25 and go1.26 (D8 pin); tier 4
and 6 "fire" or "not fired, because …" (a toolchain bump recompiles `internal/server`, `p2p`, `rendezvous`); the go.mod flip changes four
GODEBUG defaults (`cryptocustomrand`, `httpservecontentmaxranges` read as not mattering; `urlstrictcolons` unverified; `tlssecpmlkem`
opaque, unmeasured). The README badge ("Go-1.25+") and the CONTRIBUTING build note move to 1.26.


### P02 — The port, on branch `pdfcpu-v016` (D3)
Goal: v0.16.2 replaces v0.13.0 with every nib workaround re-read, every new refusal dispositioned, and the digest on rule 7.
Exit criteria: the P02 close gate below is green on the MERGED tree; `pdfcpudiff.sh` shows only declared moves; each slice closed on its named red→green set.

#### P02.S01 — Mechanical
Refs: D3, D9.
go.mod to v0.16.2, drop the fork (D9), thread `context.Context` (the deepdive's edit log
is the map; it is redone on the current tree, not copied). **Exit:** `go build ./...` and `go vet ./...` clean; the
suite RUNS — its reds are the list below, not panics.
**(plan-review pin: architect/performance/Go, 2026-10-25)** The exit is: `go build ./...` and `go vet ./...` clean, plus
`GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` (vet compiles tagged tests; `TestEveryPlatformCompiles` skips under `-short` and builds
only non-test files); the suite run ONCE with failures recorded; "no panics" is P02.S02's alone (a panic aborts its package binary). The
edit is a committed, re-runnable rewrite script with a scan that every hunk only adds or passes a context; the review checks S01 by
re-running the script, and reads S02–S09 as the semantic diff. A "dependency delta" row goes in the seam inventory: removed `hhrutter/lzw`,
`hhrutter/pkcs7`, `pkg/errors`, `yaml.v2`; added `google/jsonschema-go`, `xdg-go/stringprep`, `go.yaml.in/yaml/v3`; raised `x/crypto`
0.54→0.57 (vault, TLS, ceremony), `x/image` 0.41→0.46, `x/text` 0.40→0.42, `hhrutter/tiff`; with licence, `gen-notices.sh` re-run and a
vulnerability check. The `x/crypto` raise is why tiers 4 and 6 and the vault tests are in the close gate. The red list is compared with a fresh run on the
current tree, not the saved file.

#### P02.S02 — Port artifacts
Refs: D9.
The user-font reload (`registerMetrics` → `ReloadUserFonts`) breaking OCR and a second
stamp; `freerefs_test.go`'s `go:linkname` signature; the structure-tree panic after a refusal. **Exit:** no test
panics; OCR and stamping tests green.
**(plan-review pin: Go, 2026-10-25)** `freerefs_test.go:21`'s `go:linkname` resolves in v0.16.2 as `fixReferencesToFreeObjects(c context.Context,
ctx *model.Context) error` (`optimize.go:1722`, read) and a mismatched declaration still LINKS (undefined behaviour, no link error): the declaration
and the comment ("fails this file's link") are updated as a D7 restatement.

#### P02.S03a — Restatements (D7): the `pdfread` cluster
Refs: D7.
Work P01.S02's table. Expected heaviest: `pdfread/optimize.go` (10 citations),
`contentcost.go`, `pdfread.go`, the set-asides, `pagesim.go`, the reference door. **Exit:** every row has a verdict;
the `...NamesThePdfcpuItRestates` guards green against v0.16's source.
**(plan-review pin: architect/performance, 2026-10-25)** Split by package into separate slices (now P02.S03a–c, 2026-10-25):
the pdfread cluster, pdfops, then uacheck / contentstream / root. The exit is the completed D7 table (verdict plus v0.16.2 `file:line` per row,
census green) — the two old guards are tripwires only and "green" on them discharges nothing. The pdfread slice precedes S07 (it re-derives
`contentcost.go` and `optimize.go`, which are the cost-floor logic).

#### P02.S03b — Restatements (D7): `pdfops`
Refs: D7.
The `pdfops` rows of P01.S02's table. **Exit:** every row has a verdict, a v0.16.2 `file:line` and the census green; the turns-green set is named.

#### P02.S03c — Restatements (D7): `uacheck`, `contentstream` and the root
Refs: D7.
The remaining rows. **Exit:** as S03b; the table has no row without a verdict.

#### P02.S04 — New refusals (D4)
Refs: D4.
The ~25 kinds: the 100-level `traverse` cap (deep form chains, field DAGs), an
embedded-file entry with no `/EF` (external file specifications), ADR-164's deep/unread name trees (now fatal even
relaxed), structure-tree defects nib reports as cannot-check (cycle, self-listing, unreadable kid), duplicate
number-tree nodes, Rendition/media actions, an output of nib's own page-unlink.
**(plan-review pin: QA, 2026-10-25)** uacheck's veraPDF-agreement: record the v0.13 pair (documents with a verdict, agreements) per harness; the
v0.16 verdict count is at least as large or each lost document is named; the close fails on a skip (shown by the `suiterun` log).
 **Exit:** each kind dispositioned in
the seam inventory; uacheck's veraPDF-agreement tests at their v0.13 counts; population scan 0 newly refused.
#### P02.S05 — Forms (D5)
Refs: D5.
Radio by `Ff`; re-key ADR-172, `heldOptionValues`, formrt; the document (#164) v0.16 can
no longer export. **Exit:** `build/formrt.sh` on the 203-document list: every value read and written back as on
v0.13, with the reclassification as the only declared move.
**(plan-review pin: PDF-format/editor, 2026-10-25)** The declared moves are the 133 check boxes (11 documents), #164 (no longer exports) and
#13 (list and combo fills change) — see D5's pin; "the only declared move" is replaced by that list.

#### P02.S06 — `/AF` (D6)
Refs: D6.
**Exit:** a test per door — remove-attachments, extract, redaction — that the removed
file is reachable from nothing, red on the unstripped writer.
**(plan-review pin: privacy/PDF-format/data/forensic, 2026-10-25 — own criterion, own evidence)** (1) a census over every caller of the D6 door
in the style of `TestEveryWriteRoutesThroughTheDoor` (structcarry the named exemption); (2) a scan Kind `associatedFiles` with its row in
`TestEveryScanKindIsStrippedOrNamedAsLeft`; (3) one test per door of D6's list, asserting by a graph walk from the trailer — never a byte
search, streams are compressed — that the removed file's filespec and its /EF stream are unreachable (annotation /FS and structure-element
/AF holders included); redaction drops the whole set; (4) a page-/AF document still refuses; (5) a document whose files only /AF reaches,
put through remove-attachments, leaves no filespec object and no /EF stream; (6) the slice owns `TestWhatARewriteDoesToAnAssociatedFile`,
`ErrAssociatedFileLost` and the `associated-file` 422, and deletes the catalog refusal in the same commit as the strip. "Extract" means page
extract (`ExtractAttachment` removes nothing).

#### P02.S07 — Cost (D8)
Refs: D8.
**Exit:** every cost floor at its v0.13 value or better, measured both builds.
**(plan-review pin: performance/QA, 2026-10-25)** Per D8's pin: absolute v0.13 numbers recorded first (the stamp / `SplitRegions` floors are
ratios today, `stamphold_test.go:277-297`, `splitregionscost_test.go:57-79`), `TextWidth` per page now bounded by a test, quiet-machine medians, and
the mitigation shown to Dan before it is built. "Or better" is replaced by the absolute figure.

#### P02.S08 — Digest rule 7 (D1)
Refs: D1.
`ContentDigestVersion` 7; rules 5/6 retired via `digestSkew`; goldens regenerated
under 7 only (`-v5`/`-v6` goldens never regenerated — they become the retired rules' record); new ADR.
**Exit:** a rule-6 record on the new build answers "older-nib", never a mismatch; the comparison instrument's digest
column moves on exactly the deepdive's 24 files and nothing else.
**(plan-review pin: crypto/data/forensic/SRE/verification/architect, 2026-10-25 — own criteria, own evidence)** (1) the content-only digest
column (fixed rule, version number excluded — P01.S01 (c)) moves on exactly the regenerated 24-file list plus the named expected moves, each with a
named repair cause; a move without one fails; "every digest differs under 7" is expected by construction. (2) hand-built fixed-byte fixtures, one
per repair cause, with rule-7 goldens, and the moved files digested in N separate processes with identical output (the duplicate-subset-font
survivor is first-match over a map-built list, determinism unmeasured). (3) a FINISHED rule-5/6 ceremony document still gets its roster
verdict; a rule-5 and a rule-6 unsigned mirror can be stopped and moved to `ended/`, and an ended copy read back, on the rule-7 build;
`endCeremony` raises a visible failure, never silence. (4) the retired-rule goldens: ADR-143 §3 ("a golden for a rule nothing computes pins
nothing") and `TestEveryReadableRuleIsPinnedUnderItsOwnNumber` govern — the -v5/-v6 files are deleted and only -v7 written (git history is the
record), the six test files naming `legacyContentDigestVersion` are listed (`digestrule6_test.go`, `validatorloss_test.go`, `lawone_test.go`,
`digestambiguous_test.go`, `digestrule5_test.go`, `digestgolden_test.go`), and the slice states whether the retired rules' compute code stays and
separates "can compute" from "accept the record" (`ContentDigestRuleReadable`, `attachments.go:381-383`). (5) the "HEAD vs port tree" comparison
from P01.S01 runs here, on a tree built in P02. (6) S08 follows the S03–S05 slices; its turns-green list includes the two ContentDigest
byte-identical corpus tests. The new ADR also names the egress scan and the D1 / D6 pins.

#### P02.S09 — Pins and docs
Refs: D1, D7, D9.
`contentcostpin`, notices, the red-proof table rows that staled, every ADR citing v0.13
behaviour (superseded, not edited), `CLAUDE.md`, `CONTRIBUTING.md`.
**Exit:** no `v0.13.0` citation remains outside a superseded ADR or the retired-rule record; the notices are regenerated (`./build/gen-notices.sh`); `CLAUDE.md` and `CONTRIBUTING.md` (main checkout) name v0.16.2, Go 1.26 and rule 7.

**P02 close, before the merge:** `/code-review` of the branch; the full gate on the MERGED tree — Go suite through
`suiterun`, tiers 2–3, and **tiers 4 and 6** (the digest is what a ceremony checks across machines); `accuracy.sh`
and `formrt.sh` against their v0.13 baselines; `pdfcpudiff.sh` showing only declared moves. Then merge.
**(plan-review pin: forensic/crypto/PDF-format/QA, 2026-10-25 — own criterion, own evidence)** The close adds: over the corpus's signed documents
the `sign.Verify` verdict is identical on v0.13 and v0.16 except declared rows, including one large signed document (v0.16's always-on read budget and
~25 new refusals can turn a signed file `Invalid`/`Unchecked` with no digest move; `verify.go:925-943`); the live-unreachable-object count is
unchanged across sanitize, extract and split on the corpus; any instrument that skips fails the close. The branch is rebased before this gate, never after.


### P03 — After the merge, on main
Goal: the port ships, or waits, on Dan's word.
Exit criteria: the population scan re-run on the released binary; `/pending 898` closed; release notes carry the flag-day sentence.
Release is Dan's call (`./build.sh --publish`). Re-run the population scan on the released binary; close `/pending
898`; record the comparison instrument's last output beside the deepdive.
**(plan-review pin: SRE/forensic, 2026-10-25)** Before `--publish`: the release notes state the digest flag day and the exact older build to finish an
un-ended ceremony on (nothing offers the older binary or `.deb`), and the update popup warns when `~/nib/ceremonies` holds an un-ended ceremony. A bad rule-7
release cannot be pulled (`versionLess` offers nothing older): the answer is a fix forward under a higher version. A pre-port party and a post-port
party cannot finish one ceremony together; the pre-port side sees a dial refusal naming no cause (the post-port side sees `older-nib`). "or convene it
again" is dropped from the sentence for a document that already carries a signature.


---

## Out of scope
pdfcpu features nib does not use yet. Any change to what nib *offers*; this plan only keeps what nib does while the
library under it changes.

## Standing caveats
- The 24 digest moves and 0 population refusals were measured on a v1.218.8 tree; P01.S01 re-measures on the tree
  the port starts from.
- Field impact of D1 is unknown: no live ceremonies on this machine (`~/nib/ceremonies` empty, 2026-10-14).
- go 1.26 is a floor for anyone building nib from source.
- **(plan-review pin: editor, 2026-10-25)** The digest, refusal and form figures were measured on v1.218.8; HEAD at the review was v2.2.1.

