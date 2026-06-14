# Issues

Manage issues: list, create, update, assign, labels, close, comments.

## Read

```bash
gitlab-cli issue list --project G --state opened --compact
gitlab-cli issue get 12 --project G --fields iid,title,state,assignee
gitlab-cli issue comment list 12 --project G --compact
```

## Write

```bash
gitlab-cli issue create --project G --title "Bug: ..." --label bug
gitlab-cli issue create --project G --title "Bug: ..." --idempotency-key bug-001   # idempotent create
# update binds updated_at: if the issue changed since --dry-run, confirm → exit 6/E_CONFLICT
gitlab-cli issue update 12 --project G --add-labels urgent
gitlab-cli issue assign 12 alice --project G
gitlab-cli issue assign 12 me --project G
gitlab-cli issue label 12 --project G --add bug --remove triage

gitlab-cli issue close 12 --project G --dry-run
gitlab-cli issue close 12 --project G --confirm <confirm_token>
gitlab-cli issue reopen 12 --project G

gitlab-cli issue comment add 12 --project G --body "Repro steps: ..."
gitlab-cli issue comment delete 12 --project G --note-id 99 --confirm <confirm_token>
```

## Issue data payload (excerpt)

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
