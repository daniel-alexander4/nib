# ADR-216 — Open…'s tooltip is one line per format, alphabetical, with what it takes in parentheses

Status: accepted, 2026-11-07 (Dan: "make the tooltip on the Open menu item list each format alphabetically with the
explanation in parens next to it"). Supersedes ADR-215 §5's "one line per tool" only.

1. **One line per row of the format table, sorted by label** (case-insensitive), each followed by what it takes in
   parentheses: `built in`, `needs <tool>`, or `needs <tool>, not found on this computer`. The list is still the
   server's (`/api/status` `formats`, ADR-184); the window only sorts it.
2. **The tool is named short** (`pdfops.MissingTool.Short`, cut from `Name` at its first " ("), so "a HEIF decoder
   (libheif's heif-dec, or ImageMagick)" does not nest parentheses inside the tooltip's own. `Name` keeps the
   alternatives for every refusal and notice that says what to install.
