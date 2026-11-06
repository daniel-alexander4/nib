package pdfops

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// An email message (P04.S05, Y8, ADR-202).

func eml(headers string, body string) []byte {
	return []byte(headers + "\r\n\r\n" + body)
}

const emlHead = "Received: from mx.example (mx.example [203.0.113.9]) by mail.example; Tue, 3 Nov 2026 10:00:00 +0000\r\n" +
	"X-Originating-IP: [198.51.100.77]\r\nMessage-ID: <secret-id-4711@mx.example>\r\nReturn-Path: <bounce@mx.example>\r\n" +
	"From: =?UTF-8?Q?Zo=C3=AB_Example?= <zoe@example.org>\r\nTo: Bob <bob@example.org>, carol@example.org\r\nCc: dan@example.org\r\n" +
	"Date: Tue, 3 Nov 2026 10:00:00 +0000\r\nSubject: =?UTF-8?B?Q2Fmw6kgbWVldGluZw==?=\r\nMIME-Version: 1.0\r\n"

func mixed(parts ...string) []byte {
	return eml(emlHead+"Content-Type: multipart/mixed; boundary=\"B\"", strings.Join(parts, "")+"--B--\r\n")
}

func TestOnlyTheFiveNamedHeadersAreEverPrinted(t *testing.T) {
	page, _, dropped, err := emlToHTML(mixed(mpart("Content-Type: text/plain; charset=utf-8", "Hello there")))
	if err != nil || dropped != 0 {
		t.Fatalf("err=%v dropped=%d", err, dropped)
	}
	s := string(page)
	for _, want := range []string{"Zoë Example", "zoe@example.org", "Bob", "carol@example.org", "dan@example.org", "Café meeting", "Hello there", "3 Nov 2026"} {
		if !strings.Contains(s, want) {
			t.Errorf("page lacks %q:\n%s", want, s)
		}
	}
	for _, leak := range []string{"203.0.113.9", "198.51.100.77", "secret-id-4711", "bounce@", "mx.example", "Received", "Message-ID", "X-Originating"} {
		if strings.Contains(s, leak) {
			t.Errorf("the page prints %q, a header Nib does not print", leak)
		}
	}
}

