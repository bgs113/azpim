# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security
- Bump the indirect `golang.org/x/crypto` dependency to v0.56.0, fixing two SSH-channel deadlock DoS advisories ([GO-2026-6354](https://pkg.go.dev/vuln/GO-2026-6354), [GO-2026-6355](https://pkg.go.dev/vuln/GO-2026-6355)). azpim doesn't use the `ssh` package, so this wasn't reachable, but keeps the dependency clean of known issues.

## [1.0.1] - 2026-08-21

### Changed
- Publish the Homebrew tap as a formula (`brew install bgs113/tap/azpim`) instead of a cask. Casks re-apply the macOS quarantine attribute on install, which triggered a Gatekeeper block since the binary isn't Apple-notarized; formulas don't. Existing cask users should run `brew uninstall --cask azpim && brew install bgs113/tap/azpim`.
- Require Go 1.27+ to build from source. Drops the direct `github.com/google/uuid` dependency in favor of Go 1.27's stdlib `uuid` package.
- Replace GitHub's default CodeQL setup with a committed workflow (`.github/workflows/codeql.yml`), since default setup couldn't build with the new Go 1.27 requirement; add `actionlint` to CI to catch GitHub Actions workflow mistakes CodeQL's security-focused queries don't.
- Group Dependabot PRs to reduce noise from SHA-pinned actions.
- Bump `golangci/golangci-lint-action`.

### Removed
- Temporarily stop publishing the GHCR container image. `ko` (bundled in GoReleaser) fails to build when driven by a Go 1.27 compiler with `ko: azpim does not contain a valid local import path`, an upstream ko/Go 1.27 incompatibility unrelated to this repo. Will be re-enabled once ko ships a fix.

### Fixed
- Fetch full history in CI checkout so gitleaks can diff against older commits.

## [1.0.0] - 2026-08-16

First public release.

### Added
- `eligible`, `active`, `activate`, `deactivate`, and `extend` commands for Azure PIM role assignments.
- `requests` command to list pending PIM schedule requests.
- `version` command and `--version` flag.
- Accept subscription name in addition to ID for `--subscription`.
- Homebrew tap publishing and macOS install instructions.
- Credential and role-definition-name caching, with disk persistence and singleflight deduplication.
- Concurrent eligible/active fetches and role/scope name resolution, with per-role/scope caching of max-activation-duration lookups.
- Container images built with Ko and published to GHCR.
- Readable, collapsed error messages for Azure SDK failures.
- Accurate pending-approval reporting in `activate` (no false success on pending requests).
- Per-scope error surfacing so a partial failure across scopes isn't hidden.
- Principal ID resolved before prompts, so auth failures surface immediately instead of after input.
- Activation durations rounded to the nearest minute when building ISO 8601 durations.

### Security
- Pin GitHub Actions to commit SHAs; add minimal workflow permissions and gitleaks scanning.
- Sign release binaries and container images with Cosign; publish SBOMs.
- Add MIT license.

[Unreleased]: https://github.com/bgs113/azpim/compare/v1.0.1...HEAD
[1.0.1]: https://github.com/bgs113/azpim/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/bgs113/azpim/releases/tag/v1.0.0
