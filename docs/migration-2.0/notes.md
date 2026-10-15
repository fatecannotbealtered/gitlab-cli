# Migration notes: a command group on spec 2.0

Every command group is on spec 2.0: the foundation, `auth`, `label` and
`milestone` first, then **MR**, **Issue**, **Repo**, **CI** and **Project**, each
migrated by one agent in its own worktree and merged on 2026-10-14. Follow
`MIGRATION-2.0.md` in the spec repository; this file says how it is done here,
and the decisions below hold for every later change. The worked example is
`cmd/label.go` with `cmd/label_test.go` (and `cmd/milestone.go` for iid refs, title
lookups and a domain verb).

## Integration decisions (2026-10-14)

Settled when the five groups were merged; they hold for the whole tool.

- **Root shortcuts.** A root word runs one command path. A noun that several
  groups nest (`comment`, `bulk`, `discussion`; GitLab issues have discussions
  too) is never a bare root word: it carries its group, singular and plural
  (`mr-comment`, `mr-comments`, `mr-discussion`, `mr-discussions`, `mr-bulk`,
  `issue-comment`, `issue-comments`, `issue-bulk`). A noun only one group has keeps
  the bare word (`branch`, `file`, `commit`, `tree`, `member`). Groups keep their
  own aliases (`mr comments`, `mr notes`, `issues`). Add shortcuts with
  `app.AddShortcut(word, path...)`, never by assigning `app.Shortcuts`: the tree
  check reports a word another group already took (Q6), so a merge cannot silently
  re-point it. R02 asks a shortcut only for a nested noun under one parent.
- **Refs of sub-resources** are the parent's ref, the collection and the ID:
  `acme/web#12/notes/99`, `acme/web!5/notes/12`, `acme/web!5/discussions/<id>`. `/`
  is a scope boundary, so a grant for `acme/web!5` covers its notes. Commands keep
  accepting GitLab's web URLs and anchors (`…/merge_requests/5#note_12`,
  `acme/web!5#note_12`). A changed file of a merge request stays
  `acme/web!5:src/app.go`.
- **Project names.** `projectFlag` (cmd/refs.go) takes a path, a web URL, a numeric
  ID or a name: `--project payments` matching two projects is `E_AMBIGUOUS` with
  both refs in every command, one match is that project, none is `E_NOT_FOUND`.
  `gitlabReadCodes` and `gitlabWriteCodes` include `E_AMBIGUOUS`.
- **A 403 names the role and where it is held**: `requiresRole(role)` on the
  project, `requiresRoleOn(role, "the group")` elsewhere (a create in a group).
- **The behaviour matrix runs batches.** A command with `Meta.BatchFlag` gets the
  batch rows by itself (every item's receipt, the summary, a dangerous batch
  stopping at its first failure, a partial run with `--continue-on-error`).
  `OneRequest` marks a batch whose items go out in one request (repo commit create),
  `PreviewReads` counts the reads before the first write, and `Setup` prepares
  each row's world and returns the invocation (job artifacts: a scope that grants
  `artifact` files in a temporary directory).
- **`e.run` reads NDJSON**: a stream's every line is validated and its summary
  line becomes `r.doc`. Batch results: `r.batchItems()`, `r.summary()`,
  `itemOutcome(it)`.
- **The fake**: one registration per ServeMux pattern. `GET /api/v4/projects`,
  members, namespaces and search are the Project group's (project_test.go), whose
  `fakeProjectDetails` also fills the shared `GET /projects/{project}`; branches,
  files, commits, tags and releases are the Repo group's (repo_test.go, reuse
  them); `fin/payments-service` and `legacy/payments` are the two payments
  projects.

## What 2.0.0 changed (spec v2.0.0, synced 2026-10-15)

`.agent/SPEC_VERSION` pins `v2.0.0`, the released spec; `node scripts/sync-spec.js`
downloads it. One library change since rc.2 (RC2-1): `App.SetProfileSettings` sets
`default_profile` only when it writes the file's first profile. A login that adds a
profile to a file with several profiles and no default keeps it without one, so
commands there still need `--profile`.

## What rc.2 changed (spec v2.0.0-rc.2, synced 2026-10-14)

