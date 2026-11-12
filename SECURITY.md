# Security policy

DigitalSanDook is a file server that can be exposed to the internet, so security reports are taken seriously.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**
Use GitHub's private reporting instead: go to the **Security** tab of this repository and choose
**Report a vulnerability** (<https://github.com/TinToSer/DigitalSandook/security/advisories/new>).

Include what you found, how to reproduce it, and the version (`DigitalSanDook version`). You can expect an
acknowledgement within a few days. Fixes are released as a new patch version and credited in the release notes
unless you prefer to stay anonymous.

## Supported versions

Only the latest release receives security fixes.

## Hardening checklist for operators

* Run as an unprivileged OS user that can access only the folders you intend to share.
* Keep the program built with a current Go release (the binary is stdlib-only; Go fixes are its security fixes).
* Put it behind HTTPS (built in, or a TLS-terminating proxy together with `-trust-proxy`).
* Keep `dsd-data/` private and backed up - it holds password hashes and the TLS private key.
* Use a password and an expiry on public share links.
