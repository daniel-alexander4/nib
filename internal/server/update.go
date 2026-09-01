package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Update check. Nib never sends document data — the UI runs this once at startup
// (unless NIB_NO_UPDATE_CHECK is set) and from "Check for updates…". It compares
// the running version to the newest GitHub release and, when one is newer, links
// the release asset matching this machine's OS/arch. It downloads nothing on its
// own and replaces nothing.

// githubLatestURL is GitHub's "latest release" API for Nib. A package var so
// tests can point it at a stub.
var githubLatestURL = "https://api.github.com/repos/daniel-alexander4/nib/releases/latest"

type updateResponse struct {
	Current     string `json:"current"`
	Latest      string `json:"latest,omitempty"` // empty when no release is published yet
	Available   bool   `json:"updateAvailable"`
	URL         string `json:"url,omitempty"`         // release page
	DownloadURL string `json:"downloadUrl,omitempty"` // asset matching this OS/arch, if present
}

// **`Managed` was a FIELD here and is now a local (/pending 350, 2026-09-01).**
//
// It said whether Nib is installed under a system path — a dpkg install, so the asset is a .deb
// rather than a raw binary — and it was consumed by `assetURL` **inside this handler, before the
// response was written**, then serialised anyway. Named search: `managed` appears nowhere in
// `web/app.js` or `internal/cli/`.
//
// So the client was handed the INPUT to a choice the server had already made. The output it
// actually uses is `downloadUrl`, which was picked using this value; a second copy of the input
// tells it nothing it can act on, and the file extension is right there in the URL for a client
// that ever wants to say "this is a .deb".
//
// Found by the reader scan the first pass it could see this package (/pending 347), and it was in
// `test/jsdom/published.test.mjs`'s known-unread list before that with the reasoning already
// written out: *"a field consumed by the code that sets it has no consumer at the far end, which
// is the whole property here."* Deleting rather than surfacing follows this repo's own precedent —
// a reader scan has deleted two published-and-unread fields before.

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	rel, err := latestRelease()
	if err != nil {
		httpError(w, http.StatusBadGateway, "could not reach the update server")
		return
	}
	resp := updateResponse{Current: s.version}
	managed := managedInstall()
	if rel != nil {
		resp.Latest = strings.TrimPrefix(rel.Tag, "v")
		// Through httpOnly too. Its own doc names BOTH fields the client navigates to —
		// `location.assign(d.downloadUrl)` AND `window.open(d.url)` — and only one of them
		// went through it. Worse, they interact: httpOnly returning "" for a hostile asset
		// URL omits downloadUrl, and the client's else-branch then navigates to THIS field,
		// unchecked, from the same untrusted response. The guard routed the hostile case
		// into the field it did not guard.
		resp.URL = httpOnly(rel.URL)
		resp.Available = versionLess(resp.Current, resp.Latest)
		if resp.Available {
			resp.DownloadURL = assetURL(runtime.GOOS, runtime.GOARCH, managed, rel.Assets)
		}
	}
	writeJSON(w, resp)
}

type release struct {
	Tag    string
	URL    string
	Assets []asset
}

type asset struct {
	Name string
	URL  string
}

// latestRelease returns Nib's newest GitHub release, or (nil, nil) when none has
// been published yet (404).
func latestRelease() (*release, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no releases published yet
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errStatus(resp.StatusCode)
	}
	var raw struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	rel := &release{Tag: raw.TagName, URL: raw.HTMLURL}
	for _, a := range raw.Assets {
		rel.Assets = append(rel.Assets, asset{Name: a.Name, URL: a.URL})
	}
	return rel, nil
}

// assetURL picks the release asset for this OS/arch. Managed (dpkg) installs want
// the matching .deb (nib_<ver>_<goarch>.deb); standalone installs want the raw
// binary (nib-<ver>-<goos>-<goarch>[.exe]). It matches on the os/arch tokens
// rather than reconstructing the full name, so build.sh's naming can drift. Empty
// when nothing matches (the caller falls back to the release page).
func assetURL(goos, goarch string, managed bool, assets []asset) string {
	for _, a := range assets {
		if managed {
			if strings.HasSuffix(a.Name, "_"+goarch+".deb") {
				return httpOnly(a.URL)
			}
			continue
		}
		if strings.Contains(a.Name, "-"+goos+"-"+goarch) && !strings.HasSuffix(a.Name, ".deb") {
			return httpOnly(a.URL)
		}
	}
	return ""
}

// httpOnly returns u only if it is an http(s) URL, and "" otherwise.
//
// These come from a JSON response and go to the client, which does
// `location.assign(d.downloadUrl)` and `window.open(d.url)`. A `javascript:` URL in either
// field would execute in Nib's own origin, which holds the CSRF token and an unlocked
// vault. Nothing reaches that today — the fetch is TLS-verified to GitHub — so this closes
// a latent hole rather than a live break, and the caller's existing empty-string fallback
// (the release page) is already the right behaviour for a URL we will not hand on.
func httpOnly(u string) string {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") {
		return ""
	}
	return u
}

// versionLess reports whether semver a < b, comparing major.minor.patch
// numerically. Missing or non-numeric parts count as 0, so "dev" sorts below
// any release.
func versionLess(a, b string) bool {
	pa, pb := parseVer(a), parseVer(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func parseVer(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	for i, part := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(strings.TrimSpace(part))
	}
	return out
}

// managedInstall reports whether Nib is running from a system path (e.g. the
// dpkg-installed /usr/bin/nib), where updates come as a .deb from the package
// manager rather than a raw binary.
func managedInstall() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return strings.HasPrefix(exe, "/usr/")
}
