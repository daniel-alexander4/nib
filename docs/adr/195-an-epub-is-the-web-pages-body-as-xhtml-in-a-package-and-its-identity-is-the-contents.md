# ADR-195 — an EPUB is the web page's body as XHTML in a package, and its identity is the content's

Status: accepted, 2026-11-03 (PLAN-import-export P03.S03, v2.39.0). Extends ADR-193, ADR-194, ADR-187; D21, D22, R1, R8, Y9.

**One body, two serialisations.** `html.XHTML` is the HTML writer's own block walk (`render.blocks`) with `xhtml` set:
void elements self-close, each heading gets an id (`h-N`) and is returned for a table of contents, and a picture is a
member (`images/img-N.ext`) instead of a `data:` URL. The web page's bytes are unchanged by it
(`TestXHTMLIsWellFormedAndTheSameBodyAsTheWebPage`). `docwrite/epub` wraps that body and adds nothing to the text path:
the escape, the link allowlist and the model's `Clean` are the ones the page uses; the package-level values (title,
language, ids) go through `xmlw.Esc`.

**The package.** `mimetype` (first, stored, byte 38), `META-INF/container.xml`, `OEBPS/content.opf` (EPUB 3.0),
`OEBPS/nav.xhtml` (one flat `toc` entry per heading; a document with none gets its title, because an empty `ol` is
invalid), `OEBPS/content.xhtml` and the pictures. It is built by `ziputil.Write`, so the R8 census is unchanged. One
content document: reflowable, and a reader paginates it.

**Identity is the content's, not the clock's or the machine's.** `dc:identifier` is a `urn:uuid` made from a hash of the
title and body (the plan said "a random EPUB id"; a random one makes two exports of one document two publications and
the same model write different bytes, with nothing gained). `dcterms:modified`, which EPUB 3 requires, is the constant
`2000-01-01T00:00:00Z`, so the package says nothing about when or where it was made. No creator, source, contributor,
rights or conformance-claim key is written (`TestEPUBHoldsOnlyItsNamedPartsAndNoClaims`).

**Two review fixes.** Nested lists in `html.XHTML` (and the web page) now sit inside the parent `<li>` — a `<ul>` holds only `<li>`, which XML parsing cannot see; and `xmlw.LangTag` keeps only BCP 47 shapes, because `dc:language` is validated.

**Judged** by parsing the XML of every member strictly (`wtest.Package.WellFormed` now reads `.xhtml` and `.opf`), the
model's words in order out of the content, heading / list / table / link / picture counts, the manifest against the
members both ways, the navigation's targets against the content's ids, and the container's rootfile. LibreOffice 24.2
has no EPUB import, so its confirmatory run reads `content.xhtml` alone.

**Declared:** `epubcheck` is not installed here and was NOT RUN; the package is checked against the spec by reading, not
by the validator. No EPUB 2 `toc.ncx`; the navigation is flat (heading levels are not nested); one content document, not
one per chapter; no cover. A reader that wants any of those opens the file in an editor.
