# Nib — project instructions

## Versioning
Bump the `VERSION` file on **every** change, in the same commit as the change.
It's a single semver line and is the source of truth for `build.sh`,
`install.sh`, and the `.deb` package version.

- **Patch** bump (e.g. `0.8.0` → `0.8.1`) for fixes, refactors, docs, and small
  changes — the default.
- **Minor** bump (e.g. `0.8.4` → `0.9.0`, reset patch) for a new user-facing
  feature.

## Local-only files
Some files under `internal/vault/` and `cmd/` are present only locally and held
out of git via `.git/info/exclude` (which is never pushed, so their names stay
out of the public repo). Never commit them (and never `git add -f` them); don't
assume a fresh clone has them.

## License
GPLv3 (see `LICENSE`). The project ships as-is, no warranty.