The library now has what the groups hand-rolled; use it:

- **A lookup that names the target runs inside the gate**:
  `WriteSpec.ResolveTarget` (`BatchSpec.ResolveItems`), never a read before
  `App.Write`. At confirm it runs after the token checks, so a repeated confirm of a
  delete is `token_used`, not the lookup's `E_NOT_FOUND` (Q1). `label update|delete
  --label-id`, `variable update|delete` without a scope and `project create` without
  `--namespace` do so. `ReadBack` is fixed before it runs, so it names the input.
- **Flags**: a renamed flag goes in `Meta.RenamedFlags` (old → new; aicli reads it,
  binds the new name, adds the notice, refuses both at once), not a hidden flag and
  glue; an alias for one command in `Meta.FlagAliases` (repo's `--branch` → `--name`).
- **Helpers**: `aicli.OutputFormat(cmd)` (raw or not), `aicli.ParseDuration` and
  `aicli.ParseTime` (CLI-028: one unit, `90s`, not `1m30s`), and `wantsField`
  (cmd/list.go, over `aicli.WantsField`): fetch a costly field when the response
  shows it, a default field included (commit line counts are read by default now).
- **Local files**: `App.OpenOutput` streams a download into a temporary file and
  commits it (job artifacts); an artifact-only command's existing output is the
  gate's `E_VALIDATION` (Q9). **Native bulk calls**: `BatchSpec.ApplyAll` (repo
  commit create). **A plan can be dangerous**: `Preview.Dangerous` (a commit that
  deletes a file needs `--dangerous`).
- **Output**: a write's `result` is projected to the schema's default fields
  without `--fields` (Q16), and `meta.untrusted` lists only the fields shown (Q20).
  Users are `@carol` in every group, MR included. A search page whose order GitLab
  may or may not rank says `sort: "upstream"` and describes it in `scope.ranking`
  (CLI-029); `user search` is relevance-ordered.
- **auth**: `Meta.AllowUnknownProfile` (was `NewProfile`) on login and logout; the
  first login makes its profile `default_profile` (Q3), and `App.Profiles` lists
  them; `E_CONFIG` is in every command's error codes (Q25). `reference --command`
  may use 6,000 characters (Q7).

## What changed with the library fixes (aicli resynced 2026-10-14)

The library now fixes most of [findings.md](findings.md). If you started from an older
copy of this file, these rules changed:

- **Leaf aliases are allowed again** (P1): aliases are checked per parent, so every
  group uses the same verbs: `ls` (list), `view`, `show` (get), `add`, `new`
  (create), `edit` (update), `rm`, `remove` (delete), in `Meta.Aliases`.
- **ID flags may be ints or strings** (P2): `preview.inputs` shows an input named
  `id`, `iid` or `*_id` as a string either way. label and milestone keep string
  flags; an `Int` flag saves you the parsing. Other numbers stay numbers.
- **Preview blocks duplicates again** (P4): a repeat of an applied create with the
  same `--idempotency-key` is answered by the gate before Preview, at the dry-run
  already (`ok: true`, `receipt.replayed: true`, no token). Don't special-case the
  key in Preview.
- **The profile and every setting are bound into confirm tokens** (B1): `host`,
  `timeout`, and the token from `GITLAB_CLI_TOKEN` as a keyed digest. A test that
  changes the token, the host or `--profile` between dry-run and confirm gets
  `token_mismatch` (`parameter` names what changed, `setting.token` for the token);
  `preview.inputs` shows them under `profile` and `setting` (the token as `***`).
- **An unknown `--profile` stops every command up front** (D4) with `E_CONFIG`,
  before your RunE, except `auth login` and `auth logout` (`Meta.AllowUnknownProfile`). You
  need not handle it. G03 no longer adds `--profile` to samples (P8), and mutates
  values within their type (P9: ints +1, dates +1 day, colours, enums).
- **No `default_profile`, several profiles: name one.** A settings file a person
  wrote with two or more profiles and no default makes a keyring sign-in `E_USAGE`
  on `--profile` (a login sets the default only for the file's first profile). A test
  that writes such a file passes `--profile` (or sets `GITLAB_CLI_TOKEN`, which needs
  no profile).
- **Scope `outputs` covers new files only; `roots` covers files that exist** (C1).
  The conformance harness grants `conformanceOutputDir` in both, so a sample may
  overwrite its own file. `fullScope()` grants `change` and `configure` only: a test
  of an `artifact` command adds `outputs` (and `roots` if it overwrites).
- **Local writes count in conformance** (P7): a command whose effects are only
  `configure`, `persist` or `artifact` passes G04 with its confirmed receipt; no
  fake upstream write is needed.
- **`--fields` on a confirmed write projects `data.result`** (P12), and an unknown
  verb gets the corrected command line with its flags (P11, see `TestAgentGuesses`).
- **State filters are `--state`** (CLI-028; aicli's own `operation list --state`,
  F4). A 1.x `--status` filter (pipelines, jobs) becomes `--state`, kept in
  `Meta.RenamedFlags` since rc.2.
- **Versions**: a test build reports `1.2.18-dev` (P14) and `changelog` fits the
  size budget by itself (P3). Don't hard-code either.
- Shared code now uses the library: `clientFor` takes `app.HTTPTransport()` and
  `app.UserAgent()` (P10); a keyring token is registered with `app.AddSecret` (D2);
  auth uses `app.Profile()`, `app.SettingsPath()` and `app.SetProfileSettings` (P5,
  P6). `cmd/settingsfile.go` is gone. Groups call none of these: `gitlabFor` and
  `projectFlag` are still the way in.

## Your files, and the ones you never edit

| Group | Yours |
|---|---|
| MR | `cmd/mr.go`, `mr_write.go`, `mr_bulk.go`, `mr_comment.go`, `mr_discussion.go`, their tests, `internal/api/mr.go` (+ test) |
| Issue | `cmd/issue.go`, `issue_bulk.go`, `issue_comment.go`, their tests, `internal/api/issue.go` (+ test) |
| Repo | `cmd/repo.go`, `repo_branch.go`, `repo_file.go`, `repo_commit.go`, `repo_commit_create.go`, `release.go`, their tests, `internal/api/repo.go`, `release.go` (+ tests) |
| CI | `cmd/ci.go`, `pipeline.go`, `job.go`, `variable.go`, `variable_bulk.go`, their tests, `internal/api/pipeline.go`, `job.go`, `variable.go` (+ tests) |
| Project | `cmd/project.go`, `user.go`, `search.go`, their tests, `internal/api/project.go`, `user.go`, `search.go`, `types.go` (+ tests) |

Plus `cmd/pending_<group>.go` (delete it) and any new file named after your group.
In `internal/api/errors_test.go` edit only your own `Test<X>_HTTPErrorPaths`.

Never edit: `internal/aicli/`, `.agent/`, `contract/` (vendored); the shared
`cmd/` files (`app.go`, `client.go`, `credentials.go`, `doctor.go`, `errors.go`,
`keyring.go`, `list.go`, `refs.go`, `users.go`, `values.go`) and
shared tests (`main_test.go`, `fakegitlab_test.go`, `testenv_test.go`,
`matrix_test.go`, `conformance_test.go`, `app_test.go`); `internal/api/client.go`
and `pagination.go`; label, milestone and auth files; this file; `e2e/`, `skills/`,
README, CHANGELOG (later phases). If you need a change there, write a private helper
in your own file, prefixed with your group (`mrSomething`), and add an integration
request to your section of [findings.md](findings.md). Don't add Go dependencies.

## Steps

1. Delete `cmd/pending_<group>.go`. In each file you migrate, delete the first two
   lines (`//go:build legacy_pending` and the blank line), then rewrite it. The 1.x
   field lists are in git history (`git show e384161:internal/output/flatten_mr.go`).
