#!/usr/bin/env bash
# Tier 2 of Nib's three-tier test harness: run the front-end tests that load the
# real web/app.js into jsdom.
#
# Usage: ./build/jsdomtest.sh
#
# Nib's client is JavaScript, and the Go suite cannot see any of it — the
# server never observes the browser's state, so every client-side failure mode is
# silent to `go test ./...`. This tier closes the DOM-observable part of that gap
# by running the shipped file, not a copy.
#
# Requires Node and a `npm install` (jsdom is the only dependency, dev-only, and
# node_modules/ is git-ignored). Skips cleanly without either, the same way the
# poppler/Ghostscript/veraPDF tests and build/winrepro.sh do — a fresh clone runs
# everything else without setting this up. NIB_REQUIRE_TIERS=1 turns that skip into
# exit 77 (build/tiergate.sh).
#
# Ceiling: jsdom models the DOM, not a rendering engine — no layout, no canvas, no
# media queries, and pdf.js itself is stubbed. build/uirepro.sh (tier 3) covers
# those against a real browser. See test/jsdom/boot.mjs for the full statement.
set -uo pipefail
cd "$(dirname "$0")/.."

. "$(dirname "$0")/tiergate.sh" # nib_skip, nib_population — what this tier's verdict means
command -v node >/dev/null 2>&1 || nib_skip "node not installed; skipping the front-end tests"
[ -d node_modules/jsdom ] || nib_skip "jsdom not installed (run: npm install); skipping the front-end tests"

Nib_out="$(node --test "${NIB_FILECOUNT[@]}" test/jsdom/ 2>&1)"
Nib_code=$?
echo "$Nib_out"

# A runner that discovers NO tests also exits 0, and would look exactly like a
# passing suite forever — the harness reporting health about a population it never
# had. So the count is checked, not just the exit code — PER FILE since /pending 822,
# in `nib_population` (build/tiergate.sh), because a total could not see one file
# contributing nothing while another contributed two. (This is the harness applying
# to itself the rule it exists to enforce; the failure it prevents is the one nobody
# would ever see.)

# A floor of one is not an inventory. The argument above — "a runner that discovers
# NO tests looks exactly like a passing suite" — applies just as well to a runner
# that discovers one file of many: delete every *.test.mjs file but one and the count
# check above still passes.
#
# So the population is pinned against the file system, which is the external source
# a self-referential count cannot be: every test file must contribute at least one
# test, and the FILE count itself is pinned as a literal. A literal per test would
# be worse than nothing — it goes red on every legitimate new test and trains the
# next person to bump the number, which is how V11's equality assertion rotted. A
# file count changes only when someone adds or deletes a file, which is deliberate.
# One boot per file is this tier's standing rule — restore.test.mjs needs its own because
# its restore runs at module-evaluation time, and the same holds for every file since.
# Bumping this literal is the deliberate act the guard exists to force. It went unbumped for
# nine commits after P01.S02 added peername.test.mjs (v1.109.41), during which
# this harness EXITED 1 while still printing "# pass 96 / # fail 0" above it. Read the exit
# status, not the totals — the totals were true and the tier was red.
#
# **And it happened AGAIN, exactly as described.** v1.117.178 (P07.S05a) added
# ceremonycomplete.test.mjs and left this literal at 18, so tier 2 was red from that commit
# until P07.S05d ran it — nine slices later. The guard worked; nobody asked it. The lesson is
# not about the number, it is that a tier which "skips cleanly when its dependencies are absent"
# also skips cleanly when nobody runs it.
#
# **THIRD INSTANCE, 2026-08-28, and this time the tier WAS run every slice.** P07.S07c added
# nineparty.test.mjs (v1.117.220) and P07.S09c added tagskew.test.mjs, leaving this literal at 19
# while the directory held 21 — so tier 2 exited 1 through four commits, .220 to .226, and every
# one of those runs was watched. What was watched was `grep -E '^# (tests|pass|fail)'`: the TAP
# summary, which said `# fail 0` and was TRUE. This guard's failure goes to stderr AFTER that
# summary, so a reader looking at the totals sees a green suite and a red tier, which is the
# paragraph above stated in the first person.
#
# **FOURTH INSTANCE, 2026-09-03, and this one ran for SIX slices.** P06.S02 added
# ceremonypanel.test.mjs and P06.S05 added armprogress.test.mjs, both in one day, and this literal
# stayed at 26 while the directory held 28 — so tier 2 exited 1 from v1.117.335 through .342 and
# every one of those slices reported it green. It was found at `/pending 366`, by a sweep that
# needed a 27th file and was told the directory held 29.
#
# The four instances share one shape and it is not the literal: **a check whose verdict lives in
# the exit status, read by someone looking at the output.** `build/redproof.sh` is what caught the
# third, because it asserts on the exit status of the whole harness rather than on the lines it
# printed — which is the only reading that could have caught any of them. The fourth was caught by
# a person needing the number, which is luck, not a check.
#
# **FIFTH INSTANCE, found 2026-09-08 at P03.S04, and it had run for two slices.** 45 → 47:
# closeprompt.test.mjs landed at P01.S05 (v1.128.41) and draftconsumed.test.mjs at P03.S03
# (v1.128.44), neither bumping this literal — so tier 2 exited 1 across both, with all 236 tests
# passing, and both slices reported it green. **P03.S04 adds no jsdom file at all**; this bump is
# the earlier drift being repaired by the next slice that ran the harness and read its last line,
# which is the fourth instance's shape again (a person needing the number, not a check).
#
# **Two files once arrived on branches off one base, and each branch bumped this to 75** (ADR-036's
# modevisibility.test.mjs and ADR-039's downloaddialog.test.mjs). Taking either side at the merge
# leaves the pin one below the file count, which this script exits 1 on — the count is the thing
# it guards, so a merge that resolves it by picking a side breaks the guard rather than the code.
# Set by hand to the number of files actually present.
#
# Why each bump was made is in the commit that made it (`git log -L` on the literal below), not
# here: this comment kept a list of them, with gaps, most of it about numbers the literal no
# longer holds. The newest entries stay as the pattern for the next one — the file, what it
# holds, and why it needs a boot of its own.
#
# 104 since ADR-110 (alwaysshown.test.mjs): a vault that already hides File or Mark Up — its own boot, because only
# the status a window boots on can carry that list.
# 105 since ADR-125 (taguntagged.test.mjs): tagging what no tag owns from the tree panel — its own boot, because its
# document has nine pages and its routes answer the untagged reader, which no other file stubs.
# 107 since ADR-126 and ADR-127 (tagpromote.test.mjs, tagrolemap.test.mjs): an inline tag's one button and where
# the selection lands after it, and the role map's rows — each its own boot, because each needs a tree of its own
# shape and the other tag files' trees are asserted on by position.
Nib_expect_files=107
nib_population jsdom test/jsdom "$Nib_expect_files" "$Nib_out" || exit $?

exit "$Nib_code"
