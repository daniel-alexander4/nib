# ADR-204 — An SVG is rebuilt from an allowlist and shown only as a picture, at its declared size

Status: accepted, 2026-11-04 (PLAN-import-export P05.S01a, v2.45.0). Extends ADR-200, ADR-201; implements X11(c).

1. **One row, the browser door.** `svg` (`FamilySVG`, `ToolBrowser`, not `Image`, not `Door`) is sniffed by `isSVG` (the first markup is
   `<svg`; a page that holds an inline svg opens `<html`/`<head`/`<body` first and stays a web page) and goes through
   `ConvertSVGToPDF`. Only a name that says SVG sends it there; text and Markdown names keep it text.
2. **The picture is parsed strictly and rebuilt** (`sanitizeSVG`, encoding/xml, `Strict`): a DOCTYPE is refused (no entity can expand),
   the root must be `svg` in the SVG namespace, elements are copied from `svgKeep` and anything else is removed with its children
   (`script`, `foreignObject`, `animate`, `set`, `feImage`…). `a` and `switch` are unwrapped. An attribute is dropped if it is `on*`,
   `src`, in an unknown namespace, an `href` that is not `#fragment` (or, on `<image>`, a base64 png/jpeg/gif/webp), or holds
   `javascript:`, `@import`, `<`, `&#` or a `url(` that is not `url(#id)`. `style` text goes through `cssIsInert`.
3. **It reaches the browser only as `<img src="data:image/svg+xml;base64,…">`** in a wrapper Nib writes itself (`svgWrapper`), served by
   the same one-request origin with the same host rule and CSP header (ADR-200). A browser draws an SVG in image mode with no script and
   no fetch; the rebuild is the rule and the image mode and the origin are the walls behind it.
4. **The page is the declared size, capped, and written by Nib.** `svgSize` reads width and height (px, pt, pc, mm, cm, in, none), else the
   `viewBox`; a percentage or no size is a refusal; each side is 1 to 14,400 px. The numbers go into `@page{size:…}` and the image's
   width and height as formatted floats, never as the file's text. Measured: Chrome 157 prints 400 x 100 px as 300 x 75 pt on one page.
5. **Caps (provisional, R5):** 8 MiB of source, 200,000 elements, depth 128. Each refuses; none truncates.
6. **Evidence.** Nine vectors, each with a positive control that must fire (the same picture inline in a page from `file:`) and each
   checked three ways: the raw picture through the origin in an `<img>`, and the whole import path. Two vectors were dropped because the
   control did not fire (`onload` on a `rect`, a cross-origin `use`); a nested `<svg onload>` replaced the first. 14 sanitiser mutations
   went red after seven survivors were closed with tests (depth and byte caps both vacuous at first).
7. **Declared:** fonts the file names are not loaded; embedded raster pictures are the browser's to decode inside its own limits; SMIL
   animation and links are removed, not played or followed; Windows, macOS, Firefox and a snap browser are unrun (ADR-200).
