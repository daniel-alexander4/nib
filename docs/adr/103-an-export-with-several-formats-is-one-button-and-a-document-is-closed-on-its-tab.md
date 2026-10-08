# ADR-103 — an export with several formats is one button, and a document is closed on its tab

**Status:** accepted
**Date:** 2026-10-07
**Context:** Dan: *"remove the Close menu item from File. we already have a close icon button at top
of every page. Simplify the Export As menu. make it clean and intuitive. If necessary collapse like
options together."* The *Export & Print* card held fifteen buttons; seven of them were three exports
under different extensions (a table three ways, form data three ways, pages as pictures two ways).
`web/index.html` (the File and Secure panes, `#tabrow`, `#saveAsModal`); `openSaveAs`, `saveAsBytes`
and `syncTabs` in `web/app.js`.
**Supersedes:** two sentences and nothing else —
- ADR-022's card list for File (*"Save a Copy, Export & Print, **Close Document**"*): there is no
  *Close Document* card. Its rule — the bar holds what you reach for continuously, the rest is a
  card — stands.
- ADR-037's *"the close controls keep the appear-at-two threshold"* and its table row for them:
  *Close* and *Close view* no longer exist. *Close all* still appears at two documents. The strip
  appearing at one document stands.
**Applies:** anything added to a File card, anything that closes a document, and every caller of
`openSaveAs`.

## Decision

**1. An export with more than one format is ONE button, and the format is chosen in the Save
dialog.** `openSaveAs(blob, name, title, formats)` takes an optional list; each entry has a label, a
whole default file name, an extension, and either bytes in hand or a maker called when Save is
pressed. With no list the Format line is hidden and the dialog is what it was. **Do not add a
per-format button, and do not put a dropdown inside a card.**

| button | formats (first is the default) |
| --- | --- |
| Pages as images… | This page (PNG) · Every page (ZIP) |
| This page's table… | Excel workbook (.xlsx) · Comma-separated values (.csv) · OpenDocument spreadsheet (.ods) |
| Form data… | Comma-separated values (.csv) · JSON (.json) · XFDF — for Acrobat and Foxit (.xfdf) |

**2. The bytes written are the SELECTED format's.** One function, `saveAsSelected`, reads the
selection, and `saveAsBytes` takes the bytes from that entry alone. While a maker runs, Save and the
Format line are disabled, so the format cannot change under bytes being made and a second press
cannot start a second render; the dialog says *Preparing…* once in a polite live region. A maker
that fails says why in the dialog and leaves it open. A dialog cancelled while a maker runs writes
nothing (`saveAsSeq`).

**3. The name follows the format only as far as the name is the dialog's own.** A name the dialog
put there is replaced whole (the picture formats differ by more than an extension: `-page3.png`,
`-pages.zip`). A name the user typed keeps every character but a known-format extension, which is
swapped. Anything else is left as typed; a second extension is never appended.

**4. What an export needs is captured at the button press** (ADR-001, D7): the view, its document
id, the page number, the export name. The table's grid is read at the press; the form's default
format is fetched at the press. So *"No text on this page…"* and a document with no form are said
**before** any dialog. A maker that runs later uses what was captured: a view record is reused by the
next document opened into it, so the picture makers check the view still holds the captured id and
refuse (*That document is no longer open*) rather than export another; the form maker's request
carries the captured id and a closed document answers 409.

**5. One document is closed with the × on its tab, and with nothing else.** File's *Close* / *Close
view* button and the *Close Document* card are gone. **Close all** sits at the right-hand end of the
strip's row (`#tabrow`), shown at two or more documents, and is the strip's **sibling**: `#tabstrip`
is a `tablist` whose every child is a tab, and `syncTabs` addresses them by position.

**6. Two buttons moved to where their subject is.** *Archival PDF (PDF/A-2b)…* is the last button of
*Save a Copy* — it writes a copy of the document. *Signing certificate* is a real button in Secure →
*Sign & Timestamp*, where a forwarding twin stood; it needs no open document.

*Export & Print* is now eight: Print… · Pages as images… · Pictures in the document (ZIP) · Document
text (.txt) · This page's table… · Form data… · Split by bookmarks… · Split by page range….

## Why the Save dialog and not a menu on the button

Every one of these exports already ended in the Save dialog, so the choice costs no new surface and
no new click for the default format. A dropdown inside a card is a second widget pattern in a list
of plain buttons, and ADR-087 already refuses a dropdown inside a group that folds.

## What this costs, and what it does not cover

- **No new request and no Go change.** The every-page render moves from before the dialog to the Save
  press — the same work, later. Unmeasured, and not expected to move.
- **A non-default format is one more choice than it was** (a select in the dialog, where there was a
  button of its own).
- **Closing by keyboard is the tab's ×** (Tab to the strip, then to the ×). There is no close
  shortcut and this adds none.
- **The fold ladder is untouched.** `applyFold` reads `#toolbar .tbfixed` only (ADR-087), so a
  File card's `data-fold` rank decides nothing by width; the three remaining cards keep the ranks
  they had.
