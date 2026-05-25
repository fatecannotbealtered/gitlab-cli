# gitlab-cli

[![CI](https://github.com/fatecannotbealtered/gitlab-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/fatecannotbealtered/gitlab-cli/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

English | [中文](README_zh.md)

AI-Agent-friendly GitLab CLI. Manage merge requests, issues, pipelines, jobs, repositories, releases, labels, milestones, CI variables, and more — with a strict, machine-readable contract designed for AI Agents and automation.

Built with Go. Single static binary. Compatible with **GitLab.com, GitLab Self-Managed, and GitLab Dedicated**.

[Installation](#installation) · [Authentication](#authentication) · [Commands](#commands) · [JSON Output](#json-output) · [Security](#security) · [Contributing](#contributing) · [Disclaimer](#disclaimer)

## Why another GitLab CLI?

GitLab already ships an excellent CLI called [glab](https://gitlab.com/gitlab-org/cli) — for humans. `gitlab-cli` is **purpose-built for AI Agents** and shares the design DNA of [`jira-cli`](https://github.com/fatecannotbealtered/jira-cli):

- **Strict JSON contract** — global `--json` output in flat, token-efficient form on every command.
- **`--fields key,name,state` field projection** on supported commands (opt-in per command, not global) for further token reduction.
- **`--dry-run`** previews every write command before executing.
- **`--confirm <token>`** for non-interactive write confirmation (preferred for Agents); **`--force`** skips prompts but is restricted in agent-safe mode (see [Agent-safe mode](#agent-safe-mode-default-on)).
- **Machine-readable error envelope** with `errorCode`, `statusCode`, and actionable `hint`.
- **Semantic exit codes** (`0`/`2`/`3`/`4`/`5`/`6`/`7`/`8`/`9`/`10`).
- **JSONL audit log** of every write command at `~/.gitlab-cli/audit/`.
- **`SKILL.md`** installable via [`npx skills`](https://github.com/vercel-labs/skills) for compatible AI coding assistants.

Use `glab` interactively. Use `gitlab-cli` in scripts and Agent loops.

## Disclaimer

This project is shared for **personal learning, research, and AI tooling experiments**, not as a product with SLAs. Maintainers do not offer commercial support or **fitness-for-production** promises. Software is provided "as is".

## Installation

### Quick start

Copy the block below to your agent (or run it yourself):

```bash
# Please install gitlab-cli and use it for all GitLab operations going forward (always pass --json).
# Install CLI
npm install -g @fatecannotbealtered-/gitlab-cli

# Install CLI Skill (required) — copies into your agent-supported skills directory
npx skills add fatecannotbealtered/gitlab-cli -y -g

# Login and verify
gitlab-cli auth login
gitlab-cli doctor --json
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

> Run `gitlab-cli reference` to print every command, subcommand, and flag in machine-parseable Markdown. Use `gitlab-cli reference --json` for agent introspection (includes `path`, `write`, `outputType`, `positionalArgs`, and more).

### Authentication & diagnostics

```bash
gitlab-cli auth login [--host URL] [--token PAT] [--profile NAME]
gitlab-cli auth logout
gitlab-cli auth status
gitlab-cli auth profile list [--json]
gitlab-cli auth profile use <name>
gitlab-cli auth profile remove <name> [--json]
gitlab-cli doctor
```

### Workflow context

```bash
gitlab-cli context              # one-shot snapshot for AI Agents (current git + GitLab + project)
gitlab-cli context --json
```

### Users & Projects

```bash
gitlab-cli user me [--json]
gitlab-cli user search --query <q> [--active] [--limit N] [--json]
gitlab-cli user get <username> [--json]

gitlab-cli project list [--owned] [--membership] [--search <q>] [--visibility <v>] [--json]
gitlab-cli project get <id-or-path> [--json]
gitlab-cli project members <id-or-path> [--query <q>] [--json]
```

### Search

```bash
gitlab-cli search projects --query <q> [--json]
gitlab-cli search issues   --query <q> [--project <id>] [--json]
gitlab-cli search mrs      --query <q> [--project <id>] [--json]
gitlab-cli search code     --query <q>  --project <id>  [--json]   # blob/code search REQUIRES --project
gitlab-cli search commits  --query <q> [--project <id>] [--json]
```

### Merge Requests

```bash
gitlab-cli mr list      --project <id> [--state opened|closed|merged|all] [--json]
gitlab-cli mr get       --project <id> <iid> [--json] [--fields ...]
gitlab-cli mr current   [--json]                          # uses git context
gitlab-cli mr create    --project <id> --title <t> --source-branch <s> --target-branch main [--json]
gitlab-cli mr create    --auto [--target-branch main] [--title <t>] [--draft] [--json]
gitlab-cli mr update    --project <id> <iid> [--title ...] [--add-labels l1,l2] [--remove-labels ...] [--json]
gitlab-cli mr merge     --project <id> <iid> [--squash] [--should-remove-source-branch] [--json]
gitlab-cli mr close     --project <id> <iid> [--json]
gitlab-cli mr reopen    --project <id> <iid> [--json]
gitlab-cli mr approve   --project <id> <iid> [--json]
gitlab-cli mr unapprove --project <id> <iid> [--json]
gitlab-cli mr diff      --project <id> <iid> [--json]     # default: unified diff text; --json → {"diff":"..."}
gitlab-cli mr comment add    --project <id> <iid> --body <text> [--json]
gitlab-cli mr comment list   --project <id> <iid> [--json]
gitlab-cli mr comment delete --project <id> <iid> --note-id <id> --confirm <note-id> [--json]
```

### Issues

```bash
gitlab-cli issue list      --project <id> [--state ...] [--assignee <u>] [--label l1,l2] [--json]
gitlab-cli issue get       <iid> --project <id> [--json]
gitlab-cli issue create    --project <id> --title <t> [--description <d>] [--label l1,l2] [--json]
gitlab-cli issue update    <iid> --project <id> [--add-labels ...] [--remove-labels ...] [--json]
gitlab-cli issue close     <iid> --project <id> [--json]
gitlab-cli issue reopen    <iid> --project <id> [--json]
gitlab-cli issue assign    <iid> <username|me> --project <id> [--json]
gitlab-cli issue label     <iid> --project <id> --add l1,l2 --remove l3,l4 [--json]
gitlab-cli issue comment add    <iid> --project <id> --body <t> [--json]
gitlab-cli issue comment list   <iid> --project <id> [--json]
gitlab-cli issue comment delete <iid> --project <id> --note-id <id> --confirm <note-id> [--json]
```

### Labels & Milestones

```bash
gitlab-cli label list   --project <id> [--json]
gitlab-cli label create --project <id> --name <n> --color <#hex|named> [--priority N] [--json]
gitlab-cli label update --project <id> --label-id N [--name ...] [--color ...] [--json]
gitlab-cli label delete --project <id> --label-id N --confirm N [--json]

gitlab-cli milestone list   --project <id> [--state active|closed|all] [--json]
gitlab-cli milestone get    --project <id> --milestone-id N [--json]
gitlab-cli milestone create --project <id> --title <t> [--due-date YYYY-MM-DD] [--json]
gitlab-cli milestone update --project <id> --milestone-id N [--title ...] [--state-event close|activate] [--json]
gitlab-cli milestone close  --project <id> --milestone-id N --confirm N [--json]
```

### Pipelines & Jobs

```bash
gitlab-cli pipeline list    --project <id> [--ref <b>] [--status ...] [--json]
gitlab-cli pipeline get     --project <id> <pipeline_id> [--json]
gitlab-cli pipeline current [--json]                       # latest pipeline for current branch
gitlab-cli pipeline create  --project <id> --ref <b> [--variable KEY=VAL]... [--json]
gitlab-cli pipeline retry   --project <id> <pipeline_id> [--json]
gitlab-cli pipeline cancel  --project <id> <pipeline_id> [--json]
gitlab-cli pipeline jobs    --project <id> <pipeline_id> [--scope ...] [--json]
gitlab-cli pipeline wait    --project <id> <pipeline_id> [--timeout 300] [--interval 10] [--json]

gitlab-cli job get       --project <id> <job_id> [--json]
gitlab-cli job log       --project <id> <job_id>           # plain-text trace
gitlab-cli job log       --project <id> <job_id> --follow  # stream until job completes
gitlab-cli job retry     --project <id> <job_id> [--json]
gitlab-cli job cancel    --project <id> <job_id> [--json]
gitlab-cli job artifacts --project <id> <job_id> --output artifacts.zip
gitlab-cli job wait      --project <id> <job_id> [--timeout 300] [--interval 5] [--json]
```

### Repository (file / branch / commit / tree) & Releases

```bash
gitlab-cli repo file get    --project <id> --path <p> [--ref <b>] [--output <path>]
gitlab-cli repo file create --project <id> --path <p> --branch <b> --content ... --commit-message <m> [--json]
gitlab-cli repo file update --project <id> --path <p> --branch <b> --content ... --commit-message <m> [--json]
gitlab-cli repo file delete --project <id> --path <p> --branch <b> --commit-message <m> --confirm <path> [--json]

gitlab-cli repo branch list   --project <id> [--search <q>] [--json]
gitlab-cli repo branch create --project <id> --name <n> --ref <source> [--json]
gitlab-cli repo branch delete --project <id> --name <n> --confirm <n> [--json]

gitlab-cli repo commit list --project <id> [--ref-name <b>] [--since ...] [--until ...] [--path <p>] [--json]
gitlab-cli repo commit get  --project <id> <sha> [--json]

gitlab-cli repo tree --project <id> [--path <p>] [--ref <b>] [--recursive] [--json]

gitlab-cli release list   --project <id> [--json]
gitlab-cli release get    --project <id> --tag <tag> [--json]
gitlab-cli release create --project <id> --tag <tag> --name <n> [--description <d>] [--ref <b>] [--milestone m1,m2] [--json]
gitlab-cli release update --project <id> --tag <tag> [--name ...] [--description ...] [--json]
gitlab-cli release delete --project <id> --tag <tag> --confirm <tag> [--json]
```

### CI/CD Variables

```bash
gitlab-cli variable list   --project <id> [--json]                                     # values redacted in --json unless --show-values
gitlab-cli variable get    --project <id> --key <k> [--filter env_scope=<scope>] [--json]
gitlab-cli variable create --project <id> --key <k> --value <v> [--type env_var|file] [--protected] [--masked] [--env-scope <s>] [--json]
gitlab-cli variable update --project <id> --key <k> [--value <v>] [--protected] [--masked] [--json]
gitlab-cli variable delete --project <id> --key <k> [--filter env_scope=<scope>] --confirm <k> [--json]
```

Pass `--show-values` on any `variable` subcommand to include secret values in `--json` output (default: redacted).

## JSON Output

All commands support `--json` for machine-readable output. Some commands also support **`--fields`** (comma-separated projection from flat JSON); it is **opt-in per command** — run `gitlab-cli reference --json` and look for `supports --fields` (e.g. `mr get`, `issue list`, `project get`).

```bash
# Flat JSON (default) — minimal fields, low token cost
gitlab-cli auth status --json
gitlab-cli doctor --json

# Select only the fields you need (supported commands only)
gitlab-cli mr get --project group/proj 42 --json --fields iid,title,state,webUrl

# Clean output for scripts (suppress all non-JSON noise)
gitlab-cli doctor --json --quiet

# Compact JSON (minimal tokens)
gitlab-cli doctor --json --compact
```

Error responses include machine-readable error codes and actionable hints:

```json
{
  "error": "GitLab API error 404: 404 Project Not Found",
  "statusCode": 404,
  "errorCode": "NOT_FOUND",
  "hint": "Verify the resource (project path, IID, ID) exists and you have permission to view it"
}
```

## Global Flags

| Flag | Description |
|---|---|
| `--json` | Output result as JSON |
| `--compact` | Compact JSON without indentation (use with `--json`) |
| `--quiet` | Suppress non-JSON stdout output |
| `--dry-run` | Show what would be done without executing (write commands only) |
| `--confirm <token>` | Non-interactive confirmation for write commands (preferred; token is shown in `--dry-run` or error output) |
| `--force` | Skip interactive confirmation prompts (disabled by default in [agent-safe mode](#agent-safe-mode-default-on); requires `GITLAB_CLI_ALLOW_FORCE=1`) |

> **`--fields`** is registered on individual commands that expose a flat JSON schema, not as a root persistent flag. See [JSON Output](#json-output).

## Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 2 | Bad arguments / validation |
| 3 | Authentication error (401) |
| 4 | Resource not found |
| 5 | Forbidden |
| 6 | Rate limited |
| 7 | Network / server error |
| 8 | Timeout (`pipeline wait` / `job wait`) |
| 9 | CI/CD failure (`pipeline wait` / `job wait` finished failed/canceled/skipped) |
| 10 | User cancelled or confirmation not provided (use `--confirm <token>` for non-interactive writes) |

## Agent-safe mode (default ON)

`gitlab-cli` targets AI Agents and automation. Restrictions apply unless you opt out:

| Rule | Detail |
|---|---|
| Default | Agent-safe mode is **on** when `GITLAB_CLI_AGENT_SAFE` is unset |
| Disable | Set `GITLAB_CLI_AGENT_SAFE=0` to turn off all restrictions |
| Writes | Prefer `--dry-run --json` first, then `--confirm <token>` (token is usually the resource id/path/tag shown in dry-run or error output) |
| `--force` | Blocked unless `GITLAB_CLI_ALLOW_FORCE=1` (only when the user explicitly authorizes skipping confirmation) |
| `--show-values` | Blocked on `variable` commands unless `GITLAB_CLI_ALLOW_SHOW_VALUES=1` |

Example (merge MR after dry-run):

```bash
gitlab-cli mr merge --project group/proj 42 --dry-run --json
gitlab-cli mr merge --project group/proj 42 --confirm 42 --json
```

## Multiple profiles

Store credentials for multiple GitLab hosts under named profiles:

```bash
gitlab-cli auth login --host https://gitlab.com --profile personal
gitlab-cli auth login --host https://gitlab.corp.example --profile work
gitlab-cli auth profile list --json
gitlab-cli auth profile use work
gitlab-cli auth profile remove old --json
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
