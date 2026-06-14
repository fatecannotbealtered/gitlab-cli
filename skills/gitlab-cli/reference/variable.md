# CI/CD Variables

Secrets and config for pipelines. **Values are sensitive.**

```bash
gitlab-cli variable list --project G --compact        # values redacted
gitlab-cli variable get --project G --key MY_SECRET
gitlab-cli variable get --project G --key X --filter env_scope=production

# Show secrets only when user explicitly needs them:
# export GITLAB_CLI_ALLOW_SHOW_VALUES=1
gitlab-cli variable list --project G --show-values

# variable create/update/delete are write-dangerous: --dangerous in BOTH steps.
gitlab-cli variable create --project G --key FOO --value bar --masked --dangerous --dry-run
gitlab-cli variable create --project G --key FOO --value bar --masked --dangerous --confirm <confirm_token>
gitlab-cli variable update --project G --key FOO --value baz --dangerous --dry-run
gitlab-cli variable update --project G --key FOO --value baz --dangerous --confirm <confirm_token>
gitlab-cli variable delete --project G --key FOO --dangerous --confirm <confirm_token>

# Optional: idempotent create (Idempotency-Key header + bound into the token).
gitlab-cli variable create --project G --key FOO --value bar --idempotency-key vars-foo-001 --dangerous --dry-run
```

## Bulk import (`variable bulk-import`)

Import many variables from a `.env` (KEY=value lines) or JSON (`{"KEY":"value"}`) file: new keys are created, existing keys updated (CLI-SPEC §15). One `confirm_token`, aggregated `items[]` + `summary`.

```bash
gitlab-cli variable bulk-import --project G --file .env --dangerous --dry-run
gitlab-cli variable bulk-import --project G --file vars.json --env-scope production --dangerous --confirm <confirm_token>
```

Each `data.items[]` entry reports `{target:key, ok, action:created|updated, envScope}` or `error{code,retryable}`.

**Write-dangerous** (CLI-SPEC §15.4): because it writes/overwrites secret CI variables — exactly the secrets the single `variable create/update/delete` commands guard — it requires `--dangerous` in BOTH the `--dry-run` and `--confirm` steps; missing it returns exit `5`/`E_CONFIRMATION_REQUIRED` even with a valid token. `--continue-on-error` defaults to **false** here, so a failed write stops the batch instead of charging through the rest of the secrets.

**Imported variables are unmasked and unprotected.** Every key is created/updated with `masked=false`, `protected=false`, `raw=false`, and the single shared `--env-scope` (default `*`); there is no per-key way to mark an imported secret as masked or protected. To mask/protect specific keys, set them afterward with `variable update` or import then patch. Note: a `.env`/JSON file may contain plaintext secrets — keep it out of logs and version control.

## Variable data payload (no value)

```json
{
  "key": "MY_SECRET",
  "type": "env_var",
  "masked": true,
  "protected": false,
  "envScope": "*"
}
```

## Notes

- `--type`: `env_var` (default) or `file`
- `--env-scope` default `*`; use `production`, etc. for overrides
- `--show-values` blocked in agent-safe mode unless `GITLAB_CLI_ALLOW_SHOW_VALUES=1`
- Never log raw values; prefer redacted JSON
- `permissionTier: write-dangerous` — `create`/`update`/`delete` require `--dangerous` in both `--dry-run` and `--confirm`; missing it → exit `5`/`E_CONFIRMATION_REQUIRED`
- `--idempotency-key` on `create` sends the `Idempotency-Key` header and is bound into the confirm token
