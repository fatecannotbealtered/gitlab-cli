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
