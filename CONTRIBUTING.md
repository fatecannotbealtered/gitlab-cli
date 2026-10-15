# Contributing to gitlab-cli

*English | [中文](CONTRIBUTING_zh.md)*

Thanks for improving **gitlab-cli**, a GitLab CLI for AI agents. This document covers building, testing and submitting changes.

> This is a side project shared for AI-tooling experimentation; maintainers do not provide commercial support or production guarantees.

## The build playbook lives in the specs

This repo is an **AI-native CLI tool**: its main caller is an AI agent. Before implementing or changing a feature, read:

- **[AGENTS.md](AGENTS.md)**: the entry point and the rules every change keeps.
- **[`.agent/AGENT.md`](.agent/AGENT.md)**: the playbook: workflows, hard constraints and a self-check.
- **[`.agent/CLI-SPEC.md`](.agent/CLI-SPEC.md)**: the machine contract: output, errors, the command model, discovery, reads, the write gate and self-update (`CLI-xxx`).
- **[`.agent/SEC-SPEC.md`](.agent/SEC-SPEC.md)**: risk tiers, untrusted content, scope files, secrets, the state store and the supply chain (`SEC-xxx`).
- **[`.agent/SKILL-SPEC.md`](.agent/SKILL-SPEC.md)**: the bundled Agent Skill (`SKL-xxx`).
- **[`.agent/VERIFY-SPEC.md`](.agent/VERIFY-SPEC.md)**: the conformance suite, tests, maturity and evidence, and the agent eval (`VER-xxx`).

