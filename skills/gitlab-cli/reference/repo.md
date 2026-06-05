# Repository: files, branches, commits, tree

## Files

```bash
gitlab-cli repo file get --project G --path README.md [--ref main] [--output out.md]
gitlab-cli repo file get --project G --path README.md --format raw > README.md
gitlab-cli repo file create --project G --path src/x.go --branch main \
  --content "..." --commit-message "add x" --dry-run
gitlab-cli repo file update --project G --path src/x.go --branch main \
  --content "..." --commit-message "update x"
gitlab-cli repo file delete --project G --path src/x.go --branch main \
  --commit-message "remove x" --confirm src/x.go
```

Use `--content-file` only for trusted local paths.

## Branches

```bash
gitlab-cli repo branch list --project G --compact
gitlab-cli repo branch create --project G --name feat/x --ref main
gitlab-cli repo branch delete --project G --name feat/x --confirm feat/x
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
- `repo file delete` / `repo branch delete` require confirmation
- `--path` is repo-relative (no leading slash)
