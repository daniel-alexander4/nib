# docs/red-proofs.md, tier 3: "an authored field announces nothing" (/pending 476)
#
# `els.fieldNameGo.onclick` sends the typed text as `label` ALONGSIDE the identifier it derives
# from it; `pdfops.AuthorForm` writes `/TU` from `label` and omits the key entirely when it is
# absent. Drop the one line that sends it and every fillable form nib authors goes back to
# widgets a screen reader cannot name.
#
# **The whole Go suite stays green over that**, which is why the row is at tier 3 and not below
# it: `internal/pdfops`'s tests supply their own `FormField{Label: …}`, so they assert `/TU` from
# a string the client is no longer the source of. Only a drive that starts at a click can see it.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="no /TU was written at all"
