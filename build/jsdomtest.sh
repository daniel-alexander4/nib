#!/usr/bin/env bash
# Tier 2 of Nib's three-tier test harness: run the front-end tests that load the
# real web/app.js into jsdom.
#
# Usage: ./build/jsdomtest.sh
#
# Nib's client is 7k lines of JavaScript that the Go suite cannot see at all — the
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
# that discovers 1 of 53: delete seven of the eight *.test.mjs files and the count
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
# Sixteen since P01.S02 added peername.test.mjs (v1.109.41). Bumping this literal is the
# deliberate act the guard exists to force; it went unbumped for nine commits, during which
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
# 40 since P02.S01 (consentrecital.test.mjs): the signer's agreement statement defaults to the
# ceremony's recital, and outside a ceremony to the original sentence — two cases, because a
# change that read an absent field would leave the box empty.
#
# **FIFTH INSTANCE, found 2026-09-08 at P03.S04, and it had run for two slices.** 45 → 47:
# closeprompt.test.mjs landed at P01.S05 (v1.128.41) and draftconsumed.test.mjs at P03.S03
# (v1.128.44), neither bumping this literal — so tier 2 exited 1 across both, with all 236 tests
# passing, and both slices reported it green. **P03.S04 adds no jsdom file at all**; this bump is
# the earlier drift being repaired by the next slice that ran the harness and read its last line,
# which is the fourth instance's shape again (a person needing the number, not a check).
# 72 since /pending 502 (vaultunreadable.test.mjs): a present-but-unreadable vault gets its own
# screen, whose Retry re-reads status and never enrols.
# 73 since /pending 498 (racepins.test.mjs): the document-switch races behind the pinning guard,
# driven rather than scanned, because a scan cannot see an await between a capture and a write.
# 74 since /pending 506 (clientdoors.test.mjs): the wrong-passphrase report, keyboard reach, tab-close
# focus, and the disarm-all and keyboard doors.
# 75 since ADR-036 (modevisibility.test.mjs): the main-menu switch hides both mode lists, never
# lands on a hidden mode, and leaves Settings alone.
# 76 since ADR-039 (downloaddialog.test.mjs): the release download's dialog — progress from the
# window stream, the destination named, a terminal state, and Cancel telling the server.
# 77 since /pending 540 (windowdrop.test.mjs): the window drop listener, which had no test at any
# tier — which is how a filter that silently discarded every convertible document survived two
# rounds of work on that exact handler.
#
# 78 since /pending 513 (recvpoll.test.mjs): the receive poll survives a blip and gives up out loud.
# Its own file because it drives real `setTimeout` against a live arm, and armprogress.test.mjs's
# measured lesson is that a file which leaves one armed hangs the whole suite behind its timer.
#
# **Both arrived on branches off one base, and each bumped this to 75.** Taking either side at
# the merge leaves the pin one below the file count, which this script exits 1 on — the count is
# the thing it guards, so a merge that resolves it by picking a side breaks the guard rather
# than the code. Set by hand to the number of files actually present.
#
# 79 since /pending 566 (pickerunread.test.mjs): a failed `/api/peers` and the `change` after it.
# Its own file because the whole finding needs the peer route to FAIL, and setupsheet.test.mjs's
# every other test needs it to answer — one boot per file means those cannot share a process
# without the failure mode becoming an ordering hazard in someone else's assertions.
#
# 85 since P01.S02 (refusedsig.test.mjs): which `addedAfterCause` produces which words on the badge
# and in the details panel, and that a refused signature is named there — its own file because the
# panel needs `/api/attestations` answered and the badge cases need one document id throughout.
#
# 87 since /pending 750 (recvapplying.test.mjs): the applying stage ends on the accepted request
# settling, not on a disarm — its own file because its status route must stay armed throughout,
# which consentredraw.test.mjs's cancel-and-rearm sequence cannot share.
#
# 88 since /pending 788 (print.test.mjs): Print in a browser whose PDF frame this page may not script —
# Firefox, the stock Ubuntu path. Its own file because its frames' windows are stubbed per test.
#
# 90 since PLAN-returned-document P03.S01 (returnedsheet.test.mjs): the sheet for a document that came back — its own
# file because it opens documents one after another and closes them all, which no other file's boot can share.
#
# 91 since PLAN-returned-document P03.S02 (returnedverdict.test.mjs): the verdict over stubbed answers — its own file
# because each test re-opens a document with a different signature and a different route answer.
#
# 92 since PLAN-returned-document P03.S03 (returnedcompare.test.mjs): the sheet's "see what changed" region — its own
# file because it drives Compare's module-level state, which a file sharing its boot would inherit.
#
# 93 since PLAN-returned-document P04.S01 (finalizekeep.test.mjs): the Finalize modal's "Keep a copy" — its own file
# because it drives the modal against a stubbed /api/finalize that no other file's boot answers.
#
# 94 since ADR-085 (pan.test.mjs): whose drag a press on the page is, and Resume last session — its own file
# because it boots at a launch state with a recorded session, which no other file's boot has.
#
# 95 since ADR-086 (handoffpush.test.mjs): a launch's document arriving in the open window — its own file because it
# boots with `?open=` and `?notice=` on the address, which no other file's boot carries.
#
# 96 since ADR-087 (toolbaricons.test.mjs): the icon bar — every button an icon with its word, the page buttons and
# Undo/Redo back in the bar, the armed-tool group — its own file because it reopens one path with changing history flags.
#
# 97 since ADR-088 (refinefields.test.mjs): proposed fields corrected against the page map — a pure function, so every
# case is stated and checked as numbers; its own file because it imports detect.js alone and boots no page.
#
# 98 since ADR-089 (proposefields.test.mjs): fields read from the page map itself — a table's cells, a line to write
# on, underscores, squares — a pure function over rectangles; its own file because it states a whole form as numbers.
#
# 99 since ADR-090 (placematches.test.mjs): a search match boxed from the map's glyph boundaries, and when that box
# may replace the estimate — its own file because the property it holds is redaction's, not detection's.
#
# 100 since ADR-095 (wrappedmatch.test.mjs): a search match that wraps a line, found by both readings — its own file
# because it drives scanTextMatches end to end, which no other file does, over a pdf.js page it makes.
# 102 since ADR-104 (apppages.test.mjs): the app-page registry — the Settings entries (seven since ADR-109) and no setting in the menu, one tab a
# page, and the source guards that the registry and `#viewerWrap.hidden` have the writers they say they have.
# 103 since ADR-106 (autoprereq.test.mjs): a command that runs its own prerequisite — the one door every text command
# reads a scan through, with the recogniser stubbed, and each thing the door must never do.
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
