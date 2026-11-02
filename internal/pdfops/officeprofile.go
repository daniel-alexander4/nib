package pdfops

import (
	"os"
	"path/filepath"
)

// The LibreOffice profile each conversion starts from (PLAN-import-export D33, R5, ADR-185).
//
// A conversion runs under its own throwaway profile (so concurrent runs do not clash on the
// single-instance profile, and it reads nothing of the user's real LibreOffice configuration). That
// profile started EMPTY, and an empty profile is LibreOffice's defaults: it runs with the stock macro
// security and — **measured 2026-11-01 with a loopback listener** — it FETCHES a document's external
// resources while converting. A DOCX whose picture is `<a:blip r:link="http://127.0.0.1:PORT/x.png">`
// made two requests to the listener before the PDF was written. A document that makes this machine
// fetch a URL it names is an egress channel (README: every editing feature works with no network), and
// opening a Word file someone emailed is exactly where it happens.
//
// # What was tried, and what the canary said
//
// The plan's mechanism was "links never updating" (`Content/Update/Link` = 2 in Writer and Calc). It was
// RUN against the listener and the fetch still happened: that setting governs field and DDE links, not a
// linked picture. What stopped it is a manual proxy that nothing listens on — every HTTP, HTTPS and FTP
// request LibreOffice makes goes to 127.0.0.1:9 and is refused, with no exemption for any host. The
// listener saw 0 requests. `TestLibreOfficeFetchesNothingWhileConverting` holds that, with a positive
// control that runs the same document under an empty profile and requires the listener to be hit.
//
// # What it does not cover, declared
//
// A linked file on the LOCAL disk (`file:///…`) is not a network fetch and the proxy does not stop it;
// that is the package scan of X11(a), owned by P04.S01. Macros: the security level is the highest and
// macro execution is disabled, which is belt-and-braces over headless LibreOffice's default of not
// running them.
const officeRegistry = `<?xml version="1.0" encoding="UTF-8"?>
<oor:items xmlns:oor="http://openoffice.org/2001/registry" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<item oor:path="/org.openoffice.Office.Common/Security/Scripting"><prop oor:name="MacroSecurityLevel" oor:op="fuse"><value>3</value></prop></item>
<item oor:path="/org.openoffice.Office.Common/Security/Scripting"><prop oor:name="DisableMacrosExecution" oor:op="fuse"><value>true</value></prop></item>
<item oor:path="/org.openoffice.Office.Writer/Content/Update"><prop oor:name="Link" oor:op="fuse"><value>2</value></prop></item>
<item oor:path="/org.openoffice.Office.Calc/Content/Update"><prop oor:name="Link" oor:op="fuse"><value>2</value></prop></item>
<item oor:path="/org.openoffice.Inet/Settings"><prop oor:name="ooInetProxyType" oor:op="fuse"><value>2</value></prop></item>
<item oor:path="/org.openoffice.Inet/Settings"><prop oor:name="ooInetHTTPProxyName" oor:op="fuse"><value>127.0.0.1</value></prop></item>
<item oor:path="/org.openoffice.Inet/Settings"><prop oor:name="ooInetHTTPProxyPort" oor:op="fuse"><value>9</value></prop></item>
<item oor:path="/org.openoffice.Inet/Settings"><prop oor:name="ooInetHTTPSProxyName" oor:op="fuse"><value>127.0.0.1</value></prop></item>
<item oor:path="/org.openoffice.Inet/Settings"><prop oor:name="ooInetHTTPSProxyPort" oor:op="fuse"><value>9</value></prop></item>
<item oor:path="/org.openoffice.Inet/Settings"><prop oor:name="ooInetFTPProxyName" oor:op="fuse"><value>127.0.0.1</value></prop></item>
<item oor:path="/org.openoffice.Inet/Settings"><prop oor:name="ooInetFTPProxyPort" oor:op="fuse"><value>9</value></prop></item>
<item oor:path="/org.openoffice.Inet/Settings"><prop oor:name="ooInetNoProxy" oor:op="fuse"><value></value></prop></item>
</oor:items>
`

// seedOfficeProfile writes officeRegistry into a fresh profile directory before LibreOffice first reads
// it. A failure is a refusal of the conversion: running without the containment is the thing this exists
// to prevent, so it is never a "log and carry on".
func seedOfficeProfile(profile string) error {
	user := filepath.Join(profile, "user")
	if err := os.MkdirAll(user, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(user, "registrymodifications.xcu"), []byte(officeRegistry), 0o600)
}
