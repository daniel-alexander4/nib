# ADR-183 — A program can call nib without starting the app, writing a configuration or hearing a value back

**Status:** accepted
**Date:** 2026-11-01
**Context:** the RelaxTax handoff (`~/.claude/projects/-home-dan-repos-relaxtax/memory/handoffs/2026-11-01-nib-requirements.md`).
RelaxTax runs an installed nib as a child process over tax data, so nib must be safe to call from a program: nothing
starts but the verb, nothing is written that the caller did not name, and no value the caller supplied comes back in
a message. Audited against v2.16.9; each claim below was re-measured before it was built.

## Decision

- **`NIB_HEADLESS=1` makes every non-verb an error** (`headless()`, `cli.Run`). `nib bogus`, `nib some.pdf` and `nib` with
  no argument exit 1 and start no server and no browser; `version` and `help` still answer. Only the value `1` is the
  switch. It is an environment variable and not a flag: a flag would be a second spelling of one rule (ADR-009), and the
  caller that needs it cannot rely on its own argument list being a verb. Unset, the fall-through stands — it is how a
  double-click opens a file.
- **A verb never writes pdfcpu's configuration directory** (`keepPdfcpuConfigClosed`). Before any verb runs,
  `model.ConfigPath` is `"disable"`, which `api.DisableConfigDir` (deprecated in v0.16.2) sets and which every
  `model.NewDefaultConfiguration()` site in the tree reads. Measured: `fields` alone created `config.yml`, `certs/` and
  `fonts/Roboto-Regular.gob`. A filled f1040 (12 text fields, 2 check boxes) has the same 39 decoded streams, text and
  field values with it closed as open (the bytes differ run to run on both, so bytes are not the oracle). Cost pdfcpu
  documents: user fonts are unavailable; nib sets its own prose in faces it embeds (ADR-150). The GUI path is untouched.
- **A refusal never repeats the value it was given.** The only echo found in nib's code was `optionValuesOffered`
  (combo, list and radio); it names the field and the form's own options. pdfcpu's form package was searched for a
  message built from a value (`errors.New(... +`, `Errorf` with `%s/%q/%v`) and the rest are field ids, object numbers
  and `%T`. A decoder's type error names a type, not the text.

## Declared

`optionValuesOffered` judges a value against the `options` the data carries; a hand-written datum with none is not
judged and nothing could be echoed. Only the fill path was searched for echoes: pdfcpu's other packages were not.
A value in a path nib or pdfcpu prints elsewhere (a field name chosen by the caller) is the caller's name, not its value.
