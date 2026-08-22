package portmap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// A mock IGD: an HTTP server serving the device-description XML and answering the SOAP control
// endpoint. This drives the untrusted-input half of UPnP end to end — the XML parse, the
// control-URL resolution, and the SOAP exchange — without needing SSDP multicast (which carries
// its own hermetic-test ceiling, like mDNS discovery). The router ASSIGNS the external IP the
// client reads, so a client that echoed its own input would be caught.
// anyHost permits any host, so the SOAP-mechanics tests can run against the loopback mock;
// the private-host POLICY is tested separately (TestIsPrivateHost).
func anyHost(string) bool { return true }

type mockIGD struct {
	srv     *httptest.Server
	extIP   string
	addOK   bool
	added   chan string // records the AddPortMapping SOAP body seen
	control string      // an override control URL to test the SSRF guard
}

func newMockIGD(t *testing.T) *mockIGD {
	t.Helper()
	m := &mockIGD{extIP: "9.9.9.9", addOK: true, added: make(chan string, 4)}
	mux := http.NewServeMux()
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)

	control := m.control
	if control == "" {
		control = m.srv.URL + "/ctl"
	}
	mux.HandleFunc("/desc.xml", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<?xml version="1.0"?><root xmlns="urn:schemas-upnp-org:device-1-0">`+
			`<device><deviceList><device><serviceList><service>`+
			`<serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>`+
			`<controlURL>%s</controlURL>`+
			`</service></serviceList></device></deviceList></device></root>`, control)
	})
	mux.HandleFunc("/ctl", func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		action := r.Header.Get("SOAPAction")
		switch {
		case strings.Contains(action, "AddPortMapping"):
			m.added <- string(body)
			if !m.addOK {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `<s:Envelope><s:Body><s:Fault><detail><UPnPError>`+
					`<errorCode>718</errorCode></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
				return
			}
			fmt.Fprint(w, `<s:Envelope><s:Body><u:AddPortMappingResponse></u:AddPortMappingResponse></s:Body></s:Envelope>`)
		case strings.Contains(action, "GetExternalIPAddress"):
			fmt.Fprintf(w, `<s:Envelope><s:Body><u:GetExternalIPAddressResponse>`+
				`<NewExternalIPAddress>%s</NewExternalIPAddress>`+
				`</u:GetExternalIPAddressResponse></s:Body></s:Envelope>`, m.extIP)
		}
	})
	return m
}

func (m *mockIGD) location() string { return m.srv.URL + "/desc.xml" }

func TestUPnPControlURLAndAddPortMapping(t *testing.T) {
	m := newMockIGD(t)
	client := &http.Client{Timeout: 2 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	controlURL, st, err := controlURLFor(ctx, client, m.location(), anyHost)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(controlURL, "/ctl") {
		t.Fatalf("control URL %q not resolved", controlURL)
	}
	internal := netip.MustParseAddr("192.168.1.50")
	if err := soapAddPortMapping(ctx, client, controlURL, st, UDP, internal, 40404, 40404, 120); err != nil {
		t.Fatal(err)
	}
	// The router assigns the external IP; a client that echoed its input would fail here.
	ext, err := soapGetExternalIP(ctx, client, controlURL, st)
	if err != nil {
		t.Fatal(err)
	}
	if ext.String() != m.extIP {
		t.Errorf("external IP %v, want %v", ext, m.extIP)
	}
	// The AddPortMapping request actually carried our internal client and port.
	select {
	case body := <-m.added:
		if !strings.Contains(body, "192.168.1.50") || !strings.Contains(body, "40404") {
			t.Errorf("AddPortMapping body did not carry the internal client/port: %s", body)
		}
	case <-time.After(time.Second):
		t.Error("the IGD never saw an AddPortMapping request")
	}
}

// An IGD that refuses (UPnPError) surfaces as ErrResultCode, not a silent success.
func TestUPnPRefusalIsSurfaced(t *testing.T) {
	m := newMockIGD(t)
	m.addOK = false
	client := &http.Client{Timeout: 2 * time.Second}
	ctx := context.Background()
	controlURL, st, err := controlURLFor(ctx, client, m.location(), anyHost)
	if err != nil {
		t.Fatal(err)
	}
	err = soapAddPortMapping(ctx, client, controlURL, st, UDP, netip.MustParseAddr("192.168.1.50"), 40404, 40404, 120)
	if !errors.Is(err, ErrResultCode) {
		t.Errorf("an IGD refusal was not surfaced as ErrResultCode: %v", err)
	}
}

// SSRF guard: a device description whose control URL points at a DIFFERENT host must be
// refused, or a hostile description on the link redirects our SOAP POST at an arbitrary target.
func TestUPnPRefusesAControlURLOnAnotherHost(t *testing.T) {
	m := newMockIGD(t)
	// Rebuild the description handler to advertise a control URL on a foreign host.
	m.srv.Config.Handler.(*http.ServeMux).HandleFunc("/evil.xml", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0"?><root><device><serviceList><service>`+
			`<serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>`+
			`<controlURL>http://169.254.169.254/latest/meta-data/</controlURL>`+
			`</service></serviceList></device></root>`)
	})
	client := &http.Client{Timeout: 2 * time.Second}
	_, _, err := controlURLFor(context.Background(), client, m.srv.URL+"/evil.xml", anyHost)
	if err == nil || !strings.Contains(err.Error(), "not the private device host") {
		t.Fatalf("a control URL on a foreign host (cloud metadata!) was not refused: %v", err)
	}
}

