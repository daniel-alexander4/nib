# ADR-219 — A picture is removed from a page through one door, which refuses a signed or ceremony document

Status: accepted, 2026-11-09 (PLAN-image-edit P01.S02; D2-D5 of the plan, Dan's call to plan it on 2026-11-06).
Extends ADR-009, ADR-032, ADR-072, ADR-124, ADR-146.

1. **`pdfops.EditPageObjects` is the one writer that changes a page's content for a picture**, and
   `internal/objedit` is its only caller outside `pdfops`. `objedit` holds the document's two refusals: a signed
   document (`sign.HasSignatureBlob`; `pdfops` cannot import `sign`, whose tests import `pdfops`, which is why the door
   is two layers and not one) and one carrying a ceremony record. The record is looked for with
   `pdfops.CeremonyRecord`, never `ceremony.Extract`: Extract calls a record entry that is there and does not decode
   "no record". Only no entry at all lets an edit through. Both refusals run before any read of the page. The
   record refusal reads the bytes, not `doc.ceremony` or `hasLiveCeremony`: a convened, unsigned document reopened
   cold is refused, because every counterpart recomputes its digest against the record.
2. **An edit names a picture by (page, image-only ordinal, digest)**, the address `ReadPageObjects` hands out. The door
   re-reads the page inside its one rewrite and refuses an address that no longer names the same operator
   (`object-changed`, the only 409 among the door's own causes; `resolveDoc` and `commitMutation` still answer
   409 for a closed or moved document, so the window branches on `cause`). The route edits the server's own snapshot, so it echoes no `baseRev`
   (`lostupdate_test.go` allows that only on routes that post client bytes); `commitMutation`'s snapshot check is
   its lost-update guard.
3. **A removal cuts the painting operator and nothing around it**: `q`, `cm` and `Q` stay. The page's resources are
   then pruned to what its drawing names (`pruneKeptResources`, redaction's prune), and so is any other page that
   named the picture without drawing it, and an intermediate `/Pages` node's `/Resources` entry for it goes once no
   page names it (measured: 27 corpus pictures were held only there); pdfcpu frees what it no longer reaches, so the
   image and its `/SMask` leave the file unless a page still draws it (`SharedWith` says which) or the form's default
   resources (`/AcroForm /DR`) name it — those are the form's and are left; `StaysInFile` says either. The edited page's `/Thumb` goes. Link annotations
   over the picture are kept and counted (`LinksOver`).
4. **A picture a structure element owns goes with its element in the same rewrite, or is refused.** Its kid is taken
   off the element's `/K` and its `/ParentTree` slot cleared BEFORE ADR-124's `deleteElement`, which would otherwise
   hand the picture's MCID to the parent. Refused by name: an element or a marked sequence holding other content
   (`tagged-shared` — read from the sequence's own tokens, so a shading, which the reader keeps no span for, counts),
   the tree's last element, counted over every element the request deletes (`tagged-last`, ADR-031), and any other shape (`tagged`: two owners, an
   inline owner, a tree that does not read). A marked picture no element owns is removed with its bracket.
5. **The written page is read back** and must be the original's pictures minus exactly the removed ones, in order, by
   digest, with the same text; otherwise `rewrite-differs` and nothing is installed. Any other write failure is
   `not-rewritable`. Every refusal is `pdfops.ObjectRefusal` with a cause and one sentence from one table
   (`objectCauseWords`, count pinned); the route answers 422 `{cause, error}` (ADR-072), 409 for `object-changed`,
   400 for `bad-request`; `nib objects remove` prints the same sentence. The listing route (`GET /api/page/objects`)
   is P01.S04's, built with the window that sends it; `nib objects` lists today.
6. **The rewrite drops a PDF/UA or PDF/A identification inside the write** (`writeMutated`, ADR-032), and the
   operation is a row of `TestNoOperationCarriesAnIdentificationItDidNotVerify`, asserted driven by name.

Guards: `TestEveryPageObjectEditRoutesThroughTheDoor` (repo root, routing) and
`TestEveryPageObjectEditRoutesThroughTheDoorInsidePdfops` (by type: a function reading a picture's span and writing a
page is the door or named). Eight red-proof rows, one per refusal and one for the guard.

Declared, the census's blind spots: a span read in one function and the content written in a helper it calls; a
content write by any writer but `setPageContent` (a form stream's `sd.Content`, a direct `/Contents`); methods with
one name on two receivers; an exported `pdfops` wrapper around the door called from the server; the server or CLI
calling `pdfops.ReadPageObjects` directly, which skips `List`'s document refusal. A page pruned because it named the
picture without drawing it is held by the after-check like an edited one.

Declared: a removal is undoable only within ADR-003's history budget (one edit's undo entry is the whole document's
bytes); pictures in annotation appearances, patterns, Type 3 glyphs and soft-mask groups are not listed (S01); two
pictures in one marked sequence refuse each other as `tagged-shared`.
