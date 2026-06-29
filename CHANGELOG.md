# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **`npm ci` lockfile drift fixed.** The per-platform `optionalDependencies` subentries in `package-lock.json` (`node_modules/@fateforge/gitlab-cli-*`) were missing their `version`, so once those platform packages were published, `npm ci` failed its consistency check (`lock file's <pkg>@ does not satisfy <pkg>@<version>`). The version bump (`scripts/version-files.js`) now syncs the lockfile platform subentries too, and `check-version.js` guards them, so the drift can't silently recur.

## [1.2.13] - 2026-06-29

### Fixed

- `update` notice cache reads are now version-aware: a cached `update_available` notice is suppressed once the running binary is already at or past the cached latest version. This closes a gap on the package-manager update path, which upgrades through the manager without clearing the cache, so within the 24h cache TTL business commands no longer kept advertising an update to a version already installed.
- `update` now routes a Homebrew-managed install (detected by path or the install-method override) through the package-manager-driven path (`brew upgrade gitlab-cli`) instead of letting it fall through to the standalone in-place binary swap. Replacing a Cellar-managed binary in place would desync Homebrew's metadata; the manager now owns the upgrade (CLI-SPEC §14).

## [1.2.12] - 2026-06-25

### Added

- Canonical JSON contract is now single-sourced from the `ai-native-cli-spec` template (pinned via `.agent/SPEC_VERSION=v1.4`). `contract/contract.json` is vendored and generates `internal/contract/contract_gen.go`; the error-code → exit → retryable mapping and `schema_version` now derive from it, so they cannot drift from the fleet contract. `E_CANCELLED` is registered in `contract/contract-ext.json` (exit 5, non-retryable) to keep it valid. A new conformance test (`internal/output/contract_conformance_test.go`) asserts every emitted error code, the exit/retryable mapping, the envelope key sets, and `meta` keys match the contract.

### Changed

- The bundled `.agent/` specs are now synced (not hand-maintained) from `ai-native-cli-spec@v1.4`, which adds the machine-readable contract governance (CLI-SPEC §3.1) and the install-method dispatch note for `update`. A `check-spec` CI guard is added to the `npm-audit` job to fail closed on drift of the vendored specs/contract or the generated module.
- `update` now drives the package manager for npm/Go-managed installs. A bare `gitlab-cli update` runs `npm install -g @fateforge/gitlab-cli@<version>` (or `go install …@<version>`) on the user's behalf, then syncs the Skill, reporting `status: "installed"`. The binary is never mutated in place under a package manager. `--check`/`--dry-run` stay read-only and preview the package-manager command without running it. A package-manager failure reports `E_IO` (exit 1) with `binary_replaced: false` and the exact command to run manually.

## [1.2.11] - 2026-06-25

### Fixed

- `update` discover/download failures are now classified by the **upstream HTTP status type** through the single `ErrorCodeFromStatus` mapping instead of being collapsed into `E_NETWORK` or sniffed out of the human-readable message (CLI-SPEC §6): a `404` is the non-retryable `E_NOT_FOUND` (exit 3), `429` → `E_RATE_LIMITED`, `5xx` → `E_SERVER`, transport errors stay `E_NETWORK`. The discover-stage failure now carries the same `stage`/`current_version`/`binary_replaced` invariant as the other stages.
- The `verify_signature` stage now splits **transport vs integrity**: a failure *fetching* the Sigstore bundle is reported as a retryable network/HTTP failure (classified by status), while only a genuinely missing/invalid signature stays the non-retryable `E_INTEGRITY`. Previously any bundle-download blip was misreported as an integrity failure an agent must not retry. The TUF trust-root refresh in the in-process verifier is now bounded by a timeout and cancelled on `ctx` (SIGINT) rather than running on an unbounded `http.DefaultClient`.
- A **TUF trust-root refresh** failure during `verify_signature` is now also classified as retryable network, not `E_INTEGRITY`. When the embedded-TUF metadata refresh fails because the Sigstore TUF registry is down/slow/DNS-unresolvable — the bounded 30s refresh timing out while the parent context is still alive, or the fetch returning a transport error — the verifier wraps it as a trust-root-unavailable network condition (`E_TIMEOUT` exit 8 for a deadline, `E_NETWORK` exit 7 for a transport error, `retryable:true`). Previously this transient network step collapsed into the non-retryable `E_INTEGRITY` (exit 1), telling an agent not to retry a forged-release verdict that had not actually occurred. Only a genuine signature/identity/checksum mismatch remains `E_INTEGRITY`.
- `install_method` is now a **real probe** of the running executable's path (npm `node_modules`, Homebrew `Cellar`, `go install` GOPATH/bin, otherwise `binary`; overridable via `GITLAB_CLI_INSTALL_METHOD`). The field was previously declared but never assigned, so it was always omitted from update results and notices.
- A successful self-update now **clears the cached `update_available` notice** immediately after the binary swap, so business commands stop nagging to update to the version that was just installed (the next active check rewrites the cache).