// The bounded read: extractTag and the LimitReader must not choke on or be exhausted by a large
// body. Here just the pure tag extractor, both plain and namespaced.
func TestExtractTag(t *testing.T) {
	if got := extractTag([]byte(`<a><NewExternalIPAddress>1.2.3.4</NewExternalIPAddress></a>`), "NewExternalIPAddress"); got != "1.2.3.4" {
		t.Errorf("plain tag: got %q", got)
	}
	if got := extractTag([]byte(`<u:errorCode>718</u:errorCode>`), "errorCode"); got != "718" {
		t.Errorf("namespaced tag: got %q", got)
	}
}

// The private-host policy (diff-grill #2). An IGD lives on the LAN; loopback, link-local
// (cloud metadata!) and public hosts are refused, and a hostname is refused outright.
func TestIsPrivateHost(t *testing.T) {
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.5.4", true},
		{"fc00::1", true},
		{"169.254.169.254", false}, // cloud metadata — the SSRF target
		{"127.0.0.1", false},       // loopback — internal services
		{"8.8.8.8", false},         // public
		{"fe80::1", false},         // link-local
		{"router.local", false},    // a hostname, not an IP
	} {
		if got := isPrivateHost(tc.host); got != tc.want {
			t.Errorf("isPrivateHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

// #2 end to end: controlURLFor refuses a LOCATION on a non-private host BEFORE fetching it, so
// an on-link SSDP responder cannot make us GET cloud metadata.
func TestUPnPRefusesANonPrivateLocation(t *testing.T) {
	client := igdHTTPClient()
	_, _, err := controlURLFor(context.Background(), client, "http://169.254.169.254/desc.xml", isPrivateHost)
	if err == nil || !strings.Contains(err.Error(), "not a private LAN address") {
		t.Fatalf("a LOCATION on the cloud-metadata address was not refused: %v", err)
	}
}

// #1: the client must NOT follow a redirect. A hostile IGD passes the host check with an on-host
// control URL, then 302s the SOAP POST to a foreign host; igdHTTPClient refuses to follow, so
// the call fails rather than reaching the redirect target.
func TestUPnPDoesNotFollowRedirects(t *testing.T) {
	var foreignHit bool
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer foreign.Close()

	igd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL+"/pwned", http.StatusFound)
	}))
	defer igd.Close()

	client := igdHTTPClient()
	err := soapAddPortMapping(context.Background(), client, igd.URL+"/ctl",
		"urn:schemas-upnp-org:service:WANIPConnection:1", UDP,
		netip.MustParseAddr("192.168.1.50"), 40404, 40404, 120)
	if err == nil {
		t.Error("a redirecting IGD did not produce an error — the redirect was followed")
	}
	if foreignHit {
		t.Error("the SOAP POST reached the redirect target — CheckRedirect did not refuse the hop (SSRF)")
	}
}

// #4: extractTag survives attributes on the element.
func TestExtractTagWithAttributes(t *testing.T) {
	body := []byte(`<u:Resp xmlns:u="x"><NewExternalIPAddress foo="bar">9.9.9.9</NewExternalIPAddress></u:Resp>`)
	if got := extractTag(body, "NewExternalIPAddress"); got != "9.9.9.9" {
		t.Errorf("extractTag with attributes: got %q, want 9.9.9.9", got)
	}
}
