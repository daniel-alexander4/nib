# ADR-171 — a Web Capture record Nib could not read is said at the open, and removed when asked

**Status:** accepted
**Date:** 2026-10-13
**Amends:** ADR-164's "only the tree's own removal goes through (`ForgetNameTree`, two named sites)" — three now —
and its refusal's wording, which named three of the ten trees whatever was lost.
**Context:** /pending 889 part 2. A document whose Web Capture tree (`/URLS`) pdfcpu's validator could not read
whole is refused every change by ADR-164 — fill, rotate, anything — and was told about "its attachments, its named
destinations or its scripts". Measured over 3,785 local PDFs: one marks a tree unread, and it is this one: its
`/URLS` is one level deep with correct limits, and the validator keeps 333 of 371 names because it refuses a VALUE
in one leaf and drops the leaf. Reshaping before validation (ADR-164's depth fix) cannot save it. Dan decided
(2026-10-13, /discuss): an asked-for removal, never a carry.

## Decision

1. **The refusal names what was lost.** `RefuseNameTreeLoss` words every unread tree through ONE function
   (`pdfread.NameTreeWords`): "this document holds a record of the web pages it was captured from, which Nib could
   not read in full and cannot carry through a change — so nothing was changed". `errors.Is(…, ErrNameTreeLost)`
   holds. The server's door adds `removable: "webcapture"` where the record is what was lost.
2. **The document says so at the open.** `unreadLists` (the trees) and `unreadWhat` (the same words) ride on the
   document's answer, read off the read `ReadAnswerFacts` already makes (ADR-147: a document says what a step will
   do before the step). The window's fourth corner notice (`#listNotice`) says every change will be refused and,
   for a Web Capture record, carries *Remove it*; a refused change brings a dismissed notice back.
3. **The removal takes the whole record** (`pdfops.RemoveWebCapture`): `/URLS`, `/IDS`, `/SpiderInfo`, every
   page's `/ID` and `/PZ`, verified by re-scanning. Sanitize method `webcapture`, the scan panel's own button, and
   `nib sanitize --web-capture`, which runs first because the record refuses every other rewrite. A signed document
   is asked through `confirmSignatureLoss`, as strip and safe are. The scan reports a `webCapture` kind (exempt
   from strip: it runs nothing).
4. **`handleSanitize` answers a name-tree refusal as the refusal (422)**, never `{ok:false}` — which the window read
   as "this method failed, try the next one down" over a document every method would fail on.
5. **`ForgetNameTree` has three named callers**, held by `TestForgetNameTreeHasItsThreeNamedCallers`.

## Refused

**Carrying the dropped leaf across the validator** (ADR-164). **Re-running the refused change automatically after
the removal**: its request carries the document revision it was built on (ADR-146), and the removal moves it, so a
replay is a stale write by construction; the offer comes before the step instead.

## Consequences

- One real document in 3,785 local PDFs; the others carrying Web Capture are read whole and are untouched.
- The other nine trees are named in the refusal and the notice, with no removal offered from the notice: their
  own removals (attachments, scripts) already exist.
