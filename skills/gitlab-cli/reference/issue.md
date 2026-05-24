# Issues

Manage issues: list, create, update, assign, labels, close, comments.

## Read

```bash
gitlab-cli issue list --project G --state opened --json --compact
gitlab-cli issue get 12 --project G --json --fields iid,title,state,assignee
gitlab-cli issue comment list 12 --project G --json --compact
```

## Write

```bash
gitlab-cli issue create --project G --title "Bug: ..." --label bug --json
gitlab-cli issue update 12 --project G --add-labels urgent --json
gitlab-cli issue assign 12 alice --project G --json
gitlab-cli issue assign 12 me --project G --json
gitlab-cli issue label 12 --project G --add bug --remove triage --json

gitlab-cli issue close 12 --project G --confirm 12 --json
gitlab-cli issue reopen 12 --project G --json

gitlab-cli issue comment add 12 --project G --body "Repro steps: ..."
gitlab-cli issue comment delete 12 --project G --note-id 99 --confirm 99 --json
```

## Flat Issue JSON (excerpt)

```json
{
  "iid": 1,
  "title": "Bug: login fails",
  "state": "opened",
  "author": "alice",
  "assignee": "bob",
  "labels": "bug,urgent",
  "webUrl": "https://gitlab.example.com/G/-/issues/1"
}
```

## Notes

- Positional IID often **before** `--project`: `issue get 12 --project G`
- `issue close` requires confirmation (`riskLevel: high`)
- `--state all` on list omits state filter
