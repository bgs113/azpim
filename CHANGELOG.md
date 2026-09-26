# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Homebrew: azpim is now published as a Cask (`Casks/azpim.rb` in `bgs113/homebrew-tap`) instead of a Formula, because GoReleaser deprecated Formula generation. `brew install bgs113/tap/azpim` and `brew upgrade azpim` keep working. The Cask clears macOS's quarantine flag on install, so Gatekeeper no longer blocks the binary.

## [1.1.0] - 2026-09-26

### Added
- `go install github.com/bgs113/azpim@latest` now works: the module path is `github.com/bgs113/azpim` (was `azpim`), so the Go module proxy and pkg.go.dev can find it ([#34](https://github.com/bgs113/azpim/issues/34)). Binaries built this way report their module version in `azpim version`.
- `activate`, `deactivate` and `extend` print the identity they act as (`Acting as alice@contoso.com (tenant …)`, or `service principal <appid>`) to stderr before changing anything, so a service-principal secret left in `AZURE_CLIENT_ID`/`AZURE_CLIENT_SECRET` can't elevate unnoticed. JSON on stdout is unchanged ([#29](https://github.com/bgs113/azpim/issues/29)).
- `--cloud public|usgov|china` (or `AZURE_CLOUD`) targets Azure US Government or Azure China instead of the public cloud. Sign-in, the ARM endpoint and the token scope all follow the chosen cloud ([#25](https://github.com/bgs113/azpim/issues/25)).
- `--dry-run` and `--validate-only` on `activate`, `deactivate` and `extend`. `--dry-run` shows what would be sent and sends nothing. `--validate-only` has Azure check the request against the role policy without creating it, and exits non-zero with Azure's reason if it fails. JSON output reports `"status": "DryRun"` or `"Validated"` ([#26](https://github.com/bgs113/azpim/issues/26)).
- `activate --start` schedules an activation for a later time, e.g. a maintenance window: `--start 22:00` (today, or tomorrow if that time has passed), `--start 2026-10-01T22:00` (local time), or RFC 3339. `azpim requests` shows such a request as `Scheduled` until it starts, and its JSON has a new `starts_at` field ([#28](https://github.com/bgs113/azpim/issues/28)).
- `--management-group` (`-m`) accepts a management group's display name as well as its ID, e.g. `-m "Production"`. Names are looked up among the management groups where you have an eligible or active assignment, so no extra permission is needed. An ID still costs no extra API calls, and an unknown or ambiguous name gives an error that lists the candidates and their IDs ([#31](https://github.com/bgs113/azpim/issues/31)).

### Changed
- `--subscription` no longer defaults to the `AZURE_SUBSCRIPTION_ID` environment variable. When it was set (often by `azd` or a deployment script), commands run without scope flags quietly looked at only that subscription, and `deactivate --all` left roles in other subscriptions active. Commands without scope flags now always cover the whole tenant. To keep the old behaviour, pass `-s "$AZURE_SUBSCRIPTION_ID"`.
- JSON output of `activate`, `extend` and `deactivate`: `status` now uses the same values as `azpim requests` (`Active`, `Pending`, …), and a new `azure_status` field carries Azure's raw status. `activate`'s pending value changes from `PendingApproval` to `Pending`. The time fields (`activated_at`, `extended_at`, `deactivated_at`, `expires_at`) are omitted until Azure confirms the change; `requested_at` is always set.
- With no scope flags, `eligible`, `active`, `requests`, `activate`, `deactivate` and `extend` now list assignments with one tenant-wide query instead of querying every subscription and management group separately. This is faster in large tenants and avoids ARM throttling. Role and scope names now come from the same response, so azpim no longer makes extra lookups for them ([#23](https://github.com/bgs113/azpim/issues/23)).
- `requests` now lists requests that target you (`asTarget()`) instead of requests you submitted (`asRequestor()`), which Azure rejects at the tenant root. For self-activation the results are the same. Requests an admin made on your behalf now appear too.

### Fixed
- `extend` no longer prints `✓ extended` when Azure only queued the request for approval: it says the request is pending and not in effect yet ([#32](https://github.com/bgs113/azpim/issues/32)). `activate` and `deactivate` now check Azure's status the same way. `activate` treated `PendingAdminDecision` and some other pending statuses as success, and `deactivate` never checked the status at all. Denied, failed and timed-out requests are reported as errors.
- The interactive picker in `extend` said "Select active role to deactivate"; it now says "to extend".
- `RoleAssignmentDoesNotExist` errors, which Azure returns when you deactivate or extend a role within about 5 minutes of activating it, now explain that and suggest waiting.
- `PendingRoleAssignmentRequest` errors now say a request for the role is already pending, and point to `azpim requests --pending`.
- The MEMBERSHIP column (`membership_type` in JSON) now shows Azure's value as-is. Before, anything other than `Group` was shown as `Direct`, so an assignment inherited from a parent scope (`Inherited`) looked like a direct assignment.
- `active --include-permanent` no longer hides a permanent assignment when you also have a group-based assignment for the same role and scope. Only the shadow Direct copies that Azure creates for group members are hidden now.
- A subscription or management group that failed to list is no longer dropped silently. There is now a single query, so a failure is reported as an error.

### Removed
- The on-disk caches `~/.cache/azpim/scopes.json` and `~/.cache/azpim/roledefs.json`. azpim no longer writes to `~/.cache/azpim/`, and the directory can be deleted. The Docker examples no longer mount it.

## [1.0.2] - 2026-09-05

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

[Unreleased]: https://github.com/bgs113/azpim/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/bgs113/azpim/compare/v1.0.2...v1.1.0
[1.0.2]: https://github.com/bgs113/azpim/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/bgs113/azpim/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/bgs113/azpim/releases/tag/v1.0.0
