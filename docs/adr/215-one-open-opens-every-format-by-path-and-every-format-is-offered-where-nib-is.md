# ADR-215 — One Open… opens every format by path, and every format is offered where Nib is offered

Status: accepted, 2026-11-07 (Dan: "when I save an imported document it should open the dialog to the same folder from which I opened
it"; "for all of those doctypes that nib supports, when I rightclick on that type of document it should give me an Open with Nib
option"; "make ~/nib an easy pick if not already the save location"; Import… folded into Open…). Extends ADR-214. Supersedes ADR-033's
list of label callers, ADR-184's `Door` rows and "the pickers' `accept`" for the import picker.

1. **The `Door` flag is retired.** It marked the formats "every route opens as it stands", and every row of the format table opens at
   every route through `pdfops.Import` since PLAN-import-export P01 (path, upload, URL, hand-off). It still marked only pictures and CBZ,
   so the folder listing hid .docx, .md and .html, and `nib.desktop` claimed none of them. The listing (`IsFormatExt`), the desktop file
   (`TestTheDesktopFileMimeTypeIsTheTable`) and Windows' `nib register` now read every row.
2. **Import… is gone; Open… is the one way to convert a file, and it opens BY PATH.** The browser's file picker never tells a page a
   file's path, so a document imported through it had no folder and ADR-214's dialogs started in `~/nib`. Opened by path, a converted
   document keeps `srcPath` and the dialogs start beside its source. The Document language field moved into the Open dialog. One door,
   `withLanguage`, hands it to `Import` from both of the dialog's routes (`/api/open`, `/api/open-url`), and both answer `labelNote`.
   A door with no dialog (a double-click, Open Recent) sends none. The label's callers are `nib office` and those two routes,
   only for a language someone chose.
3. **`/api/office` is retired.** Nothing in the window called it. Each property it was tested for (a declared language, the label only
   when chosen, a bad language refused, the cap asked before a conversion, the converter ended at shutdown) is asked of `/api/open`.
4. **A by-path refusal is the refusal an upload gives.** `isImportRefusal` kept its own list of Import's errors beside `importRefusal`'s,
   and the copy drifted: the text and CSV caps, a CSV that does not read and a refused language answered "could not read file" (500).
   A failed read of the source is now marked AT the read (`sourceFile`); everything else Import says is a verdict about the file.
5. **Open…'s tooltip is built from the table** (`/api/status` `formats`, each row naming its `tool`), one line per tool, with a tool
   this machine lacks marked "not found". `exts` left the status answer: its only reader was the retired picker.
6. **Windows: one ProgId per row** (`Nib.<key>`, named with the row's label), so a .docx a user makes Nib's is not called a "PDF
   Document". Nib's value is removed from each `<ext>\OpenWithProgids`; those keys are never deleted.
7. **The destination dialogs offer `~/nib` by its whole path.** `listdir` answers `defaultDir` and `atDefault`; the button's text is the
   path, it has no tooltip, it wraps rather than being cut, and it is hidden where the dialog already is.
8. **Declared.** Drag-and-drop and Combine still upload, so a document they bring has no folder. Claiming `text/plain`, `text/html` and
   `text/csv` may list Nib for text subtypes in a file manager's Open with; a desktop entry sets no default, but where a distribution
   names none for a type Nib may become it. Windows is compiled, not run.
