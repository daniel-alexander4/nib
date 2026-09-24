package uacheck

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The file-specification door — `PLAN-ua-coverage.md` P06.S02.
//
// # The population is NOT the embedded-files name tree, and every holder was measured
//
// `7.11 t1`'s object is `CosFileSpecification`, and the obvious reading — "the specs in
// `/Names /EmbeddedFiles`" — is wrong in a way that produces a silent Pass over a file nobody looked
// at, which is this phase's recurring defect. Measured against veraPDF 1.30.2, each on a hand-built
// PDF carrying exactly ONE defective spec (`/EF` and `/F`, no `/UF`) reachable only through the holder
// under test, and each with the stimulus asserted — the spec's own filename present in the bytes:
//
//	/Names /EmbeddedFiles name tree      failed (2 checks for the one object)
//	a FileAttachment annotation's /FS    failed
//	the catalog's /AF                    failed
//	a page's /AF                         failed
//	a structure element's /AF            failed
//	a form XObject's /AF                 failed
//
// veraPDF's own `FileSpecificationKeysHelper` explains why the list is open-ended rather than short:
// it registers `/AF` keys while walking the catalog, the structure tree (recursively, including each
// element's annotations), every page, every annotation, all three appearance states, and then resources
// down through form and image XObjects, ExtGState, Patterns and Fonts, plus an image's `/Mask` and
// `/Alternates`. That is the whole reachable content graph.
//
// # So the door does not enumerate holders — enumerating them is how one gets missed
//
// It descends the object graph and collects every dictionary that is `/Type /Filespec` OR carries an
// `/EF`. Three measurements make that the right shape rather than a lazy one:
//
//   - **`/Type` alone is not the test.** A dictionary with `/EF` and NO `/Type /Filespec` is a
//     subject — measured, veraPDF fails it. Keying on `/Type` would have passed it.
//   - **`/EF` alone is not the test either.** A typed spec with NO embedded file is still a SUBJECT:
//     it satisfies the profile's first disjunct and veraPDF reports it as a PASSING check. Collecting
//     only `/EF` specs made nib answer NotApplicable where veraPDF answered Pass — caught by law 5 on
//     three media-clip documents, whose clip `/D` is exactly that shape.
//   - **A DIRECT spec is a subject.** A `/FS` written as a direct dictionary inside an annotation,
//     rather than as its own object, is graded — measured. A walk over the object table alone misses
//     it, so the descent goes through arrays and nested dictionaries too.
//
// **Only a spec with `/EF` can FAIL**, because the profile's test is
// `containsEF == false || (F != null && F != '' && UF != null && UF != '')`. So the rule skips the
// `/EF`-less specs when looking for a defect, while the door still counts them — the difference
// between "no subject" and "a check that passed", which this package's oracle grades strictly.

// fileSpec is one file specification, with where it was found.
type fileSpec struct {
	dict types.Dict
	// where names the object the specification is, or the object it was found inside when it is
	// written as a direct dictionary.
	where string
}

