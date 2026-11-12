<div align="center">

<img src="web/logo.svg" width="96" alt="DigitalSanDook logo">

# DigitalSanDook

**Your own private file cloud in a single ~7 MB executable.**
Share any folder, then browse, upload, download and share files from a fast, light web UI -
no installer, no database, no runtime, no dependencies.

[![CI](https://github.com/TinToSer/DigitalSandook/actions/workflows/ci.yml/badge.svg)](https://github.com/TinToSer/DigitalSandook/actions/workflows/ci.yml)
[![CodeQL](https://github.com/TinToSer/DigitalSandook/actions/workflows/codeql.yml/badge.svg)](https://github.com/TinToSer/DigitalSandook/actions/workflows/codeql.yml)
[![Release](https://img.shields.io/github/v/release/TinToSer/DigitalSandook?sort=semver)](https://github.com/TinToSer/DigitalSandook/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go&logoColor=white)](go.mod)
![Dependencies](https://img.shields.io/badge/dependencies-0-brightgreen)
![Platforms](https://img.shields.io/badge/platforms-windows%20%7C%20linux%20%7C%20macos%20%7C%20bsd-lightgrey)

</div>

> *Sandook* (संदूक / صندوق) means "chest" or "box" in Hindi and Urdu - a digital chest for your files.

## Why DigitalSanDook?

* **One executable.** The web UI is embedded. Copy it anywhere and run it. No CGO, no database, zero third-party Go modules.
* **Fast.** Parallel, resumable, chunked uploads; zero-copy `sendfile` downloads with HTTP Range
  (resume, seek in video); streamed ZIP with no temp files; HTTP/2 with enlarged flow-control windows.
* **Hardened by default.** HTTPS out of the box, kernel-enforced folder jail, CSRF protection, strict CSP, brute-force lockout.
* **Multi-user with real access control.** Viewer / Editor / Owner per folder or file, re-shares that follow a chain of trust, public links with expiry and password.
* **Runs everywhere.** Windows, Linux (amd64, arm64, armv7, 386, riscv64, ppc64le, s390x), macOS (Intel + Apple Silicon), FreeBSD, OpenBSD, NetBSD - and a multi-arch Docker image.

**Interface:** list or grid view with photo thumbnails, sortable (remembered per browser), light / dark /
follow-system theme, status bar (item counts, sizes, selection), preview gallery for images, video, audio,
PDF and text with arrow-key navigation, drag & drop upload and move, keyboard shortcuts
(Ctrl+A/C/X/V, Delete, F2, Enter, Backspace, arrows in preview), mobile layout.

Thumbnails (JPEG, PNG, GIF) are made on the server with the standard library, rotated per EXIF, cached in
`dsd-data/thumbs` (max 512 MB, least-recently-used cleanup) and never written into shared folders. Images
over 40 megapixels or 64 MB are skipped, which also blocks decompression-bomb files.

## Install

### Download a release

Grab the archive for your platform from the [**Releases**](https://github.com/TinToSer/DigitalSandook/releases/latest)
page, verify it against `SHA256SUMS.txt`, unpack and run:

```
# Linux / macOS
tar -xzf DigitalSanDook-<version>-linux-amd64.tar.gz && cd DigitalSanDook-<version>-linux-amd64
./DigitalSanDook -share Projects=/srv/projects

# Windows (PowerShell) - after unzipping
.\DigitalSanDook.exe -share Projects=D:\Projects
```

Release files carry a [build provenance attestation](https://docs.github.com/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds):
`gh attestation verify <file> --repo TinToSer/DigitalSandook`.

### Docker

```
docker run -d --name digitalsandook -p 8443:8443 \
  -v dsd-data:/data \
  -v /srv/projects:/shares/projects \
  ghcr.io/tintoser/digitalsandook:latest -data /data -share Projects=/shares/projects

docker logs digitalsandook      # shows the one-time admin password
```

The image runs as an unprivileged user (uid 65532), so shared folders mounted into it must be accessible to that uid.
A ready-made [docker-compose.yml](docker-compose.yml) is included. To reset a lost password:
`docker run --rm -v dsd-data:/data ghcr.io/tintoser/digitalsandook resetpw -data /data admin`

### With Go

```
go install github.com/TinToSer/DigitalSandook/cmd/digitalsandook@latest
```

### From source

```
git clone https://github.com/TinToSer/DigitalSandook.git && cd DigitalSandook
go run ./cmd/digitalsandook -share Docs=/some/folder
```

## Quick start

1. Start it with at least one `-share` (see above). The first run prints a one-time `admin` password
   (you must change it at first login).
2. Open `https://localhost:8443` (accept the self-signed certificate warning once, or pass `-cert/-key`).
3. Admin panel -> **Shared folders** to add more folders (with a folder browser), **Users** to add people,
   **Access** to decide who can open what.

State (users, certificate, audit log) lives in `dsd-data/` next to the executable (`-data` to change).
Back it up by copying that folder; `state.json.bak` is kept automatically.

## Flags

| flag | meaning |
|---|---|
| `-addr :8443` | listen address |
| `-data DIR` | data folder |
| `-share name=/path` | share a folder at startup (repeatable; `-share /path` derives the name) |
| `-cert F -key F` | your TLS certificate (hot-reloaded when the files change, e.g. certbot renewals) |
| `-http` | plain HTTP - only on localhost or behind a TLS-terminating reverse proxy |
| `-trust-proxy` | trust `X-Forwarded-For` / `X-Forwarded-Proto` from your reverse proxy |

`DigitalSanDook resetpw <user>` sets a new random password (use it if the admin password is lost).
Stop the server first - `resetpw` refuses to run while the server is using the same data folder.
`DigitalSanDook version` prints the version.

## Access model

| level | can |
|---|---|
| Viewer | browse, preview, download |
| Editor | + upload, create, rename, move, copy, delete, **share** (up to their own level) |
| Owner | + manage / revoke everyone's access beneath the path |

* Administrators can do everything and manage users, folders, access, links, sessions, audit log.
* Access is granted per user on a whole shared folder **or any sub-folder/file** and inherits downward.
  There are no deny rules, so the effective right is always the highest grant.
* **Re-shares follow a chain of trust**: a grant only counts up to the level its granter *still* holds.
  Revoking, disabling or deleting someone automatically revokes everything they re-shared.
* Editors and Owners share from the file view (Share button): with other users, or with a
  **public read-only link** (optional expiry + password). A link stops working if its creator loses access.
* A shared folder can be marked **read-only** for everyone, including admins.
* Sharing is attached to the item, not to its name: grants and links **follow a renamed or moved item**, and
  **are removed when it is deleted**, so a new file or folder created later under an old name never inherits
  the old audience.

## Sharing links on the internet (Cloudflare Tunnel)

Public links normally only work for people who can reach the server. To make them work from anywhere
without port forwarding, enable **Admin -> Internet access**. DigitalSanDook launches and supervises
[`cloudflared`](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/)
(install it, or drop the executable next to DigitalSanDook / into `dsd-data`; `-cloudflared PATH` also works).

* **Quick tunnel** - free, no account; you get a random `https://....trycloudflare.com` address that changes on every start.
* **Named tunnel** - your own domain, permanent. Create the tunnel in Cloudflare Zero Trust, paste its token, set the
  public hostname's service to `http://localhost:8444`, and enter the public URL (e.g. `https://files.example.com`).

Links you copy from the Share dialog then use that address. The tunnel points at a separate, loopback-only
listener that serves **only** share links (`/s/...` and the matching API). The login page, file manager,
admin panel and every other API return 404 through the tunnel, and the visitor's real IP is taken from
Cloudflare for rate-limiting and the audit log. Use a link password and expiry for anything sensitive.

## Security design

* **Folder jail**: every file operation goes through Go's `os.Root` (openat / `RESOLVE_BENEATH`
  semantics). `..`, absolute paths, symlinks and Windows junctions cannot escape a shared folder, even
  through races. Symlinks/devices are hidden and never followed out. Users only ever see the share name, never real paths.
* **Passwords**: PBKDF2-HMAC-SHA256, 600 000 iterations, per-user salt, constant-time compare, equal
  work for unknown users. Minimum 10 characters. Admin-set passwords must be changed at first login.
* **Sessions**: 256-bit random tokens, only their SHA-256 is kept server side; `HttpOnly`, `Secure`,
  `SameSite=Strict`, `__Host-` prefix on HTTPS; 12 h idle / 14 d absolute timeout; changing a password
  signs out all other devices; admins can end sessions.
* **CSRF**: SameSite=Strict + per-session `X-CSRF-Token` + `Origin` / `Sec-Fetch-Site` checks on every state-changing call.
* **Brute force**: per-IP and per-account exponential lockout; hashing concurrency is bounded.
* **Browser hardening**: CSP `default-src 'none'; script-src 'self'` (no inline script/style), `nosniff`,
  frame denial, no referrer, HSTS. The UI never uses `innerHTML`. Downloads are `attachment` unless the
  type is on a small safe-preview list, and always carry a sandboxing CSP, so an uploaded HTML/SVG file can never run script on the app's origin.
* **Uploads** are written to a hidden scratch folder and atomically renamed, so half-written files are never visible
  and leftovers are removed at startup. The data folder cannot be shared (it holds password hashes and the TLS key).
* **Audit log** of logins, uploads, downloads, deletes, renames, grants, admin actions (`audit.log`, rotated at 20 MB).

Notes: a locked-out account can be unlocked by waiting (max 10 min) or restarting the server. Run it as an
unprivileged OS user that has access only to the folders you intend to share - the OS permissions of that user remain the final boundary.

## Build

Requires Go 1.25+ (older Go auto-downloads it via `go.mod`). Build with the latest Go patch release:
being stdlib-only, the binary's security fixes come from the Go toolchain.

```
powershell -ExecutionPolicy Bypass -File scripts\build.ps1 [-Version 1.2.3] [-SkipTests]   # Windows
bash scripts/build.sh [1.2.3]                                               # Linux / macOS
go run ./cmd/digitalsandook -share Docs=/some/folder                        # development
go test ./...                                                               # tests
```

The scripts run `go vet` and the tests, then write to `dist/`: Windows (amd64, arm64, 386), Linux (amd64, arm64,
armv7, 386, riscv64, ppc64le, s390x), macOS (Intel, Apple Silicon), FreeBSD (amd64, arm64), OpenBSD, NetBSD, plus
`SHA256SUMS.txt`. On Linux/macOS run `chmod +x` on the file you copy. If you run a binary from `dist/`, its
`dsd-data/` folder (password hashes, TLS private key) is created there - never ship that folder.

### Releasing

Pushing a version tag publishes a release automatically (see [CONTRIBUTING.md](CONTRIBUTING.md)):

```
git tag v1.2.3 && git push origin v1.2.3
```

CI runs the tests, cross-builds every platform, uploads archives + `SHA256SUMS.txt` (with build provenance) to
GitHub Releases and pushes a multi-arch image to `ghcr.io`.

## Layout

```
cmd/digitalsandook/   entry point: flags, `version`, `resetpw`
internal/
  server/             HTTP app: routes, security middleware, handlers (files, uploads, sharing, admin, tunnel)
  store/              users, shared folders, grants, links, settings (atomic JSON state file)
  auth/               password hashing, sessions, brute-force limiter
  fsjail/             os.Root folder jail, name validation, atomic placement, recursive copy
  thumb/              stdlib thumbnailer and cache cleanup
  audit/              append-only JSON-lines audit log
  tlsutil/            self-signed / hot-reloaded TLS certificates
  randx/              random tokens
web/                  browser UI (embedded into the binary via web/embed.go)
scripts/              cross-platform build + release packaging scripts
.github/              CI, CodeQL and release workflows, issue / PR templates
Dockerfile            minimal distroless image (multi-arch)
dist/                 build output (not source; git-ignored)
```

## Contributing & security

Contributions are welcome - see [CONTRIBUTING.md](CONTRIBUTING.md). Please report vulnerabilities privately as
described in [SECURITY.md](SECURITY.md). Changes are tracked in [CHANGELOG.md](CHANGELOG.md).

## License

[MIT](LICENSE)
