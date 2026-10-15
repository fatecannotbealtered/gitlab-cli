# Open-Source Checklist

Run through this gate **before a release, or before a newly prepared branch goes public**. It is a security and quality checkpoint, not documentation: every box must be ticked, or consciously waived with a note. Once public, secrets in history cannot be un-leaked. Rule IDs refer to the specs in `.agent/` and to REPO-SPEC in ai-native-cli-spec.

## Secrets

- [ ] No credentials, tokens, API keys, or passwords anywhere in the working tree.
- [ ] No secrets in **git history**, checked with a scanner (e.g. `gitleaks detect`, `git log -p | grep`); if found, the history is rewritten or the repo re-created, not just deleted from `HEAD`.
- [ ] No internal hostnames, private IPs, internal URLs, or employer-internal identifiers in code, config, or comments.
- [ ] Test fixtures, recorded responses and evidence records (`docs/E2E.md`, `docs/evidence/` and the files they cite) contain only synthetic or redacted data: argv with secrets redacted, no real account data, no real tokens (VER-005).
- [ ] `.env`, `*.local`, credential files and everything from the state directory (`~/.gitlab-cli/`: `confirm.key`, `operations/`, `scope.json`, `settings.json`, `update-check.json`) are untracked (`git status --ignored`); runtime state never lives in the repo (REPO-050).
- [ ] Tests use a temporary state directory and an in-memory keyring, never the developer's confirm key, journal, scope file or stored token (REPO-073).
- [ ] Stored credentials follow the keyring pattern: the token in the OS keyring, `context.credentials.source` reporting where it came from, no secrets in `settings.json` (SEC-040).

## Docs

- [ ] `README.md` follows the REPO-010 skeleton: title, language switch and badges → one line → Agent Install → What It Does → Authorization → Capabilities → Agent Workflow → Machine Contract → Configuration → Uninstall → Project Structure → Development → Links.
- [ ] The Agent Install block runs as pasted in a clean shell, installs the Skill from the installed package with a pinned installer version (`npx skills@<version> add`) rather than from a branch, and runs no `context`, `doctor` or `reference` first (REPO-010, SKL-009, CLI-034).
- [ ] The Authorization section gives the scope file path, a minimal grant and the protection statement, and never tells an agent to write the scope file (REPO-011, SEC-013).
- [ ] The Capabilities map in `README.md` and `README_zh.md` and the command map in the Skill are all the `CommandMap()` output, and the map tests check them (SKL-002).
- [ ] The Configuration section lists every setting with its variable and shows the settings file's profiles; the Uninstall section removes the package or binary, the state directory, the keyring entries, a settings file kept elsewhere and the Skill directories (REPO-010, REPO-011, CLI-100).
- [ ] Each English doc and its `_zh` translation (`README`, `AGENTS`, `SECURITY`, `CONTRIBUTING`) are **content-synced** and link to each other: same sections, same commands, same filled-in values (REPO-020).
- [ ] `CHANGELOG.md` uses Keep a Changelog, has `## [Unreleased]` on top, and the release section carries its real date. The newest release's section fits the 8,000-character budget of a default `gitlab-cli changelog` call, which the conformance suite checks (CLI-042).
- [ ] `docs/UPGRADE-2.0.md` still matches what 2.x does for 1.x users: sign-in, scope, command, flag and output changes.
- [ ] `LICENSE` is present, the license is chosen deliberately (MIT), and the copyright year and holder are filled in.

## Governance

- [ ] `SECURITY.md` has a working disclosure channel and a supported-versions table, and states the risk tier with its worst case in one sentence (SEC-001).
- [ ] `SECURITY.md` also gives the scope file path, how to grant and withdraw a scope, and the honest protection statement (SEC-011, SEC-013).
- [ ] The tool sends no telemetry and connects only to the configured GitLab host and, in `update` and `doctor`, to its release source; `SECURITY.md` lists any other destination (SEC-022).
- [ ] `CONTRIBUTING.md` is present (setup, branch and commit convention, tests and evidence, PR flow).
- [ ] `CODE_OF_CONDUCT.md` is present (Contributor Covenant).
- [ ] `NOTICE.md` carries the GitLab trademark and non-affiliation notice, and `docs/COMPATIBILITY.md` lists the GitLab versions validated live.

## Build / CI

