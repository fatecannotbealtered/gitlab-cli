# Bootstrap: auth, context, doctor, update

## Auth

```bash
# Recommended: env vars (avoid --token in argv / process list)
export GITLAB_CLI_HOST=https://gitlab.example.com
export GITLAB_CLI_TOKEN=<PAT>

gitlab-cli auth status --compact
gitlab-cli auth login --host https://gitlab.example.com --profile default   # token from env
gitlab-cli auth logout
```

### Multi-profile

```bash
gitlab-cli auth login --host https://gitlab.com --profile personal
gitlab-cli auth login --host https://gitlab.corp.example --profile work
gitlab-cli auth profile list
gitlab-cli auth profile use work
gitlab-cli auth profile remove old
```

Precedence: `GITLAB_CLI_*` > `GITLAB_*` > active profile > `~/.gitlab-cli/config.json`

## Context (read first in a workflow)

```bash
gitlab-cli context --compact
gitlab-cli context --no-strict    # do not exit 3 when unauthenticated
```

Key fields: `git.remote.projectPath`, `git.currentBranch`, `gitlab.project.id`, `gitlab.username`

Typical pattern:

```bash
PROJECT=$(gitlab-cli context --compact | jq -r '.data.git.remote.projectPath')
gitlab-cli mr list --project "$PROJECT" --compact
```

## Doctor

```bash
gitlab-cli doctor --compact
```

Check `data.authValid: true` before bulk automation.

## Update CLI

```bash
gitlab-cli update --check --compact
gitlab-cli update --dry-run --compact
gitlab-cli update --confirm <confirm_token> --compact
```

`update --dry-run` returns `data.confirm_token`. `update` downloads GitHub Release assets, verifies `checksums.txt`, then replaces the current binary. On Windows, replacement may be scheduled for after the current process exits.

## Self-description

```bash
gitlab-cli reference --compact
```

Use for `requiresConfirmation`, `riskLevel`, `write`, per-command flags.
