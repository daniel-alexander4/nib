# docs/red-proofs.md, tier 3: "any stamp counts as a signature" (v1.127.1)
#
# The checklist's signature probe goes back to counting every `.ovl-stamp`, which covers the quick
# stamps — a date, a checkmark, an "approved" — so stamping today's date ticks "Place your
# signature or initials".
#
# **This is the shape the whole feature fails in**, and it shipped: a probe that is nearly right.
# The row is green, the list looks complete, and the one thing it was built to answer — what is
# left before I sign — is answered wrongly. Reported by Dan, one release after the list was added.
#
# A signature or initials comes from the Library and carries `/api/images/<id>` in its src; a quick
# stamp is a `data:` URL built on the spot. Only a rendered overlay shows that, which is why the
# row is tier 3.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="ticked \"Place your signature or initials\""
