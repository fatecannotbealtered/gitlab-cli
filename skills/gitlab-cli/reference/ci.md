# CI: Pipeline & Job

Trigger, monitor, and debug GitLab CI/CD.

## Pipeline — read

```bash
gitlab-cli pipeline list --project G --compact
gitlab-cli pipeline list --project G --ref main --status running
gitlab-cli pipeline get --project G 123
gitlab-cli pipeline current                    # git remote + branch
gitlab-cli pipeline jobs --project G 123
```

## Pipeline — write

```bash
gitlab-cli pipeline create --project G --ref main --dry-run
gitlab-cli pipeline create --project G --ref main --confirm <confirm_token>
gitlab-cli pipeline create --project G --ref main --variable FOO=bar --confirm <confirm_token>
gitlab-cli pipeline retry --project G 123
gitlab-cli pipeline cancel --project G 123 --confirm <confirm_token>
```

## Wait for pipeline

```bash
gitlab-cli pipeline wait --project G 123 --timeout 600 --interval 15 --compact
# exit 0 = success, 8 = timeout, 6 = failed/canceled/skipped
```

## Job

```bash
gitlab-cli job get --project G 456
gitlab-cli job log --project G 456                       # {"jobId":456,"log":"..."}
gitlab-cli job log --project G 456 --format raw          # trace bytes
gitlab-cli job log --project G 456 --follow --format text --timeout 300
gitlab-cli job log --project G 456 --follow          # final {id,status,log}
gitlab-cli job wait --project G 456 --timeout 300
gitlab-cli job retry --project G 456
gitlab-cli job cancel --project G 456
gitlab-cli job artifacts --project G 456 --output ./artifacts.zip
```

## Agent CI loop

```bash
PIPELINE_ID=$(gitlab-cli pipeline create --project G --ref main --confirm <confirm_token> --compact | jq -r .data.id)

gitlab-cli pipeline wait --project G "$PIPELINE_ID" --timeout 600 --compact
EXIT=$?

if [ "$EXIT" -ne 0 ]; then
  JOB=$(gitlab-cli pipeline jobs --project G "$PIPELINE_ID" --compact \
    | jq -r '[.data[] | select(.status=="failed")][0].id')
  gitlab-cli job log --project G "$JOB" --format raw
fi
```

## Pipeline data payload (excerpt)

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