- [ ] CI (`.github/workflows/ci.yml`) is **green** on the commit being pushed, and a red step blocks the merge (not advisory-only).
- [ ] CI runs format and lint, the tests with the conformance suite on every OS, `govulncheck` and `npm audit`, `check-version.js` and `check-spec.js` (REPO-070, SEC-050).
- [ ] The conformance test runs with a `Sample` for every write command and coverage recording on (R06), and filters nothing (VER-001, VER-003).
- [ ] Every envelope the tests produce is validated, and each command at `experimental` or above has its behaviour-matrix tests (VER-002, VER-004).
- [ ] Each command's maturity matches its evidence: `verified` cites a `live:` record, `stable` also an `eval:` report, and every referenced file exists (VER-005).
- [ ] `release.yml` runs every merge check on the tagged commit before anything is signed or published (VER-008).
- [ ] No retired 1.x file or field remains: no FCC guard, no `release_readiness`, no per-tool `contract_gen`, no confirm-token, key or consumed-store code of the tool's own (REPO-001).
- [ ] The formatter config is committed (`.golangci.yml`) and CI runs the format check.
- [ ] No build artifacts, caches, or IDE config are committed (covered by `.gitignore`).
- [ ] The launcher-budget job ran; if the npm launcher adds more than 100 ms per call, the README's Agent Install puts the native binary on PATH (REPO-041).

## Distribution

- [ ] `package.json` `version` matches the git tag being released (`vX.Y.Z` ↔ `X.Y.Z`); `release.yml` guards this and fails on mismatch.
- [ ] `package.json` `version` is the only hand-edited version: `npm version` syncs the lockfile, the Skill's `metadata.version` and `metadata.min_version` and the CHANGELOG, and `check-version.js` is green (REPO-031).
- [ ] The binary itself (`bin/`, `*.exe`, `dist/`) is **not committed**; CI builds it and `.gitignore` covers it.
- [ ] Release archives ship a `checksums.txt` signed with Sigstore keyless signing in the tagged release workflow, with the bundle published beside it; the binary update path verifies signature and checksum in-process and fails closed (REPO-042, SEC-050).
- [ ] npm publishes the main wrapper package plus every supported OS/CPU platform package from CI-built artifacts, with `npm publish --provenance`; installing downloads nothing else (REPO-040).
- [ ] Every channel ships the Skill of its own version: `npm pack --dry-run` lists `skills/`, and each release archive has `skills/` beside the binary (REPO-043).
- [ ] The runtime `changelog` command and the GitHub Release body both come from `CHANGELOG.md`, not hand-copied text (REPO-032).
- [ ] `update` is aicli's built-in, set up with `App.Update` for `fatecannotbealtered/gitlab-cli` and `@fateforge/gitlab-cli`; the tool has no update code of its own, so a bare `update` (one command, no confirm token) can verify the release, install it and copy the Skill from it (CLI-070, REPO-043).

## AI-native

- [ ] Root `AGENTS.md` is present and points to `.agent/AGENT.md`.
- [ ] `.agent/` holds the five specs (`AGENT`, `CLI-SPEC`, `SEC-SPEC`, `SKILL-SPEC`, `VERIFY-SPEC`) and `SPEC_VERSION`; `contract/` holds `contract.json` and `schemas/`; the aicli library exists only at `internal/aicli/` with its `MANIFEST.json`, and `check-spec.js` is green (REPO-001, REPO-062).
- [ ] `skills/gitlab-cli/SKILL.md` follows SKILL-SPEC 2.0: Agent Skills frontmatter only (`name` equal to the directory, a `description` that says what, when and not for, `license`, `compatibility`, and `metadata` with `version`, `min_version` and `requires.bins`), at most 150 lines, a generated command map, one write recipe, risk-scaled `STOP CHECKPOINT` lines, the scope, untrusted-content, ambiguity and lost-reply lines, and the end-of-task report (SKL-001 to SKL-008).
- [ ] `skills/gitlab-cli/test-prompts.json` is valid JSON, its entries have only `id`, `scenario`, `prompt`, `expected` and an optional `grader_notes`, and it covers every VER-006 scenario (SKL-011).
- [ ] `gitlab-cli reference --index`, `gitlab-cli context` and `gitlab-cli doctor` run offline from a clean install and emit valid envelopes; with a scope granted, `doctor`'s `authorization` and `version` checks pass (CLI-031, CLI-035, CLI-036).
- [ ] Without a scope file, every write command is refused with `E_FORBIDDEN` and `details.scope`, and nothing is sent (SEC-010).
- [ ] Flags use the shared vocabulary and verbs their standard meaning; settings come from `App.Settings`; no `--verbose` or `--debug` flag; upstream requests have timeouts and verified TLS and never re-send a write (CLI-028, CLI-029, CLI-100, CLI-102, CLI-110).
- [ ] The risk tier in `SECURITY.md` (T2) equals `reference.risk_tier` and is at least what the commands' effects require (SEC-001).
- [ ] No command is `stable` without a passed agent eval (VER-005, VER-006).
