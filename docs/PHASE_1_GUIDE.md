# Phase 1 Implementation Guide (for subagents)

> **Audience**: AI subagents implementing GitLab REST domains (mr, issue, pipeline, job, repo, release, etc.).
>
> **Goal**: Implement methods on the existing `*XxxAPI` stubs and add a `cmd/<domain>.go` Cobra command tree, plus tests and a SKILL.md segment.

## Hard rules

1. **Do not touch** these files (they are shared / already done):
   - `internal/api/client.go`
   - `internal/api/types.go` (DTOs go in your domain file instead)
   - `cmd/root.go`
   - `cmd/main.go` / `cmd/gitlab-cli/main.go`
   - `cmd/auth.go`, `cmd/doctor.go`, `cmd/reference.go`
   - `internal/output/*`, `internal/audit/*`, `internal/config/*`, `internal/gitctx/*`
   - `skills/gitlab-cli/SKILL.md` (root file — write a segment instead, see below)
   - `README.md`, `README_zh.md`, `CHANGELOG.md`

2. **You may modify or create**:
   - `internal/api/<domain>.go` (stub already exists with `type <Domain>API struct{ client *Client }` — add DTOs and methods)
   - `internal/api/<domain>_test.go` (create new)
   - `cmd/<domain>.go` (create new; if a domain is large, split into `cmd/<domain>_<sub>.go`)
   - `cmd/<domain>_test.go` (create new)
   - `skills/gitlab-cli/_skill_<domain>.md` (create new — a Markdown fragment that will be merged into the root SKILL.md by Phase 3)
   - `internal/output/flatten_<domain>.go` (create new — define your `FlatXxx` types and `XxxToMap` helpers here so parallel subagents don't conflict on a shared file)
   - **Do NOT** modify the existing `internal/output/flatten.go`.

3. **Verification before declaring done** — every subagent MUST run:
   ```bash
   gofmt -w .
   go vet ./...
   go build ./...
   go test ./...
   ```
   All four must succeed. If any test fails, fix it before reporting completion.

## API conventions

### URL building

- Always go through `c.client.APIPath("/...")` to get `/api/v4/...`. Do not hardcode `/api/v4`.
- Use `EncodeProjectPath(projectID)` for project paths in URLs (handles both numeric IDs and `group/subgroup/project`).
- Use `url.QueryEscape` for query parameters and `url.PathEscape` for non-project path segments.

### Pagination

Pagination is **already implemented** in `internal/api/client.go` (`GetWithPagination`, `extractPagination`). Domain list methods should use it when agents need more than one page:

- For list endpoints, accept a `limit int` parameter (or the full `ListOpts` struct).
- Default per_page = 20, cap user-supplied at 100 (GitLab limit).
- For "fetch all" use `c.client.GetWithPagination(path)` and follow `X-Next-Page` until 0 or empty.

### Method signatures

Pattern (see `internal/api/user.go` for live example):

```go
// List returns merge requests for a project.
//
// GET /api/v4/projects/:id/merge_requests
func (a *MergeRequestAPI) List(projectID string, opts *MergeRequestListOpts) ([]MergeRequest, error) {
    path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/merge_requests")
    if q := opts.encode(); q != "" {
        path += "?" + q
    }
    data, err := a.client.Get(path)
    if err != nil {
        return nil, err
    }
    var out []MergeRequest
    if err := json.Unmarshal(data, &out); err != nil {
        return nil, fmt.Errorf("parsing merge requests: %w", err)
    }
    return out, nil
}
```

### DTOs

Define DTOs in your domain file (e.g. `internal/api/mr.go`). Mirror the GitLab API JSON exactly using struct tags. Tolerate extra fields — `json.Unmarshal` ignores them by default.

```go
type MergeRequest struct {
    IID          int    `json:"iid"`
    Title        string `json:"title"`
    State        string `json:"state"`
    SourceBranch string `json:"source_branch"`
    TargetBranch string `json:"target_branch"`
    WebURL       string `json:"web_url"`
    Author       *User  `json:"author"`
    // ...add fields the CLI surfaces
}
```

### Error handling

Don't write your own retry/error logic — `c.client.Get/Post/Put/Delete` already handle 429/5xx retry and produce `*api.APIError` for 4xx.

## cmd conventions

### File layout

A domain command file follows this skeleton (see `cmd/auth.go` for a real example):

```go
package cmd

import (
    "fmt"

    "github.com/fatecannotbealtered/gitlab-cli/internal/api"
    "github.com/fatecannotbealtered/gitlab-cli/internal/output"
    "github.com/spf13/cobra"
)

var mrCmd = &cobra.Command{
    Use:   "mr",
    Short: "Manage merge requests",
}

func init() {
    rootCmd.AddCommand(mrCmd)

    // list (read)
    mrListCmd.Flags().String("project", "", "Project ID or path (required)")
    mrListCmd.Flags().String("state", "opened", "State: opened|closed|merged|all")
    mrListCmd.Flags().Int("limit", 20, "Max results (1-100)")
    mrListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
    mrCmd.AddCommand(mrListCmd)

    // create (write)
    mrCreateCmd.Flags().String("project", "", "Project ID or path (required)")
    mrCreateCmd.Flags().String("title", "", "MR title (required)")
    mrCreateCmd.Flags().String("source-branch", "", "Source branch (required)")
    mrCreateCmd.Flags().String("target-branch", "main", "Target branch")
    mrCmd.AddCommand(mrCreateCmd)
    markWrite(mrCreateCmd)
    // ...
}
```

### RunE pattern (CRITICAL — five steps)

```go
var mrListCmd = &cobra.Command{
    Use:   "list",
    Short: "List merge requests",
    RunE: func(cmd *cobra.Command, args []string) error {
        // 1. client
        client, _, err := newClient()
        if err != nil { return err }

        // 2. parse args / flags
        project, _ := cmd.Flags().GetString("project")
        if project == "" {
            output.Error("--project is required")
            setExitCode(ExitBadArgs)
            return ErrSilent
        }
        state, _ := cmd.Flags().GetString("state")
        limit, _ := cmd.Flags().GetInt("limit")

        // 3. dryRun (writes only)
        // (skip for read commands)

        // 4. call API
        mrs, err := client.MergeRequests.List(project, &api.MergeRequestListOpts{State: state, Limit: limit})
        if err != nil {
            return handleAPIError(err, jsonMode)
        }

        // 5. render
        if jsonMode {
            // Use FlatXxx + FilterMap if you implemented them; otherwise raw.
            output.PrintJSON(mrs)
            return nil
        }
        if len(mrs) == 0 {
            output.Info("No merge requests found.")
            return nil
        }
        headers := []string{"IID", "TITLE", "STATE", "SOURCE", "TARGET", "AUTHOR"}
        rows := make([][]string, len(mrs))
        for i, m := range mrs {
            author := ""
            if m.Author != nil {
                author = m.Author.Username
            }
            rows[i] = []string{
                fmt.Sprintf("%d", m.IID),
                m.Title,
                output.StatusBadge(m.State),
                m.SourceBranch,
                m.TargetBranch,
                author,
            }
        }
        output.Table(headers, rows)
        return nil
    },
}
```

### Write commands (CRITICAL)

For any command that mutates GitLab state:

1. Call `markWrite(cmdVar)` in `init()` so audit logging captures it.
2. In RunE, call `if dryRunOutput("create mr", map[string]any{"title": title, ...}) { return nil }` BEFORE the API call.
3. For destructive deletes, prompt with `confirmAction("Type %s to delete", id)` (skipped automatically when `--force` is set).

### Required flags / arg validation

Use Cobra's built-in validators when possible:

```go
Args: cobra.ExactArgs(1),
Args: cobra.MinimumNArgs(1),
```

For required flags, check inside `RunE` and emit `output.Error("--xxx is required")` + `setExitCode(ExitBadArgs)` + `return ErrSilent`.

### `--fields` projection

When implementing `--fields key,name,state`:
1. Define `type FlatXxx struct{...}` in `internal/output/flatten_<domain>.go`.
2. Define `func XxxToMap(x FlatXxx) map[string]any` in the same file.
3. In your render path:
   ```go
   if jsonMode {
       fields := getFieldsFlag(cmd)
       if len(fields) > 0 || /* always flat by default */ true {
           flat := toFlatMR(&m)
           output.PrintJSON(output.FilterMap(output.MRToMap(flat), fields))
           return nil
       }
   }
   ```

## Tests

### API tests

Use `httptest.Server`. Pattern (see `internal/api/client_test.go` `TestUser_Me`):

```go
func TestMR_List(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Assert path / method / query / body
        if !strings.HasPrefix(r.URL.Path, "/api/v4/projects/") {
            t.Errorf("path = %q", r.URL.Path)
        }
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(`[{"iid":1,"title":"x","state":"opened"}]`))
    }))
    defer srv.Close()
    c := NewClient(&config.Config{Host: srv.URL, Token: "tok"})
    mrs, err := c.MergeRequests.List("group/proj", &MergeRequestListOpts{})
    if err != nil { t.Fatalf("List: %v", err) }
    if len(mrs) != 1 || mrs[0].IID != 1 { t.Errorf("unexpected: %+v", mrs) }
}
```

### cmd tests

Cover at minimum:
- `--help` for the parent command lists all subcommands
- One smoke test per subcommand using `cobra.SetArgs` + capturing stdout
- A `--dry-run --json` test for one write command (asserts `dryRun:true`)

## SKILL segment file

Create `skills/gitlab-cli/_skill_<domain>.md` with this exact structure (no front-matter — Phase 3 will merge):

````markdown
## <Domain Title> (`gitlab-cli <domain>`)

Brief description for AI agents.

```bash
# Read
gitlab-cli <domain> list --project <ID> --json
gitlab-cli <domain> get <ID> --json --fields iid,title,state

# Write
gitlab-cli <domain> create --project <ID> --title "..." --json
gitlab-cli <domain> update <ID> --title "..." --json
gitlab-cli <domain> delete <ID> --force
```

### Flat <Domain> JSON

```json
{
  "iid": 1,
  "title": "...",
  "state": "opened"
}
```

### Notes

- Anything special about this domain that an AI agent must know (idempotency, rate limits, cross-references).
````

## Final checklist

Before reporting "done":

- [ ] `gofmt -l .` returns nothing
- [ ] `go vet ./...` exit 0
- [ ] `go build ./...` exit 0
- [ ] `go test ./...` all packages PASS
- [ ] `gitlab-cli <domain> --help` shows all subcommands you registered
- [ ] At least one `--json` test asserts machine-readable output
- [ ] All write commands have `markWrite(cmd)` and `dryRunOutput(...)` short-circuit
- [ ] No DTO or method belongs to another domain
- [ ] `_skill_<domain>.md` segment exists and follows the template
