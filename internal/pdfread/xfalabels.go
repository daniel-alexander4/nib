package pdfread

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// XFALabels reads the label an XFA form's template gives each of its fields (`/pending 924`): the text of the
// field's `<assist><speak>` — "6b. Taxable amount." — else its `<assist><toolTip>`, else the text of its `<caption>`. IRS forms carry no `/TU`; this is
// where their printed line labels are.
//
// The key is the field's XFA name path with each part's index among its same-named siblings — `topmostSubform[0].Page1[0].f1_69[0]`, the AcroForm's fully qualified name — over the enclosing `subform`s and `exclGroup`s, joined with ".". A path
// the template gives two different labels (a repeated subform) has NO entry: a label is never a guess between two.
// A document with no XFA, or whose packet does not parse, answers an empty map and no error — a label is a
// convenience over the fields, never a condition of listing them.
//
// One unvalidated parse (`ReadXFA`'s), each stream a COPY decoded, the total bounded by `maxXFAPacketScan`
// (`xfaAsksForDynamicRender`'s reading and bound).
func XFALabels(pdf []byte) (labels map[string]string, err error) {
	defer fault.Catch(&err)
	labels = map[string]string{}
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil || root == nil {
		return labels, nil
	}
	o, err := ctx.Dereference(root["AcroForm"])
	form, ok := o.(types.Dict)
	if err != nil || !ok {
		return labels, nil
	}
	packet, err := ctx.Dereference(form["XFA"])
	if err != nil || packet == nil {
		return labels, nil
	}
	var docs [][]byte
	left := maxXFAPacketScan
	decode := func(e types.Object) []byte {
		r, err := ctx.Dereference(e)
		if err != nil {
			return nil
		}
		sd, ok := r.(types.StreamDict)
		if !ok || len(sd.Raw) > left {
			return nil
		}
		cp := sd
		cp.Content = nil
		if cp.Decode() != nil || len(cp.Content) > left {
			return nil
		}
		left -= len(cp.Content)
		return cp.Content
	}
	switch v := packet.(type) {
	case types.StreamDict: // the whole packet as one XDP
		if b := decode(v); b != nil {
			docs = append(docs, b)
		}
	case types.Array: // name, stream pairs; only the template names fields
		for i := 0; i+1 < len(v); i += 2 {
			n, err := ctx.Dereference(v[i])
			if err != nil {
				continue
			}
			if s, ok := n.(types.StringLiteral); ok && s.Value() != "template" {
				continue
			}
			if b := decode(v[i+1]); b != nil {
				docs = append(docs, b)
			}
		}
	}
	seen := map[string]string{}
	ambiguous := map[string]bool{}
	for _, d := range docs {
		templateLabels(d, seen, ambiguous)
	}
	for p, l := range seen {
		if l != "" && !ambiguous[p] {
			labels[p] = l
		}
	}
	return labels, nil
}

type xfaFrame struct {
	local, name        string
	speak, tip, capt   string
	inSpeak, inTip     bool
	inCapt             bool
	idx                int            // among same-named siblings of its nearest named ancestor
	counts             map[string]int // children seen by name, for the siblings' indices
	speakSeen, tipSeen bool
}

// templateLabels walks one XFA document and records, into seen, each field's or exclusive group's label by name
// path; a path met twice with different labels goes into ambiguous. Only elements inside `<template>` count: the
// datasets packet may name an element `field`.
func templateLabels(doc []byte, seen map[string]string, ambiguous map[string]bool) {
	dec := xml.NewDecoder(bytes.NewReader(doc))
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	var stack []*xfaFrame
	inTemplate := 0
	rootCounts := map[string]int{}
	pathOf := func(upTo int) string {
		var parts []string
		for _, f := range stack[:upTo+1] {
			if f.name != "" && (f.local == "subform" || f.local == "exclGroup" || f.local == "field") {
				parts = append(parts, f.name+"["+strconv.Itoa(f.idx)+"]")
			}
		}
		return strings.Join(parts, ".")
	}
	note := func(path, label string) {
		if path == "" {
			return
		}
		if old, ok := seen[path]; ok && old != label {
			ambiguous[path] = true
		}
		seen[path] = label
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			f := &xfaFrame{local: t.Name.Local}
			for _, a := range t.Attr {
				if a.Name.Local == "name" {
					f.name = a.Value
				}
			}
			if f.local == "template" {
				inTemplate++
			}
			if f.name != "" && (f.local == "subform" || f.local == "exclGroup" || f.local == "field") {
				// an unnamed subform is transparent in a name path: its children count in the nearest named frame
				owner := rootCounts
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i].name != "" && stack[i].counts != nil {
						owner = stack[i].counts
						break
					}
				}
				f.idx = owner[f.name]
				owner[f.name]++
				f.counts = map[string]int{}
			}
			stack = append(stack, f)
			n := len(stack)
			if inTemplate > 0 && n >= 3 && stack[n-2].local == "assist" {
				switch f.local {
				case "speak":
					f.inSpeak = true
				case "toolTip":
					f.inTip = true
				}
			}
			if inTemplate > 0 && n >= 4 && f.local == "text" && stack[n-2].local == "value" && stack[n-3].local == "caption" {
				f.inCapt = true
			}
		case xml.CharData:
			if n := len(stack); n > 0 {
				f := stack[n-1]
				if f.inCapt {
					stack[n-4].capt += string(t)
				} else if f.inSpeak || f.inTip {
					// belongs to the labelled element two frames up
					owner := stack[n-3]
					if f.inSpeak {
						owner.speak += string(t)
					} else {
						owner.tip += string(t)
					}
				}
			}
		case xml.EndElement:
			n := len(stack)
			if n == 0 {
				continue
			}
			f := stack[n-1]
			if f.local == "field" || f.local == "exclGroup" {
				label := strings.Join(strings.Fields(f.speak), " ")
				if label == "" {
					label = strings.Join(strings.Fields(f.tip), " ")
				}
				if label == "" {
					label = strings.Join(strings.Fields(f.capt), " ") // a check box's own printed caption
				}
				if inTemplate > 0 {
					note(pathOf(n-1), label)
				}
			}
			if f.local == "template" {
				inTemplate--
			}
			stack = stack[:n-1]
		}
	}
}
