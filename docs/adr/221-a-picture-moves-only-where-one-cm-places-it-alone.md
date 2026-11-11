# ADR-221 — A picture moves only where one `cm` places it alone, and the window's route waits for the window

Status: accepted, 2026-11-09 (PLAN-image-edit P02.S01; D8). Extends ADR-219, ADR-220, ADR-088.

## Context

Moving or resizing a picture is a new matrix: it is the unit square under the CTM. The walk keeps the LAST `cm` at
each `q` level (`runGState.place`), so "the `cm` that places the picture" is not something the walk can vouch for:
`q A cm B cm /Im Do Q` records only B, and a `q` holding the picture may also draw, clip or set state.

## Decision

1. **Movable is read from the bytes**: exactly `q a b c d e f cm <image> Q` around the image's span
   (`aloneInBracket`) — one `cm`, nothing else drawn or clipped in the bracket — and a plain matrix (`a, d > 0`,
   `b = c = 0`), drawn by the page, wholly on the displayed page, under a clip that leaves the whole displayed page open (measured: under a tighter clip 20 of 394 corpus moves were cut wherever they went), with an outer matrix
   that can be undone. ONE function answers it (`objectMovability`), and it does not ask the removal's tag causes:
   a move writes no structure, so a tagged Figure moves and keeps its element.
2. **ONE conversion** from the listing's space to user space: `displaySpace.unpoint`/`unrect`, the inverse of
   `point` arm by arm. The new `cm` is the new matrix followed by the outer matrix undone, written by `num` over the
   old `cm`'s bytes in the page's one edit. Every target inside the page is then seen whole, so the door checks only that the box is on the page.
3. **The after-check holds a moved picture to its box** (1e-3 pt) and every other to its operator digest; a moved
   picture's address is new, because the digest covers its `cm`.
4. **The route stays a removal route until the window sends a move** (P02.S03): `objedit.DecodeEdits` reads no
   `box`, because `requestfields_test.go` refuses a field no client sends and its product-gap cap is full. A move
   reaches the door through `nib objects move`. `objedit.Remove` is now `objedit.Edit`.
5. New causes `not-alone`, `not-upright`, `partly-off-page`, `clipped`, with words in both tables (17).

## Consequences

- Two-`cm` placements and pictures drawn with other content are refused rather than merged; the corpus instrument's
  `OBJECTMOVE` line counts them by cause.
- Links over a moved picture stay where they were and are counted (`MovedObject.LinksOver`).
