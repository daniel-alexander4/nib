package portmap

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// UPnP-IGD is the third and last protocol D15's ladder tries, after PCP and NAT-PMP. It is what
// the majority of consumer routers actually have enabled, and it is also the most code and the
// most error-prone: SSDP multicast discovery, an HTTP fetch of an XML device description, and a
// SOAP AddPortMapping call — three untrusted-input surfaces where PCP/NAT-PMP had one binary
// exchange. The security posture is explicit rather than incidental:
//
//   - **No XXE.** Go's encoding/xml does not resolve external entities or DTDs, so a hostile
//     device description cannot read local files or make us fetch a URL through an entity.
//   - **No SSRF.** The control URL is taken from the device description, which is untrusted; it
//     is resolved against the LOCATION we discovered and REFUSED unless it names the same host.
//     Without this a malicious description on the link could aim our authenticated SOAP POST at
//     an arbitrary internal service.
//   - **Bounded reads.** Every response body is read through an io.LimitReader, so a router (or
//     something impersonating one on the link) cannot exhaust memory with an endless body.
//
// ssdpAddr is the SSDP multicast group and port; a discovery M-SEARCH goes here and IGDs answer
// with their device-description LOCATION.
const (
	ssdpAddr       = "239.255.255.250:1900"
	igdMaxBody     = 64 * 1024 // a device description or SOAP reply far exceeds any legitimate one
	upnpHTTPBudget = 2 * time.Second
)

// The two service types an IGD exposes a port-mapping control on, in preference order.
var igdServiceTypes = []string{
	"urn:schemas-upnp-org:service:WANIPConnection:1",
	"urn:schemas-upnp-org:service:WANPPPConnection:1",
}

// igdHTTPClient is the client every IGD fetch uses. **It refuses redirects** — a legitimate IGD
// serves its description and control endpoint directly, and following a redirect is the SSRF
// the host policy below cannot see: a hostile IGD passes the host check with an on-host control
// URL, then answers with `302 Location: http://169.254.169.254/…` and the default client would
// follow it. `http.ErrUseLastResponse` hands the 3xx back as the response, which `soapCall`/
// `httpGet` then reject as a non-200 (diff-grill #1).
func igdHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       upnpHTTPBudget,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// isPrivateHost is the SSRF host policy: an IGD lives on the LAN, so its LOCATION and control
// URL must name a PRIVATE address (RFC 1918 / ULA), never loopback, link-local or public.
//
// The diff-grill's #2 is why the LOCATION host — not only the control URL — is screened: an
// on-link attacker answers our M-SEARCH and controls BOTH the LOCATION we then GET and the
// control URL inside it, so tying the two together proves nothing. Link-local (`169.254/16`,
// `fe80::/10`) is refused explicitly because that is where cloud-metadata (`169.254.169.254`)
// lives; loopback because that is where a victim's own internal services live; public because a
// router's LAN side is never public. A hostname (not an IP) is refused — a real IGD advertises
// an IP LOCATION, and resolving a name would reopen the SSRF through DNS.
func isPrivateHost(host string) bool {
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	return ip.IsPrivate()
}

// discoverIGD sends an SSDP M-SEARCH and returns the LOCATION URLs of any IGDs that answer.
//
// **Hermetic-test ceiling, declared like internal/discovery's.** SSDP is UDP multicast to
// 239.255.255.250:1900, and a loopback multicast copy traverses the host firewall exactly as
// mDNS discovery does — so this cannot be driven on a default-deny developer box without a
// network namespace, and it is not unit-tested here. The parts that parse untrusted input —
// the device description and the SOAP reply — are split out below precisely so they ARE tested
// against a mock IGD without needing multicast. This function is the thin, untested socket half.
func discoverIGD(ctx context.Context) ([]string, error) {
	raddr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	msearch := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: " + ssdpAddr + "\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 1\r\n" +
		"ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n"
	if _, err := conn.WriteTo([]byte(msearch), raddr); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(upnpHTTPBudget)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetReadDeadline(deadline)

	var locations []string
	seen := map[string]bool{}
	buf := make([]byte, 2048)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break // deadline or error — return whatever answered
		}
		if loc := ssdpLocation(buf[:n]); loc != "" && !seen[loc] {
			seen[loc] = true
			locations = append(locations, loc)
		}
	}
	if len(locations) == 0 {
		return nil, ErrNoGateway
	}
	return locations, nil
}

