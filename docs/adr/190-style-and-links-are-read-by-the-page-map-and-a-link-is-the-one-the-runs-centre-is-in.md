# ADR-190 — font style and links are additive reads, and a link is the one a run's centre is in

Status: accepted, 2026-11-02 (PLAN-import-export P02.S03a, v2.34.1). Extends ADR-088, ADR-090.

**Style.** `MapText.Bold` / `Italic` (`omitempty`, so a plain page's payload is byte-identical) come from `fontStyle`
(fontstyle.go), once per font object: descriptor (`/FontWeight` ≥ 700, ForceBold; `/ItalicAngle` ≠ 0, Italic flag), then
the base-font name's tokens with `ABCDEF+` stripped; a descriptor that speaks to one axis does not silence the name on the
other; "Semibold" is not bold; a Type 0 font reads its descendant; `Tr 2` is bold. The detection readers ignore both.

**Links.** `pdfops.PageLinks` / `pageLinksIn` (pagelinks.go) carry `http`, `https` and `mailto` URIs as boxes in the
map's own fractions (`placeOnPage`, `/QuadPoints` per quad else `/Rect`); `At` takes the link holding a run's centre. Every
other link is dropped and counted by cause (`internal`, `remote`, `scheme`, `malformed`, `capped`). Hidden and NoView
links are not links. **The Print flag is not read** — the plan's "print-off" skip would have dropped links with `/F`
absent, which hyperref and most producers write; found by running the remedy. Declared: an `/OC`-hidden link is read as a
link (layers resolve at the model's read, S03c).

`testpdf.ExportSet` holds the oracle's hand-built fixtures that need no tree (two-column, right-to-left, scanned, covered,
styled-linked); tagged and merged-cell wait for the model reader. Nothing consumes the links or the set until S03c.