func TestAHeaderIsTextAndNeverMarkup(t *testing.T) {
	h := "From: \"<script>alert(1)</script>\" <a@b.test>\r\nTo: x@y.test\r\nSubject: <img src=http://127.0.0.1:1/p> \u202Efdp.exe\r\nContent-Type: text/plain"
	page, _, _, err := emlToHTML(eml(h, "body"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if strings.Contains(s, "<script") || strings.Contains(s, "<img") || strings.ContainsRune(s, '\u202E') {
		t.Errorf("a header became markup or kept a direction override:\n%s", s)
	}
	if !strings.Contains(s, "&lt;img") {
		t.Errorf("the subject's text is not shown as text:\n%s", s)
	}
}

func TestAnAttachmentIsCountedAndNeverRead(t *testing.T) {
	// 12 MiB of base64 is over the one-part cap: if it were decoded the message would be refused.
	huge := strings.Repeat("QUJDREVG\r\n", 3<<20)
	msg := mixed(
		mpart("Content-Type: text/plain", "see attached"),
		mpart("Content-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"a.pdf\"\r\nContent-Transfer-Encoding: base64", huge),
		mpart("Content-Type: image/png\r\nContent-Disposition: attachment; filename=\"b.png\"\r\nContent-Transfer-Encoding: base64", "iVBORw0KGgo="),
		mpart("Content-Type: message/rfc822", "From: z@z.test\r\n\r\nforwarded"),
	)
	page, _, dropped, err := emlToHTML(msg)
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 3 || !strings.Contains(string(page), "3 attachments were not imported") {
		t.Errorf("dropped=%d page=%s", dropped, page)
	}
	if strings.Contains(string(page), "a.pdf") || strings.Contains(string(page), "forwarded") {
		t.Errorf("an attachment's name or content reached the page")
	}
	_, _, d1, _ := emlToHTML(mixed(mpart("Content-Type: text/plain", "x"), mpart("Content-Type: application/zip", "PK")))
	if d1 != 1 {
		t.Errorf("one attachment counted as %d", d1)
	}
}

func TestAnInlinePictureIsOnlyOneTheMessageHoldsByCid(t *testing.T) {
	png := base64.StdEncoding.EncodeToString(testPNGBytes(t))
	msg := eml(emlHead+"Content-Type: multipart/related; boundary=\"B\"", strings.Join([]string{
		mpart("Content-Type: text/html; charset=utf-8", `<p>Hi</p><img src="cid:logo" alt="L"><img src="http://track.example/p.gif" alt="PIXEL"><img src="cid:absent" alt="Z"><img src="file:///etc/passwd" alt="F">`),
		mpart("Content-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <logo>", png),
		mpart("Content-Type: image/png\r\nContent-Transfer-Encoding: base64", png), // no name: an attachment
		"--B--\r\n"}, ""))
	page, _, dropped, err := emlToHTML(msg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if strings.Count(s, "data:image/png;base64,") != 1 || strings.Contains(s, "track.example") || strings.Contains(s, "/etc/passwd") || strings.Contains(s, "absent") {
		t.Errorf("page = %s", s)
	}
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1 (the picture with no name)", dropped)
	}
}

func TestAMessageIsHeldToItsCaps(t *testing.T) {
	var many []string
	for i := 0; i <= packageParts; i++ {
		many = append(many, mpart("Content-Type: text/plain", "x"))
	}
	if _, _, _, err := emlToHTML(mixed(many...)); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("501 parts: %v", err)
	}
	if _, _, _, err := emlToHTML(nestedMHTML(packageDepth + 1)); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("deep nesting: %v", err)
	}
	if _, _, _, err := emlToHTML([]byte("Subject: " + strings.Repeat("a", mimeHeaderBytes+10) + "\r\nFrom: a@b\r\n\r\nx")); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("a header block over its cap: %v", err)
	}
	big := mixed(mpart("Content-Type: text/plain\r\nContent-Transfer-Encoding: base64", strings.Repeat("QUJDREVG\r\n", 3<<20)))
	if _, _, _, err := emlToHTML(big); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("a text body over the part cap: %v", err)
	}
}

func TestAPlainTextBodyKeepsItsLinesAndAnyCharset(t *testing.T) {
	msg := eml("From: a@b.test\r\nTo: c@d.test\r\nContent-Type: text/plain; charset=iso-8859-1\r\nContent-Transfer-Encoding: quoted-printable", "caf=E9\r\n<b>not bold</b>\r\nline two")
	page, _, _, err := emlToHTML(msg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if !strings.Contains(s, "café") || !strings.Contains(s, "&lt;b&gt;not bold&lt;/b&gt;") || !strings.Contains(s, "line two") {
		t.Errorf("page = %s", s)
	}
}

func TestSniffAndClassifyAMailMessage(t *testing.T) {
	m := mixed(mpart("Content-Type: text/plain", "x"))
	if !isEmail(m) {
		t.Error("a mail message is not recognised")
	}
	for name, b := range map[string][]byte{
		"markdown": []byte("# Title\n\nSome: text here\n"),
		"prose":    []byte("Dear Bob,\nplease see Subject: below\n"),
		"html":     []byte("<html><body>From: me</body></html>"),
		"binary":   {'F', 'r', 'o', 'm', ':', ' ', 0, 1},
	} {
		if isEmail(b) {
			t.Errorf("%s was taken for a mail message", name)
		}
	}
	if isEmail(mhtml(t)) && sniffFamily(mhtml(t)) != FamilyMHTML {
		t.Error("a saved page was claimed as mail")
	}
	if f, err := classify(m, "note.eml"); err != nil || f.Key != "eml" {
		t.Errorf("classify = %v, %v", f.Key, err)
	}
	if f, err := classify(m, "note.txt"); err != nil || f.Key != "txt" {
		t.Errorf("a message named .txt stays text: %v, %v", f.Key, err)
	}
	if _, err := classify(m, "note.docx"); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("a message named .docx: %v", err)
	}
	if _, err := classify(m, ""); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("a nameless message: %v", err)
	}
}

