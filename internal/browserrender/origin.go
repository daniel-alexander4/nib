// Package browserrender is the one door a Chromium-family browser renders a stranger's page through
// (PLAN-import-export P04.S04, D29, R3, X4, ADR-200).
//
// A page rendered from a `file:` URL can name any file on the machine (`<meta http-equiv=refresh
// content="0;url=file:///…">` printed the secret into the PDF, measured). So the page is never a file:
// Nib serves the one sanitised document from a listener that lives for one request, on 127.0.0.1, under a
// path of 256 random bits, and tells the browser — by a host rule — that the only name it can resolve is
// the one the listener answers to. A refresh to an IP literal, to `file:` or to anywhere else cannot be
// reached; the Content-Security-Policy is a response header, so no markup in the page can drop it.
//
// This package builds the origin and the browser's arguments. It does NOT start the browser: the process
// is started by internal/pdfops through its converter door (`runConvert`), which owns the process group,
// the memory watchdog and the registry Quit kills. Both halves are needed to render, and Args is the ONE
// place the flags are spelled (TestTheBrowserIsLaunchedWithTheDoorsFlags).
package browserrender

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"nib/internal/peercred"
	"nib/internal/safe"
)

// Host is the one name the browser can resolve. `.test` is reserved (RFC 6761) and never resolves in DNS.
const Host = "nib-render.test"

// CSP is sent as a response header on the one document: nothing loads but inline style and `data:` pictures,
// no script runs, no frame, form or navigation target is allowed.
const CSP = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; script-src 'none'; " +
	"frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// Lifetime is the hard limit on an origin whatever the browser does.
const Lifetime = 30 * time.Second

// ErrNotServed: the browser finished without ever receiving the document, so whatever it wrote is not the page.
var ErrNotServed = errors.New("the browser never received the page, so what it wrote is not the page")

// Origin serves one document once.
type Origin struct {
	ln    net.Listener
	srv   *http.Server
	port  int
	token string
	doc   []byte

	// ownerOf answers "who owns the far end of this connection"; a test replaces it.
	ownerOf func(local, peer netip.AddrPort) (uint32, peercred.State)

	consumed atomic.Bool   // the token path has been asked for once
	served   atomic.Bool   // the whole document was written to that one request
	refused  atomic.Int64  // requests answered 404 (a wrong host, a wrong user, a wrong path, a second ask)
	done     chan struct{} // closed when the document has been handled
	once     sync.Once
	closed   chan struct{}
}

// Listen starts the origin for doc. The caller must Close it on every path.
func Listen(doc []byte) (*Origin, error) {
	return listen(doc, nil)
}

func listen(doc []byte, ownerOf func(local, peer netip.AddrPort) (uint32, peercred.State)) (*Origin, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		ln.Close()
		return nil, err
	}
	o := &Origin{
		ln:      ln,
		port:    ln.Addr().(*net.TCPAddr).Port,
		token:   hex.EncodeToString(raw[:]),
		doc:     doc,
		ownerOf: ownerOf,
		done:    make(chan struct{}),
		closed:  make(chan struct{}),
	}
	if o.ownerOf == nil {
		o.ownerOf = peercred.Owner
	}
	o.srv = &http.Server{
		Handler:           http.HandlerFunc(o.handle),
		ReadHeaderTimeout: 5 * time.Second,
		ConnContext:       func(ctx context.Context, c net.Conn) context.Context { return ctx },
	}
	go func() {
		defer safe.Recover("render origin serve")
		_ = o.srv.Serve(ln)
	}()
	go func() {
		defer safe.Recover("render origin lifetime")
		t := time.NewTimer(Lifetime)
		defer t.Stop()
		select {
		case <-t.C:
			o.Close()
		case <-o.closed:
		}
	}()
	return o, nil
}

// URL is the address the browser is sent to; it resolves only through the host rule in Args.
func (o *Origin) URL() string {
	return "http://" + Host + ":" + strconv.Itoa(o.port) + "/" + o.token
}