### Changed

- Windows self-update now performs the binary swap **in place atomically** via the same cross-platform rename trick used on Unix (write `.<base>.new` → rename the in-use binary to `.<base>.old` → rename `.new` into place → restore `.old` on failure → remove `.old` on success, ignoring a still-locked `.old` on Windows). The previous Windows path that wrote a `.cmd` shim and scheduled a replace-on-restart is gone: `update` now reports `status: "installed"` / `binary_replaced: true` immediately on every platform, and no longer emits the `pending_path` field or a `scheduled` status.

## [1.2.10] - 2026-06-22

### Added

- The cached update-available notice now also appears in **every command's `meta.notices`** (CLI-SPEC §3, §14), read **only from the local cache** with zero network I/O — business commands surface the cached notice without ever phoning home. It is omitted when the cache has nothing to report. The active-check commands (`context` / `doctor` / `update --check`) keep their fresh `data.notices` view.
- Update notices are now **severity-graded** at check time from the embedded CHANGELOG delta between the running and latest versions: `warning` when the delta contains a `security` entry or the latest crosses a major version, otherwise `info` (`critical` is reserved). The computed severity is stored in the cache, so the cached `meta.notices` carries the right level.

## [1.2.9] - 2026-06-21

### Changed

- `update` is now a single command with no confirm token. A bare `gitlab-cli update` performs the whole self-update in one call — resolve latest (or `--target-version`) → verify signature → verify checksum → replace binary → sync Skill — and is exempt from the `--dry-run` → `--confirm <token>` write gate (in-process Sigstore verification is the safety guarantee, not an agent's review of a preview). `update --check` and `update --dry-run` stay as optional read-only flags; `--dry-run` now issues NO `confirm_token` and NO `expires_at`. `update` is idempotent: already-latest returns ok with a no-op. Other write commands are unchanged.

### Added

- Staged failure & interruption envelope for `update`. Every update failure carries `stage` (`discover|download|verify_signature|verify_checksum|replace|skill_sync`), `current_version`, `binary_replaced`, and `skill_sync_status`. Failures are classified by next action: discover/download → retryable `E_NETWORK`/`E_TIMEOUT`/`E_RATE_LIMITED`; signature/checksum → non-retryable `E_INTEGRITY` (exit 1, unchanged fail-closed behavior); replace stage local failures → `E_IO` (exit 1) or `E_FORBIDDEN` (exit 4) instead of being misclassified as `E_NETWORK`. A skill-sync failure after a successful binary replace is now PARTIAL SUCCESS (`ok:false`, `binary_replaced:true`, `retryable:true`) carrying `skill_sync_command`, instead of a hard error that hid the binary already being updated.
- SIGINT/SIGTERM during `update` is trapped: it unwinds to a clean state, always cleans the temp dir, and still emits a terminal JSON envelope (`E_INTERRUPTED`, exit 130) stating the post-state per the stage invariant.
- New error codes `E_IO` (→ exit 1) and `E_INTERRUPTED` (→ exit 130) added to the output error package and code→exit mapping.

## [1.2.8] - 2026-06-16

### Added

- `repo commit list` now answers "commits by a person over a time range" and scales across scopes. New flags: `--author` (server-side author filter, GitLab 15.10+), `--with-stats` (per-commit `additions`/`deletions`/`total`, so an agent can size output without any diff), and `--all-branches` (all refs, not one branch). The scope selector is `--project` (now repeatable / comma-separated), `--group` (every project under the group tree, subgroups included), or `--all-projects` (every project the token can see; fail-closed without `--author`). Multi-project scopes fan out client-side (CLI-SPEC §15): each commit item carries its `project`, and `data` reports `scope`, `projectsScanned`, and `projectErrors[]` — a project that fails to scan is reported, not silently dropped, and never aborts the rest.
- `repo commit diff <sha>` returns the per-file diff for one commit (structured `files[]` with `oldPath`/`newPath`, `newFile`/`deletedFile`/`renamedFile`, computed `additions`/`deletions`, and the `diff` patch). It is a heavy sub-resource kept out of `commit list`: triage with `commit list --with-stats`, then read diffs only for the SHAs that matter. `--fields newPath,additions,deletions` yields a cheap inventory without the patch text; `--path` narrows to one file.

## [1.2.7] - 2026-06-16

### Fixed

- npm `optionalDependencies` platform-package pins now match the package version. The previous release bumped the top-level version but left the pins at the prior version, so `npm install` resolved a stale platform binary (the new wrapper with the old binary). The publish workflow now rewrites `optionalDependencies` from the package version before `npm publish`, so the pins can no longer drift from the single source of truth.

## [1.2.6] - 2026-06-16

### Changed

- `update` now verifies the release Sigstore signature on `checksums.txt` **in-process** (embedded `sigstore-go`, bootstrapped from the embedded TUF trust root) instead of shelling out to an external `cosign`. Verification is mandatory and fail-closed: a missing signature bundle, a signature that does not verify against this repo's tagged release-workflow identity, or a checksum mismatch all refuse the update — there is no skip path. Releases are now signed with `cosign sign-blob --new-bundle-format`.

### Security

- Release-integrity failures (missing/invalid signature or checksum mismatch) now return the non-retryable `E_INTEGRITY` error code (exit 1) instead of a retryable network code, so an agent treats a possible supply-chain issue as a hard stop rather than retrying.

## [1.2.5] - 2026-06-16

### Added

- Inline (diff-anchored) merge-request review primitives, so an agent can anchor review feedback to a specific file and line:
  - `mr discussion create` — start a diff-anchored discussion via `POST /discussions` with a `position`. The agent supplies `--new-path`/`--new-line` (or `--old-path`/`--old-line`); the three diff SHAs (`base`/`start`/`head`) are auto-filled from the MR's `diff_refs`, and the resolved head SHA is bound into the confirm scope. `--base-sha`/`--start-sha`/`--head-sha` override for commenting on an older diff version. Fails with a clear `E_VALIDATION` (instead of a cryptic GitLab 400) when `diff_refs` are not yet populated.
  - `mr discussion resolve` — resolve or reopen (`--unresolve`) a diff-anchored thread via `PUT /discussions/:id?resolved=`.
- `mr get` / single-MR reads now expose `diffRefs` (base/start/head SHAs); resolvable discussion notes carry `resolvable` + `resolved`.

## [1.2.4] - 2026-06-15

### Added

- Batch write commands (CLI-SPEC §15: plural input, one dry-run preview, one single-use confirm token covering the whole batch, aggregated per-item `items[]` + `{total,succeeded,failed}` summary, `--continue-on-error`):
  - `repo commit create` — one atomic multi-file commit via the native `POST /repository/commits` `actions[]` endpoint (class A); repeatable `--action 'create|update|delete|move:path=...;content=...;previous_path=...'`, up to 1000 actions.
  - `issue bulk close|reopen|update|label|assign|comment` — client-side loop over `--ids` (class B).
  - `mr bulk merge|approve|close|update` — client-side loop over `--ids` (class B); `mr bulk merge` is write-dangerous and requires the `--dangerous` two-step gate (defaults `--continue-on-error` to false).
  - `variable bulk-import --file` — import CI/CD variables from a `.env` or JSON file, creating new keys and updating existing ones.

### Changed

- npm scope migrated to `@fateforge` (the previous hyphen-suffixed npm scope is unavailable, and the hyphen-less org name is already taken on npm, so the package now publishes under `@fateforge`). The GitHub org, the Go module path, and the `npx skills add` source are unchanged.

## [1.2.3] - 2026-06-14

### Added

- `project create`; `mr discussion list` / `mr discussion reply` (threaded discussions); `job log --follow` now streams the trace as CLI-SPEC §5 NDJSON.
- `--idempotency-key` on create commands -> forwarded as the upstream `Idempotency-Key` header and bound into the confirm scope.
- `reference` now exposes a real per-command `output_schema` + `examples[]`, guarded against regression.

### Changed

- Confirm tokens are now single-use (E_CONFLICT on replay) and bind the upstream resource version where available: `repo file` update/delete bind `last_commit_id` (optimistic concurrency), `issue`/`mr` update bind `updated_at`, `mr merge` binds the head `sha` -- drift returns `E_CONFLICT`.
- T2 `--dangerous` second gate now required (both dry-run and confirm) for `repo branch delete`, `repo file delete`, `release delete`, `variable create/update/delete`, and `mr merge`.

## [1.2.2] - 2026-06-14

### Added

- `issue get` and `mr get` now return the full `description` body (tagged `_untrusted`), so an agent can read what an issue/MR actually says. List output still omits the body for token efficiency.

### Changed

- `author`/`assignee` usernames on issues and MRs, and `author` on MR notes, are now tagged `_untrusted` (attacker-controllable display-adjacent fields), matching the tagging already applied to titles and bodies.
- `meta` is now always emitted in the envelope (removed a misleading `omitempty`); every response carries `meta.duration_ms`, and `0` is a valid value an agent should always see.

## [1.2.1] - 2026-06-12

### Added

- Re-recorded live E2E acceptance against GitLab EE 18.8.10 (was CE 17.11.4): 90 leaf commands, 89 PASS / 1 SKIP-by-design, plus manual keyring and confirm-chain smoke (`docs/E2E-ACCEPTANCE-REPORT.md`, 2026-06-12).
- Integration cases for the four leaves the rewritten guard found uncovered: `auth profile list/use/remove` and `changelog`; the `auth login` case now exercises the dry-run contract path (bare invocation correctly exits 5).
- `TestLeafPathsParsesEnvelope` guard-of-the-guard: fails when e2e leaf enumeration returns an implausibly small tree instead of silently passing.
- Working FCC enumeration guard (`TestFCC_EveryLeafCommandHasTest`): enumerates every leaf command from live `reference` output and asserts each has a command-level test; skips while `fcc_status` is honestly declared non-verified, so the claim cannot be flipped without coverage.
- Command-level tests for `changelog` (`--json` and `--since`).
- `changelog [--since <version>]`, derived from `CHANGELOG.md`, so agents can refresh knowledge after an update.
- Root `AGENTS.md`, `NOTICE.md`, `CODE_OF_CONDUCT.md`, `docs/COMPATIBILITY.md`, and `docs/OPEN_SOURCE_CHECKLIST.md` to align the open-source repo layout.
- `context`, `doctor`, and `reference` now report tool version, risk tier, security metadata, and Skill minimum-version expectations.

### Changed

- Synced the `.agent/` spec copies from the ai-native-cli-spec template: stdout failure envelope (§4), HMAC confirm-token requirement (§7), signature_status/signature_verified fields (§14), Skill frontmatter `version` rule.
- Unified the golangci-lint v2 toolchain: Makefile installs from the `/v2` module path and CI uses `golangci-lint-action@v8` to match the v2 config format.
- `release_readiness` now declares `stable` with `live_smoke_status: verified`, citing the recorded acceptance evidence in `docs/E2E-ACCEPTANCE-REPORT.md` (2026-06-12: 90 leaf commands, 89 PASS / 1 SKIP-by-design against GitLab EE 18.8.10).
- Top-level `update` JSON keys are now snake_case (`current_version`, `target_version`, `update_available`, `release_url`, `pending_path`); text output reads the same keys.
- List-style JSON commands now use a consistent `items` / `count` / `hasMore` list envelope.
- JSON IDs are emitted as strings across flattened GitLab resources.
- GitLab-controlled text fields are marked with `_untrusted` so agents treat returned content as data.
- In JSON mode the failure envelope is the single JSON document on stdout and includes `meta.duration_ms`, matching CLI-SPEC §4; stderr stays a human-readable side channel.
- Confirm tokens now bind command path, normalized operation payload, configured host/source context, and expiry.
- Self-update now syncs the whole Agent Skill directory through `npx skills add fatecannotbealtered/gitlab-cli -y -g` and reports `skill_sync_status`.
- Skill, README, `.agent/` specs, and test prompts now follow the unified Agent-first update and Skill sync contract.

### Security

- The credential-file AES master key is now a random 32-byte secret held by the OS keyring (envelope `kdf: keyring-master-key-v1`), so exfiltrated `config.json`/`profiles.json` carry nothing decryptable. Machine-bound derivation remains the fallback and still decrypts legacy files; `context.data.credentials.storage` reports the active backend, and logout removes the keyring key.
- Synced `.agent/` SEC-SPEC from the template: credential-at-rest is now the keyring three-part pattern (password discarded after login / secrets in the OS keyring / zero-secret config), file encryption demoted to a visible fallback, env vars as the recommended secret channel, and an honest note on Windows `0600` semantics.
- Confirm tokens are now signed with a machine-local HMAC key (`confirm.secret`, created on first use with 0600 permissions) so they cannot be fabricated without running `--dry-run` on the same machine.
- Saved `config.json` and `profiles.json` credentials are encrypted at rest with AES-256-GCM and a machine-bound PBKDF2-SHA256 key.
- Release checksums are signed with Sigstore/Cosign, and install/update paths report signature verification status separately from checksum verification.
- The npm install script now hard-fails if checksum verification cannot be completed.

### Fixed

- The previous `TestUnit_EveryLeafCommandHasTest` was a silent no-op: it parsed `commands` at the envelope top level instead of under `data`, enumerated zero leaves, and always passed. Replaced by the guard above, which fails on a zero leaf count.
- The e2e `LeafPaths()` enumerator had the same wrong-envelope-level bug: it read top-level `commands`, returned 0 leaves, and `TestAllCommands_EveryLeaf` fataled before running a single sub-test — the first acceptance run executed zero commands while appearing to spend 13 minutes testing. Now parses `data.commands`.
- E2E runner config lost `clone_url` on re-registration, so every pipeline failed (`localhost:8929` unreachable inside the runner container) and bootstrap waited out its 20-minute deadline; restored `clone_url = "http://gitlab:8929"` and documented it in `docs/E2E.md`.


## [1.2.0] - 2026-06-07

### Added

- Unified Agent JSON envelope: successful responses include `ok`, `schema_version`, `data`, and `meta.duration_ms`; errors use the same envelope shape with stable `E_*` codes and `retryable`.
- Confirm-token write flow: `--dry-run` returns a payload-bound `confirm_token` plus expiry; writes require `--confirm <confirm_token>` in non-interactive runs.

### Changed

- JSON output now follows the Agent-facing stdout contract: stdout contains one valid JSON document by default, with progress and diagnostics on stderr.
- **README / README_zh / skills** — document the unified JSON envelope, updated exit codes, `data.confirm_token` flow, and envelope-aware `jq` examples.
- **reference** — expose schema version, CLI version, global flags, command formats, write metadata, confirmation requirements, and the current 0-8 exit-code table.

### Fixed

- **MR create** — `--assignee` with unknown user returns exit 3 (`E_NOT_FOUND`) instead of silently skipping.

## [1.1.1] - 2026-06-05

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

[Unreleased]: https://github.com/fatecannotbealtered/gitlab-cli/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/fatecannotbealtered/gitlab-cli/releases/tag/v1.2.0
[1.1.1]: https://github.com/fatecannotbealtered/gitlab-cli/releases/tag/v1.1.1
[1.1.0]: https://github.com/fatecannotbealtered/gitlab-cli/releases/tag/v1.1.0

## [1.0.0] - 2026-05-24

First public release of **gitlab-cli** - an Agent-native CLI for GitLab.com, Self-Managed, Dedicated, and Data Center.

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
- npm package `@fateforge/gitlab-cli` with postinstall binary download.
- CI matrix (Go 1.23/1.24); optional local E2E via Docker (`docs/E2E.md`).

### Known limitations

- `cmd/` package tests are sequential (shared cobra globals); `internal/*` tests are parallel-safe.
- No GitLab instance E2E in GitHub Actions CI yet (httptest + optional local Docker workflow).

[1.0.0]: https://github.com/fatecannotbealtered/gitlab-cli/releases/tag/v1.0.0
