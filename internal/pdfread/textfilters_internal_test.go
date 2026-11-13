package pdfread

import (
	"encoding/hex"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestATextFilterAfterAnImageFilterIsLeftToPdfcpu — /pending 982. decodeTextFilters decodes a pipeline only when every
// filter in it is a general one; an image or Crypt filter needs pdfcpu's own preparation (a CCITT stream's
// parameters, a DCT stream's components), so such a stream must come back exactly as it was, with no error.
func TestATextFilterAfterAnImageFilterIsLeftToPdfcpu(t *testing.T) {
	for _, first := range []string{"DCTDecode", "CCITTFaxDecode", "JBIG2Decode", "Crypt"} {
		raw := []byte(hex.EncodeToString([]byte("not a picture")) + ">")
		sd := types.NewStreamDict(types.Dict{}, 0, nil, nil, []types.PDFFilter{{Name: first}, {Name: "ASCIIHexDecode"}})
		sd.Raw = raw
		if err := decodeTextFilters(nil, &sd); err != nil {
			t.Fatalf("%s then ASCIIHexDecode: %v; want the stream left for pdfcpu", first, err)
		}
		if len(sd.FilterPipeline) != 2 || string(sd.Raw) != string(raw) {
			t.Fatalf("%s then ASCIIHexDecode was changed: %d filters left", first, len(sd.FilterPipeline))
		}
	}
	sd := types.NewStreamDict(types.Dict{"Filter": types.Name("ASCIIHexDecode")}, 0, nil, nil, []types.PDFFilter{{Name: "ASCIIHexDecode"}})
	sd.Raw = []byte("00ff>")
	if err := decodeTextFilters(nil, &sd); err != nil || sd.FilterPipeline != nil || string(sd.Raw) != "\x00\xff" || sd.Dict["Filter"] != nil {
		t.Fatalf("a hex-only stream: err %v, %d filters, raw %q, /Filter %v; want decoded with no filter", err, len(sd.FilterPipeline), sd.Raw, sd.Dict["Filter"])
	}
}
