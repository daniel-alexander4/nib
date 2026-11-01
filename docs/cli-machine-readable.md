# Calling nib from a program

For a program that runs `nib` as a child process (RelaxTax is the first). The contract is **ADR-183**; the shapes below are locked by
`internal/cli/machinereadable_test.go`, which also fails if a key named here is missing from this file. Read `nib version --json`
first: `api` is an integer that is raised only for an **incompatible** change to anything on this page (a rename, a removal, a changed
meaning, a new exit code for an old condition) and never for an addition, so a program may pin it and ignore keys it does not know.

## Calling it safely

- Set **`NIB_HEADLESS=1`**. Any first argument that is not a command is then `unknown command` (exit 1), and no argument is a usage
  error; the desktop app, its server and its browser are never started. `version` and `help` still answer.
- No command writes pdfcpu's configuration directory (`$XDG_CONFIG_HOME/pdfcpu`, or `~/.config/pdfcpu`).
- No refusal repeats a value you supplied to a form field. A field's name, a path and the options the form offers may appear in a message.
- A command that produces a PDF writes it to `-o FILE` (staged in a `.nib-*.tmp` beside FILE until it is renamed into place) or, with
  `-o -`, to stdout, where nothing is staged. `IN` and `fill --data` accept `-` for stdin (not both at once).
- `fill IN --data - --format json|xfdf -o -` is the call that touches no file: `cat answers.json | nib fill form.pdf --data - --format json -o -`.
  The JSON is pdfcpu's shape — `{"forms":[{"textfield":[{"name":"<full path>","value":"…"}]}]}`; a field two parents share a leaf name
  with is addressed by its full path.
- A filled form has no `/XFA`. nib does not add or remove `/NeedAppearances`; the 2025 Form 1040 has none.

## Exit codes

`0` success. `1` every usage, input or I/O error. `2` a verification that fails: `verify` (a signature is invalid or absent) and `ua`
(a clause fails or could not be checked); `timestamp --verify` also answers 2 for a proof that is still pending; `discover` and
`rendezvous` use 2 for a usage error because 1 is their verdict. Messages go to stderr and are for people: parse the exit code, not the text.

## `nib version --json`

`{"version": "2.19.0", "api": 1, "commands": ["attachments", …]}` — `commands` is every verb the binary has, sorted.

## `nib fields IN --json`

A JSON array, one object per form field, in the document's order (`[]`, never `null`, when there are none). Keys: `name` (the full dotted path), `type` (`text`, `checkbox`, `radio`,
`pushbutton`, `combobox`, `listbox`, `signature` or `unknown`), `fillable` (bool), `flags` (the PDF `/Ff` integer), `onPage` (bool: some page draws it),
`widgets` (array of `{page, rect}`), and, only when the form has them, `tooltip` and `label`. `rect` is `[llx, lly, urx, ury]` in the page's own
user space — points, origin bottom-left, `/Rotate` not applied — which is **not** the origin `text` uses.

## `nib text IN --json`

A JSON array, one object per page: `page` (from 1), `width`, `height` (points, the page as shown), `runs`, and, only when set, `noText` and
`unread`. Each run: `text`, `rect` = `[left, top, right, bottom]` in points on the page as shown with the **origin at the top left**, `size`,
and `hidden` (true for an OCR layer) only when true. Runs are in the order the page draws them, not reading order.

## `nib info IN --json`

A JSON array, one object per page: `page`, `width`, `height`, `rotate`, `mediaBox`, `cropBox` (each box `[llx, lly, urx, ury]`).

## `nib images IN --out-dir DIR [--json]`

Writes each embedded picture of `IN` (`-` for stdin) into `DIR` (created `0700` if missing, files `0600`, never overwriting: a name that
already exists is exit 1 and nothing is written) as `page-NNN-img-NN.EXT`. A JPEG stream comes out as the **same JPEG** (`.jpg`, byte for
byte); any other picture is decoded and re-encoded as `.png` (or `.tif`), with the same pixels. `--json` prints an array, one object per
picture, in page order: `page` and `index` (from 1; the picture's place among that page's), `width`, `height` (pixels), `filter` (the
stream's own filter pipeline in the PDF, space-separated, e.g. `FlateDecode` or `DCTDecode`), `bytes` (the file's length) and `file`
(its name in `DIR`). A picture drawn on several pages is listed once, on the first. A document with none prints `[]` and creates no folder.
