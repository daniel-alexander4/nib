# ADR-043 — a page selection may not reveal what the source hid

**Status:** accepted · v1.134.x

## Context

`internal/pdfops/pageselect.go` rebuilds a subset's catalog from a default-deny allowlist, and its
header argues the shape at length: an in-place page-tree rewrite is a blacklist, so everything
survives unless the code drops it, and the failure mode of a denylist is the key nobody thought of
leaking into every extract, split and redaction artifact a user is about to hand to someone else.
Every disposition on that list until now has been justified the same way — **a key carried onto a
subset can be a positive false statement about content that is gone, which is worse than an absent
one.**

`/pending 525` scanned every catalog in veraPDF's PDF_UA-1 corpus (295 of 297 readable) for the keys
the allowlist drops, to find out which of them a real producer actually carries. Four were not
page-indexed and therefore decidable with no destination map: `/PageLayout` (40 files), `/PageMode`
(33), `/OutputIntents` (26) and `/OCProperties` (6).

**One of the four inverts the whole argument.** An optional-content layer's content lives in the
page's own content stream, bracketed `/OC /L0 BDC … EMC`, and the page's `/Resources /Properties`
names the group. Both travel with the page dictionary, because a selection carries it whole. The
only thing that says the group is switched OFF is `/OCProperties /D /OFF` in the catalog. So
dropping the key does not lose a layer — **it reveals one.**

Measured 2026-09-16 on a three-page fixture whose page 1 carries a 300×200 pt filled box inside a
group listed in `/OFF`, subset to pages 1 and 3, page 1 rasterised at 40 dpi:

| renderer | source | after `Collect("1","3")` |
|---|---|---|
| Ghostscript 10.02.1 | 39 dark px | **18,855 dark px** |
| poppler `pdftoppm` | 6 dark px | **18,598 dark px** |

Two independent renderers, the same answer. nib's own `StripActive` deletes this key for exactly
that effect and `scan.go` calls it *"the intended hidden-content reveal"* — intended on a door the
user reached by asking to strip active content, not on "keep pages 1 and 3". And `RedactPages`
builds its runs of untouched pages through `collectWithoutStructure`, so the reveal was landing
inside a redaction, on every page the redaction did not touch.

## Decision

**A page selection never reveals content the source hid.** `/OCProperties` is carried
(`internal/pdfops/optionalcarry.go`), and the carry is **not** gated on the structure carry the way
`/Outlines` is. That gate exists because an outline title *describes* content a raster destroyed;
this key *hides* content, so the doors that must not carry a description of destroyed content are
precisely the doors where revealing hidden content is worst.

**The guard is reachability, not a key allowlist.** `/OCProperties` can do exactly one harmful
thing: an indirect reference in it that resolves to a page the selection dropped puts that page's
dictionary, and therefore its `/Contents`, back into the output, because pdfcpu writes by
reachability — `pageselect.go`'s `/StructTreeRoot` hazard one key over. So the subtree is walked and
**refused entire** if it reaches a dropped page, which leaves exactly the document this primitive
wrote before the carry existed. Every way of not knowing — an unresolvable reference, a subtree past
the depth cap — answers "refuse". A key list would have to enumerate `/Usage`'s sub-dictionaries and
every future addition to them to say the same thing less completely, and measured over the corpus
there is no page reference in any of it to enumerate: the six files carrying the key hold
`{OCGs, D{Name, Order, ON, AS, RBGroups}}` and their groups `{Type, Name, Usage}`.

**Nothing prunes the groups.** A group whose content lived only on dropped pages survives as a
layers-panel entry that toggles nothing. That is a dead label, not a disclosure. Pruning it means
finding every `/OC` reference in the kept pages — content-stream `BDC` operands, form XObject and
annotation `/OC` keys, and membership dictionaries nested arbitrarily deep — and a walk that misses
one drops the group that was hiding something, which is this decision's own defect reintroduced by
the cleanup.

**`/AS` is carried here and removed in `correctOptionalContent`, and the two are not in conflict.**
There it is stripped from a configuration nib itself just wrote, where it was measured redundant
with the `/ON` beside it. Here it belongs to somebody else's document, where a usage-application
entry is one of the two ways a layer is hidden on screen.

**`/PageLayout` is carried and `/OutputIntents` is dropped, and the second is the one that needed
saying.** The layout is a bare name that makes no claim about any particular page — which is exactly
what separates it from `/PageLabels` — and it is read as a name and re-written as one, so an
indirect `/PageLayout` is dropped rather than passing a reference of the source's out on a key
nobody would think to check. `/OutputIntents` is the ICC profile a PDF/A claim rests on, and a
subset has already removed the claim by construction: `/Metadata` is off the allowlist, and where
the structure carry re-adds one it builds a fresh title-only packet. What would be carried is the
apparatus of a claim the output does not make, and measured, that is literal — **26 of 26 corpus
entries are `/S /GTS_PDFA1`**, a subtype whose whole meaning is "this is the output intent PDF/A
requires". Writing it onto a document whose `pdfaid` the same operation just removed is ADR-032's
rule one key over. The door that wants an output intent makes its own (`injectPDFAMarkers`).

**`/PageMode` follows what survived, and nothing more.** `/UseOutlines` where an outline survived
(ADR-less, /pending 524) and `/UseOC` where the optional content did. `/UseAttachments` names
embedded files the subset drops — 8 of the corpus's 33, so with `/UseOutlines`'s 25 the two together
are all 33. `/FullScreen` is refused twice over: nobody asked a page selection to turn on a
presentation mode, and `/ViewerPreferences /NonFullScreenPageMode` — which says what the reader does
on exit — is off the allowlist, so carrying it states one half of a setting whose other half this
operation destroyed.

## Consequences

- An extract, split, reorder, page deletion or redaction of a layered document keeps its layers
  switched as the author left them. `HasStampLayer` also starts answering truthfully after a
  selection, because it reads `/OCProperties /OCGs`.
- The corpus scan's caveat travels with the decision: **283 of the 295 files are
  `veraPDF Test Builder 1.0`**, and the corpus holds one LibreOffice file and one Word file. The
  scan says a tagged document does carry these keys; it does not say the distribution resembles a
  real producer mix. That half stays gated on `PLAN-ua-coverage.md` P08's corpus.
- A source `/OCProperties` too incomplete to be safe cannot reach the carry: every subset door reads
  through `api.ReadValidateAndOptimize`, and pdfcpu's validator requires `/D`.
- `TestASubsetLeavesExactlyTheCatalogItLeftBefore` gains two declared divergences and keeps grading
  every other key — `/OutputIntents` included — against the old implementation, so a change that
  starts carrying a dropped key fails there.
- **Not addressed, deliberately:** `/OpenAction` (220 files) is page-indexed and needs the
  remap-or-drop rule `/pending 555` carries. And `api.MergeRaw` keeps only the FIRST document's
  catalog, so a hidden layer in a document merged in AFTER one is still revealed by the composition
  — pre-existing, untouched by this decision, and filed separately.
