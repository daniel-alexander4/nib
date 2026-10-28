package pdfread

import (
	"errors"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ErrNodeNamedTwice is the refusal of a document in which one node of a form, a page tree, a structure tree or a
// name or number tree is reached from two places. pdfcpu v0.16.2's validator refuses it (`model/recursion.go`,
// `ErrFormFieldDuplicate` and its four siblings) because each such structure is a TREE and a second naming makes
// the walk multiply; v0.13.0 followed it silently (/pending 898, P02.S04, D4).
var ErrNodeNamedTwice = errors.New("it names one node of a list, tree or form from more than one place")

// validatorRefusals is how the validator's bounded-walk errors (model/recursion.go) are named. Each is a refusal
// that exists to bound recursion, a cycle or a size, so each defaults to a NAMED refusal, not a carry (D4, the
// security pin): a carry needs its own bounded-walk proof, and the list below has none.
var validatorRefusals = []struct {
	pdfcpu []error
	named  error
}{
	{[]error{model.ErrMaxRecursionDepthExceeded}, ErrReferenceDepth},
	{[]error{
		model.ErrFormFieldCycle, model.ErrNameTreeCycle, model.ErrNumberTreeCycle, model.ErrPageTreeCycle,
		model.ErrStructureTreeCycle, model.ErrFunctionCycle, model.ErrHalftoneCycle, model.ErrMediaClipCycle,
		model.ErrPatternCycle, model.ErrRenditionCycle, model.ErrTargetCycle,
	}, ErrReferenceCycle},
	{[]error{
		model.ErrFormFieldDuplicate, model.ErrNameTreeDuplicate, model.ErrNumberTreeDuplicate,
		model.ErrPageTreeDuplicate, model.ErrStructureTreeDuplicate,
	}, ErrNodeNamedTwice},
}

// refusal is nib's sentence for a validator error, with pdfcpu's own words behind it. errors.Is answers for both.
type refusal struct {
	named, cause error
}

func (e refusal) Error() string   { return e.named.Error() + " (" + e.cause.Error() + ")" }
func (e refusal) Unwrap() []error { return []error{e.named, e.cause} }

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
