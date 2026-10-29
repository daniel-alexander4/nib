# ADR-175 — an update across the digest flag day says what it strands

**Status:** accepted
**Date:** 2026-10-29
**Extends:** ADR-174 §1 (rule 7 is a flag day), ADR-102 (an update downloads and nothing asks).
**Context:** `/pending 921`, the plan-review pin on `PLAN-pdfcpu-upgrade.md` P03. A ceremony convened on a build before
2.3.0 cannot be finished on 2.3.0 or later, nor the reverse; the older side sees a dial refusal that names no cause. A
bad release cannot be pulled (`versionLess` offers nothing older), and nothing offers the older binary or `.deb`. The
person who updates with a ceremony waiting finds out after it is too late.

**Decision.**

1. **The update check counts, and only across the flag day.** `GET /api/update/check` adds `openCeremonies` — how many
   proceedings on this machine have not ended (`openCeremonies()`, the one count `hasLiveCeremony` now routes through,
   ADR-009) — and only when the update is available and `crossesCeremonyFlagDay(current, latest)`: current below
   `ceremonyFlagDay` ("2.3.0") and latest at or above it. A warning on an ordinary update teaches people to ignore it
   before the one that matters. A count, never an id or a name.
2. **It informs and asks nothing.** The download popup shows `#dlCeremonies` (`ceremonyUpdateWarning`) and the pill's
   description carries the same sentence. The download has already started (ADR-102) and replaces nothing — Nib never
   installs itself; installing is the user's. The sentence names both versions and says to finish on the one running.
3. **Unknown counts as open.** `Stored.Ended` is empty for a running ceremony and for one whose end nobody observed, and
   a record that did not load has not ended either: all are counted, because the error that matters is not warning.
4. **The release page says it too.** `docs/release-notes/<version>.md`, when present, is the release's notes
   (`build/publish-release.sh --notes-file`); 2.4.0's states the flag day and which build to finish on. The default
   remains the one line `Nib <version>`.

**Declared — and this is the large one.** The warning is built into the BUILD THAT IS RUNNING, so it does not reach anyone
who crosses 2.3.0 from a build that predates it: every machine on 2.2.x or earlier, which is every machine this flag day
strands, runs an update check with no `openCeremonies` reader and is told nothing in the popup. For that crossing the release
page (`docs/release-notes/2.4.0.md`) is the only channel, and the mechanism here is for the NEXT boundary — until
`ceremonyFlagDay` is raised, `crossesCeremonyFlagDay` is false for every update from 2.3.0 on. The plan's pin assumed the
popup could protect the first crossing; it cannot, because the code that would show it is the code being installed. A ceremony whose record is on another machine
is not this machine's to count. `ceremonyFlagDay` is a constant: a later rule retirement that strands ceremonies needs
its own boundary (a superseding ADR), or this one is wrong for it.
