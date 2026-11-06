package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"nib/internal/pdfops"
)

func needScannerTestDevice(t *testing.T) {
	t.Helper()
	if !pdfops.ScannerAvailable() {
		t.Skip("scanimage is not installed")
	}
	if err := exec.Command("scanimage", "--device-name=test:0", "--help").Run(); err != nil {
		t.Skip("SANE's test device is not available: ", err)
	}
}

// getForCode is a GET that returns the status and the body.
func getForCode(t *testing.T, c *http.Client, url string) (int, string) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

// TestAScanInstallsOneNewUnsavedDocument is /pending 903 end to end over SANE's own virtual scanner: the route
// scans, the page goes through the picture door, and the result is a new document with no path behind it.
func TestAScanInstallsOneNewUnsavedDocument(t *testing.T) {
	needScannerTestDevice(t)
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	srv.scanMu.Lock()
	srv.scanKnown = map[string]bool{"test:0": true}
	srv.scanMu.Unlock()

	code, body := postForCode(t, c, csrf, ts.URL+"/api/scanner/scan", scannerRequest{Device: "test:0", DPI: 100, Mode: "Gray"})
	if code != http.StatusOK {
		t.Fatalf("scan = %d %s", code, body)
	}
	var resp docResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.Name, "Scan ") || !strings.HasSuffix(resp.Name, ".pdf") {
		t.Errorf("document name = %q, want Scan <time>.pdf", resp.Name)
	}
	srv.mu.Lock()
	n := len(srv.docs)
	var path string
	for _, d := range srv.docs {
		path = d.path
	}
	srv.mu.Unlock()
	if n != 1 || path != "" {
		t.Errorf("documents = %d, path = %q; want one document with no path (a scan is not a file until it is saved)", n, path)
	}
}

// TestAScanOfAFeederKeepsEveryPageAsOneDocument joins the pages, which is the part the single page cannot show.
func TestAScanOfAFeederKeepsEveryPageAsOneDocument(t *testing.T) {
	needScannerTestDevice(t)
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	srv.scanMu.Lock()
	srv.scanKnown = map[string]bool{"test:0": true}
	srv.scanMu.Unlock()
	code, body := postForCode(t, c, csrf, ts.URL+"/api/scanner/scan", scannerRequest{Device: "test:0", DPI: 100, Mode: "Gray", Feeder: true})
	if code != http.StatusOK {
		t.Fatalf("scan = %d %s", code, body)
	}
	srv.mu.Lock()
	var data []byte
	for _, d := range srv.docs {
		data = d.data
	}
	srv.mu.Unlock()
	pages, err := pdfops.PageCount(data)
	if err != nil {
		t.Fatal(err)
	}
	if pages < 2 {
		t.Errorf("a feeder scan gave %d page(s) in the document, want the feeder's whole stack", pages)
	}
}

// TestAScanRefusesWhatTheListDidNotGive holds the route's own checks: a device the last list did not give, a mode
// and a resolution the window does not offer. None of them reaches the tool.
func TestAScanRefusesWhatTheListDidNotGive(t *testing.T) {
	if !pdfops.ScannerAvailable() {
		t.Skip("scanimage is not installed")
	}
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	srv.scanMu.Lock()
	srv.scanKnown = map[string]bool{"test:0": true}
	srv.scanMu.Unlock()
	for name, req := range map[string]scannerRequest{
		"a device never listed":    {Device: "pnm:/etc/hostname", DPI: 100, Mode: "Gray"},
		"an option as the device":  {Device: "--version", DPI: 100, Mode: "Gray"},
		"a mode not offered":       {Device: "test:0", DPI: 100, Mode: "Binary"},
		"a resolution not offered": {Device: "test:0", DPI: 99999, Mode: "Gray"},
	} {
		if code, body := postForCode(t, c, csrf, ts.URL+"/api/scanner/scan", req); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, code, body)
		}
	}
	srv.mu.Lock()
	n := len(srv.docs)
	srv.mu.Unlock()
	if n != 0 {
		t.Errorf("a refused scan installed %d document(s)", n)
	}
}

// TestTheScannerListAnswersAListAndStatusNamesTheTool: the window shows the Scan… button from the status flag
// and fills the dialog from the list.
func TestTheScannerListAnswersAListAndStatusNamesTheTool(t *testing.T) {
	ts, srv := startServerWith(t)
	c, _ := authedClient(t, ts)
	if got := srv.currentStatus().Scanner; got != pdfops.ScannerAvailable() {
		t.Errorf("status scanner = %v, the tool is found = %v", got, pdfops.ScannerAvailable())
	}
	code, body := getForCode(t, c, ts.URL+"/api/scanner/devices")
	if !pdfops.ScannerAvailable() {
		if code != http.StatusNotFound {
			t.Errorf("without scanimage the list is %d %s, want 404", code, body)
		}
		return
	}
	if code != http.StatusOK {
		t.Fatalf("list = %d %s", code, body)
	}
	var out struct {
		Devices     []pdfops.ScanDevice `json:"devices"`
		Modes       []string            `json:"modes"`
		Resolutions []int               `json:"resolutions"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("not JSON: %v: %s", err, body)
	}
	if out.Devices == nil || len(out.Modes) == 0 || len(out.Resolutions) == 0 {
		t.Errorf("list = %+v: devices must be a list (possibly empty), and the choices must be there", out)
	}
}
