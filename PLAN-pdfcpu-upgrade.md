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


### P01 — Before the port, on main *(done 2026-10-25, v2.2.6)*
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
**Built (v2.2.4), with its declared limits.** `build/pdfcpudiff.sh` (`<list> <treeA> <treeB>`, `--rows`, `--manifest`, `--selftest`) lays `build/pdfcpudiff.go.src` into a tree through `go test -overlay`; columns: read class, digest under rule 6, `sign.Verify` verdict, answer facts, scan kinds, field counts by type and page, unreachable-object count, a hash of the page maps of pages 1–3. The corpus rule is `tiergate.sh`'s `nib_corpus_check` (empty list always fails; a manifest is verified; with `NIB_REQUIRE_TIERS=1` a missing manifest fails), called by `formrt.sh` and `accuracy.sh` too. `--selftest` proves exits (b), (h) and the refusal of an empty, changed or manifest-less list. Baseline: `memory/instruments/pdfcpu-upgrade/baseline-v2.2.3.tsv`, 2,023 read of 2,023 listed (list sha256 `611be0a4…`, local `~/nib/population-corpus.txt` regenerated by `find` — the original 2,326-file list was never saved; `/home/dan/ancestry` is excluded because another session rewrites it), 0 timeouts, 0 unreadable, 0 panics, 2,013 read ok and 10 refused in 4 classes. **Limits, not discharged here:** (d) the 24-file list and (e) the newly-accepted count need a port tree and move to P02.S08 with (f); (g) page TEXT is not a column (the map hash covers pages 1–3 only) and field names are deliberately not hashed; `unreachable` counts object-stream containers as live-but-unreferenced, equal on both trees by construction, so only its delta means anything; a trailing-whitespace path differs between bash and Go (`TrimSpace`); pin (a)'s "FAILS, never skips" holds for `pdfcpudiff.sh` always, and for `formrt.sh` / `accuracy.sh` when the list is NAMED by `NIB_FORMRT_LIST` / `NIB_ACCURACY_LIST` and absent, or under `NIB_REQUIRE_TIERS=1` — an absent list at the default path with neither still skips (a fresh clone has no corpus); the manifest and the v0.13 baseline table live in the local list's directory and `memory/instruments/`, not in the repo (the table holds an index and 12-hex digests of somebody's documents); `sha256sum` falls back to `shasum -a 256`; a document that outlasts `NIB_PDFCPUDIFF_TIMEOUT` (default 90 s; the baseline ran at 30 m) fails the run; `formrt-corpus.txt` is still absent, so the form round-trip baseline is the P02.S05 pre-step; the row writer has been compiled against v0.13 only — its pdfcpu identifiers were read against v0.16.2's declarations but not built.

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

#### P01.S03 — Go 1.26 (D2) *(done 2026-10-25, v2.2.5)*
Refs: D2.
`go.mod` `go 1.26.0`, pdfcpu still v0.13. **Exit:** full Go suite through `suiterun`,
tiers 2–3, `build.sh` (all artifacts) and `gen-notices.sh` green; `go version -m` on the built binary says 1.26.
  **Built (v2.2.5).** `go.mod` `go 1.26.0` (pdfcpu still v0.13 and the fork); README badge `Go-1.26+`; CONTRIBUTING states the floor. Two test files that could not compile for Windows were made portable (`atomicfile` `inode`, `cli` `mkfifo`: a `unix` file and a stub each), so `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` are now clean — the P02 per-commit pin is satisfiable. Measured: full Go suite OK fp `52e72896e7c6`, 23m40s; jsdom 112 files / 858 tests; uirepro 58 files / 264 tests (2 skipped); `build.sh` built all eight artifacts at 2.2.5; `gen-notices.sh` changed nothing; `go version -m dist/nib-2.2.5-linux-amd64` says go1.26.0. Cost floors (stamp, `SplitRegions`, form bomb) passed on go1.25.10 and go1.26.0 (three runs each; durations 2.4-3.5 / 0.5-0.7 / 1.5-2.5 s then 1.9-2.0 / 0.44-0.46 / 1.24-1.29 s) — durations only, the tests log no absolute figures (P02.S07 pre-step adds them). Tier 4 and 6: **not fired** — no source of `internal/server`, `p2p` or `rendezvous` changed; the toolchain recompiled them and the full Go suite ran them. The four GODEBUG defaults that move at 1.26 were read, not exercised (`urlstrictcolons` unverified, `tlssecpmlkem` unmeasured).
**(plan-review pin: Go/architect/SRE, 2026-10-25)** Also recorded, not only green: the cost floors on go1.25 and go1.26 (D8 pin); tier 4
and 6 "fire" or "not fired, because …" (a toolchain bump recompiles `internal/server`, `p2p`, `rendezvous`); the go.mod flip changes four
GODEBUG defaults (`cryptocustomrand`, `httpservecontentmaxranges` read as not mattering; `urlstrictcolons` unverified; `tlssecpmlkem`
opaque, unmeasured). The README badge ("Go-1.25+") and the CONTRIBUTING build note move to 1.26.


