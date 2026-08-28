# docs/red-proofs.md, tier 1: "A law figure an operator can edit" (P07.S09a, D33, v1.117.223)
#
# The defect: `maxCandidatesPerSource` goes back to the bare `8` it shipped as, inside the tunable
# block, with its own comment still explaining that the eight comes from `ceremony.MaxCandidates`.
#
# **This is the state the slice found, not an invented one.** D33's discharge is a guard that
# fails "if either law figure is reachable from the tunable block", and a literal an operator can
# edit is precisely that: raise the upstream bound and the per-source cap silently stays behind,
# capping an honest DHT source below what its own record is allowed. A comment asserting the two
# agree is not a mechanism — the `NominalBlockRect` shape, one slice after the guard written for it.
#
# The patch removes the `ceremony` import too, and that is the point of its second hunk: the first
# attempt at this row left the import unused, so the package would not build and the guard never
# ran. A non-compiling check is the outcome `redproof.sh` distinguishes from a real red.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestNeitherLawFigureIsReachableFromTheTunableBlock"
EXPECT="reachable from the tunable block by hand-copy"
