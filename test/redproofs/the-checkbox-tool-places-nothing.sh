# docs/red-proofs.md, tier 3: "the checkbox tool places nothing" (v1.128.0)
#
# The Checkbox tool's placement call is dead, so the button arms, the cursor changes, the click is
# swallowed and no widget appears.
#
# The tool is a thin one — `makeField('check', …)` is the widget Detect already produces, and
# `pdfops/form.go` already writes `"check"` into the AcroForm — so the ONLY thing it adds is the
# placement, and the only thing worth guarding is that a click lands one.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="placed 0 checkboxes, not one"
