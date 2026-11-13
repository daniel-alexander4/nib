# ADR-225 — `nib images --out-dir -` writes the pictures as one framed stream, and `version --json` says it can

Status: accepted, 2026-11-12 (/pending 983; RelaxTax's /pending 78). Extends ADR-183.

## Context

RelaxTax reads scanned W-2s by running the installed nib (`nib images IN --out-dir DIR`) and reading each picture
back. The pictures are a tax return's pages, so the only shape `images` had — 0600 files in a 0700 folder — left them
on disk for the length of the call and, after a crash, until RelaxTax's next start-up sweep. A program that wants the
bytes and not the files needs them on stdout, in a framing it can parse strictly, and a way to ask an installed nib
whether it has that before calling it.

## Decision

1. **`--out-dir -` is stdout**, nib's own spelling for it (`-o -`). No second flag. The stream, version 1:

       {"stream":"nib-images","version":1,"count":N}\n
       then N times:  {"page":P,"index":I,"width":W,"height":H,"filter":"F","format":"EXT","bytes":L}\n  then exactly L bytes
       then end of file

   Each header is one line of JSON (`encoding/json` escapes every control character, so it holds no newline). The
   picture keys are `--json`'s row with `file` replaced by `format` — the extension a file would have had (`png`,
   `jpg`, `tif`, `jpx`, `jbig2`). `count` is in the first line so a reader can tell a short stream from a whole one;
   nothing follows the last picture. A document with no pictures is the first line with `count` 0.
2. **Nothing reaches stdout unless everything does.** Every picture is extracted into memory before the first byte is
   written (`pdfops.ExtractImageFiles` already returns them all), so a refusal is exit 1 with stderr and an empty
   stdout. A failed write (a reader that went away) is exit 1. Nothing is written to disk: the input is read whole
   from the path or stdin, and `images` already runs with pdfcpu's configuration disabled (ADR-183).
3. **`--json` with `--out-dir -` is refused** (exit 1): the stream's headers are the rows, and a second JSON document on
   the same stdout could not be told from them. **Onto a terminal it is refused**, as `writeOutput` refuses a PDF.
4. **`nib version --json` gains `"capabilities"`**, a sorted list of strings, and `images-stdout` is its first. A
   list rather than RelaxTax's suggested `"images_stdout": true`: the next capability is a new string, not a new key
   for every program to learn. An addition, so `api` stays 1 (ADR-183 raises it only for an incompatible change). A
   capability is listed only when the binary has it; `TestEveryCapabilityIsDocumented` holds the list to the doc.
5. **One door says "stdout is a terminal"** (`stdoutIsTerminal`), for `writeOutput`'s PDF and this stream (ADR-009).

## Consequences

- `--out-dir -` used to make a folder literally named `-`. That reading is gone; nothing in nib or RelaxTax used it.
- The stream holds every picture in memory twice at most (the extraction and the write buffer); that is what the
  folder shape already did.
- Declared: there is no per-picture streaming. A document with a very large picture is held whole, as before.
