# ADR-106 — a command runs the prerequisite the app can perform

**Status:** accepted
**Date:** 2026-10-08
**Context:** Dan: *"If an action, like signing, has a prerequisite action like detecting fields, have signing
automatically call detect fields as its first step."* A sweep at v1.194.0 found the places where a command stopped
and named a step the user had to go and do: five commands that need a page's text said *run OCR first* on a scan
(Read aloud, This page's table…, Reflow paragraph, the redaction search, Document text — the last saved an empty file
and said nothing); Save as fillable form… said *Run Detect*; Autofill said *edit the profile first*; three commands
reachable with no document said *Open a PDF first*; four dialogs said *pin the other person first*. `runOCR`,
`ensureText`, `detectFields`, `openFirst`, `openPeers` and the Autofill handler in `web/app.js`;
`pdfops.TextPages` and `handleOCRPages`.
**Extends:** ADR-009 (one door), ADR-001 (operation pinning), ADR-094 and ADR-101 (a page with a text layer is not
read again; replacing one is asked for, never implied), ADR-088 (the page's reader decides what is on it).
**Applies:** every command added later that has a step the user could be told to do first.

## Decision

**1. A command whose prerequisite the app can perform performs it, visibly, and continues.** The user presses the
command they want. If something has to happen first and the app can do it, the app does it as that command's first
step, says that it is doing it, and then runs the command — one press. A sentence telling the user to go and run
another command is a defect wherever the app could have run it.

**2. One door per prerequisite (ADR-009).** The rules about a prerequisite are written once, in the door, and every
command calls it. A command never decides for itself whether a page needs reading.

- **Text** — `ensureText(owner, pages)`. It answers `{ go, outcome, unread }`; `go` is whether the command runs now.
- **Fields** — `detectFields()`, which answers how many it added (`-1`: it did not run).
- **A document** — `openFirst()`.
- **A pinned peer** — `openPeers(then)`.

**3. What is never automatic.**

- **Anything destructive or irreversible**: applying redactions, removing originals, replacing a text layer
  (ADR-101 stands: a command's read sends no `replace` and never asks ADR-101's question), and reading a **signed**
  document without asking. A signed document is asked about through `confirmSignatureLoss()`, in the words every
  edit of one uses, and "no" is a refusal that is said.
- **A document locked for signing is never read.** A read is an edit. The OCR button is not among the tools the
  lock disables (/pending 830), so the door checks `signLocked` itself and refuses with a sentence.
- **Anything that is the user's choice**: where sign-here flags go, what a profile holds, who to pin, which file to
  open. The app **takes the user to it** — the profile editor, Identity & peers, the Open dialog — and, where the
  command can then run without a second decision, runs it when they come back (Autofill on *Save profile*; a
  peer dialog re-reads its list when Identity & peers closes).

**4. Which pages a text command reads.** Only a page that sets no text, paints something, and has no text layer:
`unread` in `GET /api/ocr/pages`, from `pdfops.TextPages` — the page map's own reader (`readPageShapes`, so the
same answer as `MapPage`'s `NoText`), for every page in one parse of the file. A page with text, a **blank** page
(it paints nothing) and a page with a layer — anyone's — are never read. A per-page command reads **its one page**;
a whole-document command reads every such page and then runs once.

**5. The command's document is the one it was pressed for (ADR-001).** The door takes the view captured at the
press. The read is of that view and **stops** when it is no longer the one in front (`owner === view &&
docShowing()`), checked before each page and before the words are sent — so a switch or a close during the read
leaves nothing done. The command is told to go on only if its view is still in front; where the words were already
on their way, the door says the page was read and the command was not run.

**6. The door speaks, once.** *"Reading this page first…"* / *"Reading N scanned pages first…"* stays up for as long
as the read does — in the command's dialog where it has one, else in `#readingNote` with a **Stop** — and is
announced politely, when the read starts and not per page. Every way the read can not happen (refused, stopped,
failed, document gone) is said by the door; a command told not to go on adds no sentence of its own.

**7. A read can be stopped, and stopping leaves nothing half-done.** Stop, Escape, the dialog's Cancel, or the
command's own button pressed again (Read aloud). Nothing is sent after a stop.

**8. A read that found no words is remembered.** For that page, until the document next changes: the command gives
its plain answer and the next press does not spend the same seconds finding nothing.

**9. The redaction search never passes over a scan in silence.** When its read is refused or fails, the search still
runs — and both of its results, *found* and *not found*, name the pages that were **NOT searched**. When the server
cannot say which pages are scans, the result says that. A stopped search marks nothing.

**10. The OCR button is unchanged.** `runOCR()` with no argument is the button: the whole document less its layered
pages, its two questions, its label for progress, its toast. `runOCR(cmd)` is the door's, and now answers what
happened (`read` / `nothing-to-read` / `refused` / `cancelled` / `failed`).

**11. The recogniser is kept between reads** — one language, let go when a document closes or after two minutes
idle; one stopped in the middle of a page is let go, not kept.

## Measured

One page, press to the command's result, real binary in headless Chromium, machine load 8-10 on 8 cores (another
build and a gate were running) — a worst case rather than a typical one:

