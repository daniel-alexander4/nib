# docs/red-proofs.md, tier 1: "A deleted tag's integer MCID is kept as an integer under a parent on another page (ADR-124)"
#
# The defect: an integer means "on the owning element's page", so kept under a parent that names another page, or none, the content it names is on the wrong page.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAnIntegerMCIDStaysAnIntegerOnlyOnTheNewParentsPage"
EXPECT="owns /MCID 1 and names no page"
