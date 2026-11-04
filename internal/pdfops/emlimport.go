package pdfops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"mime"
	"net/mail"
	"strings"
	"unicode"

	"golang.org/x/net/html/charset"
)

// An email message (`.eml`) — PLAN-import-export P04.S05, Y8, ADR-202. Nib reads the message itself (net/mail
// and the MIME walker of the saved-page packages); the body becomes one page that takes ADR-200's path. Of the
// headers only From, To, Cc, Date and Subject are printed, each as text; no other header (Received, X-Originating-IP,
// Message-ID, the tracking headers) is ever shown. An attachment is never decoded, extracted or written: it is
// counted, and the PDF says how many were not imported.

// ErrMsgNotSupported: Outlook's `.msg` (an OLE compound file) is refused by name. Y8: it is not taken on a
// reader that has not been built to the bar a stranger's message needs (FAT size, cyclic chains, stream sizes).
var ErrMsgNotSupported = errors.New("Outlook .msg files are not opened: save the message as .eml (Outlook: File, Save As) and open that")

const (
	mailHeaderValue = 1000 // printed characters of one header
	mailAddresses   = 50   // addresses printed of one header
)

// isEmail: the head opens with RFC 5322 header lines — `Name: value`, folded lines allowed — and names at least
// one of the headers every message carries. A saved page (MHTML) is decided before this runs.
func isEmail(head []byte) bool {
	if bytes.IndexByte(head, 0) >= 0 {
		return false
	}
	h := bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF})
	lines := strings.Split(strings.ReplaceAll(string(h), "\r\n", "\n"), "\n")
	known, n := false, 0
	for i, ln := range lines {
		if ln == "" {
			break
		}
		if i == len(lines)-1 {
			break // the head may cut the last line anywhere
		}
		if ln[0] == ' ' || ln[0] == '\t' {
			if n == 0 {
				return false
			}
			continue
		}
		name, _, ok := strings.Cut(ln, ":")
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			if strings.HasPrefix(ln, "From ") && i == 0 { // an mbox separator line
				continue
			}
			return false
		}
		n++
		switch strings.ToLower(name) {
		case "from", "to", "subject", "date", "received", "message-id", "mime-version", "return-path":
			known = true
		}
	}
	return known && n >= 2
}

// ConvertEMLToPDF prints a message and reports how many attachments it did not import.
func ConvertEMLToPDF(ctx context.Context, data []byte) ([]byte, int, error) {
	page, resolve, dropped, err := emlToHTML(data)
	if err != nil {
		return nil, 0, err
	}
	pdf, err := convertPageToPDF(ctx, page, resolve)
	if err != nil {
		return nil, 0, err
	}
	return pdf, dropped, nil
}

// mailText makes a header value printable: a decoded RFC 2047 word, no control or bidirectional-override
// character, white space collapsed, and a length cap. It is text, never markup (the caller escapes).
func mailText(s string) string {
	dec := &mime.WordDecoder{CharsetReader: charset.NewReaderLabel}
	if d, err := dec.DecodeHeader(s); err == nil {
		s = d
	}
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
			continue
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == 0xFFFD:
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
		if b.Len() >= mailHeaderValue*4 {
			break
		}
	}
	out := []rune(b.String())
	if len(out) > mailHeaderValue {
		out = append(out[:mailHeaderValue], '…')
	}
	return string(out)
}

func mailAddressList(raw string) string {
	list, err := mail.ParseAddressList(raw)
	if err != nil {
		return mailText(raw)
	}
	var parts []string
	for i, a := range list {
		if i == mailAddresses {
			parts = append(parts, fmt.Sprintf("and %d more", len(list)-mailAddresses))
			break
		}
		if a.Name != "" {
			parts = append(parts, mailText(a.Name)+" <"+mailText(a.Address)+">")
		} else {
			parts = append(parts, mailText(a.Address))
		}
	}
	return strings.Join(parts, ", ")
}

