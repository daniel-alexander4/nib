# ADR-224 — A drawing is one object, grouped by the region reader's one door and moved by its own operands

Status: accepted, 2026-11-10 (PLAN-image-edit P03.S03; D10). Extends ADR-219, ADR-220, ADR-221; supersedes ADR-125's
grouping rule in part (§2).

## Context

A page's vector drawings — a chart, a frame, a ruled table, a highlight, a rule — were listed only by the untagged
reader (ADR-125), which grouped painted paths inline. Editing one as an object needs the same grouping, an address
of its own, a way to take it off the page and a way to move it, and a measurement of whether the grouping matches
what a person sees before the window offers it.

## Decision

1. **One grouping door, two callers.** `groupDrawnPaths` (structregion.go) is the only grouping of painted paths;
   `untaggedPieces` and the objects listing (`drawingsOf`) call it, held by `TestThePathGroupingHasTwoCallers`. It
   keeps marked paths (grouped only with paths under the same MCID, refused `tagged` by the listing, left out of the
   untagged listing), compares the stream as well as standing, and reports when `clusterBudget` ran out — then no
   drawing is listed (`DrawingsUngrouped`). Members are answered as indexes into the walk's own list, never by
   stream offset: a form drawn twice draws the same bytes at the same offsets twice. Marked paths now share the
   comparison budget with the rest; over the corpus the budget ran out on 0 of 4,683 pages. Decoration is one standing whatever `/Artifact` sequence holds it: nib's
   own commit writes one sequence per path, and comparing sequences broke ADR-125's own round trip (measured).
2. **A plain box joins nothing it holds WITH ROOM** (`holdsWithRoom`; supersedes ADR-125's "never through a plain
   rectangle that holds the other whole" where the two boxes agree to `sameShapeSlack`, one unit of user space). A fill the size of the
   outline drawn on it is one shape: six of the eleven drawings a labelled sample missed were such pairs (a browser's
   code span). The untagged listing changes on 286 of 4,683 corpus pages.
3. **An address of its own.** `PageObjects.Drawings`, ordered by the first path each draws, numbered apart from the
   pictures; an edit names `object: "drawing"` (absent reads `picture`). The digest is `drawingDigest` — its members'
   stream, matrix and bytes under its own label — through `addressDigest`, so ADR-220's twins rule holds for drawings.
4. **Delete removes every member's span**; a path object changes no graphics state, so no `q` is asked for — and a
   path whose bytes hold anything but construction operators, plain numbers, a clip operator and its paint (a colour, a
   `cm`, a `q`, which viewers draw) is refused `not-path-only` for removal and move alike (`pathOperands`). **Move is a
   translation only**, written as each member's own coordinate operands moved by the page-space step undone through
   that path's matrix (`translatedPath`) — never a `q … cm … Q` wrap (a clip would end early under `Q`, text mode is
   not tracked). The box sent must be the drawing's size; anything else is `bad-request`.
5. **Refused by cause**: `form-drawn`, `tagged` (removal only — a move writes no structure), `clips` (a path that is a
   clip as well as painted), `patterned` (move only: a pattern stays where it is laid), `clipped` and the box causes as
   for a picture, `not-path-only` (§4), `not-a-picture` for a replace. The window's and the CLI's words come from the
   one table (26 causes).
6. **The after-check holds every path** the edit did not touch, by its bytes, in order, and a moved drawing's paths
   by their construction count and the box the translation put them in (`pathsAgree`), beside the pictures and text.
7. **The window is offered drawings only past a threshold fixed before it was measured.** `build/drawinggroup.sh`
   scores a hand-labelled sample (local, never committed) by the share of drawings a group matches exactly; the
   threshold is 0.90. A fresh sample labelled after §2 scored 0.893 (108 of 121), so `drawingsInWindow` is off and the
   route answers no drawings; the window's code for them ships, exercised by the jsdom tier. `nib objects` lists and
   edits drawings regardless.

## Consequences

- Four causes join the table: `clips`, `patterned`, `not-path-only`, `not-a-picture`; the causes a drawing shares with a picture are
  worded without the noun.
- The corpus never showed a clip-and-paint path (0 of 165,611 groups); 1,179 groups are patterned and 1,853 hold paths
  under different matrices, which a translation by each path's own operands moves together.
- What keeps drawings out of the window is the grouping of forms ruled by lines that do not touch, and of a logo or a
  word drawn in parts (/pending 980). Turning the switch on needs the instrument at or over the
  threshold on a sample labelled after the change.
