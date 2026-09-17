# docs/red-proofs.md, tier 1: "booklet's folding instruction printed on stdout" (/pending 508,
# v1.133.10)
#
# The defect: `cmdBooklet`'s `fmt.Fprintln(os.Stderr, …)` put back to `fmt.Println`. The sentence
# then lands on stdout AFTER the PDF has been written there, so `nib booklet in.pdf -o - > out.pdf`
# appends it to the document's own bytes — the one command whose output carries a printing
# instruction is the one that corrupts the file it describes.
#
# Both arms go red on it, and that is the point: the instruction is not decoration (nothing in the
# PDF can say which way a printer will flip), so a fix that simply dropped it would satisfy the
# stdout arm alone.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli -run TestBookletSendsItsInstructionToTheReaderAndTheDocumentToStdout"
EXPECT="stdout carries the folding instruction"