2. Define `func register<Group>(app *aicli.App)` (`registerMR`, `registerIssue`,
   `registerRepo`, `registerCI`, `registerProject`): `addGroup(app, "mr", summary,
   "mrs")`, `app.Schema(...)`, then one `add<Group><Verb>(app, group)` per leaf. No
   package-level command vars, no `init()` registration, no global flag vars.
3. Every leaf gets `app.Register(c, aicli.Meta{...})` as in `cmd/label.go`: Summary,
   Effects, Dangerous (deletes, merges, anything irreversible; `deliver` and
   `configure` are dangerous anyway), Output, `ErrorCodes: gitlabReadCodes` or
   `gitlabWriteCodes`, Examples (a write: the `--dry-run` and
   `--confirm <confirm_token>` pair; dangerous: `--dangerous` in both), `Maturity:
   aicli.Experimental`, `Evidence: []string{"simulated:cmd/<file>_test.go"}`,
   Positional, Secrets, FileInputs, BatchFlag, Params (enums), `RenamedFlags` for
   each 1.x flag renamed with the same values (CLI-028: `--search` to `--query`), and
   `DeprecatedFlags` for a 1.x flag whose meaning changed (`--label-id`). A 1.x
   `--all` is dropped: FlagSynonyms answers it with `--limit`.
