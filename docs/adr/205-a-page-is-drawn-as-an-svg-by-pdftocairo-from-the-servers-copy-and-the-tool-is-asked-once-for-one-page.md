# ADR-205 — A page is drawn as an SVG by pdftocairo, from the server's copy, one page to a call

Status: accepted, 2026-11-04 (PLAN-import-export P05.S01b, v2.46.0). Extends ADR-189 (the Format line), ADR-185 (the outside-tool door), ADR-193 (the server's copy).

1. **One tool, one function.** `pdfops.ExportPageSVG` runs `pdftocairo -svg -f N -l N` (`pdftocairoArgs` is the ONE spelling) on the
   document's bytes as they are, in a `tempdir` of its own, under `toolEnv`, through `runConvert` (tree kill on cancel, stderr tail, memory
   ceiling, `maxConvertedBytes`, 60 s). It is `ToolPdftocairo` in `missingtool.go` (`ErrPdftocairoMissing`, vendor poppler.freedesktop.org),
   declared in the exec census as a converter that sets `Env`. The page is checked against `PageCount` first (`ErrSVGPage`); what comes back
   must hold `<svg` in its head (`ErrSVGUnreadable`).
2. **One route, `POST /api/export/svg?page=N`**, pinned by `X-Nib-Doc` (ADR-004), behind `requireUnlocked`. It reads `doc.data` and `doc.rev` under
   the lock, draws outside it, and answers 409 if the revision moved. 503 `missing-tool`, 422 `page` / `unreadable`, 400 for a bad page number.
   It does not go through `/api/assemble`: that route packages rendered PNGs and an SVG has none.
3. **The server's copy, said so.** The window's overlay (marks placed, typed values, unapplied redactions) is not in it; the Format line's note
   says so, as Document as… does (W8). It is the only Pages-as-images format with that property.
4. **Availability is a find, not a probe.** `/api/status` `svgExport` is `PdftocairoAvailable()` (no process). Without it the entry stays on the
   Format line, selectable, its label ending "needs pdftocairo (poppler)" and the reason in `#saveAsStatus`, Save off (ADR-189's WebP shape).
5. **Measured:** poppler 24.02 draws a one-page fixture in 33 ms; text is glyph paths, so the file holds no font reference; the drawing of a
   page holds no `<script>` and no absolute URL but the two namespaces (asserted on the fixture).
6. **Declared:** `pdftocairo` reads a stranger's PDF with poppler's parser and has only the converters' wall clock, environment and memory
   watchdog around it (no seccomp); an encrypted document is refused as unreadable; a link in the page may be carried into the SVG as an anchor;
   one page per file, no every-page SVG; stock Windows and macOS have no `pdftocairo` (D7); Windows, macOS and a second poppler version are unrun.