func TestOutlookMsgIsRefusedByName(t *testing.T) {
	ole := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 600)...)
	if _, err := classify(ole, "mail.msg"); !errors.Is(err, ErrMsgNotSupported) {
		t.Errorf(".msg OLE: %v", err)
	}
	if _, err := classify(ole, "mail.MSG"); !errors.Is(err, ErrMsgNotSupported) {
		t.Errorf(".MSG OLE: %v", err)
	}
	if _, err := classify(ole, "mail.doc"); errors.Is(err, ErrMsgNotSupported) {
		t.Errorf("a .doc was refused as .msg")
	}
	if f, ok := FormatForExt("msg"); ok {
		t.Errorf("the table has a row for .msg: %v", f.Key)
	}
	if got := refusalCause(ErrMsgNotSupported); got != "unsupported" {
		t.Errorf("cause = %q", got)
	}
}

func TestAMailMessageRendersThroughTheBrowserAndFetchesNothing(t *testing.T) {
	needBrowser(t)
	var hits atomic.Int32
	canary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer canary.Close()
	msg := eml(emlHead+"Content-Type: multipart/mixed; boundary=\"B\"", strings.Join([]string{
		mpart("Content-Type: text/html; charset=utf-8", fmt.Sprintf(`<p>Quarterly numbers</p><img src="%s/pixel.gif" width=1 height=1 alt=""><link rel=stylesheet href="%s/c.css">`, canary.URL, canary.URL)),
		mpart("Content-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"x.pdf\"\r\nContent-Transfer-Encoding: base64", "JVBERi0="),
		"--B--\r\n"}, ""))
	pdf, dropped, err := ConvertEMLToPDF(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	txt := pdfText(t, pdf)
	for _, want := range []string{"Quarterlynumbers", "ZoëExample", "Cafémeeting", "1attachmentwasnotimported"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the PDF lacks %q: %q", want, txt)
		}
	}
	for _, leak := range []string{"203.0.113.9", "198.51.100.77", "secret-id"} {
		if strings.Contains(txt, leak) {
			t.Errorf("the PDF shows %q", leak)
		}
	}
	if dropped != 1 || hits.Load() != 0 {
		t.Errorf("dropped=%d canary hits=%d", dropped, hits.Load())
	}
}

func TestATextAttachmentIsNotTheBodyAndIsCounted(t *testing.T) {
	page, _, dropped, err := emlToHTML(mixed(
		mpart("Content-Type: text/plain", "the body"),
		mpart("Content-Type: text/plain\r\nContent-Disposition: attachment; filename=n.txt", "SECRET NOTES"),
		mpart("Content-Type: text/html\r\nContent-Disposition: attachment; filename=n.html", "<p>SECRET HTML</p>"),
	))
	if err != nil || dropped != 2 || strings.Contains(string(page), "SECRET") || !strings.Contains(string(page), "the body") {
		t.Errorf("err=%v dropped=%d page=%s", err, dropped, page)
	}
	page, _, _, _ = emlToHTML(mixed(mpart("Content-Type: text/plain\r\nContent-Disposition: attachment", "ONLY ATTACHED")))
	if strings.Contains(string(page), "ONLY ATTACHED") {
		t.Errorf("an attached text file became the body")
	}
}

