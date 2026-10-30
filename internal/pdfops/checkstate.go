package pdfops

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Which box of a check box answered — the one answer (ADR-009, /pending 922).
//
// pdfcpu reads a check box as a boolean: its export writes `Yes` whenever `/V` is anything but `Off`
// (`form/form.go:493`) and its fill turns a boolean into `/V` (`form/fill.go:693-727`). A Yes/No pair —
// one Btn field, no radio flag, two widgets whose appearances carry DIFFERENT on-state names — is
// therefore exported as "ticked" whichever box was, an edit from No to Yes is a no-op (the requested
// boolean equals the held one), and a "true" onto an unanswered pair ticks BOTH boxes. A single box
// whose on-state is `Ja` is exported as `Yes` for the same reason.
//
// So the NAME of the on-state travels beside pdfcpu's boolean: out through the three exports, in through
// the three fills, read here and written by `applyCheckPicks`. The boolean stays pdfcpu's; a field with one
// on-state keeps every behaviour it had.

// checkState is one check box field: the on-state names its widgets draw and the one its `/V` holds.
type checkState struct {
	States []string // on-state names (the keys of each widget's /AP /N but Off), distinct, in widget order
	Answer string   // the field's /V name; "" for no answer or Off
}

// pair reports a field whose boxes disagree on the on-state: a bare "ticked" cannot say which.
func (c checkState) pair() bool { return len(c.States) > 1 }

