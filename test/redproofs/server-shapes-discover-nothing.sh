# docs/red-proofs.md, tier 1: "the JSON package is listed and discovers nothing" (/pending 347, v1.117.308)
#
# **THE VACUOUS GREEN, which is the whole reason /pending 347 was filed rather than fixed in a line.**
# The obvious repair — add `internal/server` to `observablePackages` — was MEASURED to pass while
# discovering **zero** shapes from it: the package is unexported throughout (one exported struct in
# the tree, `Server`, with no json tags), so the export test drops all 54 json-tagged shapes.
#
# A package in the list that discovers nothing READS AS COVERED — to this scan, to the graduation
# pass that dispositions its rows, and to the next person who greps the list. That is worse than
# leaving it out, and it is the same class as `discovery.Seen` (named in `published`, never
# discovered, /pending 284) and as an inventory row naming an observable nothing produces
# (/pending 345).
#
# The GLOBAL floor cannot see it: 33 shapes from the other ten packages clears "at least twenty"
# comfortably. Only a floor that names the package can, which is why one exists.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryPublishedObservableHasANamedReader -count=1"
EXPECT="READS AS COVERED"
