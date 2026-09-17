# docs/red-proofs.md, tier 1: "/Stm written back onto the StructElem" (P02.S08, v1.129.144)
#
# The defect, verbatim as it stood from P01.S06 to P02.S08: the carry writes `d["Stm"] = pl.xobj` onto
# the structure element itself. ISO 32000-1 Table 323 enumerates a structure element dictionary's keys
# and `/Stm` is not among them, so a conforming reader ignores it and resolves the MCID against
# `/Pg`'s own stream — the exact failure the key was written to prevent. Table 325 has none either, so
# the `OBJR` arm was the same mistake.
#
# **Nothing could see it.** veraPDF and `nib ua` score the broken and repaired shapes identically —
# neither looks at a kid's encoding — and the repo's own completeness gate reads `/Pg` liveness,
# `/ParentTree` ownership and form draw counts. What it cost was measurable only by reading the text
# back: 31 of 61 elements returned two source pages' words concatenated.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestOnlyAnMCRCarriesAStmKey -count=1"
EXPECT="a key Table 323 does not define"
