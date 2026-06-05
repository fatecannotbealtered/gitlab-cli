# Contracts: flags, JSON, exit codes, audit

## Global flags

| Flag | Purpose |
|------|---------|
| `--format json|text|raw` | Output format. Default: `json`; `text` is human-readable; `raw` is unwrapped bytes/logs/diffs where supported |
| `--json` | Compatibility alias for `--format json`; do not combine with `--format text/raw` |
| `--compact` | Minified JSON (only affects `--format json`) |
| `--quiet` | Suppress text helper output |
| `--fields a,b,c` | Project fields from flat JSON (case-insensitive; JSON only) |
| `--dry-run` | Preview writes without executing |
| `--confirm <token>` | Non-interactive confirmation (preferred) |
| `--force` | Skip confirmation (needs `GITLAB_CLI_ALLOW_FORCE=1` in agent-safe mode) |

List commands also support `--limit` (1–100) and `--all` (up to 10000 items).

## Agent-safe mode (default ON)

| Env | Effect |
|-----|--------|
| `GITLAB_CLI_AGENT_SAFE=0` | Disable restrictions |
| `GITLAB_CLI_ALLOW_FORCE=1` | Allow `--force` |
| `GITLAB_CLI_ALLOW_SHOW_VALUES=1` | Allow `variable --show-values` |

## List JSON envelope

```json
{
  "items": [],
  "count": 0,
  "limit": 20,
  "page": 1,
  "total": 0,
  "hasMore": false,
  "all": false
}
```

Not all list commands migrated yet; `mr list` uses this shape. Others may still return a bare array — check `reference` or prefer `mr`-style commands as reference.

## Error envelope (stderr, JSON)

```json
{
  "error": "GitLab API error 404: ...",
  "statusCode": 404,
  "errorCode": "NOT_FOUND",
  "hint": "Verify the resource exists..."
}
```

| errorCode | Typical cause |
|-----------|----------------|
| `AUTH_REQUIRED` | 401 / not logged in |
| `FORBIDDEN` | 403 / PAT scope |
| `NOT_FOUND` | 404 |
| `VALIDATION_ERROR` | Bad flags |
| `CANCELLED` | Missing/wrong `--confirm` |
| `RATE_LIMITED` | 429 |
| `NETWORK_ERROR` | Connection / DNS |

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Bad arguments |
| 3 | Auth required |
| 4 | Not found |
| 5 | Forbidden |
| 6 | Rate limited |
| 7 | Network / server |
| 8 | Timeout (wait commands) |
| 9 | CI failed (wait commands) |
| 10 | Cancelled / confirmation missing |

## Auth / doctor JSON (excerpt)

```json
{"configured":true,"host":"https://gitlab.example.com","source":"env-cli"}
{"authValid":true,"latencyMs":120,"username":"alice"}
```

## Audit

Write commands log to `~/.gitlab-cli/audit/audit-YYYY-MM.jsonl`.

| Env | Default |
|-----|---------|
| `GITLAB_NO_AUDIT=1` | disable |
| `GITLAB_AUDIT_RETENTION_MONTHS` | 3 |

Redacted flags include: `--token`, `--value`, `--content`, `--body`, `--variable`, etc.

## Machine-readable command tree

```bash
gitlab-cli reference --compact
```

Top-level fields: `globalFlags`, `exitCodes`, `commands[]` with `write`, `requiresConfirmation`, `riskLevel`, `outputType`.