// onStates are the on-state names widget draws.
func onStates(xt *model.XRefTable, widget types.Dict) []string {
	n := derefDict(xt, derefDict(xt, widget["AP"])["N"])
	var out []string
	for k := range n {
		if k != "Off" {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// checkWidgets are a field's widgets: its kids, or the field itself where it is its own widget.
func checkWidgets(xt *model.XRefTable, f formField) []types.Dict {
	var out []types.Dict
	for _, k := range derefArray(xt, f.dict["Kids"]) {
		if kd := derefDict(xt, k); kd != nil {
			out = append(out, kd)
		}
	}
	if len(out) == 0 {
		out = append(out, f.dict)
	}
	return out
}

// checkStatesOf reads every fillable check box on a page, by the name a fill matches it by.
func checkStatesOf(xt *model.XRefTable) map[string]checkState {
	fields, err := formFields(xt)
	if err != nil {
		return nil
	}
	out := map[string]checkState{}
	for _, f := range fields {
		if f.kind != "Btn" || buttonKind(f.flags) != "check" || !f.onPage || f.name == "" {
			continue
		}
		var cs checkState
		for _, w := range checkWidgets(xt, f) {
			for _, s := range onStates(xt, w) {
				if !slices.Contains(cs.States, s) {
					cs.States = append(cs.States, s)
				}
			}
		}
		if n, ok := resolved(xt, f.dict["V"]).(types.Name); ok && n.Value() != "Off" {
			cs.Answer = n.Value()
		}
		out[f.name] = cs
	}
	return out
}

// CheckStates reads pdf's check boxes (checkStatesOf) for the exports and fills.
func CheckStates(pdf []byte) (map[string]checkState, error) {
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.EXPORTFORMFIELDS // the reading the export, and so the fill's names, are made over
	ctx, err := pdfread.ReadOptimized(pdf, conf)
	if err != nil {
		return nil, err
	}
	return checkStatesOf(ctx.XRefTable), nil
}

// answerOf is the name an export writes for a check box pdfcpu reported as on: the field's own on-state, `Yes`
// where the file names none.
func answerOf(states map[string]checkState, name string) string {
	if a := states[name].Answer; a != "" {
		return a
	}
	return "Yes"
}

// withCheckStates adds `"state": "<on-state>"` to the JSON of each answered check box with several on-states —
// the one place pdfcpu's boolean cannot say which box. Any other export is returned as pdfcpu wrote it.
func withCheckStates(out []byte, states map[string]checkState) ([]byte, error) {
	any := false
	for _, s := range states {
		any = any || (s.pair() && s.Answer != "")
	}
	if !any {
		return out, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(out, &top); err != nil {
		return nil, err
	}
	var forms []map[string]json.RawMessage
	if err := json.Unmarshal(top["forms"], &forms); err != nil {
		return nil, err
	}
	for _, f := range forms {
		raw, ok := f["checkbox"]
		if !ok {
			continue
		}
		var boxes []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &boxes); err != nil {
			return nil, err
		}
		for _, b := range boxes {
			var name string
			_ = json.Unmarshal(b["name"], &name)
			if s := states[name]; s.pair() && s.Answer != "" {
				b["state"], _ = json.Marshal(s.Answer)
			}
		}
		f["checkbox"], _ = json.Marshal(boxes)
	}
	top["forms"], _ = json.Marshal(forms)
	return json.MarshalIndent(top, "", "\t")
}

// chooseCheckState reads what a fill was given for a check box with several on-states. A value that is one of the
// on-states' names (any case) selects it; an off word clears the box; and a bare "on" word keeps an answer the field
// already holds. pick is "" unless the answer CHANGES — a pick equal to the held answer is nothing to do, so the
// fill's "nothing changed" still reads as it did. A bare "on" onto an unanswered field is refused: pdfcpu would tick
// every box.
func chooseCheckState(name, raw string, cs checkState, checked func(string) bool) (pick string, clear bool, err error) {
	for _, s := range cs.States {
		if strings.EqualFold(strings.TrimSpace(raw), s) {
			if s == cs.Answer {
				return "", false, nil
			}
			return s, false, nil
		}
	}
	if !checked(raw) {
		return "", true, nil
	}
	if cs.Answer != "" {
		return "", false, nil
	}
	return "", false, fmt.Errorf("check box %q has several on-states (%s) — name the one that is ticked; a bare yes or true cannot say which box", name, strings.Join(cs.States, ", "))
}

// applyCheckChoices lays the choices over the form group a fill is about to be given: a field with several on-states
// takes pdfcpu the boolean it already holds (so pdfcpu does nothing to it) and the pick goes into picks to be
// written by applyCheckPicks. rawFor says what the data gave for a box, and whether it gave anything.
func applyCheckChoices(fg *form.FormGroup, states map[string]checkState, checked func(string) bool, rawFor func(*form.CheckBox) (string, bool)) (map[string]string, error) {
	if len(fg.Forms) == 0 {
		return nil, nil
	}
	picks := map[string]string{}
	for _, x := range fg.Forms[0].CheckBoxes {
		cs, ok := states[x.Name]
		if !ok || !cs.pair() {
			continue
		}
		raw, given := rawFor(x)
		if !given {
			continue
		}
		pick, clear, err := chooseCheckState(x.Name, raw, cs, checked)
		if err != nil {
			return nil, err
		}
		x.Value = cs.Answer != "" // what the document holds, so pdfcpu changes nothing …
		switch {
		case clear:
			x.Value = false // … unless the box is to be cleared, which pdfcpu does correctly
		case pick != "":
			picks[x.Name] = pick // … and nib writes the pick
		}
	}
	return picks, nil
}

// applyCheckPicks writes each pick: the field's /V, and on each widget the /AS that is the pick where it draws
// that state and Off where it does not.
func applyCheckPicks(xt *model.XRefTable, picks map[string]string) {
	if len(picks) == 0 {
		return
	}
	fields, err := formFields(xt)
	if err != nil {
		return
	}
	for _, f := range fields {
		pick, ok := picks[f.name]
		if !ok {
			continue
		}
		f.dict["V"] = types.Name(pick)
		for _, w := range checkWidgets(xt, f) {
			if slices.Contains(onStates(xt, w), pick) {
				w["AS"] = types.Name(pick)
			} else if _, has := w.Find("AS"); has {
				w["AS"] = types.Name("Off")
			}
		}
	}
}
