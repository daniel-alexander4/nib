// Command nib runs the Nib PDF tool: it starts a loopback-only web
// server and opens the UI in a chromeless app-mode window.
//
// Usage:
//
//	nib [file.pdf]
//
// An optional PDF path is passed to the UI so the document opens on launch.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"nib"
	"nib/internal/browser"
	"nib/internal/cli"
	"nib/internal/instance"
	"nib/internal/safe"
	"nib/internal/server"
	"nib/internal/singleton"
	"nib/internal/vault"
)

// version is set at build time via -ldflags "-X main.version=…".
var version = "dev"

// main is a wrapper and run() is the program, so that **every deferred cleanup actually
// runs**.
//
// `os.Exit` does not run deferred functions. With the body inline, the `os.Exit(code)`
// on the error path below silently skipped every `defer` above it — which P07.S01
// noticed by adding one (the instance record's removal) and finding that a serve error
// would leave the record behind for the next launch to reason about. It is the same
// shape the P05 review found in the serve goroutine's `log.Fatalf`, which skipped both
// `defer stop()` and the explicit `DisarmSession`. Fixing it here rather than adding a
// second removal call site fixes the class: any future defer in this function is now
// correct by construction instead of correct by whoever remembers this paragraph.
func main() { os.Exit(run()) }

func run() int {
	log.SetFlags(0)
	log.SetPrefix("nib: ")

	// A recognized first argument is a headless subcommand: run it and exit
	// without ever binding a port or opening a browser. Anything else (a PDF
	// path, --replace, or no argument) falls through to the desktop boot below.
	if handled, code := cli.Run(os.Args[1:], version); handled {
		return code
	}

	replace := flag.Bool("replace", false, "terminate any other running Nib instance before starting")
	flag.Parse()
	log.Printf("Nib %s", version)

	// The desktop launcher passes --replace so clicking the menu item kills the
	// previous window's server and starts fresh.
	if *replace {
		if n := singleton.ReplaceOthers(); n > 0 {
			log.Printf("replaced %d running instance(s)", n)
		}
	}

	// Listen on a random loopback port so the app is never network-exposed.
	// NIB_ADDR pins a fixed loopback port for headless/remote runs (reach it via
	// an SSH tunnel); a non-loopback address is refused — nib is never exposed.
	bind := os.Getenv("NIB_ADDR")
	if bind == "" {
		bind = "127.0.0.1:0"
	}
	if !loopbackBind(bind) {
		log.Printf("NIB_ADDR must bind a loopback address (127.0.0.1, localhost, or ::1); got %q", bind)
		return 1
	}
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		log.Printf("cannot bind %s: %v", bind, err)
		return 1
	}
	// Backstop the loopbackBind string check: if a hostname like "localhost"
	// resolved to a routable address (e.g. a doctored /etc/hosts), refuse to serve.
	if tcp, ok := ln.Addr().(*net.TCPAddr); !ok || !tcp.IP.IsLoopback() {
		// Printf-and-return rather than Fatalf: inside run(), a Fatalf would skip the
		// deferred cleanups that the whole point of this restructuring is to run.
		log.Printf("refusing to serve on non-loopback address %s", ln.Addr())
		return 1
	}
	addr := ln.Addr().String()

	// Publish the rendezvous record: where this instance is, and a token proving a
	// probe reached IT. A second launch has no other way to find this process — the
	// bind above is 127.0.0.1:0, a random port by design.
	//
	// A failure here is logged and not fatal. The record is how a LATER launch finds
	// this one; a nib that cannot write it still serves the user in front of it, and
	// refusing to start would trade a working app for a missing convenience.
	cfgDir := vault.DefaultDir()
	probeToken, err := instance.NewToken()
	if err != nil {
		log.Printf("could not mint an instance token: %v", err)
	}
	if probeToken != "" {
		rec := instance.Record{Addr: addr, Token: probeToken, Version: version}
		switch err := instance.Create(cfgDir, rec); {
		case err == nil:
			// Deferred, and that only works because run() RETURNS rather than
			// calling os.Exit — see main. A stale record is the case the next
			// launch has to reason about, so leaving fewer of them is worth
			// making the whole function defer-safe.
			defer func() { _ = instance.Remove(cfgDir) }()
		case errors.Is(err, instance.ErrExists):
			// Another instance already published. P07.S02 decides what to do about
			// it — hand the path over, or take over a stale record. Until then this
			// is a note rather than a behaviour change: today's launch carries on and
			// serves, exactly as it did before the record existed.
			log.Printf("another Nib instance is already recorded; running alongside it")
			probeToken = ""
		default:
			log.Printf("could not publish the instance record: %v", err)
			probeToken = ""
		}
	}

	s := server.New(nib.WebFS(), nib.LegalFS(), vault.DefaultDir(), version)
	s.SetInstanceToken(probeToken)
	srv := &http.Server{Handler: s.Handler()}
	// A serve failure signals the main goroutine instead of exiting from here.
	// log.Fatalf calls os.Exit, which skips every deferred function AND the explicit
	// DisarmSession below — so a serve error left an armed co-signing listener, the
	// one routable socket this app ever opens, un-torn-down. It also carried no
	// safe.Recover, unlike the three other detached goroutines in the tree, so a
	// panic outside an HTTP handler took the process down with the user's unsaved
	// document — which is what safe.Recover's own comment says it exists to prevent.
	serveErr := make(chan error, 1)
	go func() {
		defer safe.Recover("http serve")
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	uiURL := "http://" + addr + "/"
	if path := initialFile(); path != "" {
		uiURL += "?open=" + url.QueryEscape(path)
	}
	log.Printf("serving at %s", uiURL)

	// NIB_NO_BROWSER lets the app run headless (tests, remote boxes); the URL
	// above is logged so it can still be opened by hand.
	if os.Getenv("NIB_NO_BROWSER") == "" {
		if _, err := browser.Open(uiURL); err != nil {
			log.Printf("could not open a browser window: %v", err)
		}
	}

	// Run until interrupted, then shut down cleanly.
	//
	// **SIGTERM as well as SIGINT, and that is not tidiness.** This process publishes an
	// instance record and removes it on the way out, and an unhandled SIGTERM kills it
	// without running a single deferred function — so the record outlives the instance.
	// The signal is not hypothetical: `build/nib.desktop` passes `--replace` on every
	// activation and `singleton.ReplaceOthers` sends exactly SIGTERM, so before this
	// line every desktop double-click left a stale record behind, pointing at a dead
	// process, while the launch that killed it found ErrExists and published nothing.
	// Found by P07's plan review, at the line.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code := 0
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		log.Printf("server error: %v", err)
		code = 1
	}
	s.DisarmSession() // tear down any armed co-signing listener before exiting
	_ = srv.Close()
	return code
}

// loopbackBind reports whether addr is a host:port on the loopback interface.
// It mirrors the loopback set the server's Host guard enforces, so the address
// nib binds and the requests it accepts share one notion of "loopback".
func loopbackBind(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// initialFile returns an absolute path for the optional PDF argument, or "".
func initialFile() string {
	arg := flag.Arg(0)
	if arg == "" {
		return ""
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return arg
	}
	return abs
}
