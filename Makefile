# Convenience wrapper over the build scripts and the third-party-notices
# generator. build.sh / install.sh remain the source of truth for cross-compile
# and packaging; this just gives the usual `make` entry points and guarantees
# the third-party notices are regenerated before anything gets packaged.

.PHONY: notices dist install clean

# Regenerate THIRD-PARTY-NOTICES.md from the modules linked into ./cmd/nib, the
# vendored pdf.js, and the Go toolchain. Run after changing go.mod or pdf.js.
notices:
	./build/gen-notices.sh

# Cross-compile every platform + build the .deb packages into dist/. Depends on
# notices so the file packaged into the .deb / uploaded with releases is fresh.
dist: notices
	./build.sh

# Build, package, and install Nib on this machine (adds it under the Office
# menu). Also regenerates notices first so the installed .deb carries them.
install: notices
	./install.sh

clean:
	rm -rf dist
