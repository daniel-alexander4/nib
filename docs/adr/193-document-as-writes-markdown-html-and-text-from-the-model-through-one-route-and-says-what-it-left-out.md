# ADR-193 — Document as… writes Markdown, HTML and text from the model through one route, and says what it left out

Status: accepted, 2026-11-03 (PLAN-import-export P02.S04, v2.37.0). Extends ADR-192, ADR-072, ADR-004; D23, D28, X8, X9, Y9, W3, W8, W11.

**One route.** `POST /api/export/document?format=md|html|txt&title=&redactionsPending=` is pinned by `X-Nib-Doc`, captures
`doc.data` and `rev` under `s.mu` (`snapshotRegistered`), reads the model once (`read.Read`) outside it, and answers 409 if
`rev` moved. A document with nothing to write is 422 `cause: empty`; one the reader could not open is 422 `unreadable`.

**Facts ride in `X-Nib-Export`** — `ExportFacts`, built field by field from `docmodel.Report`: integers, booleans, page
numbers and ONE enum string (`source`), capped lists with `truncated`; `TestExportFactsAreIntegersAndEnumsOnly` holds that
no field can carry a string a document chose (W3). The window's `exportReport` is the one function that words them, and
`EXPORT_KEEPS` the one sentence per format; the Format-line note is both, so they cannot disagree.

**Writers** (`internal/docwrite/{markdown,html,text}`) import `docmodel` and no PDF reader. Text passes one door,
`docmodel.Clean` (invalid UTF-8, C0/C1 controls, U+FFFE/FFFF), then one escape door per syntax: Markdown backslash-escapes
every syntax character and a line's leading `#`/`>`/`-`/`1.`; HTML is `html.EscapeString` only, `http`/`https`/`mailto`
links only (re-checked in the writer), a CSP meta allowing `img-src data:` and inline style, no script, pictures inline as
`data:` URIs. A picture in Markdown and text is a line naming it: a single file carries no bytes and a sibling file under
a name the PDF chose is what Y9 forbids. The title is the name the user confirmed else "Document" (X9).

**Judged** by reading each output back with its own parser (goldmark rendered to HTML; `x/net/html`) against a manifest of
the model's words in order, over a model whose every value tries to be markup.

**The window.** *Document as…* replaces *Document text (.txt)* (`exportDocBtn`); `documentText` stays for Compare. It
runs `ensureText` first (ADR-106), confirms with Cancel as default when a redaction box is unapplied (X8) and sends the
count, fetches Markdown at the press so a refusal is the server's own words before any dialog, and the other formats on
choice.

**Declared:** no `completeness` figure (the reader's per-page tree-vs-proposal test and the oracle stand in; R1's
per-page multiset equality over a real corpus is not measured here); text from signature appearances is not specially
excluded (`signed` is reported); nested HTML lists nest a `ul` inside the parent list, not inside its `li`; `read.Read`
takes no context, so a closed connection does not stop a read in flight.
