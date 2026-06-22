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

	"nib"
	"nib/internal/browser"
	"nib/internal/cli"
	"nib/internal/server"
	"nib/internal/singleton"
	"nib/internal/vault"
)

// version is set at build time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	log.SetFlags(0)
	log.SetPrefix("nib: ")

	// A recognized first argument is a headless subcommand: run it and exit
	// without ever binding a port or opening a browser. Anything else (a PDF
	// path, --replace, or no argument) falls through to the desktop boot below.
	if handled, code := cli.Run(os.Args[1:], version); handled {
		os.Exit(code)
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
		log.Fatalf("NIB_ADDR must bind a loopback address (127.0.0.1, localhost, or ::1); got %q", bind)
	}
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		log.Fatalf("cannot bind %s: %v", bind, err)
	}
	// Backstop the loopbackBind string check: if a hostname like "localhost"
	// resolved to a routable address (e.g. a doctored /etc/hosts), refuse to serve.
	if tcp, ok := ln.Addr().(*net.TCPAddr); !ok || !tcp.IP.IsLoopback() {
		log.Fatalf("refusing to serve on non-loopback address %s", ln.Addr())
	}
	addr := ln.Addr().String()

	s := server.New(nib.WebFS(), nib.LegalFS(), vault.DefaultDir(), version)
	srv := &http.Server{Handler: s.Handler()}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
	s.DisarmSession() // tear down any armed co-signing listener before exiting
	_ = srv.Close()
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