4. Leaf `Meta.Aliases` as label and milestone have them: `ls`, `view`/`show`,
   `add`/`new`, `edit`, `rm`/`remove` (per parent, P1; an alias must not equal a
   sibling's name). Plural group aliases (`mrs`, `issues`) as before.
5. Root shortcuts for nested groups (R02, CLI-022) go in your register function:
   `app.AddShortcut("branch", "repo", "branch")`; a sub-resource several groups
   have takes the group's prefix (`mr-comment`), as "Integration decisions" says.

## Clients and errors

- Reads: `gl := gitlabFor(app)`, `project, err := projectFlag(cmd, refProject, gl)`,
  `c, err := gl.get()`.
- Writes: nothing calls GitLab before `app.Write`; call `gl.get()` inside Preview
  and Apply, so a write the scope refuses sends nothing. The only exception is
  resolving an input you cannot canonicalise locally (a title, a project's name or
  numeric ID, a deprecated `--x-id`) to build the target key.
- `readError(err, onNotFound(msg, next), requiresRole(role))` for reads and
  Preview; `classifyWriteError(err, requiresRole("developer"))` in Apply
  (`requiresRoleOn(role, "the group")` when the role is not on the project). Flag
  problems before the gate: `aicli.Usage(problem, "--flag", msg)` or
  `aicli.Validation("--flag", expected, msg)`. Messages are one sentence for the
  model and never echo inputs or GitLab's text.
- Users: `resolveUser(ctx, c, name, "--assignee")` (`carol`, `@carol`, `me`);
  milestones by iid or title: `findMilestone(ctx, c, project, iid, title)`; label
  names pass through to GitLab as they are.
- The client retries reads and never writes; add no retries and no
  `Idempotency-Key` header.

## Refs and target keys

Every resource in output carries `ref`; commands accept the ref, the web URL and
natural names, and bind the canonical form with `aicli.SetResolved` before
`app.Write` (CLI-048, CLI-054). The ref of a write's target is its `Target.Key`.

- project `acme/web` (`projectFlag` handles paths, URLs, numeric IDs and names)
- MR `acme/web!5` (`mrRef`, `parseIIDRef(raw, "!", "/-/merge_requests/")`)
- issue `acme/web#12` (`issueRef`, `parseIIDRef(raw, "#", "/-/issues/")`)
- user `@carol` (`userRef`)
- a child of an MR or an issue: the parent's ref, the collection and the ID,
  `acme/web!5/notes/12`, `acme/web!5/discussions/<id>`, `acme/web#12/notes/99`
- anything else: `<project>/-/<collection>/<id>`, mirroring GitLab's web path, as
  `acme/web/-/labels/bug` and `acme/web/-/milestones/3` do:
  `acme/web/-/pipelines/123`, `acme/web/-/jobs/456`, `acme/web/-/branches/main`,
  `acme/web/-/tags/v1.0`, `acme/web/-/releases/v1.0`, `acme/web/-/variables/KEY`.

A key must start with the project path followed by `/`, `!`, `#`, `:` or `@`, or a
scope grant for `acme/web` does not cover it; `~` and `%` do not count. No `.` or
`..` segments.

## Output

- Schemas: `<resource>` (object), `<resource>_page` (page), `<resource>_deleted`;
  a write's Output is the shape of its `result`.
- Keys snake_case; IDs strings (`idString`), times `utcTime(...)` (UTC, `Z`), dates
  `YYYY-MM-DD`; `Untrusted` lists fields people wrote (titles, descriptions, bodies,
  names, comments, commit messages).
- ID flags (`--iid`, `--note-id`, `--job-id`) may be `Int` or `String` flags: aicli
  shows `iid` and `*_id` inputs as strings either way (P2). With a string flag,
  parse the number yourself, as `--label-id` does.
- Page schemas declare DefaultFields from the questions users ask (MR: ref, title,
  state, assignees, reviewers, labels, pipeline_status, updated_at, web_url).

## Lists

`addPageFlags(c)`, `req, err := readPage(cmd)`, one GitLab page with
`api.GetPage(ctx, c, path, req.Page, req.Limit)`, then
`app.List(cmd, gitlabPage(items, len(items), req, pag, scope, sort))`. `scope`
names the project, the filters applied and, under `covers`, anything narrower than
the name suggests (direct members only). State `sort` only when it is what GitLab
does. `wantsField(cmd, ...)` tells whether `--fields` asks for an expensive field.

## Writes

`app.Write(cmd, aicli.WriteSpec{Target, ReadBack, Preview, Apply})`, or
`app.WriteBatch` with `Meta.BatchFlag` for bulk commands (CLI-063).

- Preview only reads: current values as `Before`, the request as `After`
  (`Add`/`Remove` for sets), `BaseVersion: updated_at` when the resource has it,
  `CanApply: false` with `BlockedBy` when GitLab would refuse. Deterministic: no
  counts or timestamps in Changes.
- Change field names are output field names, so the gate compares them with
  `Result.Observed` (the changed fields from GitLab's answer) and sets verification.
- Apply sends once, `classifyWriteError` on failure, `Result{Observed, Data,
  Untrusted}` on success; a delete reads back and reports `exists: false`.
- Creates take `--idempotency-key`. Preview blocks a duplicate (`already_exists`)
  whether or not a key is given: the gate replays a repeat of an applied create
  before Preview runs (P4).
- `ReadBack` is built from the agent's input only, never from GitLab's text.

## Tests

In an `init()` of your test file (see `cmd/label_test.go`):

```go
addFakeSeed(func(gl *fakeGitLab) { gl.state["mrs"] = seedMRs(gl) })
addFakeRoutes(fakeRoute{"GET /api/v4/projects/{project}/merge_requests", fakeMRList}, ...)
addConformanceSamples(map[string][]string{"mr update": {"5", "--project", "acme/web", "--title", "x"}})
```

- The fake: projects `acme/web` (id 1) and `acme/empty` (id 2); users carol
  (`testToken`, the default), dave (`daveToken`) and erin. Helpers: `gl.project(w,
  r)`, `fakeJSON`, `fakePage`, `fakeBody`, `gl.id()`, `gl.now()`; touch state from a
  test with `gl.locked(fn)`. Routes are ServeMux patterns; `/users` is shared.
- Behaviour matrix: one `matrixCase` per command and `runMatrix` (VER-004); rows
  that do not apply skip themselves, `Skip` and `Rows` override. Batch commands
  get the batch rows by themselves; `Setup` gives each row a prepared world (see
  "Integration decisions"). Artifact outputs go under `conformanceOutputDir`;
  deliver destinations into `conformanceDestinations`.
- Focused tests for refs and URL forms, drift, blocked plans, deprecated flags, and
  your command-map line (like `TestCommandMapLines`).
- Every registered command must run in some test (R06), and every write needs a
  conformance sample (G00).
- `go test ./cmd/` (full: R06 runs after it); `go test -run 'TestConformance|TestMR'
  ./cmd/` while iterating; `go build ./...`, `go vet ./...`,
  `node scripts/check-spec.js`.

## Done means

1. No file of your group carries the build tag; `pending_<group>.go` is gone and
   `register<Group>` is defined; no 1.x helper (`output.*`, `failArg`,
   `prepareWrite`, `handleAPIError`) remains.
2. Every leaf has Meta; every write goes through the gate and has a sample.
3. `go build ./...`, `go vet ./...`, `go test ./...` (conformance and R06
   included) and `node scripts/check-spec.js` pass.
4. Spec, guide or library problems are in your section of [findings.md](findings.md).
