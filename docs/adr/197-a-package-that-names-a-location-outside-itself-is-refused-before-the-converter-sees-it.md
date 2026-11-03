# ADR-197 — a package that names a location outside itself is refused before the converter sees it

Status: accepted, 2026-11-03 (PLAN-import-export P04.S01, v2.39.2). Extends ADR-185, ADR-187; X11(a), X11(d).

**The scan.** `pdfops.scanPackageReferences` (packagescan.go) opens a zip document package through `ziputil` under
`packageLimits` and reads, with a streaming strict XML decoder, every `*.rels` part (OOXML `Relationship/@Target`) and,
for an OpenDocument package, every `content.xml`, `styles.xml`, `settings.xml` and `meta.xml` (at the root or in an
embedded object) for `xlink:href`. A reference is refused (`ErrPackageNamesOutside`, 415, counter cause `names-outside`)
when it is a scheme other than `http`, `https`, `mailto` or `tel`, a Windows path or UNC share, a path with a backslash,
an absolute path (an OOXML internal target may be absolute only when it names a member), or a path that climbs out of
the package. An OOXML target is resolved from the directory of the part that owns its `.rels` file (`../customXml/x.xml`
from `word/_rels/` is inside), an External relative target is a file on this machine and is refused, and an ODF relative
reference must name a member or a directory of members. Each reference is checked as written and percent-decoded. An
OpenDocument package is one with a `mimetype` member or with `content.xml` and `META-INF/manifest.xml`. A part that cannot be read or parsed refuses the package; "not scanned" is never "passed".

**One door.** It is called from `ConvertOfficeToPDF`, the single function every LibreOffice route reaches (`Import`,
Combine, a direct caller), before the tool is looked for. `Import`'s own `inspectPackage` keeps the name-and-type check;
the scan is not a second copy of it.

**Where this departs from X11(a), and why.** X11(a) says to refuse *any* external OOXML relationship and each fixture
"must print the secret with the scan removed". Both were measured on this machine (LibreOffice 24.2.7.2): a section
source, a table source (`copy-results-only`), a `WEBSERVICE("file:///…")` formula, an `INCLUDETEXT` field and an external
`altChunk` sub-document converted without the local file's text reaching the PDF. So the positive control the plan asked
for does not exist for these five, and the scan is a precaution, said so in the README. Refusing every external target
would also turn away ordinary documents (web links are external relationships), and web fetches are already closed by the
dead proxy (ADR-185); the scan therefore refuses LOCAL references and leaves web and mail links alone.

**Declared:** field codes (`INCLUDETEXT`, `INCLUDEPICTURE`) and `w:instr` are not read; a part in a non-UTF-8 encoding is refused as damaged; OLE files (.doc, .xls, .ppt, .pub, .vsd), RTF and an iWork file's protobuf parts are not scanned (there is
no parser here to read an external reference from them); spreadsheet formulas that read a file are not scanned; nothing
here proves LibreOffice follows none of the refused references in another version.

**X11(d) is already built.** `toolEnv` sets an allowlisted environment at the converter sites and `execdoor_test.go`
checks it; the census is per FILE, not per exec site, so a second `exec.Command` in a file that already sets `Env`
passes — left, noted.

**The rows are not added.** Visio (.vsd, .vsdx), Publisher (.pub) and iWork (.pages, .numbers, .key) have installed
LibreOffice filters but no fixture and no oracle (LibreOffice cannot write them, Apple's software cannot run here), and a
format-table row is a claim that the format opens. They wait for a measured file.