// ssdpLocation pulls the LOCATION header out of an SSDP response. Pure, so the header parsing
// is testable without a socket.
func ssdpLocation(resp []byte) string {
	for _, line := range strings.Split(string(resp), "\r\n") {
		if len(line) >= 9 && strings.EqualFold(line[:9], "LOCATION:") {
			return strings.TrimSpace(line[9:])
		}
	}
	return ""
}

// igdDevice is the subset of the device-description XML we read: nested devices, each with a
// list of services carrying a serviceType and controlURL.
type igdDevice struct {
	Devices  []igdDevice  `xml:"deviceList>device"`
	Services []igdService `xml:"serviceList>service"`
}

type igdService struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

type igdRoot struct {
	XMLName xml.Name  `xml:"root"`
	Device  igdDevice `xml:"device"`
}

// controlURLFor fetches the device description at location and returns the absolute control URL
// and service type for a WAN connection service.
//
// The control URL is resolved against location and REFUSED if it names a different host — the
// SSRF guard. serviceType is returned because the SOAP action namespace is keyed on it.
func controlURLFor(ctx context.Context, client *http.Client, location string, hostOK func(string) bool) (controlURL, serviceType string, err error) {
	loc0, err := url.Parse(location)
	if err != nil {
		return "", "", err
	}
	// The LOCATION host itself is untrusted (diff-grill #2) — an on-link responder chose it.
	if !hostOK(loc0.Hostname()) {
		return "", "", fmt.Errorf("portmap: IGD LOCATION host %q is not a private LAN address", loc0.Hostname())
	}
	body, err := httpGet(ctx, client, location)
	if err != nil {
		return "", "", err
	}
	var root igdRoot
	if err := xml.Unmarshal(body, &root); err != nil {
		return "", "", fmt.Errorf("portmap: bad IGD description: %w", err)
	}
	loc, err := url.Parse(location)
	if err != nil {
		return "", "", err
	}
	for _, st := range igdServiceTypes {
		if svc := findService(&root.Device, st); svc != nil {
			ref, err := url.Parse(svc.ControlURL)
			if err != nil {
				continue
			}
			abs := loc.ResolveReference(ref)
			// SSRF guard: the control URL must live on the device we discovered AND that host
			// must pass the same private-LAN policy the LOCATION did.
			if abs.Hostname() != loc.Hostname() || !hostOK(abs.Hostname()) {
				return "", "", fmt.Errorf("portmap: IGD control URL host %q is not the private device host %q", abs.Hostname(), loc.Hostname())
			}
			return abs.String(), st, nil
		}
	}
	return "", "", fmt.Errorf("portmap: IGD exposes no WAN connection service")
}

// findService walks the (possibly nested) device tree for a service of the given type.
func findService(d *igdDevice, serviceType string) *igdService {
	for i := range d.Services {
		if d.Services[i].ServiceType == serviceType {
			return &d.Services[i]
		}
	}
	for i := range d.Devices {
		if svc := findService(&d.Devices[i], serviceType); svc != nil {
			return svc
		}
	}
	return nil
}

// soapAddPortMapping issues AddPortMapping and returns the external IP the IGD reports (via a
// follow-up GetExternalIPAddress). External port equals internal port for IGD (it does not
// choose one the way PCP/NAT-PMP do); a router that refuses the requested external port is a
// miss, not a negotiation.
// soapAddPortMapping POSTs AddPortMapping and reports whether the request was actually WRITTEN
// to the connection, which is a different fact from whether it succeeded.
//
// The distinction is the whole of /pending 257 on this path: a POST that was written and whose
// response was then lost may well have installed a mapping, and a delete handle must be kept for
// it — while a call that never left the host (an already-dead context, a dial failure) must NOT
// be recorded, because a UPnP delete is keyed on (external port, protocol) with no ownership
// check and would remove whatever else happens to hold that port.
func soapAddPortMapping(ctx context.Context, client *http.Client, controlURL, serviceType string, proto Protocol, internalIP netip.Addr, internalPort, externalPort uint16, leaseSec uint32) (wrote bool, err error) {
	body := fmt.Sprintf(`<u:AddPortMapping xmlns:u="%s">`+
		`<NewRemoteHost></NewRemoteHost>`+
		`<NewExternalPort>%d</NewExternalPort>`+
		`<NewProtocol>%s</NewProtocol>`+
		`<NewInternalPort>%d</NewInternalPort>`+
		`<NewInternalClient>%s</NewInternalClient>`+
		`<NewEnabled>1</NewEnabled>`+
		`<NewPortMappingDescription>`+upnpMappingDescription+`</NewPortMappingDescription>`+
		`<NewLeaseDuration>%d</NewLeaseDuration>`+
		`</u:AddPortMapping>`, serviceType, externalPort, soapProto(proto), internalPort, internalIP.String(), leaseSec)
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				wrote = true
			}
		},
	})
	_, err = soapCall(ctx, client, controlURL, serviceType, "AddPortMapping", body)
	return wrote, err
}