// fileSpecs is every file specification in the document — the ONE door (ADR-009). specsErr is why the
// population may be short, when it may be.
//
// # Indirect references are NOT followed, and that is what makes the walk safe
//
// Every object in the table is visited by the loop below, so following a reference from inside one
// object would only reach something the loop reaches anyway — while making three defects possible,
// all of them measured on the first version of this walk:
//
//   - **An exponential blow-up.** Arrays carried no visited set and spent no budget, so a chain of
//     objects each holding `[prev prev]` was re-walked 2^n times: n=18 took 25 ms and n=21 took 198 ms
//     from a 3 KB file, with the budget never firing. A PDF the user opened is untrusted input.
//   - **A refusal on an ordinary document.** Depth was counted from each root object THROUGH
//     references, so any 64-long chain of linked dictionaries exhausted the bound — and an outline is
//     exactly that chain, so a document with 64 bookmarks made the whole clause `CannotCheck`.
//   - **A misreported location.** `where` was fixed at the root object, so an indirect specification
//     first reached through some lower-numbered holder was labelled with the holder rather than itself.
//
// Not following them fixes all three at once: each object is walked once, its INLINE content is a
// tree rather than a graph, and a specification that is its own object is reported as that object.
func (d *Document) fileSpecs() ([]fileSpec, string) {
	if d.specsDone {
		return d.specList, d.specsErr
	}
	d.specsDone = true
	budget := maxSpecNodes

	var descend func(o types.Object, from string, depth int)
	descend = func(o types.Object, from string, depth int) {
		if depth > maxWalkDepth {
			if d.specsErr == "" {
				d.specsErr = fmt.Sprintf("an object nests more than %d levels deep INSIDE ITSELF and "+
					"nib stopped reading there, so any file specification below was never seen", maxWalkDepth)
			}
			return
		}
		// **Every node spends budget, not only dictionaries.** The first version decremented in the
		// dictionary branch alone, so arrays and scalars were free and the ceiling bounded nothing.
		if budget <= 0 {
			if d.specsErr == "" {
				d.specsErr = fmt.Sprintf("the document holds more than %d objects and values, and nib "+
					"stopped reading there, so any file specification beyond that was never seen", maxSpecNodes)
			}
			return
		}
		budget--
		switch v := o.(type) {
		case types.StreamDict:
			// **A stream dictionary is a dictionary and must be walked as one.** `types.StreamDict`
			// EMBEDS `types.Dict` rather than being one, so it matches neither case below — and a form
			// XObject is a stream, so an `/AF` on one was invisible. That is a holder this file's own
			// table records veraPDF as failing, missed by TYPE after the holder list was measured.
			// `rules_language.go`'s whole-table walk has always had this case.
			descend(v.Dict, from, depth+1)
		case types.Dict:
			if isFileSpec(d, v) {
				d.specList = append(d.specList, fileSpec{dict: v, where: from})
			}
			// **In key order, because a map's iteration order is random.** Sorting the object numbers
			// alone left this loop random, and a document with two defective specifications under one
			// holder reported two different reasons across runs of the same binary on the same bytes:
			// measured, 35 runs said one and 5 said the other.
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				// An embedded-file STREAM's own dictionary is not a file specification, and a
				// specification nested below an `/EF` is unreachable here by construction. No real
				// shape writes one; it is skipped to keep the walk off the file's payload.
				if k == "EF" {
					continue
				}
				descend(v[k], from, depth+1)
			}
		case types.Array:
			for _, e := range v {
				descend(e, from, depth+1)
			}
		}
	}

	// **In object-number order, so the population — and therefore which failure a document reports —
	// is the same on every run.**
	nrs := make([]int, 0, len(d.Ctx.XRefTable.Table))
	for nr := range d.Ctx.XRefTable.Table {
		nrs = append(nrs, nr)
	}
	sort.Ints(nrs)
	for _, nr := range nrs {
		o, err := d.Ctx.Dereference(types.IndirectRef{ObjectNumber: types.Integer(nr)})
		if err != nil {
			// A single unreadable object is recorded, not fatal: the rest of the population is still
			// worth grading, and the refusal says the answer may be short.
			//
			// **A DECLARED unreached branch.** Probed: removing this refusal leaves the package green,
			// because pdfcpu does not currently error for an object number present in the table — a
			// FREE entry dereferences to `(nil, nil)`, which the walk simply ignores. It is kept
			// because the day pdfcpu starts returning an error here, the alternative is a population
			// that shortens in silence, which is the defect this phase has found twice.
			if d.specsErr == "" {
				d.specsErr = fmt.Sprintf("object %d could not be read, so a file specification it "+
					"holds or reaches was never seen", nr)
			}
			continue
		}
		descend(o, fmt.Sprintf("object %d", nr), 0)
	}
	return d.specList, d.specsErr
}

// resolve dereferences an object, returning nil where it cannot — a reference nib cannot follow is a
// branch it did not read rather than an error to return.
func (d *Document) resolve(o types.Object) types.Object {
	r, err := d.Ctx.Dereference(o)
	if err != nil {
		return nil
	}
	return r
}

// isFileSpec reports whether a dictionary is a file specification.
//
// **`/Type /Filespec` OR an `/EF`, and both halves are measured** (P06.S02): an UNTYPED dictionary
// carrying an embedded file is graded by veraPDF exactly as a typed one is, and a TYPED specification
// with no embedded file is still a SUBJECT — it satisfies the profile's first disjunct and is reported
// as a PASSING check rather than an absent one.
func isFileSpec(d *Document, v types.Dict) bool {
	if _, hasEF := v["EF"]; hasEF {
		return true
	}
	return d.name(v["Type"]) == "Filespec"
}

// maxSpecNodes bounds how many objects and values the walk will visit in total. The document's object
// count is the file's choice, and this is the only walk in the package that starts from the whole
// table rather than from a named root, so it carries the package's usual kind of ceiling: a document
// past it is one nib declines to finish rather than one it grades on a part.
const maxSpecNodes = 2000000
