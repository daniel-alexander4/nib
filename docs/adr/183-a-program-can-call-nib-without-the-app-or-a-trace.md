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
- **The three JSON shapes are documented and locked** (`docs/cli-machine-readable.md`, `TestTheMachineReadableShapesAreLocked`):
  `fields --json` (origin bottom-left, `/Rotate` not applied), `text --json` (origin TOP left, draw order) and `info --json` (one object
  per page). Each key, its type and each rectangle's length are pinned; the test fails on a rename or removal, which is the change
  that raises `APIVersion`, and the document must name every key. Exit codes are in the same document. The shapes themselves did not change.
- **`nib images IN --out-dir DIR [--json]` writes a PDF's embedded pictures** over `pdfops.ExtractImageFiles`, which the archive
  route (`ExtractImagesZip`) now also goes through: one door finds pictures (ADR-009). A JPEG stream is written byte for byte,
  anything else re-encoded with the same pixels. pdfcpu's renderer returns no dimensions or filter, so width and height come from
  the written file's header and the filter from the stream dictionary. Files are `0600`, a name that exists refuses the whole
  extraction before any write, and a document with none creates no folder. A page's pictures are ordered by object number: pdfcpu returns them from a map
  and the full-suite inspection comparison caught two swapped between runs on a real document.
- **A release carries `SHA256SUMS` and says how to install one version.** `build.sh` writes it over the binaries and `.deb`
  packages of the version it built (not the licence files) and `--publish` uploads it; `TestAReleaseCarriesItsChecksums` runs the
  block on a stand-in `dist/`. The README's *Install a specific version* gives the download and `sha256sum -c`. Measured: the
  remote's highest tag was v2.7.0 against VERSION 2.16.9 (`git ls-remote --tags origin`), so a pin to an unpublished version could
  not be satisfied until a release is cut; tagging and publishing stay the maintainer's act (`./build.sh --publish`).
- **`nib flatten IN -o OUT` is a vector flatten, and the window's rasterising Flatten is not its model** (RelaxTax Group B1;
  `pdfops.FlattenForm`). pdfcpu v0.16.2 has no flatten (it only locks fields or deletes annotations), so nib writes one: each
  widget's `/AP /N` (or `/N[/AS]` for a button) is reused as a Form XObject and drawn on the widget's rectangle by the matrix of
  ISO 32000-1 12.5.5 (the form's `/BBox` through its `/Matrix`, scaled and moved onto `/Rect`), inside `/Artifact … EMC`, with the
  page's existing content wrapped in `q … Q`; an appearance with no `/Resources` takes the form's `/DR` before the AcroForm goes
  (the key is unlinked, not freed: `dropKey` freed the font). A widget with a value and no appearance refuses the call by name; a
  hidden or no-view one is removed undrawn; a signed document is refused unless `--drop-signature`. Tagged: the widgets' OBJRs
  leave the tree as a removed annotation's do (`unnameInStructure`) and the Form elements that held nothing else are deleted
  (ADR-124's `deleteElement`): left behind they failed veraPDF's 7.18.4 t2, measured by the UA census. Measured on the public
  f1040 filled in 14 fields: 16 flattened, 183 blank, 176,769 → 87,245 bytes, 0.6 s, the value found as text by `nib text` and
  `pdftotext`, `nib fields` empty. The 73 check boxes have a dictionary of states for `/N` and need `/AS` to choose one.
  Declared: a screen reader does not announce a flattened value; a ticked box's ZapfDingbats mark has no Unicode mapping
  (veraPDF 7.21.7); an unbalanced page graphics state is only closed once; widgets under optional content are not special-cased.
- **`--no-clobber` on `fill --out-dir` and `split --out-dir`** (RelaxTax B7) is checked in `writeSplitFiles`' own first pass, before the
  first write, beside the source-overwrite refusal (ADR-009: that helper is the one door both verbs share). It is opt-in: the
  default still replaces, because a script that reruns over a folder may rely on it; `nib images` refuses by default because it is new.
- **`nib merge --bookmark-per-file`** (RelaxTax B6, `pdfops.BookmarkMergedFiles`) builds the whole outline from the INPUTS — each
  file's title at its running page offset, its own outline one level down — and sets it with `SetOutline`, rather than reading what
  `Combine` carried: the offsets are then the inputs' page counts, checked to sum to the result's. Titles are made unique across
  the list because pdfcpu keys a destination by title. Opt-in; a plain merge is unchanged.

## Declared

`optionValuesOffered` judges a value against the `options` the data carries; a hand-written datum with none is not
judged and nothing could be echoed. Only the fill path was searched for echoes: pdfcpu's other packages were not.
A value in a path nib or pdfcpu prints elsewhere (a field name chosen by the caller) is the caller's name, not its value.
