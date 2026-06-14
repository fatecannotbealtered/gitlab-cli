# Merge Requests (MR / PR)

Manage MRs: list, create, review, merge, close, approve, diff, comments.

## Read

```bash
gitlab-cli mr list --project G --state opened --compact
gitlab-cli mr list --project G --all --compact          # up to 10k items
gitlab-cli mr get --project G 42 --fields iid,title,state,webUrl
gitlab-cli mr current                                   # git branch → open MR
gitlab-cli mr diff --project G 42                       # {"diff":"..."}
gitlab-cli mr diff --project G 42 --format raw          # unified diff bytes

gitlab-cli mr comment list --project G 42 --compact

# Threaded discussions (richer than flat comments: each thread keeps its notes)
gitlab-cli mr discussion list --project G 42 --compact
```

## Write

Always preview: `--dry-run`. Then confirm with `--confirm <confirm_token>` from `data.confirm_token`.

```bash
# Create (optional --idempotency-key sends Idempotency-Key header + binds the token)
gitlab-cli mr create --project G --title "feat: x" \
  --source-branch feat/x --target-branch main --compact
gitlab-cli mr create --auto --title "feat: x"           # from git context
gitlab-cli mr create --auto --find-existing             # return existing open MR if any
gitlab-cli mr create --project G --title "feat: x" --source-branch feat/x --idempotency-key mr-featx-001

# Update / review (update binds updated_at: a concurrent edit → exit 6/E_CONFLICT)
gitlab-cli mr update --project G 42 --title "..." --add-labels "needs-review"
gitlab-cli mr approve --project G 42
gitlab-cli mr unapprove --project G 42

# Merge is write-dangerous: --dangerous in BOTH steps. Merge binds updated_at + head sha.
gitlab-cli mr merge --project G 42 --dangerous --dry-run
gitlab-cli mr merge --project G 42 --dangerous --confirm <confirm_token>
gitlab-cli mr merge --project G 42 --dangerous --confirm <confirm_token> --should-remove-source-branch
gitlab-cli mr close --project G 42 --confirm <confirm_token>
gitlab-cli mr reopen --project G 42

# Comments
gitlab-cli mr comment add --project G 42 --body "LGTM, nit: rename foo"
gitlab-cli mr comment add --project G 42 --body-file review.txt
gitlab-cli mr comment delete --project G 42 --note-id 99 --confirm <confirm_token>

# Discussion threads: reply into an existing thread (discussion-id from `discussion list`)
gitlab-cli mr discussion reply --project G 42 --discussion-id <id> --body "addressed" --dry-run
gitlab-cli mr discussion reply --project G 42 --discussion-id <id> --body "addressed" --confirm <confirm_token>
```

## MR data payload (excerpt)

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

## List data payload

```json
{"items":[...],"count":5,"limit":20,"hasMore":true,"all":false}
```

## Discussion thread payload (excerpt)

`mr discussion list` returns threads, each with its ordered notes. Note `body`
and `author` are **untrusted** (each note tagged `_untrusted`):

```json
{
  "id": "a1b2c3",
  "individualNote": false,
  "notes": [
    {"id": 101, "author": "alice", "body": "please extract helper", "created": "...", "_untrusted": ["body","author"]}
  ]
}
```

## Workflows

### Review → comment → approve

```bash
gitlab-cli mr get --project G 42 --compact
gitlab-cli mr diff --project G 42 --format raw
gitlab-cli mr comment add --project G 42 --body "Please extract helper"
gitlab-cli mr approve --project G 42
```

### Create MR → wait CI → merge

See [ci.md](ci.md). After pipeline success:

```bash
gitlab-cli mr merge --project G 42 --dangerous --dry-run
gitlab-cli mr merge --project G 42 --dangerous --confirm <confirm_token>
```

## Notes

- `mr current` → exit **4** if no open MR for current branch
- `mr merge` → `permissionTier: write-dangerous`, `riskLevel: critical`; needs `--dangerous` in both `--dry-run` and `--confirm`, and binds `updated_at` + head `sha` (mismatch → exit `6`/`E_CONFLICT`)
- `mr update` binds `updated_at`; `mr create` accepts `--idempotency-key`
- Positional IID: `mr get --project G 42` (flags before IID)