### P02 — The port, on branch `pdfcpu-v016` (D3) *(done 2026-10-29, v2.3.0, merged `b73eb024`; main before the merge `f7c553fa`)*
Goal: v0.16.2 replaces v0.13.0 with every nib workaround re-read, every new refusal dispositioned, and the digest on rule 7.
Exit criteria: the P02 close gate below is green on the MERGED tree; `pdfcpudiff.sh` shows only declared moves; each slice closed on its named red→green set.

#### P02.S01 — Mechanical *(done 2026-10-25, merged in v2.3.0; branch sha ccb58743 predates the close rebase)*
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
**Built (branch `pdfcpu-v016`, worktree `/home/dan/repos/nib-pdfcpu-v016`, no VERSION bump).** go.mod v0.16.2, `third_party/pdfcpu`, its `replace` and `TestThePdfcpuPatchIsTheCryptFilterLineOnly` removed (D9; the "sixteen walkers' skips" name no pdfcpu path — every `third_party` skip is the digitorus forks', so none was removed). `build/pdfcpuctx.go` (a `//go:build ignore` tool, committed on the branch) threads `context.Background()` from the compiler's own `not enough arguments` diagnostics — 356 + 47 + 18 + … edits over ~500 files, a table of callees that also gained a trailing `nil` (`api.TextWatermark`/`ImageWatermark`/`PDFWatermark` conf; `ReadValidateAndOptimize`/`Optimize`/`Validate` progress options). Hand edits the new signatures force: `Node.Value` (3 returns) ×5, `font.CharWidth`/`Descent`/`BoundingBox`/`IsUserFont` (errors) ×6, `ctx.CurObj` gone (refgraph's save/restore dropped), `font.UserFontMetrics` registry gone (`registerMetrics` → `font.UserFont`/`ReloadUserFonts`, stampwrite reads `font.UserFont`), `mdpdf.mustWidth` (a panic as v0.13's loud failure; `LineHeight` keeps v0.13's 0). Policy: `Background()` in library code — no request context reaches `pdfread`/`pdfops` — so the C6 pin's "scan that fails on Background() in a handler file" is still owed. `go build`, `go vet`, `go test -run '^$'` clean for linux. First full run (suiterun, 41m50s): **52 tests + 2 panics red**, saved as `memory/instruments/pdfcpu-upgrade/branch-red-P02S01.txt`; the 2 panics hid the rest of two packages, so that list is incomplete — the true red list is the one S02's run records. The deepdive's saved 48 and this 52 differ by 5 new (the `/pending 899` fixture whose premise v0.16 removes; the slice-gate commit-message rule; two restatement guards; gofmt of a file I edited) and 1 green (`TestSplitRegionsReadsThePageOnce`).

#### P02.S02 — Port artifacts *(done 2026-10-27, merged in v2.3.0; branch sha 62914a33 predates the close rebase)*
Refs: D9.
The user-font reload (`registerMetrics` → `ReloadUserFonts`) breaking OCR and a second
stamp; `freerefs_test.go`'s `go:linkname` signature; the structure-tree panic after a refusal. **Exit:** no test
panics; OCR and stamping tests green.
**Built (branch, no VERSION bump).** (1) `freerefs_test.go`'s `go:linkname` now declares v0.16.2's `(c context.Context, ctx *model.Context)` — the one-parameter form had LINKED and dereferenced nil inside pdfcpu (no link error: the declaration is not checked against its target). (2) `restoreCIDSetsOf` (cidset.go): v0.16 refuses to rebuild a face whose descriptor has no `/CIDSet` ("corrupt fontDict: missing CIDSet reference", `font/fontDict.go:212`) and nib drops it after every stamp (`dropCIDSetsOf`, PDF/A-1 only wants one), so nib's own faces get an empty one back before the next stamp and lose it again after — called from `stampTextWatermarks` (faces named) and `addWatermarksWith` (none named: every face pdfcpu has registered as a user font, which is also the OCR door). Declared: with no faces named, a user document's own font that shares a name with an installed face gets the empty stream too (v0.13 rewrote such a font as well). (3) `TestACyclicTree…` guards a nil tree (the read now refuses; S04 owns the refusal); `stamptext_test.go` reads `Args[1]` (Args[0] is the context); `stamphold_test.go` compares the refusal's tail (v0.16 prefixes where it happened); `stampface_test.go` restores the fixture's CIDSet so pdfcpu alone proceeds to the rewrite the case exists to show. **The true red list (suiterun fp `4d2a4758`, 39m36s): 55 tests, 0 panics** — `branch-red-P02S02.txt`. S02's turns-green set: 11 (`TestTheFreeReferencePassLeavesWhatPdfcpusLeaves`, `TestASecondStampKeepsEmbedding…` ×2, `TestASecondOCR…`, `TestStampedTextEmbedsEveryFaceItDrawsWith`, `TestAStampLeavesAFontItCannotVouchForAlone`, `TestStampingWithTheContentHeld…`, `TestAPageNotAskedFor…`, `TestEveryStampSiteGoesThroughStampText`, `TestSourceIsFormatted`, `TestEveryCommitTouchingATriggerPath…`); 14 more became VISIBLE once the panics stopped hiding them (reference door, page tree, name-tree markers — S03a/S04). Not discharged here: the diff review of S02 (the slice was verified by its named tests and the full run; no reviewer read the branch diff yet — owed before the P02 close).
**(plan-review pin: Go, 2026-10-25)** `freerefs_test.go:21`'s `go:linkname` resolves in v0.16.2 as `fixReferencesToFreeObjects(c context.Context,
ctx *model.Context) error` (`optimize.go:1722`, read) and a mismatched declaration still LINKS (undefined behaviour, no link error): the declaration
and the comment ("fails this file's link") are updated as a D7 restatement.

#### P02.S03a — Restatements (D7): the `pdfread` cluster *(done 2026-10-27, merged in v2.3.0; branch sha 85ecdc4e predates the close rebase)*
Refs: D7.
Work P01.S02's table. Expected heaviest: `pdfread/optimize.go` (10 citations),
`contentcost.go`, `pdfread.go`, the set-asides, `pagesim.go`, the reference door. **Exit:** every row has a verdict;
the `...NamesThePdfcpuItRestates` guards green against v0.16's source.
**Built (branch, no VERSION bump).** All 77 `internal/pdfread/` rows of `restatements.tsv` have a verdict (58 re-cite, 15 re-derive, 4 delete across the file; the other packages' 90 stay `pending` for S03b/c) and a v0.16.2 cite, read by agents from source — **no behavioural probe** was run for any `delete`, so none was executed. Code changed: `simulatePages` answers `ErrPageNotFound` as v0.16.2's `PageDict` does (3 tests); the `/pending 899` fixture's migration-model case is no longer refused (v0.16.2's relaxed mode accepts it), the other four shapes are; the reference-door tests re-derived to pdfcpu's own 100-level bound, the over-refusal controls kept, `testpdf/refloops.go` repaired (a `/3D` annotation with a `/3DD`, a media rendition's `/P`); both `...NamesThePdfcpuItRestates` pins moved to v0.16.2 with the re-read in their comments; the census gained the bare `v0.13` form and five of the port's own comments were reworded. **Measured:** v0.16.2 does NOT survive three of the 21 reference loops (`GoTo3DView /TA` through Link `/A`, `Screen /A`, `Screen /AA` — every guard resets at each hop), tiling-pattern sharing is still exponential (89 ms at 14 deep, 5.2 s at 20), so the **reference door is kept whole**; its guarded-edge walks are redundant but only over-refuse. `pdfread` is red on ONE test, `TestATreeTheValidatorDropsIsMarkedAndNotWritten` — a looped/duplicate/over-deep name tree is now a FATAL refusal (`validate/tree_traversal.go:76`), so ADR-164's unread-tree mechanism has no fixture that triggers it; that is S04's (D4, "ADR-164 trees fatal"), and the server's `TestAnUnreadNameTreeRefusalIsA422NamingItsCause` with it. The owed follow-ups (narrow `ErrAssociatedFileLost` to page /AF — S06; delete the duplicate-walk and nil-subtype guards after probes; time a flat 20,000-page document before deleting `pagebalance.go`; stale cites) are on `memory/hardening/P02-pdfcpu-upgrade.md`, with the upstream report not yet filed. The slice's diff was reviewed by no one but its author and the verdict agents; a reviewer read is owed before the P02 close, with S02's.
**(plan-review pin: architect/performance, 2026-10-25)** Split by package into separate slices (now P02.S03a–c, 2026-10-25):
the pdfread cluster, pdfops, then uacheck / contentstream / root. The exit is the completed D7 table (verdict plus v0.16.2 `file:line` per row,
census green) — the two old guards are tripwires only and "green" on them discharges nothing. The pdfread slice precedes S07 (it re-derives
`contentcost.go` and `optimize.go`, which are the cost-floor logic).

#### P02.S03b — Restatements (D7): `pdfops` *(done 2026-10-27, merged in v2.3.0; branch sha 6b8e1a68 predates the close rebase)*
Refs: D7.
The `pdfops` rows of P01.S02's table. **Exit:** every row has a verdict, a v0.16.2 `file:line` and the census green; the turns-green set is named.
**Built (branch, no VERSION bump).** All 66 `pdfops` rows have a verdict (36 re-cite, 27 re-derive, 3 delete — the three PDF/A rows, pdfcpu v0.16.2 has no `pdfaid`/`injectPDFAMarkers`) and a v0.16.2 cite, read by three agents and then checked by a fourth that opened every re-cite/delete cite: 8 cites were corrected (reachability rows now `write.go:398-470` + `writePages.go`, a direct-dict dereference, a narrowed `/Metadata` claim); two agent objections were rejected on reading (`DereferenceStreamDict`, `Dict.Clone`). The writer still writes only what the catalog and page tree reach, but now also carries unlisted catalog keys (`write.go:393-395`). Turns-green set: `TestAPageThatPdfcpuAnswersAsNothingIsNamed`, `TestTheDigestFallbackWalksThePageTreeOnce` (ErrPageNotFound), `TestASlideshowIsSeenAndStripped` (the spec's `/Resources` dictionary is READ when each script names a `/Type`; typed variant added). **The full `pdfops` package run (26m50s, before these fixes) had 25 reds; 22 belong to S04/S06/S07/S08 and are triaged on `hardening/P02-pdfcpu-upgrade.md`, one (`TestNoOperationAddsAUA1ClauseItsInputDidNotFail`, 7.21.4.2 t2 on StampImages/StampTextLayer) is unattributed.** Not done, declared: the slice's full-suite close gate (S03c's is the next full run), `/redproof` over the three edited tests, the seam-inventory re-check, and six false-claim comments left worded for v0.13 (listed in the hardening file); the verdicts rest on agent reads of v0.16.2 source, no probes.

#### P02.S03c — Restatements (D7): `uacheck`, `contentstream` and the root *(done 2026-10-27, merged in v2.3.0; branch sha d6e0fba5 predates the close rebase)*
Refs: D7.
The remaining rows. **Exit:** as S03b; the table has no row without a verdict.
**Built (branch, no VERSION bump).** The last 24 rows have verdicts: the table is 167 rows, 117 re-cite, 43 re-derive, 7 delete, **0 pending**; both census guards green. Verdicts rest on agent reads of v0.16.2 source (no probes); several carry a half-claim the reader marked UNREAD (listed per row in the cite). Substantive changes found: inline-image `/L` is trusted (`parseContent.go:200-218`), a bad `/AP` entry is deleted in relaxed mode, no clause fails a CIDFontType2 for a missing `/CIDToGIDMap`, an indirect `/P` dereferences, the structure-tree root `/K` refusal is strict-mode only. Package runs: contentstream and p2p green; `uacheck` 9 and `mdpdf` 1 red, **all ten new validator refusals — S04's**, triaged on `hardening/P02-pdfcpu-upgrade.md`; S03c turns nothing green. **Not done, declared:** the full-suite close gate (the branch is red by design until S04-S08; it runs at S04's close), `/redproof` (no test edited in this slice), the inventory re-check.

#### P02.S04 — New refusals (D4) *(done 2026-10-28, merged in v2.3.0; branch sha 7648e204 predates the close rebase)*
Refs: D4.
The ~25 kinds: the 100-level `traverse` cap (deep form chains, field DAGs), an
embedded-file entry with no `/EF` (external file specifications), ADR-164's deep/unread name trees (now fatal even
relaxed), structure-tree defects nib reports as cannot-check (cycle, self-listing, unreadable kid), duplicate
number-tree nodes, Rendition/media actions, an output of nib's own page-unlink.
**(plan-review pin: QA, 2026-10-25)** uacheck's veraPDF-agreement: record the v0.13 pair (documents with a verdict, agreements) per harness; the
v0.16 verdict count is at least as large or each lost document is named; the close fails on a skip (shown by the `suiterun` log).
 **Exit:** each kind dispositioned in
the seam inventory; uacheck's veraPDF-agreement tests at their v0.13 counts; population scan 0 newly refused.
**Built (branch, no VERSION bump).** Eleven refusal kinds dispositioned (`instruments/pdfcpu-upgrade/S04-refusals.md`): carry for name trees past the walk budget, external-file filespecs (empty `/EF` for the validator only), the checker's mistyped `/Widths`, ExtGState `/Font` and MediaClip `/Alt`, and the structure tree (checker only); refuse-named for nesting past 100 (`ErrNestedTooDeep`), reference cycles (`ErrReferenceCycle`) and duplicate nodes (`ErrNodeNamedTwice`), each over pdfcpu's own words (`pdfread/refusal.go`); adopt for the Bookmark kid (strict-mode only upstream) and the media rendition `/P`, with the fixtures fixed. Measured: uacheck veraPDF agreement 73,605 of 73,605 pairs over 701 documents on both trees; population scan **0 newly refused** over 2,021 documents (documents 704 and 1837 dropped from a scratchpad copy of the list, 704 absent, 1837 hangs v0.13, `/pending 897`); 35 rows differ, 24 of them `digest6` moves for S08. Also fixed: an empty CIDSet left behind by S02's restore (`dropRestoredCIDSets`). The first close gate was red on five unexpected tests (notices regenerated; 3 restatement rows added, 4 moved; two typed reads moved into `uacheck/document.go`'s door; one red proof retired because v0.16.2's `Node.Remove` preserves the surviving subtree, measured green under the mutation, `recorded` 604 → 603); the re-run (`83523415fd3a`, 33m15s) is red only on the five tests that belong to later slices: S06 `TestWhatARewriteDoesToAnAssociatedFile`, S07 `TestAPageOfManyWordsIsNotDeflatedOncePerWord`, S08 the two digest golden tests and `TestEveryExternalCorpusDocumentAgreesWithTheReferenceWalk`. **Declared gaps:** the gate-fix commit has had no independent review, nor has the S02 to S04 diff (owed with the P02 close `/code-review`); no `/redproof` run over S03b/S04 tests; verdicts rest on source reads; the `facts` change at population index 1886 is not investigated; eleven comments still word a false claim for v0.13.
#### P02.S05 — Forms (D5) *(done 2026-10-28, merged in v2.3.0; branch sha 432716fa predates the close rebase)*
Refs: D5.
Radio by `Ff`; re-key ADR-172, `heldOptionValues`, formrt; the document (#164) v0.16 can
no longer export. **Exit:** `build/formrt.sh` on the 203-document list: every value read and written back as on
v0.13, with the reclassification as the only declared move.
**(plan-review pin: PDF-format/editor, 2026-10-25)** The declared moves are the 133 check boxes (11 documents), #164 (no longer exports) and
#13 (list and combo fills change) — see D5's pin; "the only declared move" is replaced by that list.
**Built (branch, no VERSION bump).** `build/formrt.sh` on a rebuilt 206-document list (`~/nib/formrt-corpus.txt`; the 203-list was not on disk), v0.13 (main) against the branch: 83 forms read both ways and 9,633 fields on no page, equal; 10 of 206 documents differ, **all declared**: 133 multi-widget check boxes in 9 documents (radio 180 → 47 on the window side, now "checkbox: several widgets", and pdfcpu → window they read and write back as check boxes, 0 wrong) and #13's list boxes (the v0.13 fill refusal is gone; 4 list values read back as another value). **One undeclared move found and fixed:** 2 forms of ~9 MB lost their export, because v0.16.2 answers "corrupt field" at a widget whose `/Parent` reaches nothing (`form/form.go:154-156`) where v0.13 skipped it; carried by `unwalkableWidgets` (the widget leaves `/Annots` and pdfcpu's `PageAnnots` cache for the walk and is put back; `TestAWidgetWhoseParentIsEmptyDoesNotEndTheExport`, three mutations red). The S04 population scan could not see it (no form export). `buttonKind(ff)` is the one answer for push button / radio / check box (`widgetKind` and `formField.pushButton` route through it). With the push-button half of `pushButtonsAside` off the formrt rows are identical on all 206 documents (v0.16.2 skips push buttons itself): kept as belt and braces, retiring it needs a superseding ADR (S09). ADR-172's patch is pdf.js-side and kept (argued from source; `test/ui/checkpair.test.mjs` not run). Gate (fp `07223b99246e`, 39m30s): red on the five later-slice tests plus `TestSplitRegionsReadsThePageOnce`, a latent flake (unsigned allocation difference wrapped when the 16-region run allocated less than the 1-region run; fixed, 8/8 green, a read-per-region mutation red at 1.16 GB). **The gate was not re-run after that test-only fix** (recorded assumption; S06's gate re-certifies it). One read of the 4 MB page now allocates ~77 MB against the comment's ~5 MB: S07's evidence. Tiers 4 and 6 did not fire (no `internal/server` session/ceremony/delivery/discovery, `p2p` or `rendezvous` change). Declared: no independent review of the S05 diff (owed with the whole-branch `/code-review`).

#### P02.S06 — `/AF` (D6) *(done 2026-10-28, merged in v2.3.0; branch sha 55a6c100 predates the close rebase)*
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
**Built (branch, no VERSION bump).** Measured red first: pdfcpu v0.16.2 writes the catalog's `/AF` and the files it names (`write.go:382`), so remove-files and sanitize left the catalog's and a structure element's file reachable from the trailer whenever a PDF/A-3 file was listed in both `/EmbeddedFiles` and `/AF`. `removeAllAttachments` (the one door both reach) now strips `/AF` from every dictionary (`dropAssociatedFileKeys`) and clears the read mark (`pdfread.ForgetAssociatedFiles`, one named caller, held by `TestForgetAssociatedFilesHasOneNamedCaller`); a document whose files only `/AF` reaches goes through the removal instead of being refused. The write refusal narrows to a **page's** `/AF` (`writePages.go` still has no `AF`); the catalog's alone is written as it stands. The scan reports `/AF` under the existing `attachment` kind (already removed by `StripActive`, so `TestEveryScanKindIsStrippedOrNamedAsLeft` needed no new row). One graph-walk test per door (RemoveFilesAndMedia, StripActive, Collect, RedactPages, SplitPage, SplitRegions): extract and redaction were already clean (allowlist and structure strip), only the removal doors leaked. Mutation-probed: strip, forget, allowlist, element strip and a second caller each red. `TestWhatARewriteDoesToAnAssociatedFile` re-derived and green. Gate (fp `3c148f4a6b0f`, 41m36s): red on the four later-slice tests (S07 `TestAPageOfManyWordsIsNotDeflatedOncePerWord`; S08 the two digest goldens and `TestEveryExternalCorpusDocumentAgreesWithTheReferenceWalk`) and two `internal/server` ceremony timing tests (`TestADeclinedSpokenCheckSpendsTheArm`, `TestAnAbandonedConnectionIsFollowedByAWorkingSession`: a 10 s and a 15 s wait for the spoken check, both at 07:30 while `pdfops` ran; green twice alone, in a package this slice did not touch: recorded as load flakes, not credited as proof). This run also re-certifies S05's test-only fix. Tiers 4 and 6 did not fire. Declared: `SplitPage`/`SplitRegions` may still answer the refusal for a listed page file (the mark does not follow into the new document); the S06 diff has had no independent review (owed with the whole-branch `/code-review`); rule 7's `/AF` hashing is S08's.

#### P02.S07 — Cost (D8) *(done 2026-10-29, merged in v2.3.0; branch sha 32b770ef predates the close rebase)*
Refs: D8.
**Exit:** every cost floor at its v0.13 value or better, measured both builds.
**(plan-review pin: performance/QA, 2026-10-25)** Per D8's pin: absolute v0.13 numbers recorded first (the stamp / `SplitRegions` floors are
ratios today, `stamphold_test.go:277-297`, `splitregionscost_test.go:57-79`), `TextWidth` per page now bounded by a test, quiet-machine medians, and
the mitigation shown to Dan before it is built. "Or better" is replaced by the absolute figure.
**Built so far (branch, no VERSION bump).** Absolute figures, medians of 7 on both builds under a load average of 6-8 (not quiet; the ordering held in all four configurations, `nogreenteagc` moves neither): stamp 600 words content held 409 ms / 575 MB on v0.13.0 against **767 ms** / 208 MB on v0.16.2; `SplitRegions` 1 region 22 ms / 81 MB against **46 ms** / 73 MB. Memory is better everywhere, so `TestAPageOfManyWordsIsNotDeflatedOncePerWord` (red, its 0.62 ratio had no premise once upstream stopped deflating once per word) is re-derived to the old 600 MB absolute held figure plus "the hold costs no more than pdfcpu's own stamp" (three conjuncts, each mutated red), and `SplitRegions` gains a 90 MB floor. The wall-clock cause is read from a CPU profile: `wordInk` calls `font.CharWidth` per rune and v0.16.2 clones the whole metrics table per call (`font/repository.go:230-243`), 36% of the stamp. `TestStampingSixHundredWordsIsNotSlowerThanItWas` (600 ms) is written and skipped. **Parked for Dan:** the per-fit width cache (option A, ~25 lines in `ocrfit.go`; B and C in the note) is logic on the OCR request path, so D8's pin and the hot-path rule require his go before it is built; the exit ("every cost floor at its v0.13 value or better") is not met until then, and 22 → 46 ms on `SplitRegions` is not yet attributed. Marked blocked so the arc takes S08; clearing the block is Dan's.

**Width cache (Dan's go, 2026-10-29, option A via /discuss).** `runeWidths` (`ocrfit.go`) holds `font.CharWidth` per rune and `font.Descent` per size for one `StampTextLayer` call and is dropped with it; nil asks pdfcpu as before (the page-map reader's `fittedInk` keeps that). A CPU profile of the 600-word stamp puts `fitWord` at 21% before and 4% after. `TestStampingSixHundredWordsIsNotSlowerThanItWas` is un-skipped: green with the cache and red (best of 5 over 600 ms) with `var widths *runeWidths` in its place, both at load ~16; the prototype measured held 242 ms at load 5-6. **Declared:** no quiet-machine median was taken after the build (load never fell under 5 in 20 minutes); the `SplitRegions` 22 to 46 ms move did not reproduce in the prototype and is read as load noise, unresolved; `fittedInk` per run still clones per rune. Per-commit tier: `pdfops` and `pdfread` whole packages green (`-timeout 90m`); tiers 4/6 did not fire (`internal/pdfops` only).

#### P02.S08 — Digest rule 7 (D1) *(done 2026-10-29, merged in v2.3.0; branch sha bbde337f predates the close rebase)*
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

**Built (branch, no VERSION bump).** `ContentDigestVersion` 7 = rule 6's axes read by pdfcpu v0.16.x. `ContentDigestRuleReadable` is the current rule only;
`ContentDigestRuleRetired` (5, 6) is accepted by every record-level question (`CheckRecord`, delivery, `ReadMirrorToEnd` for stop / close-out / an ended
folder's read-back) and refused with the older-nib skew (wire 18) where a digest is computed (`DocumentHashFor`) or a ceremony is continued (`ReadMirror`, a
live folder, `checkArrival` via `RequireComputableRule`, which also covers hop 2+ where no hash is asked for). Goldens: `-v5`/`-v6` deleted, `-v7` written;
the retired contract is held by `TestARecordUnderARetiredDigestRuleIsARecordButNotAHash` (four mutations red). Gate: tier 4 PASS over both transports, tier 6
PASS=31 FAIL=0; `-n 4` not run (relay and round untouched). Full suite fp 6b887a763f4b (S07+S08 tree, 45m44s, load 6-9): red only on
`TestTheFreeReferencePassLetsPdfcpuSkipItsOwnRecursion` and three `internal/server` ceremony timing tests (`TestADeclinedSpokenCheckSpendsTheArm`, the two
abandoned-connection tests), all four green alone on the same tree; the two recovered nil-pointer panics are the pre-existing ones. Recorded assumption
(reversible): those four are load flakes, one more than S06's two.
**Declared gaps:** pin 1 per-file causes; pin 2 fixtures and the N-process determinism run; `endCeremony` visible failure; the comparison instrument not re-run on
the rule-7 tree; the dead rule-5 branch in `hashEffectivePage`; the ADR is written at the merge (S09 / P02 close).

#### P02.S09 — Pins and docs *(done 2026-10-29, merged in v2.3.0; branch sha 45e095da predates the close rebase)*
Refs: D1, D7, D9.
`contentcostpin`, notices, the red-proof table rows that staled, every ADR citing v0.13
behaviour (superseded, not edited), `CLAUDE.md`, `CONTRIBUTING.md`.
**Exit:** no `v0.13.0` citation remains outside a superseded ADR or the retired-rule record; the notices are regenerated (`./build/gen-notices.sh`); `CLAUDE.md` and `CONTRIBUTING.md` (main checkout) name v0.16.2, Go 1.26 and rule 7.

**Built (branch, no VERSION bump).** ADR-174 (supersedes ADR-173, ADR-143's readable set, ADR-136's scope; indexed; `CLAUDE.md` on main names it; `CONTRIBUTING.md` names v0.16.2, rule 7 and Go 1.26). Two guards the plan pinned and no earlier slice had built, each shown red when planted:
`TestTheDigestRuleNamesThePdfcpuItWasMeasuredWith` (`digestRulePdfcpu`; go.mod alone, the table alone, `ContentDigestVersion` alone) and `TestNibNeverTurnsOnPdfcpuNetworkCode` (`ValidateLinks`; the `sign` import arm planted-red not yet shown, owed to `/redproof`).
74 Go-source `v0.13` mentions swept by two agents against the v0.16.2 module source (comments only, line counts unchanged), spot-checked by me on three cites: 26 `restatements.tsv` rows retired because their comment now cites v0.16.2, 2 re-anchored; the contentcost pins' failure messages name v0.16.2. Three `red-proofs.md` rows annotated. `gen-notices.sh` run: no diff.
Per-commit tier: build, `go vet` linux/windows/darwin, whole packages root, `pdfread`, `contentstream`, `uacheck` green; `pdfops` targeted only (comment edits; its full run is the P02 close gate). **Exit clause not met literally, by decision (rung 2, reversible):** 44 `v0.13.0` mentions remain outside ADRs, each a past-tense or "measured then, not re-measured on v0.16.2" record paired with a v0.16.2 cite or an explicit unverified note; erasing them would erase what was measured. **Not done, owed to the P02 close** (the hardening list): the class-C code the sweep found resting on a weakened v0.13 premise (`duplicateLoop` and `unnamed` refusals, `pagebalance.go`, the text-annotation `/State` set-aside, `pages.go`/`pagesim.go` mirror) — deletion needs probes; unverified claims listed by the agents (`apiread.go` per-function cites, `refgraph.go` recursions, `decode.go` per-stage limit, `tagfate.go` table, uacheck dangling-reference and non-stream `/AP` shapes). No independent review of the S09 diff (owed with the whole-branch `/code-review`).

**P02 close, before the merge:** `/code-review` of the branch; the full gate on the MERGED tree — Go suite through
`suiterun`, tiers 2–3, and **tiers 4 and 6** (the digest is what a ceremony checks across machines); `accuracy.sh`
and `formrt.sh` against their v0.13 baselines; `pdfcpudiff.sh` showing only declared moves. Then merge.
**(plan-review pin: forensic/crypto/PDF-format/QA, 2026-10-25 — own criterion, own evidence)** The close adds: over the corpus's signed documents
the `sign.Verify` verdict is identical on v0.13 and v0.16 except declared rows, including one large signed document (v0.16's always-on read budget and
~25 new refusals can turn a signed file `Invalid`/`Unchecked` with no digest move; `verify.go:925-943`); the live-unreachable-object count is
unchanged across sanitize, extract and split on the corpus; any instrument that skips fails the close. The branch is rebased before this gate, never after.


**P02 close — built 2026-10-29 (merged `b73eb024`, v2.3.0; revert target `f7c553fa`, `git revert -m1`).** Phase review: six scoped agents, 0 critical, 15 warnings; F01/F02/F04/F08 fixed, F06 in part, F03 corrected as a doc change, the rest filed (/pending 919, 920); redproof of 54 conditions, 40 red, 12 distinct survivors, 3 closed by tests, 9 filed (920). Rebased once (24 commits, no overlap) BEFORE the gate. Gate on the final tree (fp `42b5830a3044`): Go suite OK 22m04s; tier 2 858 tests, tier 3 264 (2 skipped by design); tier 4 over both transports, 4d `-n 4` and tier 6 31/31 green; `formrt.sh` (206 documents), `accuracy.sh` (19, identical in every leaf but the timestamp) and `pdfcpudiff.sh` (2,022 of 2,022 read on both trees; `sign.Verify` verdict, fields and page maps identical; no document newly refused) show only declared moves — detail `memory/instruments/pdfcpu-upgrade/P02-close-gate.md`. **Found at the close by instruments the per-commit tier does not run, and fixed on the branch:** (1) the Markdown converter was 4x slower (v0.16.2's `font.CharWidth` clones the whole face per call; a per-rune advance cache in `mdpdf` restores 2m35s on a 22 MB fixture); (2) `checkpair.test.mjs` read a v0.13 radio group — v0.16.2 exports the Yes/No pair as one check box (`Yes` for either box), now declared in ADR-174 §4; (3) the `pdfcpudiff.sh` selftest was vacuous once rule 6 stopped being computed (a `digestcur` column) and gained an `opsunreach` column for the unreachable-object criterion (one benign extra Font dictionary after sanitize in 1 document of 2,022). Declared gaps carried: the digest column cannot compare rules 6 and 7; the largest document needs `NIB_PDFCPUDIFF_TIMEOUT=30m`; tiers 2-3 need a `node_modules` in the worktree.

### P03 — After the merge, on main *(done 2026-10-31, v2.7.2)*
Goal: the port ships, or waits, on Dan's word.
Exit criteria: the population scan re-run on the released binary; `/pending 898` closed; release notes carry the flag-day sentence.
Release is Dan's call (`./build.sh --publish`). Re-run the population scan on the released binary; close `/pending
898`; record the comparison instrument's last output beside the deepdive.
**(plan-review pin: SRE/forensic, 2026-10-25)** Before `--publish`: the release notes state the digest flag day and the exact older build to finish an
un-ended ceremony on (nothing offers the older binary or `.deb`), and the update popup warns when `~/nib/ceremonies` holds an un-ended ceremony. A bad rule-7
release cannot be pulled (`versionLess` offers nothing older): the answer is a fix forward under a higher version. A pre-port party and a post-port
party cannot finish one ceremony together; the pre-port side sees a dial refusal naming no cause (the post-port side sees `older-nib`). "or convene it
again" is dropped from the sentence for a document that already carries a signature.

**P03 pre-publish — built 2026-10-29, v2.4.0 (Dan: "build the popup warning then publish"; ADR-175, /pending 921).** The update check counts the ceremonies that have not ended (`openCeremonies`, one count with `hasLiveCeremony`) and sends it only across the flag day (`crossesCeremonyFlagDay`, below 2.3.0 to at or above); the download popup and the pill say it and ask nothing (ADR-102); `docs/release-notes/2.5.1.md` carries the flag-day sentence, the build to finish on and the dropped "convene again" for a signed document, and `build/publish-release.sh` publishes a version's notes file. Tests red under six single-condition mutants (the gate, its upper bound, the count, the hidden toggle, the singular, the pill text) and under the notes-file branch dropped. **Found while building it, and the plan's pin was wrong about it:** the warning lives in the build that is RUNNING, so it never reaches a machine crossing 2.3.0 from 2.2.x — the only machines the flag day strands (the last release published is v2.2.0). For this crossing the release page is the only channel; the popup is for the next boundary (ADR-175 "Declared").


**P03 close — 2026-10-31, v2.7.2.** Population scan re-run on the released tree (v2.7.0, `98552869`) against the pre-port v2.2.3 (`68d24764`) over `~/nib/population-corpus-2022.txt` (the 2,023-line list less one `.cargo/registry` file that no longer exists; manifest regenerated): 2,022 of 2,022 read on both trees. Digest columns differ on every document by design (rule 6 not computed; rule 8 current). Otherwise: 581 `maphash` moves (the only `MapPage` change since the gate is v2.5.3's ADR-176 /Differences reading — attribution inferred, not diffed per document), 9 refused-to-refused wording changes (no document newly refused), 36 `opsunreach` (21 a refusal's wording in `meta`; one document that was refused in every operation now reads), `unreachable` 518 to 519 on one document, `facts` last field 1 to 0 on one. Output kept at `memory/instruments/pdfcpu-upgrade/P03-population-v2.2.3-vs-v2.7.0.out` (indices and classes only). Release notes carry the flag-day sentence (v2.5.1).

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

