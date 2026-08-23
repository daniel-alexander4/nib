# docs/red-proofs.md, tier 1: "a goroutine outside internal/server, recovered SECOND" (/pending 255, v1.117.113)
#
# The defect: `defer wg.Done()` registered BEFORE `defer safe.Recover(...)`. Defers run LIFO, so the
# recover still fires — which is why this looked fine for as long as it sat there — but it no longer
# covers the goroutine's OTHER defers, and a panic inside wg.Done() ("sync: negative WaitGroup
# counter") escapes and takes the desktop process down. safe.Recover's own doc requires it at the
# very top for exactly that reason. The row also proves the guard reaches internal/ots at all: the
# guard it replaced was scoped to `pkgs["server"]` and could not see this file.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryDetachedGoroutineIsRecovered -count=1"
EXPECT="FIRST statement is not"
