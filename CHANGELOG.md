# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Built-in `update` command to check GitHub Releases, verify `checksums.txt`, and self-update the current binary with `--dry-run` / `--confirm` agent workflow support.
- Global `--format json|text|raw` output selector. JSON is now the default; `--json` remains as a compatibility alias for `--format json`, while `raw` is reserved for unwrapped bytes/logs/diffs.

### Changed

- Documentation and skill references now recommend default JSON plus `--compact`, with `--format text` only for human-readable output.

## [1.1.0] - 2026-05-26

### Fixed

- **Auth / profiles** — `auth profile use` no longer resets active profile to `default`; `auth logout` clears both `config.json` and `profiles.json`.
- **Agent-safe dry-run** — delete commands (`repo file/branch delete`, `release delete`, `mr comment delete`) run `--dry-run` before `--confirm`, matching the SKILL workflow.
- **CI waits** — `job log --follow` and `pipeline wait` treat `manual` as a terminal state (no infinite polling).
- **MR create** — `--assignee` with unknown user returns exit 4 (`NOT_FOUND`) instead of silently skipping.
- **Pipeline create** — invalid `--variable` format (missing `=`) returns validation error instead of being silently dropped.
- **auth status** — `source` field reports `profile` / `file` / env correctly via `authStatusSource()`.

### Changed

- **README / README_zh** — align `--force` vs `--confirm` with agent-safe defaults; fix `doctor --fields` example; document profile precedence and multi-profile auth.
- **reference** — simplify `collectRefFlags` (cobra local flags already include inherited persistent flags).
- **main** — extract `run()` for testability; remove unreachable git-context error branches in `mr current` / `pipeline current`.

### Added

- **100% statement test coverage** across all packages (`cmd`, `internal/*`, `cmd/gitlab-cli`).
- New tests: agent-safe / confirm flows, auth profile & interactive login, plain-text command paths, HTTP client edge cases.
- **npm local publish helpers** — `scripts/npm-publish-local.ps1`, `scripts/npm-token.local.example`.
- Expanded `.gitignore` for coverage artifacts from local `go test -coverprofile` runs.

[Unreleased]: https://github.com/fatecannotbealtered/gitlab-cli/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/fatecannotbealtered/gitlab-cli/releases/tag/v1.1.0

## [1.0.0] - 2026-05-24

First public release of **gitlab-cli** — an AI-Agent-friendly CLI for GitLab.com, Self-Managed, Dedicated, and Data Center.

### Highlights

- **17 top-level commands**, ~85 subcommands across MR, issue, pipeline, job, repo, release, label, milestone, variable, search, and more.
- **Strict Agent contract** — global `--json` / `--compact` / `--fields`, semantic exit codes, machine-readable errors, `--dry-run`, `--confirm <token>`, and JSONL audit logs.
- **Agent-safe mode (default on)** — `--force` and `--show-values` require explicit env opt-in; `context --strict` fails fast when unauthenticated.
- **Multi-profile auth** — `auth profile list|use|remove` for switching GitLab hosts/tokens.
- **Skills** — install via `npx skills add fatecannotbealtered/gitlab-cli -y -g` (see `skills/gitlab-cli/`).

### Commands

- `context` — composite git × GitLab snapshot (recommended first call).
- `auth` — `login`, `logout`, `status`, `profile` management.
- `doctor`, `reference` — diagnostics and self-documenting command tree (`reference --json` includes write/confirm/risk metadata).
- `user`, `project`, `search` — discovery and identity.
- `mr`, `issue` — full lifecycle, comments, approve, diff, `--find-existing` on create.
- `label`, `milestone`, `release`, `variable` — project metadata and secrets (values redacted by default).
- `pipeline`, `job` — list/get/current, create/retry/cancel, `wait`, `log --follow`, artifacts download.
- `repo` — file CRUD, branches, commits, tree.

### Agent & output

- List envelope for `mr list` (`items`, `count`, `limit`, `hasMore`, `all`) with `--all` pagination.
- Flat JSON types and `--fields` projection across domains.
- Exit codes `0`–`10` (including timeout `8`, CI failure `9`, cancelled/confirm `10`).
- Network retry on 429/5xx; `PaginateGET` for multi-page lists.
- `SIGINT` cancels in-flight HTTP and wait loops.

### Security

- Credentials at `~/.gitlab-cli/config.json` (`0600`); audit dir `0700`.
- Audit redaction for tokens, passwords, `--value`, `--variable`, `--body`, `--content`, `--commit-message`, and long positional args.
- `variable` values hidden in `--json` unless `--show-values` with `GITLAB_CLI_ALLOW_SHOW_VALUES=1`.
- HTTPS required by default; `http://` only for loopback hosts.
- Path traversal guards on `repo file get --output` and `job artifacts --output`.
- HTTP redirects do not forward `Authorization` to a different host.

### Distribution

- GoReleaser cross-platform binaries (linux/darwin/windows, amd64/arm64).
- npm package `@fatecannotbealtered-/gitlab-cli` with postinstall binary download.
- CI matrix (Go 1.23/1.24); optional local E2E via Docker (`docs/E2E.md`).

### Known limitations

- `cmd/` package tests are sequential (shared cobra globals); `internal/*` tests are parallel-safe.
- No GitLab instance E2E in GitHub Actions CI yet (httptest + optional local Docker workflow).

[1.0.0]: https://github.com/fatecannotbealtered/gitlab-cli/releases/tag/v1.0.0
