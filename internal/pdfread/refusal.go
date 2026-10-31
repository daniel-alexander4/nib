package pdfread

import (
	"errors"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ErrNodeNamedTwice is the refusal of a document in which one node of a form, a page tree, a structure tree or a
// name or number tree is reached from two places. pdfcpu v0.16.2's validator refuses it (`model/recursion.go`,
// `ErrFormFieldDuplicate` and its four siblings) because each such structure is a TREE and a second naming makes
// the walk multiply; the release before followed it silently (/pending 898, P02.S04, D4).
var ErrNodeNamedTwice = errors.New("it names one node of a list, tree or form from more than one place")

// ErrNestedTooDeep is the refusal of a document nested past pdfcpu v0.16.2's 100 levels (`model/recursion.go:118`:
// forms in forms, patterns in patterns, fields in fields). It is the VALIDATOR's refusal, named; the reference
// door's own is `ErrReferenceDepth`, and the two are kept apart so a test can tell which of them spoke.
var ErrNestedTooDeep = errors.New("it nests one thing inside another more levels deep than nib reads")

// validatorRefusals is how the validator's bounded-walk errors (model/recursion.go) are named. Each is a refusal
// that exists to bound recursion, a cycle or a size, so each defaults to a NAMED refusal, not a carry (D4, the
// security pin): a carry needs its own bounded-walk proof, and the list below has none.
var validatorRefusals = []struct {
	pdfcpu []error
	named  error
}{
	{[]error{model.ErrMaxRecursionDepthExceeded}, ErrNestedTooDeep},
	{[]error{
		model.ErrFormFieldCycle, model.ErrNameTreeCycle, model.ErrNumberTreeCycle, model.ErrPageTreeCycle,
		model.ErrStructureTreeCycle, model.ErrFunctionCycle, model.ErrHalftoneCycle, model.ErrMediaClipCycle,
		model.ErrPatternCycle, model.ErrRenditionCycle, model.ErrTargetCycle,
		// The annotation reply thread, the font's CMap chain and a colour space naming itself are loops of the same
		// kind; they reached the user as pdfcpu's words alone (/pending 919 F12).
		model.ErrAnnotationIRTCycle, model.ErrCMapCycle, model.ErrColorSpaceCycle,
	}, ErrReferenceCycle},
	{[]error{
		model.ErrFormFieldDuplicate, model.ErrNameTreeDuplicate, model.ErrNumberTreeDuplicate,
		model.ErrPageTreeDuplicate, model.ErrStructureTreeDuplicate,
	}, ErrNodeNamedTwice},
}

// StructureRefusals are the sentences of the refusals above and of the reference door's two, the words the
// server's error door recognises to answer 422 with a cause (ADR-072): the fault is the document's, not the
// request's and not the server's.
func StructureRefusals() []string {
	return []string{
		ErrNestedTooDeep.Error(), ErrReferenceCycle.Error(), ErrNodeNamedTwice.Error(),
		ErrReferenceDepth.Error(), ErrReferencePaths.Error(),
	}
}

// refusal is nib's sentence for a validator error, with pdfcpu's own words behind it. errors.Is answers for both.
type refusal struct {
	named, cause error
}

func (e refusal) Error() string   { return e.named.Error() + " (" + elide(e.cause.Error()) + ")" }
func (e refusal) Unwrap() []error { return []error{e.named, e.cause} }

// elide keeps the head and the tail of a long cause. pdfcpu names every level it walked, so a depth refusal is
// a hundred "traverse … obj#N" hops (7 KB measured) ahead of the sentence that says why; the tail is that sentence.
func elide(s string) string {
	const head, tail = 120, 200
	if len(s) <= head+tail+len(" … ") {
		return s
	}
	return s[:head] + " … " + s[len(s)-tail:]
}

// nameValidatorRefusal puts nib's sentence in front of a bounded-walk error from the validator and leaves every
// other error as it was.
func nameValidatorRefusal(err error) error {
	if err == nil {
		return nil
	}
	for _, r := range validatorRefusals {
		for _, p := range r.pdfcpu {
			if errors.Is(err, p) {
				return refusal{named: r.named, cause: err}
			}
		}
	}
	return err
}
