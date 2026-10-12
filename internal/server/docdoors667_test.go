package server

import (
	"context"
	"testing"
)

// TestAPageSizeIsANumberInRange — `NaN` parses without error and is neither <= 0 nor > the
// ceiling, so a test written as refusals let it through to the assemble barrier (/pending 667).
func TestAPageSizeIsANumberInRange(t *testing.T) {
	for _, bad := range [][2]string{
		{"NaN", "NaN"}, {"NaN", "792"}, {"612", "nan"}, {"Inf", "792"}, {"612", "+Inf"},
		{"-Inf", "792"}, {"0", "792"}, {"612", "-1"}, {"14401", "792"}, {"", "792"}, {"abc", "792"},
	} {
		if w, h, ok := pageSize(bad[0], bad[1]); ok {
			t.Errorf("pageSize(%q, %q) = %v, %v, ok — a page cannot have that size", bad[0], bad[1], w, h)
		}
	}
	for _, good := range [][2]string{{"612", "792"}, {"0.5", "14400"}, {"1e2", "595.276"}} {
		if _, _, ok := pageSize(good[0], good[1]); !ok {
			t.Errorf("pageSize(%q, %q) refused a legal page", good[0], good[1])
		}
	}
}

// TestOnlyALinuxSystemPathIsAManagedInstall — the .deb is offered to a package-managed Linux
// binary and to nothing else (/pending 667).
func TestOnlyALinuxSystemPathIsAManagedInstall(t *testing.T) {
	for _, c := range []struct {
		goos, exe string
		want      bool
	}{
		{"linux", "/usr/bin/nib", true},
		{"linux", "/usr/lib/nib/nib", true},
		{"linux", "/usr/local/bin/nib", false},
		{"linux", "/home/u/bin/nib", false},
		{"linux", "/usrlocal/nib", false},
		{"darwin", "/usr/local/bin/nib", false},
		{"darwin", "/usr/bin/nib", false},
		{"freebsd", "/usr/bin/nib", false},
		{"windows", `C:\Program Files\Nib\nib.exe`, false},
	} {
		if got := managedPath(c.goos, c.exe); got != c.want {
			t.Errorf("managedPath(%q, %q) = %v, want %v", c.goos, c.exe, got, c.want)
		}
	}
}

// TestAFetchStatusAlwaysCarriesItsNumber — a code with no registered name used to render as
// "unexpected status " and nothing else (/pending 653).
func TestAFetchStatusAlwaysCarriesItsNumber(t *testing.T) {
	for code, want := range map[int]string{
		404: "unexpected status 404 Not Found",
		520: "unexpected status 520",
		599: "unexpected status 599",
	} {
		if got := errStatus(code).Error(); got != want {
			t.Errorf("errStatus(%d) = %q, want %q", code, got, want)
		}
	}
}

// TestADownloadThatEndsReleasesItsRequest — every terminal state cancels the request's context,
// including the ones that ended without a cancel (/pending 628).
func TestADownloadThatEndsReleasesItsRequest(t *testing.T) {
	for _, status := range []string{"done", "failed", "cancelled"} {
		ctx, cancel := context.WithCancel(context.Background())
		var d downloadState
		if !d.begin(cancel, "/tmp/x", 0) {
			t.Fatal("the slot was not free")
		}
		d.finish(status, "")
		if ctx.Err() == nil {
			t.Errorf("after finish(%q) the request's context is still live", status)
		}
		if d.stop() {
			t.Errorf("after finish(%q) there was still a download to stop", status)
		}
		cancel()
	}
}
