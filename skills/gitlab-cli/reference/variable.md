# CI/CD Variables

Secrets and config for pipelines. **Values are sensitive.**

```bash
gitlab-cli variable list --project G --compact        # values redacted
gitlab-cli variable get --project G --key MY_SECRET
gitlab-cli variable get --project G --key X --filter env_scope=production

# Show secrets only when user explicitly needs them:
# export GITLAB_CLI_ALLOW_SHOW_VALUES=1
gitlab-cli variable list --project G --show-values

gitlab-cli variable create --project G --key FOO --value bar --masked
gitlab-cli variable update --project G --key FOO --value baz
gitlab-cli variable delete --project G --key FOO --confirm FOO
```

## Flat Variable JSON (no value)

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