// soapGetExternalIP asks the IGD for its public IPv4.
func soapGetExternalIP(ctx context.Context, client *http.Client, controlURL, serviceType string) (netip.Addr, error) {
	resp, err := soapCall(ctx, client, controlURL, serviceType,
		"GetExternalIPAddress",
		fmt.Sprintf(`<u:GetExternalIPAddress xmlns:u="%s"></u:GetExternalIPAddress>`, serviceType))
	if err != nil {
		return netip.Addr{}, err
	}
	ip := extractTag(resp, "NewExternalIPAddress")
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("portmap: IGD returned no usable external IP")
	}
	return addr, nil
}

// soapDeletePortMapping removes a mapping — the delete form S07's lifecycle will call.
func soapDeletePortMapping(ctx context.Context, client *http.Client, controlURL, serviceType string, proto Protocol, externalPort uint16) error {
	body := fmt.Sprintf(`<u:DeletePortMapping xmlns:u="%s">`+
		`<NewRemoteHost></NewRemoteHost>`+
		`<NewExternalPort>%d</NewExternalPort>`+
		`<NewProtocol>%s</NewProtocol>`+
		`</u:DeletePortMapping>`, serviceType, externalPort, soapProto(proto))
	_, err := soapCall(ctx, client, controlURL, serviceType, "DeletePortMapping", body)
	return err
}

func soapProto(p Protocol) string {
	if p == TCP {
		return "TCP"
	}
	return "UDP"
}

// upnpMappingDescription is what Nib writes into every IGD mapping it creates, and what a
// delete requires to find there before it will remove one. One constant, two call sites — the
// string was a literal at the write site and nothing read it back (ADR-009).
const upnpMappingDescription = "Nib"

// soapCall POSTs one SOAP action and returns the response body, or an error carrying the IGD's
// UPnPError code when the router refuses.
func soapCall(ctx context.Context, client *http.Client, controlURL, serviceType, action, inner string) ([]byte, error) {
	env := `<?xml version="1.0"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" ` +
		`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` +
		inner + `</s:Body></s:Envelope>`

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL, strings.NewReader(env))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", fmt.Sprintf(`"%s#%s"`, serviceType, action))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, igdMaxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if code := extractTag(data, "errorCode"); code != "" {
			return nil, fmt.Errorf("%w: IGD UPnPError %s", ErrResultCode, code)
		}
		return nil, fmt.Errorf("%w: IGD HTTP %d", ErrResultCode, resp.StatusCode)
	}
	// **A fault carried on a 200 is still a refusal, and reading success from the absence of an
	// error is how it got in.** `soapAddPortMapping` never inspects the body — it infers success
	// from a nil error — so a device answering its UPnPError with 200 produced a confirmed
	// mapping out of a refused one: the loop recorded a delete handle, GetExternalIPAddress
	// succeeded (it is an unrelated action), and Nib published a signed record naming a public
	// port that was never forwarded. D19 then read havePortMap as true and told the user their
	// trouble was "most likely a firewall". No legitimate response to any action here carries an
	// errorCode, so its presence is decisive whatever the status line says.
	if code := extractTag(data, "errorCode"); code != "" {
		return nil, fmt.Errorf("%w: IGD UPnPError %s on a 200 response", ErrResultCode, code)
	}
	return data, nil
}