// emlDecode is which leaf parts walkMIME decodes: text, and a picture a body could name by `cid:`. Anything
// else is an attachment and is not read.
func emlDecode(typ, disp string) bool {
	if disp == "attachment" {
		return false
	}
	return typ == "text/plain" || typ == "text/html" || strings.HasPrefix(typ, "image/")
}

func emlToHTML(data []byte) (page []byte, resolve func(string) (string, bool), dropped int, err error) {
	end := len(data)
	for _, sep := range []string{"\r\n\r\n", "\n\n"} {
		if i := bytes.Index(data, []byte(sep)); i >= 0 && i < end {
			end = i
		}
	}
	if end > mimeHeaderBytes {
		return nil, nil, 0, ErrPackageTooBig
	}
	msg, err := mail.ReadMessage(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%w: %v", ErrPackageUnreadable, err)
	}
	var parts []mimePart
	total := 0
	if err := walkMIME(func(k string) string { return msg.Header.Get(k) }, msg.Body, 0, &parts, &total, emlDecode); err != nil {
		return nil, nil, 0, err
	}
	pics := &packagePictures{byName: map[string]string{}}
	var htmlBody, textBody []byte
	haveHTML, haveText := false, false
	for _, p := range parts {
		switch {
		case p.disp == "attachment" || !emlDecode(p.typ, p.disp):
			dropped++
		case p.typ == "text/html":
			if !haveHTML {
				htmlBody, haveHTML = p.body, true
			}
		case p.typ == "text/plain":
			if !haveText {
				textBody, haveText = p.body, true
			}
		case strings.HasPrefix(p.typ, "image/"):
			if p.cid == "" {
				dropped++ // a picture nothing can name is an attachment
				continue
			}
			pics.add([]string{"cid:" + p.cid}, p.typ, p.body)
		}
	}

	var head strings.Builder
	head.WriteString("<table>")
	for _, h := range []struct{ key, label string }{{"From", "From"}, {"To", "To"}, {"Cc", "Cc"}, {"Date", "Date"}, {"Subject", "Subject"}} {
		v := msg.Header.Get(h.key)
		if strings.TrimSpace(v) == "" {
			continue
		}
		switch h.key {
		case "From", "To", "Cc":
			v = mailAddressList(v)
		default:
			v = mailText(v)
		}
		fmt.Fprintf(&head, "<tr><th>%s:</th><td>%s</td></tr>", h.label, html.EscapeString(v))
	}
	head.WriteString("</table>")

	var styles, body []byte
	switch {
	case haveHTML:
		out, _, serr := sanitizeHTMLWith(htmlBody, pics.resolver(""))
		if serr != nil {
			return nil, nil, 0, fmt.Errorf("%w: %v", ErrHTMLUnreadable, serr)
		}
		styles, body = splitOwnDocument(out)
	case haveText:
		body = []byte(`<pre style="white-space:pre-wrap;font-family:monospace">` + html.EscapeString(mailText2(textBody)) + `</pre>`)
	default:
		body = []byte("<p>(This message has no text.)</p>")
	}
	var note string
	switch {
	case dropped == 1:
		note = "<p><i>1 attachment was not imported.</i></p>"
	case dropped > 1:
		note = fmt.Sprintf("<p><i>%d attachments were not imported.</i></p>", dropped)
	}
	page = []byte("<!doctype html><html><head><meta charset=utf-8>" + string(styles) + "</head><body>" + head.String() + "<hr>" + string(body) + note + "</body></html>")
	return page, nil, dropped, nil
}

// mailText2 prepares a plain-text body: line breaks kept, control and bidirectional-override characters dropped.
func mailText2(b []byte) string {
	var out strings.Builder
	for _, r := range strings.ToValidUTF8(string(b), "�") {
		switch {
		case r == '\n' || r == '\t':
			out.WriteRune(r)
		case r == '\r', unicode.IsControl(r), unicode.Is(unicode.Cf, r):
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
