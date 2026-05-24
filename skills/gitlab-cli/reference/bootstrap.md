# Bootstrap: auth, context, doctor

## Auth

```bash
# Recommended: env vars (avoid --token in argv / process list)
export GITLAB_CLI_HOST=https://gitlab.example.com
export GITLAB_CLI_TOKEN=<PAT>

gitlab-cli auth status --json --compact
gitlab-cli auth login --host https://gitlab.example.com --profile default   # token from env
gitlab-cli auth logout --json
```

### Multi-profile

```bash
gitlab-cli auth login --host https://gitlab.com --profile personal
gitlab-cli auth login --host https://gitlab.corp.example --profile work
gitlab-cli auth profile list --json
gitlab-cli auth profile use work
gitlab-cli auth profile remove old --json
```

Precedence: `GITLAB_CLI_*` > `GITLAB_*` > active profile > `~/.gitlab-cli/config.json`

## Context (read first in a workflow)

```bash
gitlab-cli context --json --compact
gitlab-cli context --no-strict --json    # do not exit 3 when unauthenticated
```

Key fields: `git.remote.projectPath`, `git.currentBranch`, `gitlab.project.id`, `gitlab.username`

Typical pattern:

```bash
PROJECT=$(gitlab-cli context --json --compact | jq -r '.git.remote.projectPath')
gitlab-cli mr list --project "$PROJECT" --json --compact
```

## Doctor

```bash
gitlab-cli doctor --json --compact
```

Check `authValid: true` before bulk automation.

## Self-description

```bash
gitlab-cli reference --json --compact
```

Use for `requiresConfirmation`, `riskLevel`, `write`, per-command flags.