func httpGet(ctx context.Context, client *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("portmap: IGD description HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, igdMaxBody))
}

// extractTag returns the inner text of the first element with the given local name, tolerating
// attributes, whitespace and a namespace prefix (`<u:errorCode ...>`). The diff-grill's #4 was
// that the old literal `<local>` match missed `<NewExternalIPAddress foo="x">` and turned a
// successful reply into a miss. Only the two scalar SOAP replies use it.
func extractTag(body []byte, local string) string {
	b := body
	for {
		i := bytes.IndexByte(b, '<')
		if i < 0 {
			return ""
		}
		b = b[i+1:]
		if len(b) == 0 || b[0] == '/' || b[0] == '?' || b[0] == '!' {
			continue // closing tag, declaration or comment
		}
		j := bytes.IndexAny(b, " \t\r\n>/")
		if j < 0 {
			return ""
		}
		name := b[:j]
		if k := bytes.IndexByte(name, ':'); k >= 0 {
			name = name[k+1:] // drop the namespace prefix
		}
		if string(name) != local {
			continue
		}
		gt := bytes.IndexByte(b, '>')
		if gt < 0 || b[gt-1] == '/' {
			return "" // malformed, or self-closing (no content)
		}
		rest := b[gt+1:]
		if end := bytes.Index(rest, []byte("</")); end >= 0 {
			return string(rest[:end])
		}
		return ""
	}
}

// mapViaUPnP is the whole IGD path: discover, resolve the control URL, add the mapping, read
// the external IP. Returns the external IP:port on success.
//
// **It needs no gateway** (diff-grill #3): SSDP discovery is its own, so the tier is reachable
// even where `DefaultGateway` fails — including every non-Linux platform, where PCP/NAT-PMP are
// absent but UPnP is not. The internal client IP for `AddPortMapping` is determined per IGD by
// dialing its (private, screened) host and reading the local address on that path, so the caller
// supplies nothing about the local network.
//
// `locations` is how the IGD list is obtained, and it is a PARAMETER for the reason
// `feedCandidates` takes its interval and `punchLoop` its cadence: a policy that cannot be
// driven has no test. `discoverIGD` does real SSDP multicast — `client_test.go` refuses to drive
// it for exactly that reason, because it "would reach the developer's actual router" — so
// everything this loop decides was untestable until the seam existed (/pending 265): what happens
// when an IGD refuses, when one answers and then fails, and when a second location has to be
// tried after the first.
func mapViaUPnP(ctx context.Context, proto Protocol, internalPort uint16, leaseSec uint32, hostOK func(string) bool, record func(Mapping), locations func(context.Context) ([]string, error)) (ext netip.Addr, port uint16, controlURL, serviceType string, err error) {
	client := igdHTTPClient()
	locs, err := locations(ctx)
	if err != nil {
		return netip.Addr{}, 0, "", "", err
	}
	for _, loc := range locs {
		ctl, st, err := controlURLFor(ctx, client, loc, hostOK)
		if err != nil {
			continue
		}
		internalIP, err := internalIPToward(ctl)
		if err != nil {
			continue
		}
		wrote, addErr := soapAddPortMapping(ctx, client, ctl, st, proto, internalIP, internalPort, internalPort, leaseSec)
		// Record BEFORE reading the external IP, and record a written-but-unanswered POST too.
		// Two leaks close here: a 200 followed by a failed GetExternalIP (the mapping exists and
		// this loop used to `continue` past it, dropping ctl/st), and a POST whose response was
		// lost. A DEFINITIVE refusal is the one case not recorded — ErrResultCode means the IGD
		// said no, so nothing was installed, and 718 ConflictInMappingEntry specifically means
		// that external port belongs to somebody else.
		if addErr == nil || (wrote && !errors.Is(addErr, ErrResultCode)) {
			if record != nil {
				record(Mapping{
					Protocol: proto, InternalPort: internalPort, ExternalPort: internalPort,
					via: mechUPnP, upnpControlURL: ctl, upnpServiceType: st,
				})
			}
		}
		if addErr != nil {
			continue
		}
		ip, err := soapGetExternalIP(ctx, client, ctl, st)
		if err != nil {
			continue
		}
		return ip, internalPort, ctl, st, nil
	}
	return netip.Addr{}, 0, "", "", ErrNoMapping
}

// unmapViaUPnP deletes a UPnP mapping via its stored control URL — S07's delete for the IGD
// path. Best-effort, bounded by the http client's own timeout; a slow or absent IGD must not
// stall the caller's teardown, so the caller runs this under a short fresh context.
func unmapViaUPnP(ctx context.Context, controlURL, serviceType string, proto Protocol, externalPort uint16) error {
	client := igdHTTPClient()
	// ASK THE ROUTER WHETHER THIS IS OURS, and refuse if it will not say so.
	//
	// `DeletePortMapping` carries only (remote host, external port, protocol) — **no internal
	// client** — so IGD removes whichever host on the LAN holds that external port. That is
	// harmless when the mapping was created by us seconds earlier, and it stopped being only
	// that at /pending 257: a handle is now recorded for a POST that was written and never
	// answered, and a router that never actually installed it leaves us aiming a cross-host
	// destructive call at an external port somebody else may hold. The external port is the
	// internal one, a Linux ephemeral 32768-60999, which is exactly where a console or a
	// torrent client pins its UDP port.
	//
	// So the delete is identity-checked: the entry must carry the description this client
	// writes AND name this host as its internal client. Both, because a description alone is
	// a string any LAN device could also use, and an internal client alone would let us delete
	// a different mapping of our own that something else installed on our behalf.
	_, _, err := verifiedUPnPEntry(ctx, client, controlURL, serviceType, proto, externalPort)
	if err != nil {
		if errors.Is(err, errEntryNotOurs) {
			return err
		}
		// The IGD says there is no such entry (714), or it will not answer. Either way there is
		// nothing this call may safely remove. Not an error the caller acts on: a mapping that
		// is already gone is the outcome a delete wanted.
		return nil
	}
	return soapDeletePortMapping(ctx, client, controlURL, serviceType, proto, externalPort)
}

// errEntryNotOurs is the refusal: the IGD answered, and what it holds on that external port is
// not this host's Nib mapping.
var errEntryNotOurs = errors.New("portmap: that IGD mapping is not ours")

// verifiedUPnPEntry asks the IGD what it holds on an external port and answers only for an entry
// this host owns — returning that entry's lease.
//
// **One door for two callers (ADR-009).** `DeletePortMapping` carries (remote host, external
// port, protocol) and no internal client, so IGD acts on whichever host holds that port; a lease
// read-back is keyed identically and has the same exposure with a different consequence —
// recording a stranger's lease as ours would poison both the refresh cadence and the evidence
// /pending 258's gate turns on. The two checks were inline in the delete until a second caller
// needed them; they are here now and both routes come through.
//
// `leaseOK` distinguishes a lease that was PRESENT and parsed from one that was not. It has to:
// `extractTag` returns "" for a missing tag and for a body it could not parse alike, so a bare
// uint32 would report 0 — which the caller reads as "permanent" — for a parse failure.
func verifiedUPnPEntry(ctx context.Context, client *http.Client, controlURL, serviceType string, proto Protocol, externalPort uint16) (leaseSec uint32, leaseOK bool, err error) {
	desc, internalClient, lease, leaseOK, err := soapGetPortMappingEntry(ctx, client, controlURL, serviceType, proto, externalPort)
	if err != nil {
		return 0, false, err
	}
	if desc != upnpMappingDescription {
		return 0, false, fmt.Errorf("%w: external port %d is described %q, not %q",
			errEntryNotOurs, externalPort, desc, upnpMappingDescription)
	}
	mine, err := internalIPToward(controlURL)
	if err != nil {
		return 0, false, err
	}
	if internalClient != mine.String() {
		return 0, false, fmt.Errorf("%w: external port %d belongs to %s, not to this host (%s)",
			errEntryNotOurs, externalPort, internalClient, mine)
	}
	return lease, leaseOK, nil
}

// soapGetPortMappingEntry asks the IGD who owns an external port. It is what makes a UPnP delete
// safe to aim: the description this client wrote, and the LAN address the entry points at.
//
// An empty answer is NOT "ours by default" — extractTag returns "" for a missing tag and for a
// body it could not parse alike, so the caller compares for equality and refuses on anything else.
func soapGetPortMappingEntry(ctx context.Context, client *http.Client, controlURL, serviceType string, proto Protocol, externalPort uint16) (description, internalClient string, leaseSec uint32, leaseOK bool, err error) {
	resp, err := soapCall(ctx, client, controlURL, serviceType, "GetSpecificPortMappingEntry",
		fmt.Sprintf(`<u:GetSpecificPortMappingEntry xmlns:u="%s">`+
			`<NewRemoteHost></NewRemoteHost>`+
			`<NewExternalPort>%d</NewExternalPort>`+
			`<NewProtocol>%s</NewProtocol>`+
			`</u:GetSpecificPortMappingEntry>`, serviceType, externalPort, soapProto(proto)))
	if err != nil {
		return "", "", 0, false, err
	}
	lease, leaseOK := uint32(0), false
	if raw := strings.TrimSpace(extractTag(resp, "NewLeaseDuration")); raw != "" {
		if v, perr := strconv.ParseUint(raw, 10, 32); perr == nil {
			lease, leaseOK = uint32(v), true
		}
	}
	return strings.TrimSpace(extractTag(resp, "NewPortMappingDescription")),
		strings.TrimSpace(extractTag(resp, "NewInternalClient")), lease, leaseOK, nil
}

// internalIPToward returns this host's source address on the path to the IGD, by opening (but
// not writing) a UDP socket toward it — the standard no-traffic way to learn the local address
// the kernel would use, without enumerating interfaces.
func internalIPToward(controlURL string) (netip.Addr, error) {
	u, err := url.Parse(controlURL)
	if err != nil {
		return netip.Addr{}, err
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "80")
	}
	c, err := net.Dial("udp", host)
	if err != nil {
		return netip.Addr{}, err
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).AddrPort().Addr(), nil
}
