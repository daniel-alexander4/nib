# Contributing to Nib

Nib ships as a single Go binary with the web UI embedded (`embed.go`). There is no
build system beyond the Go toolchain, and nothing in this file is needed to *use*
Nib — it is here so that "did I break anything?" has one answer instead of three
half-answers.

## The build & verify contract

Run all four after a change. Each states what it can see, and — more importantly —
what it cannot, because a check whose blind spots are undocumented gets trusted
for things it never looked at.

| # | Command | What it verifies |
|---|---------|------------------|
| 0 | `go build ./...` | it compiles |
| 1 | `go test ./...` | the Go side: server, PDF operations, vault, signing, CLI |
| 2 | `./build/jsdomtest.sh` | the front end's logic and DOM behaviour, in jsdom |
| 3 | `./build/uirepro.sh` | the whole app: the real binary, in a real browser |

Add `node --check web/app.js` after editing JavaScript, and `go test -race
./internal/server/` after touching anything concurrent.

Tiers 2 and 3 **skip cleanly** when their dependencies are absent — they print a
line saying which one is missing and exit 0. A fresh clone therefore runs 0 and 1
with no setup at all. To enable the other two: `npm install` (jsdom and
playwright-core, dev-only, `node_modules/` is git-ignored) and have a
Chromium-family browser on `PATH` — the same browser Nib itself needs to show its
UI, so if Nib runs at all, tier 3 can run.

There is also `./build/winrepro.sh`, which runs the Windows binary under wine to
check the places `path/filepath` answers differently, and to run a **second
launch** against a live first one — the single-instance hand-off, on the platform
where double-click is the ordinary way in and where the mechanism it replaced
never worked at all. It is not part of the routine loop — run it when touching
path handling, file dialogs, launch/hand-off, or packaging.

## The three tiers, and why there are three

They are not redundancy. Each one exists because the tier below it is blind to
something, and each says so in its own file rather than only here.

**Tier 1 — `go test ./...`**
Sees: every server route, the PDF pipeline, the vault, signing and verification,
the CLI. This is the bulk of the suite and the fastest feedback.
**Cannot see: the client at all.** The server never observes the browser's state,
so no amount of Go testing can tell you whether a button is enabled, whether a
teardown cleared a sidebar, or whether an armed tool survived a document switch.
Every client-side failure mode is silent here. → **tier 2**

**Tier 2 — `./build/jsdomtest.sh`** (ceiling written in `test/jsdom/boot.mjs`)
Sees: the real `web/app.js`, loaded into a jsdom document built from the real
`web/index.html`. Module behaviour, DOM state, control enablement, teardowns,
which of pdf.js's calls the app made.
**Cannot see: anything that needs rendering.** jsdom models the DOM, not an
engine — no layout (every `clientWidth` is 0), no canvas, no media queries, and
pdf.js itself is stubbed. → **tier 3**

**Tier 3 — `./build/uirepro.sh`** (ceiling written in `build/uirepro.sh`)
Sees: everything above, plus real layout, real canvas, the real pdf.js parsing a
real PDF, real files on disk, and the real Go server answering — because it builds
the binary, runs it under a throwaway `HOME`, and drives it in a browser.
**Cannot see: other engines.** Chromium only, which is deliberate — Nib opens its
UI in an installed Chromium-family browser (`internal/browser`), so Chromium is
what users get. The fallback path can still land someone in Firefox, and that gap
is tracked as a `VERIFY` item on the pending list rather than pretended away.

That chain is the point. A gap named and delegated is a decision; a gap nobody
wrote down is discovered later by a user.

### Proving a tier can fail

Every tier here has been **proven red at least once** against a deliberately
reintroduced defect, and that matters more than any of them being green: a check
never seen to fail can only ever report pass. Two guards enforce the parts of this
that rot quietly — `verify_test.go` (the commands in the table above exist and are
named) and `internal/browser/browser_test.go` (tier 3 hunts for the same browsers
Nib does). Both are in tier 1, so they run first and for free.

## The multiple-documents laws

Nib holds several documents at once (`PLAN.md` P03–P06). Three rules constrain code
whose author will never read the plan that produced them, so they are written here —
in the committed file a fresh clone actually gets — rather than only in an ADR or in
a local `CLAUDE.md`.

1. **No operation may act on a document it did not capture at its start.** ADR-001.
   An operation that bakes document A's bytes and then posts them must name A, or
   the server applies them to whatever is active when the request arrives. Capture
   the id before the first `await` and pass it as `apiFetch`'s `docId`.
   *Guarded by* `test/jsdom/pinning.test.mjs` — "no mutating call is unpinned".

   The law has a **third** failure mode on the client, and it fails independently of
   the other two: a gesture holding a `setPointerCapture` keeps receiving
   `pointermove` after its view is hidden, because capture is released when the
   element leaves the DOM and `display:none` does not remove it. Such a gesture
   cannot pin an owner and carry on — a hidden container reports `clientWidth` 0, so
   continuing would lay the field out against nothing. It must be **cancelled**:
   `enableStampGestures` and `enableMarkerGestures` register an `end` in
   `activeGestures`, and `abortDrags()` drains it. Anything new that takes a pointer
   capture registers there too.
   *Guarded by* `test/ui/gestures.test.mjs` — "a drag in flight when the user switches
   documents neither moves the flag nor records onto the new document" (tier 3: jsdom
   has neither `setPointerCapture` nor layout).
2. **Every reload names the view it lands in.** The same law's other half, and they
   fail apart: `setDocumentFromServer(meta)` with no target writes into whatever
   view is active when the round-trip *returns*, wiping that view's overlays, redact
   marks and undo stack. Only the document-installing routes (`open`, `open-url`,
   `upload`, `combine`, `office`) may omit a target, because installing into the
   active view is what opening a document means.
   *Guarded by* `pinning.test.mjs` — "every reload names the view it lands in".
3. **`views` is mutated in exactly three places** — `addView`, `removeView`,
   `resetViews` — and each re-renders the tab strip. The strip is a rendering of
   that array; a fourth mutator leaves it describing a set of documents the app does
   not hold, and nothing else would say so.
   *Guarded by* `test/jsdom/view.test.mjs` — "views is mutated in exactly three
   places, and each re-renders the strip".

Each guard has been proven red against the defect it names. If you are changing one
of these and the guard is in your way, the guard is the thing that is right.

## Repo conventions

- **Bump `VERSION` in the same commit as the change.** It is a single semver line
  and the source of truth for `build.sh`, `install.sh`, and the `.deb`. Patch for
  fixes and refactors; minor for a new user-facing feature.
- **Keep docs in step with the code** in the same change, not afterwards.
- `THIRD-PARTY-NOTICES.md` is generated — run `build/gen-notices.sh`, and
  `TestNoticesUpToDate` will tell you if you forgot.
- **A new architectural decision gets an ADR in the same change** —
  `docs/adr/`, numbered, with `_index.md` listing them. ADRs are immutable in
  their decision content: supersede with a new one rather than editing an old one.
- Licensed AGPLv3; the project ships as-is, with no warranty.
