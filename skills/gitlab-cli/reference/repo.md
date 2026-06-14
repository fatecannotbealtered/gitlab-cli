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

## Atomic multi-file commit (`repo commit create`)

One commit applies many file actions atomically via the native `actions[]` endpoint (CLI-SPEC §15, class A). Each `--action` is `type:field=value;field=value` (repeatable), up to 1000 actions.

```bash
gitlab-cli repo commit create --project G --branch main --message "sync config" \
  --action 'create:path=docs/a.md;content=hello' \
  --action 'update:path=README.md;content_file=./README.md' \
  --action 'delete:path=old.txt' \
  --action 'move:path=new/x.go;previous_path=old/x.go' \
  --dry-run
gitlab-cli repo commit create --project G --branch main --message "sync config" \
  --action 'create:path=docs/a.md;content=hello' --confirm <confirm_token>
```

Result: the resulting `commitId`/`shortId`/`webUrl` plus aggregated `items[]` (one per action) and `summary`. The commit is server-side atomic — all actions land in one commit or none do, so on failure every item reports the same error.

The 1000-action cap is a **hard limit**, not an auto-chunked one. Unlike the chunked batches in CLI-SPEC §15.6, an atomic commit cannot be split across calls without breaking atomicity, so a batch over 1000 actions is rejected with `E_VALIDATION` (exit 2) rather than silently chunked — split the work into separate commits yourself if you need more.

## Notes

- `repo file get` → JSON metadata/content by default; use `--format raw` for unwrapped bytes or `--output` to save locally
- `repo file delete` / `repo branch delete` are `permissionTier: write-dangerous` — require `--dangerous` in both `--dry-run` and `--confirm` (missing → exit `5`/`E_CONFIRMATION_REQUIRED`)
- `repo file update`/`repo file delete` bind `last_commit_id`: if the file moved since `--dry-run`, confirm returns exit `6`/`E_CONFLICT`
- `repo file create` / `repo branch create` accept `--idempotency-key` (Idempotency-Key header + bound into the token)
- `--path` is repo-relative (no leading slash)
