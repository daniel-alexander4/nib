# ADR-081 — a cyclic component is charged by its guarded targets and its unguarded path

**Status:** accepted. `/pending 803`'s regression (2026-10-03), found by the `/runpending` sweep. Supersedes ADR-077
in part — the charge `((n+2)/2)²` for a strongly connected component. The rest of ADR-077 stands: the depth pass, its
upper bound over every order pdfcpu could take, its guarded edges, and the 8,192 bound.

## Context

ADR-077 charged a cyclic component of n nodes `((n+2)/2)²`: the most levels a stack could spend inside it under the
worst split of those n into guarded and unguarded objects. It is the worst split of n, not this component's split,
so it is wrong in both directions:

- **Too much.** 2,000 forms whose `/Resources` name the one `/XObject` dictionary holding them (`/pending 706`'s
  `formsNamingTheirDictionary`) were charged ~1,000,000 and refused, so `TestAPageOperationOnHostileFormsFinishesAndKeepsEveryForm`
  went red on `main` from v1.182.23. Every edge there is guarded and the dictionary is no node of its own; pdfcpu goes
  2,000 forms deep.
- **Too little.** k forms, each the tiling pattern of the next (unguarded: `validatePattern` dereferences without a
  mark, pattern.go:127), the last drawing all k as forms (guarded), make pdfcpu re-walk the rest of the pattern chain
  under each form it enters: k(k+1)/2 levels. ADR-077 charged k = 124 3,969 levels.

Measured on `api.ValidateContext` with `debug.SetMaxStack(8 MiB)`, bisected: a plain chain of forms overflows at
7,763 forms, forms naming their own dictionary at 7,762, and the pattern/form shape at k = 124 — 7,750 levels by
k(k+1)/2. Each is about **1,081 bytes of stack a level**, and the three agree, so the level is the right unit and the
quadratic re-walk is real.

## Decision

- **A cyclic component is charged (g+1)·l**: g the members a guarded edge inside the component enters, l the longest
  path through its unguarded edges, in nodes. pdfcpu marks a guarded object before recursing (xObject.go:762), so a
  stack enters each of the g at most once, and between entries it follows only unguarded edges, which carry no loop
  (`validatorPaths` refused them) — at most l nodes a stretch, and at most g+1 stretches. An acyclic node is 1.
- **A guarded edge is told from an unguarded one by the edge, not the object**: the full expansion's edges beyond the
  unguarded expansion's, as a multiset, because one object can be named both ways (a form that is also a pattern) and
  only the guarded naming enters it once.
- **An unguarded loop left inside a component** can only be a tree's, which pdfcpu cuts at `MaxRecursionDepth` itself;
  l is then the larger of that cut and n.
- **An indirect dictionary that only holds guarded edges is a node of its own, and no level** — an indirect
  `/Resources`, `/XObject`, `/Font`, `/AP` or appearance subdictionary (`passThrough`, refgraph.go). The pass had read
  through each as if every object naming it held its entries, so n objects naming one dictionary of n entries were n²
  edges: 4,000 forms naming their own `/XObject` dictionary (~300 KB) took the depth pass **22 s** and 8,000 **84 s**,
  ADR-077's pass and this one alike, before pdfcpu's validator started. Now 2n edges: 29 ms at 4,000. It costs no level
  because pdfcpu reads it in the frames it reads a direct one in — measured on `/XObject`, 1,081 bytes a form level
  either way. Only the depth pass emits these nodes; the path count is unchanged.

## Consequences

- `formsNamingTheirDictionary(2000)` reads (charged 2,001); a ring of 9,000 forms and the k = 130 pattern/form shape
  (charged 131 × 130) are refused. `TestAComponentIsChargedByItsGuardedTargetsAndItsUnguardedPath`,
  `TestAnUnguardedPathIsWalkedOncePerGuardedTarget`, `TestASharedDictionaryOfGuardedEntriesIsOneNode` and
  `TestAnIndirectResourceDictionaryIsNoLevel` hold both directions and the cost.
- **Declared:** pdfcpu's own validation of the shared-dictionary shape is still quadratic — it re-reads the dictionary's
  n entries under every form (1.4 s at 2,000 forms, 5–9 s at 4,000, measured under load). That is pdfcpu's walk, not
  the door's, and the door does not bound it.
- **Headroom.** 8,192 levels × ~1,081 bytes is ~9 MB of stack. Go's goroutine ceiling is 1 GB and stacks grow by
  doubling, so ~512 MiB is usable: ~497,000 levels, matching ADR-077's measurement (300,000 links returned, 700,000
  overflowed). The bound sits ~60× under the computed ceiling and ~36× under the deepest chain measured to return.
- 0 of 2,536 real documents refused (the corpus ADR-077 used). The pass expands each object twice, once without its
  guarded edges; measured under load, the depth pass took 1.01 s over the corpus (96 ms at worst) against the path
  count's 1.11 s.
