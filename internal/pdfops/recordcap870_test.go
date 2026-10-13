package pdfops

import "testing"

// TestTheCeremonyRecordIsInflatedOnlyAsFarAsARecordCanBe — `/pending 870`. `CeremonyRecord` read its entry through
// the extraction's uncapped decode, so a document whose "record" inflates to 200 MiB had it inflated whole on every
// arrival and handed to the JSON decoder. Measured on this fixture: 512 MiB allocated before, 16 MiB after.
func TestTheCeremonyRecordIsInflatedOnlyAsFarAsARecordCanBe(t *testing.T) {
	doc := func(stream string) []byte {
		return assembleFixture(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles << /Names [(" + CeremonyRecordName + ") 8 0 R] >> >> >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
			4: optStream("0 0 1 1 re f", ""),
			8: "<< /Type /Filespec /F (" + CeremonyRecordName + ") /UF (" + CeremonyRecordName + ") /EF << /F 9 0 R >> >>",
			9: stream,
		})
	}
	// Control: an ordinary record is read whole, and so is one larger than the largest that verifies.
	if got, err := CeremonyRecord(doc(optStream(`{"id":"abc"}`, "/Type /EmbeddedFile"))); err != nil || string(got) != `{"id":"abc"}` {
		t.Fatalf("control: an ordinary record read %q (%v)", got, err)
	}
	const largest = 2 << 20 // above the ~1.6 MiB the record's own text bounds allow
	if got, err := CeremonyRecord(doc(optStream(flatedSpaces(largest), "/Type /EmbeddedFile /Filter /FlateDecode"))); err != nil || len(got) != largest {
		t.Fatalf("control: a %d-byte record read as %d bytes (%v), want it whole — the cap must clear every record that verifies", largest, len(got), err)
	}
	// Stimulus.
	bomb := doc(optStream(flatedSpaces(decodeBomb), "/Type /EmbeddedFile /Filter /FlateDecode"))
	var got []byte
	var err error
	alloc := allocated(func() { got, err = CeremonyRecord(bomb) })
	t.Logf("a record inflating to %d MiB: %d MiB allocated, %d bytes returned, %v", decodeBomb>>20, alloc>>20, len(got), err)
	if err == nil || got != nil {
		t.Errorf("a record inflating to %d MiB was returned (%d bytes, %v), want it refused", decodeBomb>>20, len(got), err)
	}
	if most := uint64(8 * maxCeremonyRecord); alloc > most {
		t.Errorf("reading the record allocated %d MiB over a stream inflating to %d MiB, want under %d MiB — it decodes "+
			"within maxCeremonyRecord", alloc>>20, decodeBomb>>20, most>>20)
	}
}
