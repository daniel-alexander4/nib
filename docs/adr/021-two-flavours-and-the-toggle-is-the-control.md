# ADR-021 — two flavours, and the toggle is the whole control

**Status:** accepted
**Date:** 2026-09-06
**Context:** Dan: *"remove the frappe and the machiato options and let's just stick with the other
two. Remove them from the settings menu. the icon will be enough, but keep the colors that are
being used for the sidebar pills"*
**Supersedes:** ADR-019's four-flavour half and the radio picker it put in the settings menu. The
rest of ADR-019 stands — a theme still exists in more than one place, the lists are still compared,
and the card tint is still a measured per-theme token.
**Applies:** anything that adds a theme, or reads `data-appearance`.

## Decision

**Two flavours: Latte as `light` and Mocha as `dark`.** Frappé and Macchiato are removed from the
stylesheet, the server's whitelist, the contrast guard's list, and the settings menu.

**There is no theme picker.** The sun/moon button in the status cluster is the whole control, and
the settings menu's four-way radio list is gone. Two states do not need a list to choose from, and
the toggle was already the control everyone used.

**A retired flavour normalises where it is read, not where it is stored.** `applyAppearance` treats
anything that is not `light` as dark. Vaults on real machines hold `frappe` and `macchiato` and
nothing rewrites them; the value survives until the user's next toggle saves a current one.

## Why normalise rather than migrate

A migration pass would have to run somewhere — at vault load, or in the settings handler — and
would rewrite a preference the user last set deliberately, on a machine where nothing had gone
wrong. Normalising at the point of use is one line in the one function that already decides what
the attribute becomes, and it is correct for a value that arrives from anywhere: a hand-edited
vault, a restored backup, a vault written by a newer Nib and opened by an older one.

**What it prevents is invisible, which is why it is guarded.** Without it the stylesheet finds no
`:root[data-appearance="frappe"]` block, falls through to the bare `:root` tokens, and renders
Mocha — the right pixels by accident — while `<html>` carries a `data-appearance` that no rule
claims. Nothing looks wrong today, and the next rule keyed on that attribute inherits the defect.

## What did not change

**The card tint.** ADR-019's `--card-tint` is a per-theme token because accent-coloured text fails
all six accents in Latte and a tint over `--surface0` leaves light red at 3.92; the two surviving
levels (Mocha 28%, Latte 22%) are unchanged, and the sidebar pills keep their six accents in both.
The two levels that went with Frappé and Macchiato were only ever about those palettes.

**The agreement guard, which now asserts an ABSENCE.** The picker was one of the four lists it
compared. Dropping it from the comparison would make re-adding a picker — offering a value the
stylesheet and the server have never heard of — invisible to the one test written to catch exactly
that, so the guard asserts instead that no `themechoice` control exists.
