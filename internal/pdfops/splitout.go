package pdfops

import "os"

// The two rules a derived part obeys once it stops being bytes and becomes a FILE.
//
// # Why they are here and not at the two doors that need them (ADR-009)
//
// Every multi-file operation in nib writes through one of two writers — `internal/cli`'s
// `writeSplitFiles` and `internal/server`'s `writeSplitParts` — and both had made these two
// decisions for themselves, silently and differently:
//
//   - **Neither checked that a part's name is the document it came from.** Measured through the
//     real command (/pending 569, and /pending 550 before it): `nib split collide/foo1-2.pdf
//     --out-dir collide --ranges 1-2 --prefix foo` replaced the 105,102-byte input with the
//     94,254-byte part, exit 0, nothing printed. The GUI door reaches the same loss by another
//     route — it holds no descriptor, but the open document's file on disk is just as replaceable
//     — measured at 1,239 bytes → 1,076, status 200.
//   - **They disagreed about the mode** (/pending 570): 0600 from the GUI, 0644 from the CLI, for
//     the same operation over the same re-derivable output. Nobody chose that; it fell out of
//     which helper each door happened to call.
//
// ADR-009: a rule holding at more than one call site is written ONCE and every site calls it, and
// the guard asserts routing through the door rather than the text each site prints. Each surface
// words its own refusal — `internal/server/handoff.go` states the repo's reading, *"ADR-009
// unifies the CHECKS; it explicitly does not require every site to print the same sentence"* —
// so this door answers WHICH part collides and says nothing about how to tell the user.
//
// They live beside `SplitPart` because that is the type both doors write, and because the
// alternative — `internal/atomicfile` — is the package of WRITE doors, where a predicate that
// writes nothing would have to be classified as a non-durable door by
// `internal/cli/atomicdurable_test.go` and refused. `pdfops.MissingToolFor` is the same shape
// (ADR-040): a classification the surfaces share, sitting next to the operation it is about.

// SplitPartMode is the permission mode a NEW part is created with, at every door.
//
// # 0644, because a part is a user's document and not one of nib's files
//
// 0600 is nib's habit for what nib owns — the vault, the ceremony mirror, the sidecars — and it is
// the wrong habit for a file the user asked to be written into a folder they named. The repo has
// already decided this once, one door over: `atomicfile.ReplaceDurable` exists BECAUSE forcing
// 0600 was wrong for a user document (/pending 499) — *"saving a user's 0644 original left it
// 0600 — a document shared with a group or served by a local web server stopped being readable the
// moment Nib saved it, with no message"*. A split part is the same kind of file, and choosing 0600
// here would make the GUI the surface that quietly breaks sharing, which it already was.
//
// # The declared exposure, because 0600 is not a silly answer
//
// Splitting a 0600 original yields 0644 parts, so a private document's pages become as readable as
// the destination folder allows. That is a real widening and it is the price of this choice.
// Three things bound it: the folder's own permissions still gate access; the CLI has always
// written 0644, so this is the status quo at one door rather than a new exposure; and the
// destination is a folder the user picked for output.
//
// **Carrying the SOURCE document's mode was considered and refused**, which is the option that
// would have closed the exposure. The GUI door has no source mode to carry for a document that was
// uploaded through the browser rather than opened from disk (`document.path` is empty), so the two
// doors would agree only for some documents — which is /pending 570 itself, rebuilt out of a
// better motive.
//
// Note that every door here applies this with an explicit `Chmod`, so the process umask does not
// narrow it. That is `atomicfile`'s behaviour for every write and is not decided here.
const SplitPartMode os.FileMode = 0o644

// OutputOverwritingSource returns the first path in outs that names the same FILE as src, and
// whether there was one. An empty src (stdin, or a document with no path on disk) collides with
// nothing, and so does an out that does not exist yet.
//
// # Identity, never the path string
//
// `os.SameFile` compares the device and inode (the volume and file index on Windows), so it sees
// through every spelling of one file that a string comparison does not: `dir/../dir/x.pdf`, a
// hard link, and — the one that matters — a SYMLINK in the output folder pointing at the input.
// That last case is not hypothetical: `internal/cli`'s write door resolves a symlink before
// writing (/pending 515), so a link named as a part sends the write to whatever it points at. A
// path-string check is green for it and the input dies anyway;
// `TestASplitSeesThroughASymlinkToItsOwnInput` is that case, and it was red against a tree with
// the containment check alone.
//
// # It takes the whole set, because "before the first write" is the rule
//
// A per-path predicate called inside the write loop would refuse the colliding part after the
// earlier ones had already landed, leaving a half-filled folder under a non-zero exit. Answering
// over every output first makes that structural rather than a convention each caller must keep.
//
// Both stats are taken just before the writes, so a file created in the gap is not seen — the same
// bound `ReplaceDurable` states about its own permission stat. The window is the width of the
// write loop, and the case it would miss (someone links the input into the output folder mid-split)
// is not the one this exists for.
//
// # The two early returns are contract, NOT guards, and that is measured
//
// Both were probed by deleting them and both mutants SURVIVED, and neither is a coverage hole:
// `os.Stat("")` fails, and `os.Stat` on a missing path returns a nil `FileInfo` that `os.SameFile`
// answers false for — measured (`os.SameFile(nil, real) = false`), because `SameFile` type-asserts
// both arguments to `*os.fileStat` and a nil interface fails the assertion. So the comparison below
// already answers "no collision" for both cases without them.
//
// They stay because they state the contract where a caller reads it — an empty src is a document
// with no file on disk, which is a real and ordinary state on both surfaces — and they are marked
// as redundant here so nobody weakens the comparison believing these are what covers it. Said
// rather than left to be rediscovered: a clause a mutation cannot kill looks identical to one
// nothing exercises, and the difference is the whole point.
func OutputOverwritingSource(src string, outs []string) (string, bool) {
	if src == "" { // contract, not a guard — see above
		return "", false
	}
	si, err := os.Stat(src)
	if err != nil {
		// No source to protect: it was never a file (stdin), or it is already gone. Refusing
		// here would turn an unreadable input into a second, unrelated failure. Contract, not
		// a guard — see above.
		return "", false
	}
	for _, out := range outs {
		oi, oerr := os.Stat(out)
		if oerr != nil {
			continue // nothing there yet: writing it cannot destroy anything
		}
		if os.SameFile(si, oi) {
			return out, true
		}
	}
	return "", false
}
