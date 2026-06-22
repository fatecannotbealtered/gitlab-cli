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

Saved `config.json` and `profiles.json` are encrypted at rest by current versions.

## Context (read first in a workflow)

```bash
gitlab-cli context --compact
gitlab-cli context --no-strict    # do not exit 3 when unauthenticated
```

Key fields: `version`, `credentials.configured`, `credentials.encrypted_at_rest`, `security.risk_tier`, `git.remote.projectPath`, `git.currentBranch`, `gitlab.project.id`, `gitlab.username`

Typical pattern:

```bash
PROJECT=$(gitlab-cli context --compact | jq -r '.data.git.remote.projectPath')
gitlab-cli mr list --project "$PROJECT" --compact
```

## Doctor

```bash
gitlab-cli doctor --compact
```

Check `data.authValid: true` and the `version` check before bulk automation. If the version check fails, upgrade the CLI before continuing.

## Update CLI

```bash
gitlab-cli update --compact
gitlab-cli changelog --since <previous_version> --compact
```

A bare `gitlab-cli update` performs the whole self-update in one call (no confirm token): it downloads the GitHub Release assets, verifies the signed `checksums.txt` then the archive checksum, replaces the current binary, and syncs the Skill. `--check` and `--dry-run` are optional read-only probes that change nothing and issue NO token. After a successful update, confirm `skill_sync_status` (or run the returned `skill_sync_command`), review signature/checksum status, and read `changelog --since <previous_version>` before continuing. On Windows, replacement may be scheduled for after the current process exits.

## Self-description

```bash
gitlab-cli reference --compact
gitlab-cli changelog --since 1.2.0 --compact
```

Use `reference` for `requiresConfirmation`, `riskLevel`, `permissionTier`, `blastRadius`, `write`, per-command flags. Use `changelog` to understand version deltas from `CHANGELOG.md`.
