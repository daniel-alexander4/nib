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
- **`nib fill --data -` reads the record from stdin, `--format json|xfdf` names it** (`fillFormat`, the one place the
  format is decided). Without `--format` the extension decides as before and a name with none — stdin included — is JSON.
  `--format` beside a `.csv` is refused (a mail-merge has no other reading). `IN` and `--data` both `-` is refused.
  The JSON stays pdfcpu's shape. `-o -` writes no file; `-o FILE` stages `.nib-*.tmp` in FILE's directory until the rename
  (the help says so). The oracle is a syscall trace of the whole process tree, because a staged file that is made and
  removed leaves nothing for a listing to find; its control run (`-o FILE`) must show the `.nib-` file.
- **Measured on the 2025 Form 1040, which carries an `/XFA` packet:** a filled copy has no `/XFA` (ADR-145) and no
  `/NeedAppearances` — the original has none either. A fill neither adds nor removes `/NeedAppearances`, so a form that
  carries it keeps it; do not describe the output as free of it in general. Two fields sharing a leaf name under different
  parents are addressed by full path and only that one is written (`TestAFillByFullPath…`).
- **`nib version --json` is `{"version","api","commands"}`, and a plain build reports the real version.** `api` is
  `cli.APIVersion`, an integer raised for an incompatible change to a command's flags, a JSON key or an exit code and never
  for an addition; `commands` is every verb, sorted. The VERSION file is embedded (`nib.VersionFile()`), and `main` uses it when
  no `-ldflags -X main.version` was set, so `go build ./cmd/nib` no longer prints `nib dev`; build.sh's `-X` and the embedded
  file read the same line and cannot disagree. Golden tests pin the object and the verb list.

## Declared

`optionValuesOffered` judges a value against the `options` the data carries; a hand-written datum with none is not
judged and nothing could be echoed. Only the fill path was searched for echoes: pdfcpu's other packages were not.
A value in a path nib or pdfcpu prints elsewhere (a field name chosen by the caller) is the caller's name, not its value.