| document | page | press → result | making the recogniser | recognition |
|---|---|---|---|---|
| a 3-page form scan, 1.0 MB | 1 (first read) | 13.1 s | 1.0 s | 8.0 s |
| | 2 | 16.4 s | 0.7 s | 10.3 s |
| a 14-page scan, 5.0 MB | 1 (first read) | 19.7 s | 0.7 s | 13.5 s |
| | 2 | 7.4 s | 0.6 s | 2.0 s |
| a generated page, two lines | 1 / 2 | 3.0 s / 2.9 s | — | — |

The reading note is up 0.10-0.12 s after the press. `GET /api/ocr/pages` on the two real files: 17-18 ms.
**The cost is the recognition, not the engine's start**: making the recogniser is 0.6-1.0 s of 7-20 s, so keeping
it (decision 11) takes about a second off every read after the first and no more. A command on a page that needs
no read costs one request (67-470 ms measured, the larger on the 14-page file).

## Declared gaps

- **Text drawn as outlines is read like a scan.** It sets no glyph, so no reader here can tell it from a picture of
  text; it gains a text layer. So does a page that paints only lines — read once, to no words, and remembered.
- **Find shows 0/0 on a scan** with no word about why, and does not read it.
- **Propose tags lists scanned pages** without offering to read them.
- **Edit text on a scan** has nothing to edit and does not read it.
- **Compare → Text on a scan** shows nothing and does not read it (the second document is not an open one).
- **Save as fillable form… on a scan** still names its fields `field_N`: it does not read the page to find labels.
- **Detect fields still clears every page's detections** before it runs (`clearDetected`), so Save as fillable form…
  on a second page drops the first page's. Unchanged here.
- **Save as fillable form… detects the page in view only.** A form over several pages is still Detect, page by page.
- **Autofill fills real form fields by name**; it does not run Detect, because a detected field is an overlay with no
  name for a profile to match.
- **A stamp pressed with no document open opens the Open dialog and stops.** It does not place the stamp once a
  document is open: where on which page is the user's choice.
- **The ~30 other "Open a PDF first" backstops** are behind controls that are disabled with no document; they are
  left as they are.
- **The reading note is not a progress bar.** It says how many pages; it does not count them off.

## Guards

- `test/jsdom/autoprereq.test.mjs` — every rule of the door against a stubbed recogniser: one page read and no
  other, no `replace`, no question; text / blank / layered pages unread; the remembered empty read; signed asked,
  locked refused; Stop, Escape, Cancel, and a switch during the read and after the send; each command; the search's
  NOT-searched naming in all three causes; Detect from Save as fillable; Autofill through the editor; the Open
  dialog; the four peer hints; the kept recogniser.
- `test/ui/autoprereq.test.mjs` — a real read in a real browser: the table export on a scanned page (and the timings
  above), the redaction search finding a word on a scan in one press, Save as fillable form… naming detected fields.
- `internal/pdfops` `TestTheUnreadPagesAreTheOnesThatSetNoTextAndAreNotBlank`, `internal/server`
  `TestTheWindowIsToldWhichPagesNothingHasRead` — which pages are unread, and that a read page stops being one.
- `test/jsdom/ocrhierarchy.test.mjs` — the OCR button, unchanged.
