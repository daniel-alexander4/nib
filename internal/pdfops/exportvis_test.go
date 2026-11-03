package pdfops

import "testing"

// opaqueImage covers only an image that nothing stops hiding what is below it: each condition alone turns it into a
// non-cover (PLAN-import-export X6).
func TestOnlyAnUnqualifiedImageCoversText(t *testing.T) {
	ok := drawnImage{upright: true}
	if !opaqueImage(ok) {
		t.Fatal("the control: an upright plain image must cover")
	}
	for name, im := range map[string]drawnImage{
		"translucent": {upright: true, translucent: true},
		"clipped":     {upright: true, clipped: true},
		"layer off":   {upright: true, layerOff: true},
		"turned":      {upright: false},
		"stencil":     {upright: true, cause: PlaceholderStencil},
		"masked":      {upright: true, cause: PlaceholderMasked},
		"soft-masked": {upright: true, cause: PlaceholderSoftMask},
	} {
		if opaqueImage(im) {
			t.Errorf("%s image covers text", name)
		}
	}
}
