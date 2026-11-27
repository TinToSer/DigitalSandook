# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/),
and the project uses [Semantic Versioning](https://semver.org/).

## [1.1.0]

### Security
* Added built-in zero-dependency **TOTP 2FA (RFC 6238)** with replay protection, 8 single-use recovery codes, and admin 2FA reset.
* Added **HMAC-SHA256 hash-chained audit logging** with integrity verification (`GET /api/admin/audit/verify`).
* Hardened uploads with size caps, per-user & global FD caps (checked before file creation), `ioMu` locking against concurrent abort/finish races, mid-upload permission re-checks, and optional SHA-256 header verification.
* Made grant/link path matching (`store.Under`, `PathEqual`) OS-aware (case-insensitive on Windows/macOS, exact on Linux) and prevented silent case-rename overwrites on Linux.
* Hardened CSRF `Origin` (scheme-aware) and tunnel `Sec-Fetch-Site` checks, restricted `-trust-proxy` to loopback/private proxy IPs, added HSTS `includeSubDomains`, rotated sessions on password change, and capped active sessions.
* Prevented locked public links from leaking filenames/metadata before unlock, isolated anonymous ZIP downloads into `pubZipSem` with per-write deadlines, and added per-token link brute-force limiting.
* Replaced global single-IP account lockout DoS with `(IP, user)` lockouts and bounded limiter eviction.

### Performance
* Rewrote thumbnail downscaling with a **2-pass separable area-averaging filter** and direct pixel-slice fast paths (`YCbCr`, `RGBA`, `NRGBA`, `Paletted`), pooled readers, singleflight deduplication, and lazy LRU `Chtimes`.
* Upgraded the upload engine with **true resumable chunked uploads**, adaptive chunk sizing (8–64 MiB), higher small-file concurrency, Windows NTFS sparse temp files, and in-place DOM progress updates.
* Pooled `gzip.Writer` and 1 MiB ZIP `bufio.Writer`, removed synchronous `fsync` on logins, and eliminated O(N log N) string allocations in directory sorting.

### Added
* **Recycle Bin (Trash)** with 30-day automatic retention, one-click restore, and Empty Trash.
* **File-Drop (upload-only)** and **max-uses (burn-after-N)** public share links.
* **Cross-mount copy and move**.
* **In-browser text/code/markdown editor** with optimistic-locking conflict detection.
* **SHA-256 checksum** viewer in the file context menu.
* Tested reverse-proxy (nginx / Caddy) guide.

## [1.0.0]

Initial public release: multi-user web file manager in a single dependency-free binary, with kernel-enforced
folder jail, per-folder access control with chain-of-trust re-sharing, public share links (expiry + password),
resumable chunked uploads, streamed ZIP downloads, thumbnails, audit log and optional Cloudflare Tunnel for
share links.
