# docs/red-proofs.md, tier 4 --lan: "the namespace re-exec drops -n" (P07.S05c, v1.117.183)
#
# The defect: `FLAGS` carries `--lan --keep --v6` and not `-n`, so `--lan -n 4` re-executes INSIDE
# the namespace as `--lan` alone. The run then does a TWO-PARTY ceremony and prints its pass, while
# the operator believes they drove an N-party one on the link.
#
# **It is the exact defect `FLAGS` was created for, one flag later.** The comment on the re-exec
# already records that it used to hard-code `--lan`, so `--lan --keep` silently dropped `--keep`.
# `-n` arrived afterwards and never joined the list. That is what a hand-maintained list looks like
# when a new member shows up, and it is why this row exists rather than a comment.
#
# It was one of THREE independent barriers between the LAN clause and its only driver — the others
# being an explicit `--lan is N=2-only` refusal and the `N != 2` block's `exit 0` three lines before
# the LAN block. Any one alone made `--lan -n 9` impossible, which is why nobody had ever run it and
# why a ceremony had never been measured on a link (see `/pending 299`).
#
# **The first attempt at this row came back "the check still PASSED", and that is how the guard came
# to exist.** There WAS no check: a two-party run passes its own assertions perfectly well, so the
# substituted run printed a clean pass and exited 0. The defect is invisible by construction —
# nothing downstream can tell a two-party ceremony it was asked to be a four-party one.
#
# So the requested N now travels OUT OF BAND, in the environment, and the child compares what it
# parsed against what was asked. `$FLAGS` is the wrong thing to assert on — a structural test over
# a string says nothing about what the child received — and this catches the whole class: any
# future flag that goes missing from the hand-maintained list fails here by name.
TIER="tier 4 --lan — two real binaries in a namespace"
PROVE="./build/pairrepro.sh --lan -n 4"
EXPECT="the re-exec into the"
