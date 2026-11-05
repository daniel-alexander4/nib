# ADR-209 — A DOCX or ODT template is filled from the paragraph's text, and a split placeholder is joined

Status: accepted, 2026-11-05 (/pending 937, RelaxTax handoff B4, the Office half; v2.49.0). Extends ADR-183 (the Markdown half), ADR-185, ADR-187.

1. **`pdfops.FillOfficeTemplate` is the one place a DOCX or ODT's `{{key}}` is filled**, called from `ConvertDocToPDFWith` before LibreOffice reads the
   package, so `nib office --set`, `nib import --set` and `Import` reach it by the same function. Page size, margin and images stay Markdown's
   (`optionsNeedMarkdown`, which the import door's gate also asks); placeholders on any other Office file (`.xlsx`, `.pptx`, `.ods`, `.odp`) are still
   `ErrOptionsNeedMarkdown`, and its sentence now says which formats take them.
2. **A placeholder is read from a PARAGRAPH's text, not from one run's.** Word and LibreOffice split `{{key}}` across `<w:r>` runs or `<text:span>`s
   for reasons the author never chose (a spell-check boundary, an edit session, one bold word), so a per-node replace left the literal braces in a
   "filled" document. The paragraph's text nodes (`<w:t>`; any text in a `text:p` or `text:h`) are joined, the placeholders are found in the joined
   text, and the value is written into the node where the placeholder BEGINS — it takes that run's formatting — while the rest of the placeholder is cut
   out of the nodes that held it. **This is the opposite of the Markdown path's refusal of a split key, on purpose:** in Markdown a split is something
   the author did with emphasis marks; in Word it is something the editor did without being asked, and refusing it would refuse most real templates.
3. **A tab, a break, a field, a picture or a note between two pieces of text is not text, and braces do not run across it** (`interrupts`: a NUL in
   the paragraph's text, which no key holds). `{{na<tab/>me}}` is `*mdpdf.MalformedPlaceholderError`. A placeholder never crosses a paragraph. Text in
   deleted runs (`w:delText`), field instructions (`w:instrText`) and every non-text node is not read.
4. **The rest is the Markdown rule, with the same error types:** a value is text and is escaped; a key with no value is
   `*mdpdf.MissingPlaceholderError`; braces that look like a placeholder and are not are `*mdpdf.MalformedPlaceholderError`; `{{=key}}` prints
   `{{key}}`; no error carries a value. New: a value with a character XML 1.0 cannot carry is `*UnwritableValueError` naming the key (a control
   character would make the part unreadable); a tab or newline in a value becomes `<w:tab/>` / `<w:br/>` or `<text:tab/>` / `<text:line-break/>`, and a
   Word text element is written `xml:space="preserve"` so a value's leading or trailing space survives.
5. **Only the parts that hold the document's text are rewritten** (`word/document.xml`, headers, footers, footnotes, endnotes; `content.xml` and
   `styles.xml` for an ODT) and only the bytes of a replaced text node change: the edit is a byte range found with the decoder's own offsets, so
   namespaces, attributes and every other part come through as they were. The package is read and written through `internal/ziputil` (ADR-187), the
   `mimetype` of an ODT stays first and stored, and the values added to one part are counted BEFORE anything is built against `packagePartCap` (a
   64 KiB value named a million times is refused as `ErrTemplateFilledTooLarge`; the first version of that test allocated the product and was killed).
6. **Declared, not fixed:** the value takes the formatting of the run where the placeholder begins, so a key typed half in bold gets bold; a
   placeholder inside a text box that Word also stores as an `mc:AlternateContent` fallback is filled in each copy; `.dotx`, `.ott` and the legacy
   `.doc` are not templates; ODT `text:s` and a field between the braces refuse rather than join.
