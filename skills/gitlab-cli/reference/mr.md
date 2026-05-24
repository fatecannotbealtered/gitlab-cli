# Merge Requests (MR / PR)

Manage MRs: list, create, review, merge, close, approve, diff, comments.

## Read

```bash
gitlab-cli mr list --project G --state opened --json --compact
gitlab-cli mr list --project G --all --json --compact          # up to 10k items
gitlab-cli mr get --project G 42 --json --fields iid,title,state,webUrl
gitlab-cli mr current --json                                   # git branch → open MR
gitlab-cli mr diff --project G 42                              # unified diff (text)
gitlab-cli mr diff --project G 42 --json                       # {"diff":"..."}

gitlab-cli mr comment list --project G 42 --json --compact
```

## Write

Always preview: `--dry-run --json`. Then confirm: `--confirm <token>` (token is usually the IID).

```bash
# Create
gitlab-cli mr create --project G --title "feat: x" \
  --source-branch feat/x --target-branch main --json --compact
gitlab-cli mr create --auto --title "feat: x" --json           # from git context
gitlab-cli mr create --auto --find-existing --json             # return existing open MR if any

# Update / review
gitlab-cli mr update --project G 42 --title "..." --add-labels "needs-review" --json
gitlab-cli mr approve --project G 42 --json
gitlab-cli mr unapprove --project G 42 --json

# Merge / close (requires confirmation)
gitlab-cli mr merge --project G 42 --confirm 42 --json
gitlab-cli mr merge --project G 42 --confirm 42 --should-remove-source-branch --json
gitlab-cli mr close --project G 42 --confirm 42 --json
gitlab-cli mr reopen --project G 42 --json

# Comments
gitlab-cli mr comment add --project G 42 --body "LGTM, nit: rename foo"
gitlab-cli mr comment add --project G 42 --body-file review.txt
gitlab-cli mr comment delete --project G 42 --note-id 99 --confirm 99 --json
```

## Flat MR JSON (excerpt)

```json
{
  "iid": 42,
  "title": "feat: add login",
  "state": "opened",
  "source": "feat/login",
  "target": "main",
  "author": "alice",
  "webUrl": "https://gitlab.example.com/G/-/merge_requests/42",
  "draft": false
}
```

## List JSON envelope

```json
{"items":[...],"count":5,"limit":20,"hasMore":true,"all":false}
```

## Workflows

### Review → comment → approve

```bash
gitlab-cli mr get --project G 42 --json --compact
gitlab-cli mr diff --project G 42
gitlab-cli mr comment add --project G 42 --body "Please extract helper"
gitlab-cli mr approve --project G 42 --json
```

### Create MR → wait CI → merge

See [ci.md](ci.md). After pipeline success:

```bash
gitlab-cli mr merge --project G 42 --confirm 42 --json
```

## Notes

- `mr current` → exit **4** if no open MR for current branch
- `mr merge` → `requiresConfirmation`, `riskLevel: critical`
- Positional IID: `mr get --project G 42` (flags before IID)
