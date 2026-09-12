package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The optional-content configuration a stamp leaves behind — `/pending 473`.
//
// # What was wrong
//
// Every stamping operation puts its output in an optional-content group, and pdfcpu writes the
// catalog's default configuration dictionary (`/OCProperties /D`) with an `/AS` array and no
// `/Name`. PDF/UA forbids both, by name:
//
//   - ua1 **7.10 t1** — *"Each optional content configuration dictionary … shall contain the Name
//     key"* → *"Missing or empty Name entry"*.
//   - ua1 **7.10 t2** — *"The AS key shall not appear in any optional content configuration
//     dictionary"* → *"AS key is present"*.
//
// Measured 2026-09-11: a stamp adds exactly those two clauses to a document that passed both, and
// it does so on all five stamping doors.
//
// # Why removing `/AS` is safe, which `/pending 473` required be measured and not assumed
//
// The written `/D` is `[ON Order RBGroups AS]`: `/ON` names the group explicitly, there is no
// `/OFF` and no `/BaseState`, and the three `/AS` entries each set that same group ON for View,
// Print and Export. So `/AS` is telling a reader to apply usage settings that already agree with
// `/ON` — it is redundant with the key beside it, not load-bearing. A configuration with no
// `/BaseState` defaults to ON, and `/ON` says ON, so the group is visible either way.
//
// The OCG itself is untouched: it already carries `/Name (Watermark)` and a `/Usage` dict, and
// neither clause is about it.

// honestOptionalContent strips `/AS` from every optional-content configuration dictionary and gives
// each one a `/Name`, so a stamp costs the document neither ua1 clause. See the file header for the
// measurement that made removing `/AS` safe.
func honestOptionalContent(pdf []byte) []byte {
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		ocp, oerr := ctx.DereferenceDict(root["OCProperties"])
		if oerr != nil || ocp == nil {
			return nil // nothing stamped an optional-content group; nothing to correct
		}
		for _, key := range configDictKeys(ctx, ocp) {
			d, derr := ctx.DereferenceDict(key)
			if derr != nil || d == nil {
				continue
			}
			delete(d, "AS")
			if _, has := d["Name"]; !has {
				d["Name"] = types.StringLiteral("Default")
			}
		}
		return nil
	})
	if err != nil {
		// A stamp that could not have its configuration corrected is still a stamp. Failing here
		// would cost the user the operation to gain a clause, which is the trade P01 already
		// refused in the other direction.
		return pdf
	}
	return out
}

// configDictKeys returns every optional-content CONFIGURATION dictionary in the catalog — the
// default one under `/D` and each alternate under `/Configs`.
//
// Both clauses say "each … configuration dictionary", so a door that corrected only `/D` would be
// a rule reaching one site of two (ADR-009). pdfcpu writes no `/Configs` today; the loop is here
// because the clause's population is not "the one pdfcpu happens to write".
func configDictKeys(ctx *model.Context, ocp types.Dict) []types.Object {
	out := []types.Object{}
	if d, ok := ocp["D"]; ok {
		out = append(out, d)
	}
	if arr, err := ctx.DereferenceArray(ocp["Configs"]); err == nil {
		out = append(out, arr...)
	}
	return out
}
