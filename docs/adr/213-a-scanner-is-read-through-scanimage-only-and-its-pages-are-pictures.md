# ADR-213 — A scanner is read through `scanimage` only, and its pages are pictures

Status: accepted, 2026-11-05 (/pending 903, v2.50.0). Decided with Dan the same day: Linux first, Windows and macOS declared gaps.

1. **`scanimage` is the only door** (`internal/pdfops/scanner.go`). Nib speaks to no scanner itself. The Scan… button exists where the tool is found
   (`/api/status` `scanner`, which starts no process) and is absent elsewhere; Windows (WIA) and macOS (ICA) have no tool to ask and are declared gaps,
   listed under external validation, not stubbed.
2. **The list is asked when the dialog opens and never at start-up** (`GET /api/scanner/devices`): a network scanner is found by discovery and that took
   about 15 seconds on this machine. The route remembers the names it gave (`Server.scanKnown`) and a scan is accepted only for one of them, so a device
   name is not a string taken on trust (a `pnm:/path` name would read a file as a scan). The name is also held to a shape (no leading `-`, no control
   characters) and passed as `--device-name=NAME`, one argument, never through a shell.
3. **A page is a PNG and goes through the picture door.** `scanimage --format=png` writes each page; each is turned into a PDF by `Import` (so the pixel
   caps are the picture door's, ADR-185) and the pages are joined with `Combine` into ONE new, pathless document. Save As gives it a path (ADR-212 §5).
4. **A feeder run ends well only on the out-of-paper exit.** `scanimage --batch` exits 7 when the feeder is empty; `runConvertPickOK` takes that code as
   success and no other. A jam or a timeout after some pages is refused whole rather than returned as a document missing the end of the stack. The source
   name is read from the scanner's own `--help` (`feederSource`), because backends call it `ADF` or `Automatic Document Feeder`.
5. **The scan holds the import gate** (`ImportGate`, one outside tool at a time with the converters) with its own budget: five minutes for the glass,
   fifteen for a feeder. A cancelled request ends the tool (`killTreeOnCancel`), so Cancel in the dialog stops a scan.
6. **Verified** against SANE's own `test` device (one page, and a feeder run of ten) and by listing a real network scanner; **not** scanned on a physical
   flatbed or feeder. Red against each: the out-of-paper exit not accepted, an option-shaped name accepted, the feeder name not recognised, the list check
   skipped, the mode check skipped, the client always sending the glass, and the button always shown.
