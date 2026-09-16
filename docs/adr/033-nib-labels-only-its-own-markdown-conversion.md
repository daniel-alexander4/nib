# ADR-033 — nib writes the PDF/UA identification only on its own Markdown conversion, with a language someone chose

**Status:** accepted
**Date:** 2026-09-15
**Context:** `/pending 486`, option B (Dan, 2026-09-15). Rests on ADR-032 and `/pending 471`.
**Applies:** `pdfops.LabelUA` and its two callers, `cmdOffice` and `handleOffice`.

## Decision

`pdfuaid:part 1` is written by exactly one door, `pdfops.LabelUA`, called from exactly two places: the CLI's
`nib office` and the server's `/api/office`, and **only for a Markdown source**. The door refuses, each with
its own error, unless all of these hold:

- **the language was asserted** — `--lang` on the CLI, or a Document language the user *chose* in the app
  (`langChosen`); the field's pre-fill is this computer's language, and a claim cannot rest on a guess;
- **the document is tagged**, over a tree that describes it — never the untagged fallback;
- **it declares a language** in the catalog;
- **it has a `dc:title` and asks viewers to display it**;
- **it carries the packet nib wrote**, so the identification lands where readers look.

It is the last write. Every later change drops the claim again (ADR-032).

## Why this and not "label when nib's checker passes"

nib's checker covers 19 of the 106 rules veraPDF evaluates; a document can pass all 19 and fail one it does
not check (`counterexample_test.go`). A label on the checker's say-so would be a conformance assertion
over a non-conformant document — ADR-031 law 1. So the claim does not rest on the checker at all. It rests on
**veraPDF's measurement of the conversion itself**, across every construct mdpdf renders:
`TestNibsOwnMarkdownConversionConformsAcrossEveryConstruct` runs veraPDF on a labelled conversion holding one
of each, and `TestTheLabelFixtureCoversEveryNodeKindMdpdfRenders` fails when mdpdf learns a node kind the
fixture lacks. Measured at the grill: every kind passed except the thematic break, whose rule was untagged
content (7.1 t3) — it is now bracketed `/Artifact`, and the labelled conversion fails nothing.

## Why only a chosen language

Dan chose the GUI field's default as the machine's locale (`/pending 471`, option B). That is a reasonable
default for `/Lang` and a poor foundation for a conformance claim: a German document converted on an English
machine would be labelled conformant while declaring itself English. veraPDF checks that a language is
present, not that it is right, so the standing test cannot catch that. So the label needs a person to have
chosen the language. A user who never touches the field gets a correct-by-default `/Lang` and no label:
**a visible loss rather than a false claim**, ADR-031's trade.

## What it costs a user

Nothing but the label. A refusal never fails the conversion: the CLI prints a note naming the reason, and
the server logs any refusal other than "not chosen", which is the ordinary case.

## Declared limits

- **The standing test runs only where veraPDF is installed**, and skips as "UNMEASURED", never as a pass.
- **Office conversions are never labelled.** Their structure comes from LibreOffice, which nib did not measure.
- **A labelled, signed document keeps the label** through a later edit — ADR-032's declared gap.
