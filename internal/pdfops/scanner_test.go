package pdfops

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"os/exec"
	"strings"
	"testing"
)

func TestParseScanDevicesKeepsAPipeInTheDeviceName(t *testing.T) {
	out := "Brother|HL-L2395DW|multi-function peripheral|airscan:w1:Brother HL|2395\n" +
		"Noname|frontend-tester|virtual device|test:0\n" +
		"\n" +
		"x|y|z|-evil\n" + // an option-shaped name is not a device
		"short line\n"
	got := parseScanDevices(out)
	if len(got) != 2 {
		t.Fatalf("devices = %v, want 2", got)
	}
	if got[0].ID != "airscan:w1:Brother HL|2395" || got[0].Label != "Brother HL-L2395DW" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].ID != "test:0" || got[1].Label != "Noname frontend-tester" {
		t.Errorf("second = %+v", got[1])
	}
}

func TestValidScanDeviceName(t *testing.T) {
	for name, want := range map[string]bool{
		"test:0": true, "airscan:w1:Brother HL-L2395DW series": true,
		"": false, "-d": false, "--help": false, "a\nb": false, "a\x00b": false, strings.Repeat("a", scanMaxDeviceName+1): false,
	} {
		if got := validScanDeviceName(name); got != want {
			t.Errorf("validScanDeviceName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFeederSourceReadsTheSourceLineWhateverTheBackendCallsIt(t *testing.T) {
	for _, c := range []struct {
		name, help, want string
		ok               bool
	}{
		{"ADF", "    --source Flatbed|ADF [Flatbed]\n        Selects the scan source", "ADF", true},
		{"spelled out", "  --source Flatbed|Automatic Document Feeder [Flatbed]", "Automatic Document Feeder", true},
		{"a name with a space", "  --source flatbed|adf duplex [flatbed]", "adf duplex", true},
		{"glass only", "  --source Flatbed [Flatbed]", "", false},
		{"no source option", "  --mode Color|Gray", "", false},
	} {
		got, ok := feederSource(c.help)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: feederSource = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// scanTestDevice reports whether SANE's own virtual device is there; the tests that scan skip without it.
func scanTestDevice(t *testing.T) {
	t.Helper()
	if !ScannerAvailable() {
		t.Skip("scanimage is not installed")
	}
	if err := exec.Command("scanimage", "--device-name=test:0", "--help").Run(); err != nil {
		t.Skip("SANE's test device is not available: ", err)
	}
}

func TestAFlatbedScanOfTheTestDeviceIsOnePNGPage(t *testing.T) {
	scanTestDevice(t)
	pages, err := ScanPages(context.Background(), ScanOptions{Device: "test:0", DPI: 100, Mode: "Gray"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(pages))
	}
	if _, err := png.Decode(bytes.NewReader(pages[0])); err != nil {
		t.Errorf("the page is not a PNG: %v", err)
	}
}

func TestAFeederScanOfTheTestDeviceKeepsEveryPageAndTheOutOfPaperExit(t *testing.T) {
	scanTestDevice(t)
	pages, err := ScanPages(context.Background(), ScanOptions{Device: "test:0", DPI: 100, Mode: "Gray", Feeder: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 2 {
		t.Errorf("a feeder run gave %d page(s); the test device feeds more than one", len(pages))
	}
}

func TestAScanRefusesADeviceNameThatIsNotAName(t *testing.T) {
	_, err := ScanPages(context.Background(), ScanOptions{Device: "--version", DPI: 100, Mode: "Gray"})
	if !errors.Is(err, ErrScanDevice) {
		t.Errorf("err = %v, want ErrScanDevice", err)
	}
}
