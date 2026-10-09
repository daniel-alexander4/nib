# ADR-113 — the invitation field goes with the ceremonies switch, and a hidden invitation is not sent

**Status:** accepted
**Date:** 2026-10-09
**Context:** Dan asked whether the Advanced features switches take their menu items with them. Three surfaces
followed the ceremonies switch (the panel, its tab and header, the Signing entry); one did not — the *Ceremony
invitation (optional)* field in Send & Receive's arm form, and the paragraph under it disclosing what an
invitation does. Dan: *"hide the invitation field with the switch."* `#srvInviteRow`, `#srvInviteNote` in
`web/index.html`; `armRecv` in `web/app.js`.
**Applies:** Send & Receive's arm form, and any other input that feeds a switched-off feature.

## Decision

**1. The field and its disclosure are marked `data-adv="ceremony"`** and so are hidden and shown by the sweep
`applyAdvanced` already runs. The ceremonies switch and not *Reach peers over the internet*: the field is a
ceremony's invitation, and with ceremonies off the server refuses what one leads to.

**2. An invitation is sent only while ceremonies are on.** `armRecv` reads the field through
`advanced.ceremony`. Hiding alone is half: a hidden input keeps its text, so an invitation pasted before the
switch went off would ride along on an arm the user believes is the plain one — the one whose own disclosure
says nothing reaches the internet.

**3. Ordinary co-signing is untouched.** The arm form, the peer list and the listen address stay; only the two
marked elements go (ADR-036's and `/pending 451`'s rule: cut at the surface, never at the mode).

## Consequences

- The disclosure about the public DHT is not shown with ceremonies off. Nothing it describes can happen then.
- **Covered by** `test/jsdom/advanced.test.mjs`: both elements follow the switch, a typed invitation is sent
  with ceremonies on and omitted with them off while the field still holds it. Each half was removed separately
  and the test went red.
