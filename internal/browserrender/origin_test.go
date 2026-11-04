package browserrender

import (
	"io"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nib/internal/peercred"
)

func get(t *testing.T, o *Origin, method, host, path string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, "http://127.0.0.1:"+itoa(o.Port())+path, nil)
	req.Host = host
	res, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func itoa(n int) string { return fmtInt(n) }
func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func tokenPath(o *Origin) string { return "/" + o.token }

func newOrigin(t *testing.T, owner func(l, p netip.AddrPort) (uint32, peercred.State)) *Origin {
	t.Helper()
	o, err := listen([]byte("<p>doc</p>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(o.Close)
	return o
}

func TestTheDocumentIsServedOnceWithItsHeaders(t *testing.T) {
	o := newOrigin(t, nil)
	host := Host + ":" + fmtInt(o.Port())
	res, body := get(t, o, "GET", host, tokenPath(o))
	if res.StatusCode != 200 || body != "<p>doc</p>" {
		t.Fatalf("first GET = %d %q", res.StatusCode, body)
	}
	if res.Header.Get("Content-Security-Policy") != CSP || !res.Close {
		t.Errorf("headers = %v", res.Header)
	}
	if !o.Served() {
		t.Error("Served is false after a whole delivery")
	}
	if res2, _ := get(t, o, "GET", host, tokenPath(o)); res2.StatusCode != 404 {
		t.Errorf("a second GET answered %d, want 404", res2.StatusCode)
	}
}

func TestAWrongHostPathMethodOrUserIsA404ThatDoesNotSpendTheUse(t *testing.T) {
	var uid uint32 = uint32(os.Geteuid())
	state := peercred.Found
	o := newOrigin(t, func(_, _ netip.AddrPort) (uint32, peercred.State) { return uid, state })
	host := Host + ":" + fmtInt(o.Port())
	for _, c := range []struct {
		name, method, host, path string
		uid                      uint32
		st                       peercred.State
	}{
		{"wrong host", "GET", "127.0.0.1:" + fmtInt(o.Port()), tokenPath(o), uint32(os.Geteuid()), peercred.Found},
		{"bare host", "GET", Host, tokenPath(o), uint32(os.Geteuid()), peercred.Found},
		{"wrong path", "GET", host, "/nope", uint32(os.Geteuid()), peercred.Found},
		{"root path", "GET", host, "/", uint32(os.Geteuid()), peercred.Found},
		{"head", "HEAD", host, tokenPath(o), uint32(os.Geteuid()), peercred.Found},
		{"another user", "GET", host, tokenPath(o), uint32(os.Geteuid()) + 7, peercred.Found},
	} {
		uid, state = c.uid, c.st
		if res, _ := get(t, o, c.method, c.host, c.path); res.StatusCode != 404 {
			t.Errorf("%s answered %d, want 404", c.name, res.StatusCode)
		}
		if o.Served() {
			t.Fatalf("%s spent the use", c.name)
		}
	}
	if o.Refused() != 6 {
		t.Errorf("Refused = %d, want 6", o.Refused())
	}
	uid, state = uint32(os.Geteuid()), peercred.Found
	if res, body := get(t, o, "GET", host, tokenPath(o)); res.StatusCode != 200 || body == "" {
		t.Errorf("the real request after six refusals = %d", res.StatusCode)
	}
	// Root's and an unknown owner's connections are served (ADR-157's rule). A gone socket is NOT: this server
	// has just accepted the connection, so "no such socket" is a far end that closed before the lookup, and
	// that is how another user would spend the one use (P04 review).
	o2 := newOrigin(t, func(_, _ netip.AddrPort) (uint32, peercred.State) { return 99999, peercred.Unknown })
	if res, _ := get(t, o2, "GET", Host+":"+fmtInt(o2.Port()), tokenPath(o2)); res.StatusCode != 200 {
		t.Errorf("unknown owner answered %d, want 200", res.StatusCode)
	}
	og := newOrigin(t, func(_, _ netip.AddrPort) (uint32, peercred.State) { return 0, peercred.Gone })
	if res, _ := get(t, og, "GET", Host+":"+fmtInt(og.Port()), tokenPath(og)); res.StatusCode != 404 || og.Served() {
		t.Errorf("a gone socket answered %d (served %v), want 404 unspent", res.StatusCode, og.Served())
	}
	o3 := newOrigin(t, func(_, _ netip.AddrPort) (uint32, peercred.State) { return 0, peercred.Found })
	if res, _ := get(t, o3, "GET", Host+":"+fmtInt(o3.Port()), tokenPath(o3)); res.StatusCode != 200 {
		t.Errorf("root answered %d, want 200", res.StatusCode)
	}
}

func TestTheTokenIs256BitsAndTheListenerIsLoopbackOnly(t *testing.T) {
	a, b := newOrigin(t, nil), newOrigin(t, nil)
	if len(a.token) != 64 || a.token == b.token {
		t.Errorf("tokens %q %q", a.token, b.token)
	}
	if ap, _ := netip.ParseAddrPort(a.ln.Addr().String()); !ap.Addr().IsLoopback() || ap.Addr() != netip.MustParseAddr("127.0.0.1") {
		t.Errorf("listening on %s", a.ln.Addr())
	}
}

func TestCloseIsIdempotentAndStopsTheListener(t *testing.T) {
	o := newOrigin(t, nil)
	port := o.Port()
	o.Close()
	o.Close()
	if _, err := (&http.Client{Timeout: time.Second}).Get("http://127.0.0.1:" + fmtInt(port) + "/"); err == nil {
		t.Error("the listener still answers after Close")
	}
}

// TestTheBrowserIsLaunchedWithTheDoorsFlags holds Args: each required flag present, each forbidden one absent.
func TestTheBrowserIsLaunchedWithTheDoorsFlags(t *testing.T) {
	o := newOrigin(t, nil)
	args := strings.Join(o.Args("/p", "/o.pdf"), "\n")
	for _, must := range []string{"--headless", "--no-proxy-server", "--host-resolver-rules=MAP " + Host + " 127.0.0.1:", "MAP * ~NOTFOUND", "--user-data-dir=/p", "--print-to-pdf=/o.pdf",
		"--disable-extensions", "--no-first-run", "--disable-background-networking", "--disable-crash-reporter", "--disable-breakpad", "--disable-component-update", "--no-pings", "--disable-sync", "--no-pdf-header-footer", o.URL()} {
		if !strings.Contains(args, must) {
			t.Errorf("Args lacks %q", must)
		}
	}
	for _, never := range []string{"--no-sandbox", "--allow-file-access", "--disable-web-security", "file:", "--disable-site-isolation", "--enable-logging"} {
		if strings.Contains(args, never) {
			t.Errorf("Args contains %q", never)
		}
	}
}

func TestCloseFromTwoPlacesAtOnceDoesNotPanic(t *testing.T) {
	for i := 0; i < 50; i++ {
		o := newOrigin(t, nil)
		var wg sync.WaitGroup
		for j := 0; j < 4; j++ {
			wg.Add(1)
			go func() { defer wg.Done(); o.Close() }()
		}
		wg.Wait()
	}
}
