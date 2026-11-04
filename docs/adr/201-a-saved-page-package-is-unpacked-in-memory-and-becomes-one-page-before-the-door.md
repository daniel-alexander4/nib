# ADR-201 — A saved-page package is unpacked in memory and becomes one page before the door

Status: accepted, 2026-11-03 (PLAN-import-export P04.S04b, v2.43.0). Extends ADR-200; W34, Y8.

**Two rows, one door.** `mhtml` (`.mhtml`, `.mht`; `FamilyMHTML`, sniffed as `MIME-Version:` in the first 2 KB and
`multipart/related` in the head, so a mail message — multipart/mixed or alternative — is not claimed) and `epub` (`.epub`;
`FamilyZip`, held to its name by the `mimetype` member, as ODF is). Both have `ToolBrowser`. Neither is a new renderer: each
becomes ONE HTML document and takes ADR-200's path (`sanitizeHTMLWith`, the one-request origin, the probe).

**Nothing is written and no path comes from the package.** A picture survives only as a `data:` URL built from a part of the
same package that the page names by the package's own name for it: an MHTML `Content-Location` or `cid:`, an EPUB manifest
`href` resolved from the chapter's folder with `path.Join` and looked up in the archive's member table. A name that matches no
member (`../../etc/passwd`, an `https:` address) matches nothing and the picture becomes its `alt` text. The bytes are typed by
`sniffImage`, never by what the part says; the types admitted are the table's (`dataImageTypeOK`, ADR-184). `image/svg+xml`
is not inlined (SVG is P05).

**Caps, each with a hostile fixture.** 500 parts, 8 MiB a decoded part, 96 MiB together, 4 levels of nested multipart, a 64 KiB
header block (MHTML); the archive caps `packageLimits`, 300 spine items, the same part sizes (EPUB). Over a cap is
`ErrPackageTooBig` (counter cause `cap:archive`, 422); a package with no page, no container, no spine or a spine naming nothing
is `ErrPackageUnreadable`. A damaged MIME tail ends the walk and keeps what was read.

**EPUB is joined from Nib's own output.** Each chapter is sanitised on its own (so its pictures resolve against its own
folder); the join cuts each result into style elements and body at the fixed shape `sanitizeHTML` writes, with a page break
between chapters. That cut is exact only because the input is Nib's: a style sheet containing `<` is refused so it cannot
fake `</head>`. Style sheets carried by `<link>` are dropped, so an EPUB prints with the browser's default look plus any inline
style; this is declared.

**Declared.** Fixed-layout EPUB, EPUB 3 navigation documents, fonts and SVG pictures are not read. Character sets: an MHTML
HTML part is converted from its declared charset; an EPUB chapter is read as the parser reads it. No real-world MHTML from a
browser other than the hand-built fixtures was run, and no `.msg`/`.eml` (S05).
