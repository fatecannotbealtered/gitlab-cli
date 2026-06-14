# Repository: files, branches, commits, tree

## Files

```bash
gitlab-cli repo file get --project G --path README.md [--ref main] [--output out.md]
gitlab-cli repo file get --project G --path README.md --format raw > README.md
gitlab-cli repo file create --project G --path src/x.go --branch main \
  --content "..." --commit-message "add x" --dry-run
gitlab-cli repo file create --project G --path src/x.go --branch main \
  --content "..." --commit-message "add x" --idempotency-key file-x-001   # idempotent create
# update binds last_commit_id (optimistic concurrency): a concurrent commit → exit 6/E_CONFLICT
gitlab-cli repo file update --project G --path src/x.go --branch main \
  --content "..." --commit-message "update x"
# delete is write-dangerous (--dangerous in BOTH steps) and also binds last_commit_id
gitlab-cli repo file delete --project G --path src/x.go --branch main \
  --commit-message "remove x" --dangerous --dry-run
gitlab-cli repo file delete --project G --path src/x.go --branch main \
  --commit-message "remove x" --dangerous --confirm <confirm_token>
```

Use `--content-file` only for trusted local paths.

## Branches

```bash
gitlab-cli repo branch list --project G --compact
gitlab-cli repo branch create --project G --name feat/x --ref main
gitlab-cli repo branch create --project G --name feat/x --ref main --idempotency-key br-featx-001
# branch delete is write-dangerous: --dangerous in BOTH steps.
gitlab-cli repo branch delete --project G --name feat/x --dangerous --dry-run
gitlab-cli repo branch delete --project G --name feat/x --dangerous --confirm <confirm_token>
```

## Commits & tree

```bash
gitlab-cli repo commit list --project G --ref-name main --limit 20
gitlab-cli repo commit get --project G abc1234
gitlab-cli repo tree --project G --path src --ref main
gitlab-cli repo tree --project G --recursive --limit 100
```

## Notes

- `repo file get` → JSON metadata/content by default; use `--format raw` for unwrapped bytes or `--output` to save locally
- `repo file delete` / `repo branch delete` are `permissionTier: write-dangerous` — require `--dangerous` in both `--dry-run` and `--confirm` (missing → exit `5`/`E_CONFIRMATION_REQUIRED`)
- `repo file update`/`repo file delete` bind `last_commit_id`: if the file moved since `--dry-run`, confirm returns exit `6`/`E_CONFLICT`
- `repo file create` / `repo branch create` accept `--idempotency-key` (Idempotency-Key header + bound into the token)
- `--path` is repo-relative (no leading slash)
