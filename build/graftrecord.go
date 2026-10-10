//go:build ignore

// graftrecord attaches a ceremony record to a document that was never convened, in the one shape a
// convened document carries it — for `build/pairrepro.sh`'s decoy step, which offers a party a
// DIFFERENT document carrying a genuine ceremony's record and requires the arrival gate to refuse it.
//
// Usage: go run build/graftrecord.go <in.pdf> <record.json> <out.pdf>
//
// It exists because the harness built its decoy with `nib attachments --add --name nib-ceremony.json`,
// and that door has refused the record's name since /pending 822 (v1.182, 2026-10-03): a file a user
// attaches under it would leave the content digest. The refusal is right, and it turned the decoy step
// red at its own setup — so tier 4 with four parties stopped before the substitution it exists to
// try, and every later step with it. An attacker does not ask Nib's attach door; this writes the
// record through `pdfops.EmbedCeremonyRecord`, the writer convening itself uses, which is the shape
// the gate must refuse on a document the record does not describe.
//
// Not a `nib` subcommand, and it must not become one: a shipped command that grafts a record onto an
// arbitrary document is the attack, packaged. `//go:build ignore` keeps it out of `go build ./...`.
package main

import (
	"fmt"
	"os"

	"nib/internal/pdfops"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: go run build/graftrecord.go <in.pdf> <record.json> <out.pdf>")
		os.Exit(2)
	}
	die := func(what string, err error) {
		fmt.Fprintf(os.Stderr, "%s: %v\n", what, err)
		os.Exit(1)
	}
	pdf, err := os.ReadFile(os.Args[1])
	if err != nil {
		die("reading the document", err)
	}
	rec, err := os.ReadFile(os.Args[2])
	if err != nil {
		die("reading the record", err)
	}
	out, err := pdfops.EmbedCeremonyRecord(pdf, rec)
	if err != nil {
		die("attaching the record", err)
	}
	if err := os.WriteFile(os.Args[3], out, 0o644); err != nil {
		die("writing "+os.Args[3], err)
	}
}