The specs, `contract/` and the aicli library (`internal/aicli/`) are vendored from [ai-native-cli-spec](https://github.com/fatecannotbealtered/ai-native-cli-spec) at the tag in `.agent/SPEC_VERSION`. They take priority over default habits, and they are read-only here: `node scripts/check-spec.js` fails CI when they differ from the pinned spec. A change they need goes to the spec repository as an issue or pull request; once it is released, bump `.agent/SPEC_VERSION` and run `node scripts/sync-spec.js` (REPO-063). Code that writes its own output, errors or write gate instead of using the library will not be merged (CLI-001, CLI-050).

## Development setup

You need Go (the version in `go.mod`) and Node.js 16 or later for the npm wrapper and the spec and version scripts.

```bash
# Clone
git clone https://github.com/fatecannotbealtered/gitlab-cli.git
cd gitlab-cli

go mod download
make build                      # or: go build -o bin/gitlab-cli ./cmd/gitlab-cli
make test                       # go vet, then go test -race ./... (conformance suite included)
./bin/gitlab-cli --help

# Vendored files and version locations
node scripts/check-spec.js && node scripts/check-version.js
```

If dependency download is slow, use a regional proxy (for example `GOPROXY=https://goproxy.cn,direct`). Race tests need `CGO_ENABLED=1` and a C compiler.

## Commands

| Goal | Command |
|------|---------|
| Build | `make build` |
| Test, conformance suite included | `make test` → `go test -race -timeout 45m ./...` |
| Conformance suite only | `make conformance` |
| Lint | `make lint` → `golangci-lint run ./...` |
| Format | `make fmt` → `gofmt -w .` |
| Vulnerability audit | `make vuln` → `govulncheck ./...` |
| Vendored files match the pin | `node scripts/check-spec.js` (`make spec-check`) |
| Re-vendor after a pin change | `node scripts/sync-spec.js` (`make spec-sync`) |
| Version locations match | `node scripts/check-version.js` |
| Command maps current | `UPDATE_SKILL_MAP=1 go test ./cmd -run CommandMap` rewrites them |

`make` targets are variable-driven; on Windows fall back to the underlying tool command. The `cmd` tests take about 9 minutes, longer with `-race`, which is why `make test` raises the timeout. Run **lint + test** locally before pushing.

## Branch & commit convention

- Branch from the default branch: `git checkout -b feat/your-feature`.
- Keep one logical change per branch; rebase the default branch before requesting review.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>: <description>

<optional body>
```

| Type | Use for |
|------|---------|
| `feat` | New features |
| `fix` | Bug fixes |
| `refactor` | Code restructuring without behavior change |
| `docs` | Documentation changes |
| `test` | Adding or updating tests |
| `chore` | Build, CI, dependency, or tooling changes |
| `perf` | Performance improvements |
| `ci` | CI/CD pipeline changes |

Examples: `feat(mr): add --reviewer to mr list`, `fix(job): keep log follow from repeating lines`, `docs: sync README_zh`. Cite spec rules by ID where they apply (`fix: bind the body file by content (CLI-054)`).

## CI checks

`.github/workflows/ci.yml` runs on every push and pull request: format and lint; the tests, conformance suite included, on Linux, macOS and Windows; the dependency audits (`govulncheck` and `npm audit`); `check-version.js` and `check-spec.js`. A red step blocks the merge. `release.yml` runs the same checks on the tagged commit before anything is signed or published (REPO-070, REPO-071, VER-008).

## Tests and evidence

What "tested" means is defined in [`.agent/VERIFY-SPEC.md`](.agent/VERIFY-SPEC.md):

- **Conformance suite (VER-001).** `cmd/conformance_test.go` runs `aicli.RunConformance` with the fake GitLab (`cmd/fakegitlab_test.go`), which counts writes, a `Sample` for every write command and a scope. It checks the write gate of every write command (G00 to G10) and `reference`, the command tree, usage errors, envelopes, `context`, `doctor` and `operation` for the whole tool (R01 to R05). A write command without a sample fails it. Never skip, filter or weaken a check.
- **Real output (VER-002).** Command-level tests run the CLI through one helper (`newTestEnv` and `e.run` in `cmd/testenv_test.go`) that executes the App, validates the envelope, or each line of an NDJSON stream, with the library's validator and checks the exit code against contract.json.
- **Coverage from runs (VER-003).** Every command in the registry, built-ins included, runs in at least one command-level test. `TestMain` in `cmd/main_test.go` points `GITLAB_CLI_COVERAGE_FILE` at a temporary file the library records every executed command into; after a full run, `aicli.CheckCoverage` names every command that never ran (R06).
- **Behaviour matrix (VER-004).** `runMatrix` (`cmd/matrix_test.go`) runs, for each command at `experimental` or above, the rows that apply to it: success, invalid input, auth and permission, upstream failure, empty, pagination, exit code, no side effect for reads, and for writes the gate and the apply failures; bulk commands get per-item rows. A row that does not apply is a skipped subtest whose reason starts with `not applicable:`. Every bug fix that changes observable behavior adds a regression test.
- **Maturity (VER-005).** Each command declares `Maturity` and `Evidence` in its `Meta`, and the level never exceeds the evidence: `draft` (declared), `experimental` (behaviour matrix against the fake GitLab, `simulated:` evidence), `verified` (also a recorded run against a real GitLab, `live:` evidence in `docs/evidence/`, see [docs/E2E.md](docs/E2E.md)), `stable` (also a passed agent eval and no open P1 defect, `eval:` evidence). A change to a command's request, parsing or output voids its `live` and `eval` evidence: re-run them or lower the maturity in the same change.
- **Agent eval (VER-006).** Before a release that changes commands, outputs or the Skill, run the agent eval with `skills/gitlab-cli/test-prompts.json`, with and without the Skill. It is required before any command is marked, or stays, `stable`.
- **No user state (REPO-073).** Tests point the state directory at a temporary directory (`App.StateDir` or `GITLAB_CLI_STATE_DIR`) and swap the OS keyring for an in-memory one, never touching the user's confirm key, journal, scope file or stored token.

## Adding or changing a command

1. **Client**: add the upstream call to the domain's file in `internal/api/`. Reuse the shared client and its helpers (`GetPage` for one page of a list), build URLs from parameters (never by concatenating user input), and keep the client's rules: a timeout on every request, verified TLS, reads retried at most twice and writes never (CLI-110).
2. **Command**: each group registers itself from one function in its own files (`registerMR` in `cmd/mr.go`, and so on); `cmd/label.go` is the shortest worked example. Register every leaf with `app.Register(cmd, aicli.Meta{...})`: summary, effects, `Dangerous`, maturity and evidence, output schema (`app.Schema`), error codes (`gitlabReadCodes` or `gitlabWriteCodes`) and examples (a write shows the `--dry-run` and `--confirm` pair, a dangerous command `--dangerous`), plus `Positional`, `FileInputs`, `Secrets`, `BatchFlag`, `RenamedFlags` or `DeprecatedFlags` where they apply (CLI-020 to CLI-027). Name flags from the shared vocabulary (`--limit`, `--offset`, `--query`, `--state`, `--since`) and keep the standard verb meanings (CLI-028, CLI-029). `--help` and `reference` are generated from the registry, so the command appears in both without extra wiring.
3. **Names and refs**: take the project with `projectFlag` (a path, web URL, numeric ID or name; several matches are `E_AMBIGUOUS`) and users with `resolveUser` (`carol`, `@carol`, `me`). Every resource in output carries a `ref` in GitLab's own form (`acme/web!5`, `acme/web#12`, `acme/web/-/pipelines/123`; a sub-resource continues its parent's: `acme/web!5/notes/12`), commands accept it back with the web URL, and a write's target key is its ref (CLI-048, SEC-012). A root shortcut for a nested group uses `app.AddShortcut`; a noun that several groups nest carries the group (`mr-comment`, `issue-bulk`).
4. **Reads** return through `app.OK` or `app.List` (`gitlabPage` for one GitLab page), with a `scope` that says what the list really covers (CLI-041). **Writes** return through `app.Write` (or `app.WriteBatch`) with a read-only `Preview` and an `Apply` that sends once and reads back what it changed; build the GitLab client inside them (`gitlabFor(app)`), so a write the scope refuses sends nothing, and wrap an `Apply` error in `NotDispatched` or `Rejected` only when that is true (CLI-050, CLI-059).
5. **Tests**: the behaviour-matrix rows above, a conformance `Sample` for every new write command, and fake GitLab routes next to the group's tests.
6. **Skill**: regenerate the command map, which sits between the `aicli:command-map` markers in `skills/gitlab-cli/SKILL.md` and in the Capabilities section of `README.md` and `README_zh.md`. It is rendered from the registry and checked by the map tests; `UPDATE_SKILL_MAP=1 go test ./cmd -run CommandMap` rewrites every block. Never edit them by hand. Then update the playbooks and the domain-knowledge lines in `SKILL.md`, and add or adjust `test-prompts.json` entries (`id`, `scenario`, `prompt`, `expected`, optional `grader_notes`). `SKILL.md` stays within 150 lines and does not repeat flags or schemas, which `gitlab-cli reference --command <path>` gives (SKL-002, SKL-003, SKL-011). `metadata.version` and `metadata.min_version` follow `package.json` through `npm version`; never edit them by hand.
7. **Docs**: add a line under `## [Unreleased]` in `CHANGELOG.md`. Agents read it after an update through `gitlab-cli changelog`, so name the commands, flags and fields that changed. A command or flag that is going away gets `Meta.Deprecated`, `Meta.RenamedFlags` or `Meta.DeprecatedFlags` and a line under `Deprecated`; it is removed only in a MAJOR release (CLI-081). Keep a release's section short: `gitlab-cli changelog` without `--since` returns at least the newest release whole, and the conformance suite holds that response to 8,000 characters (CLI-042).

## Pull request guidelines

1. **One logical change per PR** when possible.
2. **Tests**: add or update tests for behavior changes.
3. **Docs**: update user-facing docs and the Skill when commands, flags or outputs change.
4. **Commits**: Conventional Commits; no secrets or real tokens anywhere.

### PR checklist

- [ ] One logical change, focused diff
- [ ] Tests added/updated and passing (`make test`), conformance suite included
- [ ] Every new write command has a conformance `Sample`, and every new command runs in a test (VER-001, VER-003)
- [ ] Each changed command's maturity still matches its evidence (VER-005)
- [ ] Lint and format passing
- [ ] `node scripts/check-spec.js` passes: nothing vendored was edited by hand
- [ ] `node scripts/check-version.js` passes
- [ ] Command maps regenerated (Skill and README Capabilities); playbooks and `test-prompts.json` adjusted
- [ ] Agent eval run if the change touches commands, outputs or the Skill and any command is `stable` (VER-006)
- [ ] `CHANGELOG.md` (`## [Unreleased]`) updated
- [ ] **Bilingual docs synced**: every `*.md` change mirrored in its `*_zh.md` counterpart (and vice versa)
- [ ] No secrets, tokens, or real credentials in code, tests, fixtures, evidence records, or commit history
- [ ] Commit messages follow Conventional Commits

## Security

Do not open public issues for undisclosed security vulnerabilities. See [SECURITY.md](SECURITY.md).
