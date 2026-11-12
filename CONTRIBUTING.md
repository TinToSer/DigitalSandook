# Contributing

Thanks for your interest! Bug reports, ideas and pull requests are welcome.

## Ground rules

* **No third-party Go dependencies.** The project is standard-library-only on purpose (tiny, auditable, easy to
  cross-compile). Please keep it that way.
* **No frontend build step.** `web/` is plain HTML/CSS/JS embedded into the binary. The UI never uses
  `innerHTML` and the CSP forbids inline script/style - keep it that way.
* Security-sensitive changes (auth, sessions, `fsjail`, downloads, share links) need a test.

## Development

```
go run ./cmd/digitalsandook -share Docs=/some/folder      # run from source
go vet ./... && go test ./...                              # checks
gofmt -l .                                                 # must print nothing
bash scripts/build.sh dev                                  # build all platforms into dist/
```

Requires Go 1.25 or newer.

## Pull requests

1. Fork, create a branch, make focused commits.
2. Make sure `gofmt`, `go vet` and `go test ./...` pass (CI checks these on Linux, Windows and macOS).
3. Describe what changed and why. Add an entry under **Unreleased** in [CHANGELOG.md](CHANGELOG.md).

Please never commit real data: `dsd-data/`, certificates, keys, or logs from your own machine.

## Releasing (maintainers)

```
# 1. move "Unreleased" entries in CHANGELOG.md under a new version heading and commit
git tag v1.2.3
git push origin v1.2.3
```

The **Release** workflow tests, cross-builds all platforms, publishes archives + `SHA256SUMS.txt` (with build
provenance attestation) to a GitHub Release, and pushes a multi-arch Docker image to GHCR. A tag containing a
hyphen (`v1.2.3-rc.1`) is published as a pre-release and does not move the `latest` Docker tag.
