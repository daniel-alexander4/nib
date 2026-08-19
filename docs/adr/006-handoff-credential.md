# ADR-006: A hand-off credential on disk — single-purpose, separate from the probe token

**Status:** Accepted
**Date:** 2026-08-17

## Context

A second launch of nib — a double-click on a PDF, or `nib file.pdf` while nib is
already running — must be able to tell the running instance to open that path.

The mechanism this replaced was worse than nothing: `internal/singleton` **SIGTERMed
every other process running the same executable**. It was also Linux-only, so on
Windows — where `nib register` makes double-click the ordinary path — a second launch
did nothing at all, silently, because the non-Linux build of `ReplaceOthers` was
`return 0`.

Giving the second launch a way to drive the running instance is **a change of
posture, not an implementation detail.** Today nib's CSRF token lives only in memory,
so no co-resident process can drive its API. A credential on disk is **the first
thing that lets another local process cause nib to act**, and that deserves to be
decided rather than discovered in a diff.

## Decision

**Adopted, with three constraints that are the substance of the decision:**

1. **A separate secret, not the probe token.** `internal/instance` says of the probe
   token that it "is not a capability: it proves identity to `GET /api/instance` and
   grants nothing else" — and reusing it would make that sentence false, turning
   every read of the rendezvous record into a capability grant. Two fields in one
   `0600` file cost nothing and keep a leak of the cheap, widely-presented secret
   from handing over the expensive one.

2. **One purpose, one route.** The secret authorises `POST /api/handoff` and nothing
   else. `/api/open`'s guard (CSRF + loopback origin + unlocked vault) is untouched,
   and **the CSRF token never goes to disk** — writing *that* down would hand a
   co-resident process the whole API rather than one verb.

3. **No new install path.** The hand-off resolves through the same machinery an
   ordinary open does — `LooksLikePDF`, the size cap, `addDocCapped` — differing only
   in how it is authenticated. A self-contained handler would be a second,
   less-checked way to install a document, which is the duplicate-derivation defect
   this repo has paid for most often.

## Consequences

**The residual, stated plainly:** a process running as the user that can read
`~/.config/nib/instance.json` can make nib open a document. It cannot read anything
it could not already read — it has the user's own access — so the marginal capability
is "make nib display a file". That is bounded further by Open now **adding** rather
than replacing, so a hand-off cannot swap the document under a user mid-signature.

**Surfacing the running instance's window has a limit, and it is not fixable here.**
No reliable cross-platform raise exists for a window you do not own — Wayland refuses
it by design — so the browser decides. On some combinations this produces a second
window pointing at the same nib rather than raising the first. That is survivable: a
second window is a second client, and the reload restore brings it up showing the
same documents including the one just handed off.

## Alternatives considered

- **OS-level peer credentials** — a unix socket with `SO_PEERCRED`, so the kernel
  vouches for the uid and no secret touches disk. **Genuinely stronger than any
  file-based token**, and refused because it needs a second transport (named pipes on
  Windows) and so **reintroduces exactly the platform split this work exists to
  remove**. `singleton` was Linux-only, and that is the whole reason the phase
  existed. Named here rather than omitted, because the weaker mechanism was chosen
  deliberately and a later reader is entitled to know the stronger one was on the
  table.
- **Reusing the CSRF token from disk.** Rejected — see constraint 2. It converts one
  verb into the whole API.
- **Keeping replace-and-kill.** Rejected: it destroys the incumbent's unsaved state,
  and it never worked anywhere but Linux.

## Provenance

Settled as decision D20 in the multiple-open-documents plan, with that plan's review
of the same date as its input. Built as P07 (v1.108.2); `internal/singleton` was
deleted in the same phase. The plan is retired; this ADR is the surviving record, and
`cmd/nib/main.go` cites it for the window-raise limit.
