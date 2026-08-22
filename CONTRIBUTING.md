# Contributing

Thanks for considering a contribution to azpim.

## Reporting bugs

Open a [GitHub issue](https://github.com/bgs113/azpim/issues) with:

- What you ran and what you expected vs. what happened
- `azpim version` output and OS/architecture
- Steps to reproduce, if possible

For security vulnerabilities, see [SECURITY.md](SECURITY.md) instead of opening a public issue.

## Suggesting features

Open an issue describing the use case before writing code — for anything beyond a small fix, it's worth agreeing on the approach first so the work isn't wasted.

## Submitting changes

1. Fork the repo and create a branch from `main`.
2. Make your change. Keep it scoped to the issue at hand.
3. Add or update tests for the behavior you changed.
4. Run locally before opening a PR:
   ```bash
   go test ./...
   make build
   ```
5. If the change is user-facing (Added/Changed/Fixed/Removed/Security), add an entry under `## [Unreleased]` in `CHANGELOG.md` — see [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
6. Open a pull request. CI must pass: tests, `golangci-lint`, `govulncheck`, `gitleaks`, and CodeQL all run on every PR.

## Commit messages

This repo uses [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/): `type(scope): description`, imperative mood, lowercase description. Common types: `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`. Scope is optional (e.g. `fix(activate): ...`).

## Code style

Go code is formatted with `gofmt` and linted with `golangci-lint` (see `.golangci.yml`) — CI enforces both.
