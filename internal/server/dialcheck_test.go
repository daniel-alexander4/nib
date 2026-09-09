package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// **The convened-document refusal, reached by the CLIENT'S OWN PAYLOAD** (P01.S02's deepdive,
// verified live 2026-09-09).
//
// The refusal itself is right and must stay: signing a ceremony document outside its proceeding
// produces a signature every other party would reject. What this test pins is that the refusal is
// reachable from the bytes and fields `web/app.js` actually posts — not from a synthetic request —
// because the answer it gives that user is *"Use the ceremony to sign it"*, and **the product has
// no such door** (`/pending 436`). If a ceremony-aware dial is ever built, this test goes red on
// its own subject and should be re-pointed at that route rather than deleted: the gate is correct,
// what is missing is a path that does not need it.
//
// This posts the EXACT form fields `web/app.js`'s `sessionInit` posts — `pdf`, `params`,
// `appearance`, `address` — on a document this machine has just convened. The client sends no
// `invitation` field: `grep -c sinInvite web/app.js web/index.html` returns 0/0, and the only
// `invitation` in any request body is `/api/session/arm`'s, which is the RECEIVING side.
//
// `handleSessionInitiate` takes its ceremony from `r.FormValue("invitation")` — *"the dialing side's
// ceremony identity, from the same pasteable invitation the arm takes"* — so with the field absent
// `cer` is nil, the roster is empty, and `buildCoSigned` reaches its convened-document refusal.
func TestTheClientsOwnPayloadReachesTheConvenedDocumentRefusal(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	code0, body0 := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath})
	if code0 != http.StatusOK {
		t.Fatalf("open: %d %s", code0, body0)
	}
	var opened docResponse
	if err := json.Unmarshal([]byte(body0), &opened); err != nil {
		t.Fatal(err)
	}
	me := myFingerprint(t, c, ts.URL)
	them := strings.Repeat("2b", 32)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Convener", Signs: true},
			{Fingerprint: them, Label: "B", Signs: true},
		},
		Intent: "co-sign the lease", ConvenerSigns: true,
		Expires: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
	})
	if code != http.StatusOK {
		t.Fatalf("convene: %d %s", code, body)
	}
	var conv conveneResponse
	if err := json.Unmarshal([]byte(body), &conv); err != nil {
		t.Fatal(err)
	}
	// SETUP: there really is a ceremony on this document, with an invitation the convener holds —
	// so a refusal below is about the DIAL and not about a document with no proceeding on it.
	if len(conv.Invites) != 1 {
		t.Fatalf("setup: convene issued %d invitations, want 1", len(conv.Invites))
	}

	// **The SERVER's current bytes, not the file on disk** — and the first version of this probe
	// read the file and measured nothing. `convene` commits the ceremony into the OPEN document;
	// the path still holds the pre-convene PDF, so posting it made `ceremony.Extract` fail, the
	// convened-document gate never fired, and the dial ran on to the network. The client posts
	// `bakedForm(owner)`, which is the open document — so the probe has to as well.
	pdf := pdfOf(t, ts, c, opened.ID)
	if _, cerr := ceremony.Extract(pdf); cerr != nil {
		t.Fatalf("setup: the bytes this probe posts carry no ceremony record (%v), so the gate it "+
			"exists to reach cannot fire and a green result would mean nothing", cerr)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	mw.WriteField("params", `{"fingerprint":"`+them+`","intent":"I agree to sign this document."}`)
	// The appearance the client renders and posts — omitting it is refused earlier, at
	// "missing appearance", which is a different gate and would make this test about that one.
	iw, _ := mw.CreateFormFile("appearance", "attestation.png")
	png.Encode(iw, image.NewRGBA(image.Rect(0, 0, 240, 60)))
	mw.WriteField("address", "127.0.0.1:1")
	mw.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/session/initiate", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)

	t.Logf("the client's own dial on a document it convened: %d %s", res.StatusCode, strings.TrimSpace(string(out)))

	if res.StatusCode == http.StatusOK {
		t.Fatal("the dial SUCCEEDED — this test's premise is wrong and the finding it records " +
			"should be struck rather than believed")
	}
	if !strings.Contains(string(out), "part of a signing ceremony") {
		t.Errorf("the dial was refused %d for some other reason: %s. This test exists to pin ONE "+
			"refusal — the convened-document gate in buildCoSigned — and a different one means the "+
			"finding is about a different defect", res.StatusCode, strings.TrimSpace(string(out)))
	}
}
