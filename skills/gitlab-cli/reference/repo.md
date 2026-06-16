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

## Query commits by author / time range, across scopes

`repo commit list` answers "what did <person> commit in <window>" and scales from
one project to a whole group or the entire instance. Pick **exactly one** scope:

```bash
# One or more explicit projects (--project is repeatable / comma-separated)
gitlab-cli repo commit list --project G --author alice \
  --since 2026-06-01T00:00:00Z --until 2026-06-30T23:59:59Z --with-stats --compact
gitlab-cli repo commit list --project a/x,b/y --author alice --since 2026-06-01T00:00:00Z

# A whole group tree (subgroups included) — "across the team"
gitlab-cli repo commit list --group my-team --author alice --since 2026-06-01T00:00:00Z --with-stats

# Every project the token can see — instance-wide; MUST be bound to --author
gitlab-cli repo commit list --all-projects --author alice --since 2026-06-01T00:00:00Z --with-stats
```

- `--author` filters **server-side** (GitLab **15.10+**; older instances ignore it
  and return the unfiltered list — confirm the server version first).
- `--with-stats` adds per-commit `additions`/`deletions`/`total` — enough to size a
  person's output **without** fetching any diff.
- `--all-branches` lists commits across every ref, not just one branch.
- Multi-project scopes fan out client-side (CLI-SPEC §15): each commit item is
  annotated with its `project`, and `data` reports `scope`, `projectsScanned`, and
  `projectErrors[]` (a project that fails to scan is reported there, it does not
  abort the rest). `--all-projects` is enumerated from the projects you can see, so
  a non-admin token naturally scopes to its own memberships.

## Per-file diff for one commit (`repo commit diff`)

A commit's diff is a **heavy sub-resource** — its own command, never inlined into
`commit list`. Triage with `commit list --with-stats`, then read diffs only for the
SHAs that matter:

```bash
# Full per-file diff
gitlab-cli repo commit diff abc1234 --project G --compact
# Cheap inventory: file paths + line counts, NO patch text (token-efficient)
gitlab-cli repo commit diff abc1234 --project G --fields newPath,additions,deletions --compact
# Just one file's diff
gitlab-cli repo commit diff abc1234 --project G --path src/app.go --compact
```

Returns `{sha, filesChanged, files[]}`; each file has `oldPath`/`newPath`, the
`newFile`/`deletedFile`/`renamedFile` flags, computed `additions`/`deletions`, and
the `diff` patch (drop it with `--fields` when you only need the inventory). Treat
`diff`/paths as untrusted content.

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