func TestABodyCarriesNoControlOrDirectionCharacters(t *testing.T) {
	page, _, _, err := emlToHTML(eml("From: a@b.test\r\nTo: c@d.test\r\nContent-Type: text/plain; charset=utf-8", "ab\x01c‮d​e"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if strings.ContainsAny(s, "\x01‮​") || !strings.Contains(s, "abcde") {
		t.Errorf("page = %q", s)
	}
}

func TestOnlyAWholeHeaderBlockWithAKnownHeaderIsAMessage(t *testing.T) {
	for name, b := range map[string]string{
		"one header":      "From: a@b.test\r\n\r\nbody",
		"unknown headers": "Foo: 1\r\nBar: 2\r\nBaz: 3\r\n\r\nbody",
	} {
		if isEmail([]byte(b)) {
			t.Errorf("%s was taken for a message", name)
		}
	}
	if !isEmail([]byte("From: a@b.test\r\nX-Foo: 1\r\n\r\nbody")) {
		t.Error("two headers with a known one is a message")
	}
}

func TestALongHeaderAndALongAddressListAreCut(t *testing.T) {
	var to []string
	for i := 0; i < mailAddresses+10; i++ {
		to = append(to, fmt.Sprintf("u%d@x.test", i))
	}
	page, _, _, err := emlToHTML(eml("From: a@b.test\nTo: "+strings.Join(to, ", ")+"\nSubject: "+strings.Repeat("s", mailHeaderValue+500)+"\nContent-Type: text/plain", "x"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if !strings.Contains(s, "and 10 more") || strings.Contains(s, "u59@x.test") || !strings.Contains(s, "…") || strings.Contains(s, strings.Repeat("s", mailHeaderValue+1)) {
		t.Errorf("page not cut: %.300s", s)
	}
	if _, _, _, err := emlToHTML([]byte("Subject: " + strings.Repeat("a", mimeHeaderBytes+10) + "\nFrom: a@b\n\nx")); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("an LF-only header block over its cap: %v", err)
	}
}

func TestATextAttachmentOverThePartCapIsNeverDecoded(t *testing.T) {
	huge := strings.Repeat("QUJDREVG\r\n", 3<<20)
	if _, _, d, err := emlToHTML(mixed(mpart("Content-Type: text/plain", "x"), mpart("Content-Type: text/plain\r\nContent-Disposition: attachment\r\nContent-Transfer-Encoding: base64", huge))); err != nil || d != 1 {
		t.Errorf("err=%v dropped=%d", err, d)
	}
}

func TestALargeMessageWithShortLfHeadersIsRead(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		m := "From: a@b.test" + nl + "To: c@d.test" + nl + "Content-Type: text/plain" + nl + nl + strings.Repeat("word ", 40000)
		if _, _, _, err := emlToHTML([]byte(m)); err != nil {
			t.Errorf("%q line ends: %v", nl, err)
		}
	}
}

func TestAMessagesStyleSheetCannotReachNibsHeaderTable(t *testing.T) {
	page, _, _, err := emlToHTML(eml(emlHead+"Content-Type: text/html", "<style>table{display:none} th{content:'x'}</style><p>hello body</p>"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "display:none") || strings.Contains(string(page), "<style") {
		t.Errorf("the message's style sheet reached the page: %s", page)
	}
	if !strings.Contains(string(page), "hello body") {
		t.Error("the body is gone")
	}
}

// /pending 944 (c): a picture the message names and the door will not take is an attachment not imported, counted and
// said, not silently an alt text.
func TestAPictureTheDoorRefusesIsCountedAsNotImported(t *testing.T) {
	png := base64.StdEncoding.EncodeToString(testPNGBytes(t))
	svg := base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
	page, _, dropped, err := emlToHTML(mixed(
		mpart("Content-Type: text/html; charset=utf-8", `<p>Hi</p><img src="cid:ok"><img src="cid:vec"><img src="cid:empty">`),
		mpart("Content-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <ok>", png),
		mpart("Content-Type: image/svg+xml\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <vec>", svg),
		mpart("Content-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <empty>", "")))
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 2 || !strings.Contains(string(page), "2 attachments were not imported") {
		t.Errorf("dropped = %d, page = %q", dropped, page)
	}
}
