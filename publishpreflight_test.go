package nib

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestThePublishPreflightRefuses — /pending 505.
//
// `./build.sh --publish` pushed, tagged and uploaded binaries built from the working tree with no
// check that the tree matched the commit being tagged, or that the version being published was
// the one that commit's VERSION file carries. `build/publish-preflight.sh` is the refusal; this
// drives it in a throwaway repository — never this one, and never `build.sh` itself, which
// publishes — through each case, and requires the clean case to pass so a script that refuses
// everything cannot satisfy it.
func TestThePublishPreflightRefuses(t *testing.T) {
	for _, dep := range []string{"git", "bash"} {
		if _, err := exec.LookPath(dep); err != nil {
			t.Skipf("%s not installed", dep)
		}
	}
	script, err := filepath.Abs("build/publish-preflight.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("build/publish-preflight.sh is gone: %v", err)
	}

	dir := t.TempDir()
	home := t.TempDir()
	// A hermetic git: no global or system config (so no signing, hooks or templates of the
	// developer's), and a fixed identity.
	env := append(os.Environ(),
		"HOME="+home, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The machine half (/pending 670, 777): nfpm and gh are stubs this test can take away, on a PATH
	// that holds nothing else but the five tools the script runs — so a real gh in /usr/bin cannot
	// answer for a missing one — and `origin` is a bare repository beside the work tree.
	stubs, tools := t.TempDir(), t.TempDir()
	for _, tool := range []string{"git", "head", "tr", "tail", "cut"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s not installed", tool)
		}
		if err := os.Symlink(p, filepath.Join(tools, tool)); err != nil {
			t.Skipf("SKIP (not a pass): symlinks unavailable here: %v", err)
		}
	}
	stub := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	unstub := func(name string) {
		t.Helper()
		if err := os.Remove(filepath.Join(stubs, name)); err != nil {
			t.Fatal(err)
		}
	}
	const signedIn, signedOut = `[ "$1 $2" = "auth status" ]`, "exit 1"
	stub("nfpm", "exit 0")
	stub("gh", signedIn)
	bash, _ := exec.LookPath("bash")
	origin := filepath.Join(t.TempDir(), "origin.git")
	preflight := func(version string) (bool, string) {
		cmd := exec.Command(bash, script, version)
		cmd.Dir, cmd.Env = dir, append(env, "PATH="+stubs+string(os.PathListSeparator)+tools)
		out, err := cmd.CombinedOutput()
		return err == nil, string(out)
	}

	git("init", "-q")
	write("VERSION", "1.2.3\n")
	write("main.go", "package main\n")
	write(".gitignore", "local-only.go\n")
	git("add", "VERSION", "main.go", ".gitignore")
	git("commit", "-q", "-m", "base")
	git("init", "-q", "--bare", origin)
	git("remote", "add", "origin", origin)

	steps := []struct {
		name    string
		mutate  func()
		version string
		allow   bool
		why     string // a word the refusal must contain, so a refusal for another reason cannot pass
	}{
		{"a clean tree at the committed version", func() {}, "1.2.3", true, ""},
		{"an ignored local-only file is not a refusal", func() { write("local-only.go", "package main\n") }, "1.2.3", true, ""},
		{"a scratch note the build never reads is not a refusal", func() { write("notes.txt", "x") }, "1.2.3", true, ""},
		{"a version HEAD does not carry", func() {}, "1.2.4", false, "VERSION"},
		{"an uncommitted edit to a tracked file", func() { write("main.go", "package main // edited\n") }, "1.2.3", false, "tracked files differ"},
		{"a staged but uncommitted edit", func() { git("add", "main.go") }, "1.2.3", false, "tracked files differ"},
		{"after committing it, clean again", func() { git("commit", "-q", "-m", "edit") }, "1.2.3", true, ""},
		{"an untracked Go file the build would compile", func() { write("extra.go", "package main\n") }, "1.2.3", false, "untracked"},
		{"no nfpm on this machine", func() { git("add", "extra.go"); git("commit", "-q", "-m", "extra"); unstub("nfpm") }, "1.2.3", false, "nfpm not found"},
		{"no gh on this machine", func() { stub("nfpm", "exit 0"); unstub("gh") }, "1.2.3", false, "gh (GitHub CLI) not found"},
		{"gh signed out", func() { stub("gh", signedOut) }, "1.2.3", false, "not signed in"},
		{"both there and signed in, clean again", func() { stub("gh", signedIn) }, "1.2.3", true, ""},
		{"the tag already on origin, at an earlier commit", func() { git("push", "-q", "origin", "HEAD~1:refs/tags/v1.2.3") }, "1.2.3", false, "already exists on origin"},
		{"the tag on origin at HEAD is a re-publish", func() { git("push", "-q", "-f", "origin", "HEAD:refs/tags/v1.2.3") }, "1.2.3", true, ""},
		{"an annotated tag at an earlier commit", func() {
			git("tag", "-a", "-m", "old", "v1.2.3", "HEAD~1")
			git("push", "-q", "-f", "origin", "refs/tags/v1.2.3")
		}, "1.2.3", false, "already exists on origin"},
		{"an annotated tag at HEAD", func() {
			git("tag", "-f", "-a", "-m", "now", "v1.2.3", "HEAD")
			git("push", "-q", "-f", "origin", "refs/tags/v1.2.3")
		}, "1.2.3", true, ""},
		{"an origin that cannot be asked", func() { git("remote", "set-url", "origin", origin+"-gone") }, "1.2.3", false, "could not ask origin"},
	}
	for _, s := range steps {
		s.mutate()
		ok, out := preflight(s.version)
		if ok != s.allow {
			t.Fatalf("%s: preflight allowed=%v, want %v\n%s", s.name, ok, s.allow, out)
		}
		if !s.allow && !strings.Contains(out, s.why) {
			t.Fatalf("%s: refused, but not for this reason (want %q in the message):\n%s", s.name, s.why, out)
		}
	}

	// redproof.sh clears what a row may set before it sources the row (/pending 670): an EXPECT
	// left in the caller's environment graded a row that recorded none against an unrelated string.
	rp, err := os.ReadFile("build/redproof.sh")
	if err != nil {
		t.Fatal(err)
	}
	before, _, found := strings.Cut(string(rp), `. "$spec"`)
	if !found {
		t.Fatal("setup: build/redproof.sh no longer sources a row as `. \"$spec\"`")
	}
	for _, v := range []string{"PROVE", "TIER", "EXPECT"} {
		if !strings.Contains(before, v+`=""`) {
			t.Errorf("build/redproof.sh does not reset %s before sourcing a row, so the caller's own %s answers for a row that sets none", v, v)
		}
	}

	// The door: build.sh must run the preflight on --publish, and BEFORE it builds or pushes. A
	// preflight nothing calls refuses nothing.
	b, err := os.ReadFile("build.sh")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	call := strings.Index(src, "./build/publish-preflight.sh")
	build := strings.Index(src, "go build")
	push := strings.Index(src, "git push")
	if call < 0 {
		t.Fatal("build.sh does not call build/publish-preflight.sh, so --publish refuses nothing")
	}
	if build < 0 || push < 0 {
		t.Fatalf("setup: build.sh has no `go build` (%d) or `git push` (%d) to order the preflight against", build, push)
	}
	if call > build || call > push {
		t.Errorf("build.sh calls the publish preflight after it builds (%d > %d) or pushes (%d > %d) — the refusal must come first", call, build, call, push)
	}
}
