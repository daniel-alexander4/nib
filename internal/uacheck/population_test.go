package uacheck

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

// TestNoPopulationReadsAsWholeWhenItsBuildNeverReturned — `/pending 730`: five populations set their done flag BEFORE
// building (`annots`, `fileSpecs`, `mediaClips`, `trueTypeFonts`, `structNodes`), the shape `contentEvents` was fixed
// for, so a panic mid-build (recovered per rule by `runOne`) left every later rule reading a partial population with no
// error. No input panics them today, so the state is set by hand: started, not finished, no error recorded. The table is
// held against the Document's `population` fields, so a seventh population needs a row.
func TestNoPopulationReadsAsWholeWhenItsBuildNeverReturned(t *testing.T) {
	rows := map[string]func(d *Document) string{
		"contentBuild": func(d *Document) string { _, why := d.contentEvents(); return why },
		"annotsBuild":  func(d *Document) string { _, why := d.annots(); return why },
		"specsBuild":   func(d *Document) string { _, why := d.fileSpecs(); return why },
		"clipsBuild":   func(d *Document) string { _, why := d.mediaClips(); return why },
		"ttBuild":      func(d *Document) string { _, why := d.trueTypeFonts(); return why },
		"nodesBuild":   func(d *Document) string { _, why := d.structNodes(); return why },
	}
	ty := reflect.TypeOf(Document{})
	fields := 0
	for i := 0; i < ty.NumField(); i++ {
		f := ty.Field(i)
		if f.Type != reflect.TypeOf(population{}) {
			continue
		}
		fields++
		read, ok := rows[f.Name]
		if !ok {
			t.Errorf("Document.%s is a population with no row here", f.Name)
			continue
		}
		d, err := open(glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", "(a) Tj", nil))
		if err != nil {
			t.Fatal(err)
		}
		build := (*population)(unsafe.Pointer(reflect.ValueOf(d).Elem().FieldByName(f.Name).UnsafeAddr()))
		if why := read(d); why != "" || !build.finished {
			t.Fatalf("control: %s's build returned and reports %q (finished %v)", f.Name, why, build.finished)
		}
		build.finished = false
		if why := read(d); !strings.Contains(why, "stopped part-way") {
			t.Errorf("%s: a build that never returned reads as complete (%q)", f.Name, why)
		}
	}
	if fields != len(rows) {
		t.Errorf("Document has %d population fields and this table %d rows", fields, len(rows))
	}
}
