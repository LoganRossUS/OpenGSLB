# Security Policy

## Supported Versions

Security fixes are released as a new patch release of the latest minor version. Older minor versions do not receive fixes: upgrade to the latest release.

| Version | Supported |
|---------|-----------|
| 1.1.x (latest release) | Yes |
| Older than 1.1 | No |

The `latest` container image tag at `ghcr.io/loganrossus/opengslb` follows the `main` branch. Images and binaries of older releases are not rebuilt when a base image or the Go toolchain receives a fix.

## Reporting a Vulnerability

Please do not report security problems in public issues, discussions, Discord or Reddit.

Report them through GitHub private vulnerability reporting:

1. Open the [Security tab](https://github.com/LoganRossUS/OpenGSLB/security) of the repository.
2. Choose **Report a vulnerability**. This opens a private advisory draft that only you and the maintainers can read. The direct link is <https://github.com/LoganRossUS/OpenGSLB/security/advisories/new>.

Include as much of the following as you can:

- The affected version (`opengslb --version`) or commit, and the mode (overwatch or agent).
- The relevant part of the configuration, with secrets removed.
- Steps to reproduce, or a proof of concept. For DNS issues a `dig` command or a packet capture is ideal.
- What an attacker gains: for example wrong answers, a crash, access to the API, or a forged health state.

## What to Expect

- An acknowledgment in the advisory thread within 7 days.
- An assessment of the report, and a plan for the fix if it is confirmed, discussed with you in the same thread.
- A fix developed in private, released as a patch release, and published together with a GitHub security advisory and a CHANGELOG entry. You are credited in the advisory unless you ask not to be.
- Please give us up to 90 days from your report before you disclose the issue publicly. We will tell you if we need less or more time and why.

## Scope

In scope:

- The `opengslb` and `opengslb-cli` binaries built from this repository.
- The container image published from this repository.
- The bootstrap scripts shipped with a release (`scripts/bootstrap-linux.sh`, `scripts/bootstrap-windows.ps1`).

Out of scope:

- The demo environments under `demos/`. They are for local evaluation and are not hardened.
- Vulnerabilities that require a deployment that ignores the hardening guidance, for example an API bound to a public address with no authentication. See [docs/security/api-hardening.md](docs/security/api-hardening.md) and [docs/operations/security/hardening.md](docs/operations/security/hardening.md). Reports that show the defaults or the guidance to be unsafe are welcome.

## How the Project Tracks Known Vulnerabilities

- `govulncheck` runs on every pull request and once a week against `main`. It fails when the code calls a function with a published advisory, in the Go standard library or in a dependency.
- CodeQL analyzes the Go code on every pull request and on pushes to `main` and `develop`.
- The container image is scanned with Trivy before it is published. A HIGH or CRITICAL finding stops the publication.
- Dependabot opens pull requests for Go module, GitHub Actions and base image updates.