// HostRule is the browser's resolver rule: one name to the listener, every other name unresolvable.
func (o *Origin) HostRule() string {
	return "MAP " + Host + " 127.0.0.1:" + strconv.Itoa(o.port) + ", MAP * ~NOTFOUND"
}

// Served reports whether the document went out whole to the one request.
func (o *Origin) Served() bool { return o.served.Load() }

// Refused counts requests answered 404.
func (o *Origin) Refused() int64 { return o.refused.Load() }

// Port is the listener's port (for the census and tests).
func (o *Origin) Port() int { return o.port }

func (o *Origin) deny(w http.ResponseWriter) {
	o.refused.Add(1)
	w.Header().Set("Connection", "close")
	http.NotFound(w, nil)
}

// handle answers exactly one request. The checks run per request on RemoteAddr, not per connection; a
// wrong user, a wrong Host or a wrong path is a 404 that does NOT spend the use (X4 b).
func (o *Origin) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.Host != Host+":"+strconv.Itoa(o.port) {
		o.deny(w)
		return
	}
	if !o.peerIsMe(r) {
		o.deny(w)
		return
	}
	if r.URL.Path != "/"+o.token {
		o.deny(w)
		return
	}
	if !o.consumed.CompareAndSwap(false, true) {
		o.deny(w) // a retry after the use is spent
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", CSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Length", strconv.Itoa(len(o.doc)))
	h.Set("Connection", "close")
	n, err := w.Write(o.doc)
	if err == nil && n == len(o.doc) {
		o.served.Store(true)
	}
	// Shutdown waits on a Chromium pre-connect that never ends (~5 s, measured by the plan), so it is asked
	// from a goroutine of its own and bounded; the handler only signals.
	o.once.Do(func() { close(o.done) })
}

// peerIsMe: a request is answered only when the kernel names this user as the owner of the connection, or
// cannot say (off Linux; the door's declared gap, ADR-200). Another user's socket is refused, root's is not.
func (o *Origin) peerIsMe(r *http.Request) bool {
	local, err1 := netip.ParseAddrPort(o.ln.Addr().String())
	peer, err2 := netip.ParseAddrPort(r.RemoteAddr)
	if err1 != nil || err2 != nil {
		return false
	}
	uid, st := o.ownerOf(local, peer)
	switch st {
	case peercred.Found:
		return uid == uint32(os.Geteuid()) || uid == 0
	case peercred.Gone, peercred.Unknown:
		return true
	}
	return false
}

// Done is closed once the document has been handled.
func (o *Origin) Done() <-chan struct{} { return o.done }

// Close ends the origin: Shutdown bounded at one second, then Close. Safe to call more than once.
func (o *Origin) Close() {
	select {
	case <-o.closed:
		return
	default:
	}
	close(o.closed)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := o.srv.Shutdown(ctx); err != nil {
		_ = o.srv.Close()
	}
	_ = o.ln.Close()
}

// Args is the ONE spelling of the browser's arguments for rendering a page to a PDF. profile is a fresh
// 0700 directory the caller made and removes; out is the PDF to write. The flags are held by
// TestTheBrowserIsLaunchedWithTheDoorsFlags: dropping the host rule, adding --no-sandbox or
// --allow-file-access-from-files turns it red.
func (o *Origin) Args(profile, out string) []string {
	return []string{
		"--headless",
		"--disable-gpu",
		"--no-pdf-header-footer",
		"--user-data-dir=" + profile,
		"--host-resolver-rules=" + o.HostRule(),
		"--no-proxy-server",
		"--disable-extensions",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-crash-reporter",
		"--disable-breakpad",
		"--disable-component-update",
		"--no-pings",
		"--disable-sync",
		"--print-to-pdf=" + out,
		o.URL(),
	}
}

func (o *Origin) String() string { return fmt.Sprintf("render origin on port %d", o.port) }
