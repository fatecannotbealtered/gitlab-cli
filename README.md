# gitlab-cli

[![CI](https://github.com/fatecannotbealtered/gitlab-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/fatecannotbealtered/gitlab-cli/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

English | [中文](README_zh.md)

AI-Agent-friendly GitLab CLI. Manage merge requests, issues, pipelines, jobs, repositories, releases, labels, milestones, CI variables, and more — with a strict, machine-readable contract designed for AI Agents and automation.

Built with Go. Single static binary. Compatible with **GitLab.com, GitLab Self-Managed, and GitLab Dedicated**.

[Installation](#installation) · [Authentication](#authentication) · [Commands](#commands) · [JSON Output](#json-output) · [Security](#security) · [Contributing](#contributing) · [Disclaimer](#disclaimer)

## Why another GitLab CLI?

GitLab already ships an excellent CLI called [glab](https://gitlab.com/gitlab-org/cli) — for humans. `gitlab-cli` is **purpose-built for AI Agents** and shares the design DNA of [`jira-cli`](https://github.com/fatecannotbealtered/jira-cli):

- **Strict JSON contract** — JSON is the default output; every response is wrapped in a stable `ok` / `schema_version` envelope.
- **Unified output format** — global `--format json|text|raw`; use `text` for human-readable output and `raw` for unwrapped bytes/logs/diffs. `--json` remains a compatibility alias for `--format json`.
- **`--fields key,name,state` field projection** on supported commands (opt-in per command, not global) for further token reduction.
- **`--dry-run`** previews every write command before executing.
- **`--confirm <confirm_token>`** for non-interactive write confirmation (preferred for Agents); tokens are returned by `--dry-run`.
- **Machine-readable error envelope** with stable `E_*` codes, details, and `retryable`.
- **Semantic exit codes** (`0` through `8`) aligned with retry behavior.
- **JSONL audit log** of every write command at `~/.gitlab-cli/audit/`.
- **`SKILL.md`** installable via [`npx skills`](https://github.com/vercel-labs/skills) for compatible AI coding assistants.

Use `glab` interactively. Use `gitlab-cli` in scripts and Agent loops.

## Disclaimer

This project is shared for **personal learning, research, and AI tooling experiments**, not as a product with SLAs. Maintainers do not offer commercial support or **fitness-for-production** promises. Software is provided "as is".

## Installation

### Quick start

Copy the block below to your agent (or run it yourself):

```bash
# Please install gitlab-cli and use it for all GitLab operations going forward (JSON is the default output).
# Install CLI
npm install -g @fatecannotbealtered-/gitlab-cli

# Install CLI Skill (required) — copies into your agent-supported skills directory
npx skills add fatecannotbealtered/gitlab-cli -y -g

# Login and verify
gitlab-cli auth login
gitlab-cli doctor
```

### Alternative: Go install

```bash
go install github.com/fatecannotbealtered/gitlab-cli/cmd/gitlab-cli@latest
```

### Alternative: Download binary

Download from [GitHub Releases](https://github.com/fatecannotbealtered/gitlab-cli/releases) and add to your PATH.

## Authentication

`gitlab-cli` supports **GitLab Personal Access Tokens (PATs)** with at least the `api` scope.

### Interactive login

```bash
gitlab-cli auth login
# GitLab host (e.g. https://gitlab.example.com): https://gitlab.example.com
# Personal Access Token (PAT): ****
# ✔ Logged in as Alice (alice)

gitlab-cli doctor       # Verify connectivity
gitlab-cli auth status  # Check current auth state
gitlab-cli auth logout  # Remove credentials
```

### Non-interactive login (CI / AI Agent)

```bash
gitlab-cli auth login --host https://gitlab.example.com --token <PAT>
```

### Environment variables (recommended for CI/Agent)

| Variable | Description |
|---|---|
| `GITLAB_CLI_HOST` | Host URL — **highest precedence**, isolates this CLI from `glab` |
| `GITLAB_CLI_TOKEN` | PAT — highest precedence |
| `GITLAB_HOST` | Host URL — compatible with `glab` and other GitLab tooling |
| `GITLAB_TOKEN` | PAT — compatible with `glab` |
| `NO_COLOR` | Disable colored output ([no-color.org](https://no-color.org)) |
| `GITLAB_CLI_USER_AGENT` | Custom User-Agent for HTTP requests |
| `GITLAB_NO_AUDIT` | Set to `1` to disable audit logging |
| `GITLAB_AUDIT_RETENTION_MONTHS` | Auto-delete audit files older than N months (default `3`, `0` = keep forever) |

Precedence: `GITLAB_CLI_*` > `GITLAB_*` > active profile > `~/.gitlab-cli/config.json`.

### Generating a PAT

1. Open your GitLab instance → **User settings** → **Access Tokens**.
2. Create a token with the `api` scope (write access to projects you'll edit).

## Commands

> Run `gitlab-cli reference` for agent introspection in JSON (includes `path`, `write`, `outputType`, `formats`, `positionalArgs`, and more). Use `gitlab-cli reference --format text` for human-readable Markdown.

### Authentication & diagnostics

```bash
gitlab-cli auth login [--host URL] [--token PAT] [--profile NAME]
gitlab-cli auth logout
gitlab-cli auth status
gitlab-cli auth profile list
gitlab-cli auth profile use <name>
gitlab-cli auth profile remove <name>
gitlab-cli doctor
gitlab-cli update [--check] [--target-version vX.Y.Z] [--reinstall]
```

### Workflow context

```bash
gitlab-cli context              # one-shot snapshot for AI Agents (current git + GitLab + project)
gitlab-cli context
```

### Users & Projects

```bash
gitlab-cli user me
gitlab-cli user search --query <q> [--active] [--limit N]
gitlab-cli user get <username>

gitlab-cli project list [--owned] [--membership] [--search <q>] [--visibility <v>]
gitlab-cli project get <id-or-path>
gitlab-cli project members <id-or-path> [--query <q>]
```

### Search

```bash
gitlab-cli search projects --query <q>
gitlab-cli search issues   --query <q> [--project <id>]
gitlab-cli search mrs      --query <q> [--project <id>]
gitlab-cli search code     --query <q>  --project <id>    # blob/code search REQUIRES --project
gitlab-cli search commits  --query <q> [--project <id>]
```

### Merge Requests

```bash
gitlab-cli mr list      --project <id> [--state opened|closed|merged|all]
gitlab-cli mr get       --project <id> <iid> [--fields ...]
gitlab-cli mr current                            # uses git context
gitlab-cli mr create    --project <id> --title <t> --source-branch <s> --target-branch main
gitlab-cli mr create    --auto [--target-branch main] [--title <t>] [--draft]
gitlab-cli mr update    --project <id> <iid> [--title ...] [--add-labels l1,l2] [--remove-labels ...]
gitlab-cli mr merge     --project <id> <iid> [--squash] [--should-remove-source-branch]
gitlab-cli mr close     --project <id> <iid>
gitlab-cli mr reopen    --project <id> <iid>
gitlab-cli mr approve   --project <id> <iid>
gitlab-cli mr unapprove --project <id> <iid>
gitlab-cli mr diff      --project <id> <iid>     # JSON by default; use --format raw for unified diff bytes
gitlab-cli mr comment add    --project <id> <iid> --body <text>
gitlab-cli mr comment list   --project <id> <iid>
gitlab-cli mr comment delete --project <id> <iid> --note-id <id> --confirm <confirm_token>
```

### Issues

```bash
gitlab-cli issue list      --project <id> [--state ...] [--assignee <u>] [--label l1,l2]
gitlab-cli issue get       <iid> --project <id>
gitlab-cli issue create    --project <id> --title <t> [--description <d>] [--label l1,l2]
gitlab-cli issue update    <iid> --project <id> [--add-labels ...] [--remove-labels ...]
gitlab-cli issue close     <iid> --project <id>
gitlab-cli issue reopen    <iid> --project <id>
gitlab-cli issue assign    <iid> <username|me> --project <id>
gitlab-cli issue label     <iid> --project <id> --add l1,l2 --remove l3,l4
gitlab-cli issue comment add    <iid> --project <id> --body <t>
gitlab-cli issue comment list   <iid> --project <id>
gitlab-cli issue comment delete <iid> --project <id> --note-id <id> --confirm <confirm_token>
```

### Labels & Milestones

```bash
gitlab-cli label list   --project <id>
gitlab-cli label create --project <id> --name <n> --color <#hex|named> [--priority N]
gitlab-cli label update --project <id> --label-id N [--name ...] [--color ...]
gitlab-cli label delete --project <id> --label-id N --confirm <confirm_token>

gitlab-cli milestone list   --project <id> [--state active|closed|all]
gitlab-cli milestone get    --project <id> --milestone-id N
gitlab-cli milestone create --project <id> --title <t> [--due-date YYYY-MM-DD]
gitlab-cli milestone update --project <id> --milestone-id N [--title ...] [--state-event close|activate]
gitlab-cli milestone close  --project <id> --milestone-id N --confirm <confirm_token>
```

### Pipelines & Jobs

```bash
gitlab-cli pipeline list    --project <id> [--ref <b>] [--status ...]
gitlab-cli pipeline get     --project <id> <pipeline_id>
gitlab-cli pipeline current                       # latest pipeline for current branch
gitlab-cli pipeline create  --project <id> --ref <b> [--variable KEY=VAL]...
gitlab-cli pipeline retry   --project <id> <pipeline_id>
gitlab-cli pipeline cancel  --project <id> <pipeline_id>
gitlab-cli pipeline jobs    --project <id> <pipeline_id> [--scope ...]
gitlab-cli pipeline wait    --project <id> <pipeline_id> [--timeout 300] [--interval 10]

gitlab-cli job get       --project <id> <job_id>
gitlab-cli job log       --project <id> <job_id>           # JSON by default; use --format raw for trace bytes
gitlab-cli job log       --project <id> <job_id> --follow  # JSON buffer by default; use --format text/raw to stream
gitlab-cli job retry     --project <id> <job_id>
gitlab-cli job cancel    --project <id> <job_id>
gitlab-cli job artifacts --project <id> <job_id> --output artifacts.zip
gitlab-cli job wait      --project <id> <job_id> [--timeout 300] [--interval 5]
```

### Repository (file / branch / commit / tree) & Releases

```bash
gitlab-cli repo file get    --project <id> --path <p> [--ref <b>] [--output <path>]
gitlab-cli repo file create --project <id> --path <p> --branch <b> --content ... --commit-message <m>
gitlab-cli repo file update --project <id> --path <p> --branch <b> --content ... --commit-message <m>
gitlab-cli repo file delete --project <id> --path <p> --branch <b> --commit-message <m> --confirm <confirm_token>

gitlab-cli repo branch list   --project <id> [--search <q>]
gitlab-cli repo branch create --project <id> --name <n> --ref <source>
gitlab-cli repo branch delete --project <id> --name <n> --confirm <confirm_token>

gitlab-cli repo commit list --project <id> [--ref-name <b>] [--since ...] [--until ...] [--path <p>]
gitlab-cli repo commit get  --project <id> <sha>

gitlab-cli repo tree --project <id> [--path <p>] [--ref <b>] [--recursive]

gitlab-cli release list   --project <id>
gitlab-cli release get    --project <id> --tag <tag>
gitlab-cli release create --project <id> --tag <tag> --name <n> [--description <d>] [--ref <b>] [--milestone m1,m2]
gitlab-cli release update --project <id> --tag <tag> [--name ...] [--description ...]
gitlab-cli release delete --project <id> --tag <tag> --confirm <confirm_token>
```

### CI/CD Variables

```bash
gitlab-cli variable list   --project <id>                                     # values redacted in JSON unless --show-values
gitlab-cli variable get    --project <id> --key <k> [--filter env_scope=<scope>]
gitlab-cli variable create --project <id> --key <k> --value <v> [--type env_var|file] [--protected] [--masked] [--env-scope <s>]
gitlab-cli variable update --project <id> --key <k> [--value <v>] [--protected] [--masked]
gitlab-cli variable delete --project <id> --key <k> [--filter env_scope=<scope>] --confirm <confirm_token>
```

Pass `--show-values` on any `variable` subcommand to include secret values in JSON output (default: redacted).

## JSON Output

JSON is the default. Stdout contains one valid JSON document for normal commands; progress, warnings, and diagnostics go to stderr. Use `--format text` for human-readable output and `--format raw` for unwrapped bytes/logs/diffs on commands that support raw output. `--json` is kept as a compatibility alias for `--format json`.

Some commands also support **`--fields`** (comma-separated projection from the `data` payload); it is **opt-in per command** and only works with JSON output. Run `gitlab-cli reference` and look for `supportsFields` (e.g. `mr get`, `issue list`, `project get`).

```bash
# JSON envelope (default)
gitlab-cli auth status
gitlab-cli doctor

# Select only the fields you need (supported commands only)
gitlab-cli mr get --project group/proj 42 --fields iid,title,state,webUrl

# Clean output for scripts (suppresses text helper output)
gitlab-cli doctor --quiet

# Compact JSON (minimal tokens)
gitlab-cli doctor --compact
```

Successful responses use this shape:

```json
{
  "ok": true,
  "schema_version": "1.0",
  "data": {},
  "meta": {
    "duration_ms": 0
  }
}
```

Error responses use the same envelope shape:

```json
{
  "ok": false,
  "schema_version": "1.0",
  "error": {
    "code": "E_NOT_FOUND",
    "message": "GitLab API error 404: 404 Project Not Found",
    "details": {
      "status_code": 404,
      "hint": "Verify the resource (project path, IID, ID) exists and you have permission to view it"
    },
    "retryable": false
  }
}
```

## Global Flags

| Flag | Description |
|---|---|
| `--format json|text|raw` | Output format. Default: `json`; `text` is human-readable; `raw` is unwrapped bytes/logs/diffs where supported |
| `--json` | Compatibility alias for `--format json`; do not combine with `--format text` or `--format raw` |
| `--compact` | Compact JSON without indentation (only affects `--format json`) |
| `--quiet` | Suppress text helper output |
| `--dry-run` | Show what would be done without executing (write commands only) |
| `--confirm <confirm_token>` | Non-interactive confirmation for write commands; use the token returned by `--dry-run` |
| `--force` | Deprecated for writes; blocked in agent-safe mode. Use `--dry-run` and `--confirm <confirm_token>` |

> **`--fields`** is registered on individual commands that expose a projectable `data` payload, not as a root persistent flag. See [JSON Output](#json-output).

## Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Generic error |
| 2 | Bad arguments / validation |
| 3 | Resource not found |
| 4 | Authentication / permission failure |
| 5 | Confirmation required or cancelled |
| 6 | Conflict / state drift / non-success CI terminal state |
| 7 | Retryable transient error (rate limit, network, server) |
| 8 | Timeout |

## Agent-safe mode (default ON)

`gitlab-cli` targets AI Agents and automation. Restrictions apply unless you opt out:

| Rule | Detail |
|---|---|
| Default | Agent-safe mode is **on** when `GITLAB_CLI_AGENT_SAFE` is unset |
| Disable | Set `GITLAB_CLI_AGENT_SAFE=0` to turn off all restrictions |
| Writes | Run `--dry-run` first, inspect `data.preview`, then retry with `--confirm <confirm_token>` from `data.confirm_token` |
| `--force` | Deprecated for write commands; use the confirm-token flow |
| `--show-values` | Blocked on `variable` commands unless `GITLAB_CLI_ALLOW_SHOW_VALUES=1` |

Example (merge MR after dry-run):

```bash
gitlab-cli mr merge --project group/proj 42 --dry-run
gitlab-cli mr merge --project group/proj 42 --confirm <confirm_token>
```

Example (self-update after dry-run):

```bash
gitlab-cli update --check
gitlab-cli update --dry-run
gitlab-cli update --confirm <confirm_token>
```

## Multiple profiles

Store credentials for multiple GitLab hosts under named profiles:

```bash
gitlab-cli auth login --host https://gitlab.com --profile personal
gitlab-cli auth login --host https://gitlab.corp.example --profile work
gitlab-cli auth profile list
gitlab-cli auth profile use work
gitlab-cli auth profile remove old
```

Environment variables (`GITLAB_CLI_*` / `GITLAB_*`) still override the active profile when set.

## Security

- Credentials are stored at `~/.gitlab-cli/config.json` with `0600` permissions; the directory is `0700`.
- All write commands are recorded as JSONL under `~/.gitlab-cli/audit/`.
- Token-bearing flags (`--token`, `-t`, `--private-token`, `--oauth-token`, `--job-token`) and secret values (`--value`, `--variable`) are redacted in audit logs.
- HTTPS is required by default; `http://` is allowed only for local development.

For vulnerability reports see [SECURITY.md](SECURITY.md).

## Project Structure

```
gitlab-cli/
├── cmd/
│   ├── gitlab-cli/main.go       # Entry point (semantic exit codes)
│   ├── root.go                  # Global flags, error handling, audit hooks
│   ├── reference.go             # Self-documenting command reference
│   ├── context.go               # Agent bootstrap snapshot
│   ├── auth.go, doctor.go       # Auth & diagnostics
│   ├── update.go                # Self-update from GitHub Releases
│   ├── mr.go, mr_comment.go     # Merge requests
│   ├── issue.go, issue_comment.go
│   ├── pipeline.go, job.go
│   ├── repo.go, release.go
│   ├── label.go, milestone.go, variable.go
│   ├── project.go, user.go, search.go
│   └── helpers.go
├── internal/
│   ├── api/                     # GitLab REST v4 client + domain APIs
│   ├── audit/                   # Write-operation JSONL audit log
│   ├── config/                  # Config + env var precedence
│   ├── gitctx/                  # Local git context (remote, branch)
│   └── output/                  # JSON, tables, flatten/*, error envelope
├── e2e/                         # Integration tests (build tag: integration)
├── docs/                        # E2E guide (Windows-focused), acceptance reports
├── skills/gitlab-cli/SKILL.md   # AI Agent Skill
├── scripts/                     # npm install, e2e Docker/runner helpers
├── package.json                 # npm distribution
├── .goreleaser.yml              # Release automation
├── Makefile                     # Local development
└── .github/workflows/           # CI/CD
```

Local end-to-end testing: see [docs/E2E.md](docs/E2E.md) (**Windows / PowerShell** scripts; Linux/macOS manual steps inside).

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md). Release notes: [CHANGELOG.md](CHANGELOG.md).

## License

MIT © fatecannotbealtered
