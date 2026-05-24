# CI: Pipeline & Job

Trigger, monitor, and debug GitLab CI/CD.

## Pipeline — read

```bash
gitlab-cli pipeline list --project G --json --compact
gitlab-cli pipeline list --project G --ref main --status running --json
gitlab-cli pipeline get --project G 123 --json
gitlab-cli pipeline current --json                    # git remote + branch
gitlab-cli pipeline jobs --project G 123 --json
```

## Pipeline — write

```bash
gitlab-cli pipeline create --project G --ref main --dry-run --json
gitlab-cli pipeline create --project G --ref main --confirm main --json
gitlab-cli pipeline create --project G --ref main --variable FOO=bar --confirm main --json
gitlab-cli pipeline retry --project G 123 --json
gitlab-cli pipeline cancel --project G 123 --confirm 123 --json
```

## Wait for pipeline

```bash
gitlab-cli pipeline wait --project G 123 --timeout 600 --interval 15 --json --compact
# exit 0 = success, 8 = timeout, 9 = failed/canceled/skipped
```

## Job

```bash
gitlab-cli job get --project G 456 --json
gitlab-cli job log --project G 456
gitlab-cli job log --project G 456 --follow --timeout 300
gitlab-cli job log --project G 456 --follow --json          # final {id,status,log}
gitlab-cli job wait --project G 456 --timeout 300 --json
gitlab-cli job retry --project G 456 --json
gitlab-cli job cancel --project G 456 --json
gitlab-cli job artifacts --project G 456 --output ./artifacts.zip
```

## Agent CI loop

```bash
PIPELINE_ID=$(gitlab-cli pipeline create --project G --ref main --confirm main --json --compact | jq -r .id)

gitlab-cli pipeline wait --project G "$PIPELINE_ID" --timeout 600 --json --compact
EXIT=$?

if [ "$EXIT" -ne 0 ]; then
  JOB=$(gitlab-cli pipeline jobs --project G "$PIPELINE_ID" --json --compact \
    | jq -r '[.[] | select(.status=="failed")][0].id')
  gitlab-cli job log --project G "$JOB"
fi
```

## Flat Pipeline JSON (excerpt)

```json
{
  "id": 123,
  "ref": "main",
  "status": "success",
  "webUrl": "https://gitlab.example.com/G/-/pipelines/123"
}
```

## Notes

- `pipeline create` / `cancel` require confirmation
- Job IDs are global numeric IDs (use `pipeline jobs` to discover)
- `pipeline wait` progress on stderr; final JSON on stdout
