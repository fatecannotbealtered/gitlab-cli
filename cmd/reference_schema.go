package cmd

// This file is the machine-readable output contract for `reference`: it maps
// every leaf command to a data-shape label and enumerates each shape's fields
// and untrusted fields. The field lists are the source of truth that agents key
// off, so they MUST track the *ToMap builders in internal/output/flatten_*.go
// and the inline maps emitted by the command RunE bodies. reference_schema_test.go
// guards that every command points at a defined, non-empty schema and carries a
// runnable example.

// referenceSchemas returns the shape catalogue keyed by the label that each
// command's output_schema references. Single-resource reads (get) use the detail
// shape (includes the description body); list reads omit the body for token
// efficiency, so list and detail are distinct labels.
func referenceSchemas() map[string]referenceDataSchema {
	return map[string]referenceDataSchema{
		// ── Issue ────────────────────────────────────────────────────────────
		"issue":           {Shape: "object", Fields: []string{"iid", "title", "state", "author", "assignee", "labels", "milestone", "description", "webUrl", "_untrusted"}, UntrustedFields: []string{"title", "labels", "milestone", "author", "assignee", "description"}},
		"issue[]":         {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].title", "items[].labels", "items[].milestone", "items[].author", "items[].assignee"}},
		"issue_comment":   {Shape: "object", Fields: []string{"id", "body", "_untrusted"}, UntrustedFields: []string{"body"}},
		"issue_comment[]": {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].body"}},
		"deleted_comment": {Shape: "object", Fields: []string{"deleted", "noteId"}},

		// ── Merge request ────────────────────────────────────────────────────
		"mr":          {Shape: "object", Fields: []string{"iid", "title", "state", "source", "target", "author", "webUrl", "draft", "description", "diffRefs", "_untrusted"}, UntrustedFields: []string{"title", "author", "description"}},
		"mr[]":        {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].title", "items[].author"}},
		"mr_note":     {Shape: "object", Fields: []string{"id", "author", "body", "created", "resolvable", "resolved", "_untrusted"}, UntrustedFields: []string{"body", "author"}},
		"mr_note[]":   {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].body", "items[].author"}},
		"mr_approval": {Shape: "object", Fields: []string{"iid", "approved"}},
		"mr_diff":     {Shape: "object", Fields: []string{"diff", "_untrusted"}, UntrustedFields: []string{"diff"}},
		// A discussion is a thread of notes; the nested notes carry the untrusted
		// body/author fields (each note already tagged _untrusted by MRNoteToMap).
		"mr_discussion":   {Shape: "object", Fields: []string{"id", "individualNote", "notes[]", "notes[].id", "notes[].author", "notes[].body", "notes[].created", "notes[].resolvable", "notes[].resolved", "_untrusted"}, UntrustedFields: []string{"notes[].body", "notes[].author"}},
		"mr_discussion[]": {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].notes[].body", "items[].notes[].author"}},

		// ── Pipeline / job ───────────────────────────────────────────────────
		"pipeline":   {Shape: "object", Fields: []string{"id", "iid", "projectId", "ref", "sha", "status", "source", "webUrl", "createdAt", "updatedAt", "username", "_untrusted"}, UntrustedFields: []string{"ref"}},
		"pipeline[]": {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].ref"}},
		"job":        {Shape: "object", Fields: []string{"id", "name", "status", "stage", "ref", "webUrl", "createdAt", "startedAt", "finishedAt", "duration", "username", "pipelineId", "_untrusted"}, UntrustedFields: []string{"name", "stage", "ref"}},
		"job[]":      {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].name", "items[].stage", "items[].ref"}},
		// Non-follow (default): single object {jobId, log}. With --tail/--grep/
		// --max-bytes the object also carries {totalBytes, returnedBytes, truncated}
		// so an agent knows the trace was filtered. With --follow --json the
		// command streams NDJSON instead (CLI-SPEC §5): one
		// {ok,schema_version,type:"chunk",data:{jobId,offset,bytes,data}} line per
		// new trace range (data.data carries the untrusted log bytes), then a final
		// {type:"summary",data:{jobId,status,chunks,totalBytes}} line.
		"job_log":       {Shape: "object|ndjson", Fields: []string{"jobId", "log", "totalBytes", "returnedBytes", "truncated", "stream:type", "stream:data.offset", "stream:data.bytes", "stream:data.data", "stream:summary.status", "stream:summary.chunks", "stream:summary.totalBytes", "_untrusted"}, UntrustedFields: []string{"log", "stream:data.data"}},
		"job_wait":      {Shape: "object", Fields: []string{"id", "status", "log", "_untrusted"}, UntrustedFields: []string{"log"}},
		"job_artifacts": {Shape: "object", Fields: []string{"path", "bytes"}},

		// ── Release ──────────────────────────────────────────────────────────
		"release":         {Shape: "object", Fields: []string{"tagName", "name", "description", "createdAt", "releasedAt", "author", "commitId", "assetCount", "_untrusted"}, UntrustedFields: []string{"tagName", "name", "description"}},
		"release[]":       {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].tagName", "items[].name", "items[].description"}},
		"deleted_release": {Shape: "object", Fields: []string{"tag", "action", "_untrusted"}, UntrustedFields: []string{"tag"}},

		// ── Label / milestone ────────────────────────────────────────────────
		"label":         {Shape: "object", Fields: []string{"id", "name", "color", "description", "priority", "_untrusted"}, UntrustedFields: []string{"name", "description"}},
		"label[]":       {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].name", "items[].description"}},
		"deleted_label": {Shape: "object", Fields: []string{"deleted", "labelId"}},
		"milestone":     {Shape: "object", Fields: []string{"id", "iid", "title", "state", "startDate", "dueDate", "webUrl", "_untrusted"}, UntrustedFields: []string{"title"}},
		"milestone[]":   {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].title"}},

		// ── Variable ─────────────────────────────────────────────────────────
		"variable":         {Shape: "object", Fields: []string{"key", "value", "type", "protected", "masked", "raw", "envScope", "description", "_untrusted"}, UntrustedFields: []string{"key", "value", "envScope", "description"}},
		"variable[]":       {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].key", "items[].value", "items[].envScope", "items[].description"}},
		"deleted_variable": {Shape: "object", Fields: []string{"deleted", "key", "_untrusted"}, UntrustedFields: []string{"key"}},

		// ── Project / user ───────────────────────────────────────────────────
		"project":          {Shape: "object", Fields: []string{"id", "pathWithNamespace", "name", "visibility", "webUrl", "defaultBranch", "_untrusted"}, UntrustedFields: []string{"pathWithNamespace", "name", "defaultBranch"}},
		"project[]":        {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].pathWithNamespace", "items[].name", "items[].defaultBranch"}},
		"project_member[]": {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].username", "items[].name"}},
		"user":             {Shape: "object", Fields: []string{"id", "username", "name", "email", "state", "webUrl", "_untrusted"}, UntrustedFields: []string{"username", "name", "email"}},
		"user[]":           {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].username", "items[].name", "items[].email"}},

		// ── Repo (branch / commit / tree / file) ─────────────────────────────
		"branch":         {Shape: "object", Fields: []string{"name", "default", "protected", "merged", "webUrl", "commitId", "_untrusted"}, UntrustedFields: []string{"name"}},
		"branch[]":       {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].name"}},
		"deleted_branch": {Shape: "object", Fields: []string{"name", "action", "_untrusted"}, UntrustedFields: []string{"name"}},
		"commit":         {Shape: "object", Fields: []string{"id", "shortId", "title", "authorName", "authoredDate", "committedDate", "webUrl", "additions", "deletions", "total", "_untrusted"}, UntrustedFields: []string{"title", "authorName"}},
		// commit[] carries the single-project pagination shape; a multi-project
		// scope (--group / --all-projects / plural --project) adds items[].project,
		// scope, projectsScanned, and projectErrors[] (per-project failures never
		// abort the scan). additions/deletions/total appear when --with-stats is set.
		"commit[]":        {Shape: "list", Fields: []string{"items[]", "items[].project", "items[].additions", "items[].deletions", "items[].total", "count", "limit", "page", "total", "hasMore", "all", "scope", "projectsScanned", "projectErrors"}, UntrustedFields: []string{"items[].title", "items[].authorName"}},
		"commit_diff":     {Shape: "object", Fields: []string{"sha", "filesChanged", "files[]", "files[].oldPath", "files[].newPath", "files[].newFile", "files[].deletedFile", "files[].renamedFile", "files[].additions", "files[].deletions", "files[].diff", "_untrusted"}, UntrustedFields: []string{"files[].oldPath", "files[].newPath", "files[].diff"}},
		"tree[]":          {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].name", "items[].path"}},
		"repo_file":       {Shape: "object", Fields: []string{"project", "path", "ref", "bytes", "encoding", "content", "output", "_untrusted"}, UntrustedFields: []string{"content"}},
		"repo_file_write": {Shape: "object", Fields: []string{"path", "branch", "action"}},

		// ── Batch results (CLI-SPEC §15) ─────────────────────────────────────
		// Aggregated per-item results: items[] carry target + ok + on failure
		// error{code,retryable}; summary always reports {total,succeeded,failed}.
		// repo commit is atomic class-A, so it also returns the resulting commit.
		"batch_commit": {Shape: "object", Fields: []string{"commitId", "shortId", "branch", "webUrl", "items[]", "items[].target", "items[].ok", "items[].action", "summary", "_untrusted"}, UntrustedFields: []string{"items[].target"}},
		"batch_result": {Shape: "object", Fields: []string{"items[]", "items[].target", "items[].ok", "items[].error", "items[].action", "summary", "skipped", "_untrusted"}, UntrustedFields: []string{"items[].target"}},

		// ── Search ───────────────────────────────────────────────────────────
		"search_project[]": {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].name", "items[].pathWithNamespace"}},
		"search_issue[]":   {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].title"}},
		"search_mr[]":      {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].title"}},
		"search_blob[]":    {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].filename", "items[].path", "items[].ref", "items[].data"}},
		"search_commit[]":  {Shape: "list", Fields: []string{"items[]", "count", "limit", "page", "total", "hasMore", "all"}, UntrustedFields: []string{"items[].title", "items[].authorName"}},

		// ── Auth / self-description ──────────────────────────────────────────
		"auth_status":    {Shape: "object", Fields: []string{"configured", "host", "tokenSet", "source"}},
		"auth_login":     {Shape: "object", Fields: []string{"status", "username", "name", "host", "profile", "_untrusted"}, UntrustedFields: []string{"username", "name"}},
		"auth_logout":    {Shape: "object", Fields: []string{"status"}},
		"profile[]":      {Shape: "list", Fields: []string{"items[]", "count", "limit", "hasMore"}, UntrustedFields: []string{"items[].name", "items[].host"}},
		"profile_use":    {Shape: "object", Fields: []string{"active", "status"}},
		"profile_remove": {Shape: "object", Fields: []string{"status", "name"}},

		"context":   {Shape: "object", Fields: []string{"version", "env", "account", "config", "credentials", "security", "git", "gitlab", "_untrusted", "notices"}},
		"doctor":    {Shape: "object", Fields: []string{"checks", "notices", "version", "skill_min_version", "risk_tier", "configExists", "authValid", "latencyMs", "host", "username", "name", "error"}},
		"changelog": {Shape: "object", Fields: []string{"current_version", "since", "entries"}},
		"reference": {Shape: "object", Fields: []string{"schema_version", "tool", "version", "risk_tier", "blastRadius", "release_readiness", "security", "globalFlags", "exitCodes", "commands", "schemas"}},
		"update":    {Shape: "object", Fields: []string{"status", "current_version", "target_version", "update_available", "release_url", "asset", "signature_status", "skill_sync_command", "skill_sync_status", "binary_replaced", "preview", "path", "previous_version", "checksum_verified", "signature_verified", "hint", "downgrade", "notices"}},
	}
}

// schemaLabelForCommand maps a leaf command path to its output_schema label.
// Paths are the space-joined command words exactly as `reference` emits them.
func schemaLabelForCommand(path string) string {
	if label, ok := commandSchemaLabels[path]; ok {
		return label
	}
	return ""
}

// examplesForCommand returns one runnable example per leaf command. Write
// commands show the dry-run/confirm pair so an agent sees the full safe path.
func examplesForCommand(path string) []string {
	return commandExamples[path]
}

// commandSchemaLabels keys every leaf command path to a defined schema label.
var commandSchemaLabels = map[string]string{
	"auth login":          "auth_login",
	"auth logout":         "auth_logout",
	"auth profile list":   "profile[]",
	"auth profile remove": "profile_remove",
	"auth profile use":    "profile_use",
	"auth status":         "auth_status",
	"changelog":           "changelog",
	"context":             "context",
	"doctor":              "doctor",
	"reference":           "reference",
	"update":              "update",

	"issue assign":         "issue",
	"issue bulk assign":    "batch_result",
	"issue bulk close":     "batch_result",
	"issue bulk comment":   "batch_result",
	"issue bulk label":     "batch_result",
	"issue bulk reopen":    "batch_result",
	"issue bulk update":    "batch_result",
	"issue close":          "issue",
	"issue comment add":    "issue_comment",
	"issue comment delete": "deleted_comment",
	"issue comment list":   "issue_comment[]",
	"issue create":         "issue",
	"issue get":            "issue",
	"issue label":          "issue",
	"issue list":           "issue[]",
	"issue reopen":         "issue",
	"issue update":         "issue",

	"mr approve":            "mr_approval",
	"mr bulk approve":       "batch_result",
	"mr bulk close":         "batch_result",
	"mr bulk merge":         "batch_result",
	"mr bulk update":        "batch_result",
	"mr close":              "mr",
	"mr comment add":        "mr_note",
	"mr comment delete":     "deleted_comment",
	"mr comment list":       "mr_note[]",
	"mr discussion create":  "mr_discussion",
	"mr discussion list":    "mr_discussion[]",
	"mr discussion reply":   "mr_note",
	"mr discussion resolve": "mr_discussion",
	"mr create":             "mr",
	"mr current":            "mr",
	"mr diff":               "mr_diff",
	"mr get":                "mr",
	"mr list":               "mr[]",
	"mr merge":              "mr",
	"mr reopen":             "mr",
	"mr unapprove":          "mr_approval",
	"mr update":             "mr",

	"pipeline cancel":  "pipeline",
	"pipeline create":  "pipeline",
	"pipeline current": "pipeline",
	"pipeline get":     "pipeline",
	"pipeline jobs":    "job[]",
	"pipeline list":    "pipeline[]",
	"pipeline retry":   "pipeline",
	"pipeline wait":    "pipeline",

	"job artifacts": "job_artifacts",
	"job cancel":    "job",
	"job get":       "job",
	"job log":       "job_log",
	"job play":      "job",
	"job retry":     "job",
	"job wait":      "job_wait",

	"label create": "label",
	"label delete": "deleted_label",
	"label list":   "label[]",
	"label update": "label",

	"milestone close":  "milestone",
	"milestone create": "milestone",
	"milestone get":    "milestone",
	"milestone list":   "milestone[]",
	"milestone update": "milestone",

	"release create": "release",
	"release delete": "deleted_release",
	"release get":    "release",
	"release list":   "release[]",
	"release update": "release",

	"repo branch create": "branch",
	"repo branch delete": "deleted_branch",
	"repo branch list":   "branch[]",
	"repo commit create": "batch_commit",
	"repo commit diff":   "commit_diff",
	"repo commit get":    "commit",
	"repo commit list":   "commit[]",
	"repo file create":   "repo_file_write",
	"repo file delete":   "repo_file_write",
	"repo file get":      "repo_file",
	"repo file update":   "repo_file_write",
	"repo tree":          "tree[]",

	"search code":     "search_blob[]",
	"search commits":  "search_commit[]",
	"search issues":   "search_issue[]",
	"search mrs":      "search_mr[]",
	"search projects": "search_project[]",

	"project create":  "project",
	"project get":     "project",
	"project list":    "project[]",
	"project members": "project_member[]",

	"user get":    "user",
	"user me":     "user",
	"user search": "user[]",

	"variable bulk-import": "batch_result",
	"variable create":      "variable",
	"variable delete":      "deleted_variable",
	"variable get":         "variable",
	"variable list":        "variable[]",
	"variable update":      "variable",
}

// commandExamples gives one runnable example per leaf. Read commands use
// --compact; write commands show the dry-run then the confirm step.
var commandExamples = map[string][]string{
	"auth login":          {"gitlab-cli auth login --host https://gitlab.com --token <pat> --compact"},
	"auth logout":         {"gitlab-cli auth logout --compact"},
	"auth profile list":   {"gitlab-cli auth profile list --compact"},
	"auth profile remove": {"gitlab-cli auth profile remove work --compact"},
	"auth profile use":    {"gitlab-cli auth profile use work --compact"},
	"auth status":         {"gitlab-cli auth status --compact"},
	"changelog":           {"gitlab-cli changelog --since 0.1.0 --compact"},
	"context":             {"gitlab-cli context --compact"},
	"doctor":              {"gitlab-cli doctor --compact"},
	"reference":           {"gitlab-cli reference --compact"},
	"update":              {"gitlab-cli update --compact", "gitlab-cli update --check --compact", "gitlab-cli update --dry-run --compact"},

	"issue assign":         {"gitlab-cli issue assign 42 alice --dry-run --compact", "gitlab-cli issue assign 42 alice --confirm <confirm_token> --compact"},
	"issue bulk assign":    {"gitlab-cli issue bulk assign alice --project group/repo --ids 1,2,3 --dry-run --compact", "gitlab-cli issue bulk assign alice --project group/repo --ids 1,2,3 --confirm <confirm_token> --compact"},
	"issue bulk close":     {"gitlab-cli issue bulk close --project group/repo --ids 1,2,3 --dry-run --compact", "gitlab-cli issue bulk close --project group/repo --ids 1,2,3 --confirm <confirm_token> --compact"},
	"issue bulk comment":   {"gitlab-cli issue bulk comment --project group/repo --ids 1,2 --body \"triaged\" --dry-run --compact", "gitlab-cli issue bulk comment --project group/repo --ids 1,2 --body \"triaged\" --confirm <confirm_token> --compact"},
	"issue bulk label":     {"gitlab-cli issue bulk label --project group/repo --ids 1,2 --add bug --dry-run --compact", "gitlab-cli issue bulk label --project group/repo --ids 1,2 --add bug --confirm <confirm_token> --compact"},
	"issue bulk reopen":    {"gitlab-cli issue bulk reopen --project group/repo --ids 1,2 --dry-run --compact", "gitlab-cli issue bulk reopen --project group/repo --ids 1,2 --confirm <confirm_token> --compact"},
	"issue bulk update":    {"gitlab-cli issue bulk update --project group/repo --ids 1,2 --add-labels triage --dry-run --compact", "gitlab-cli issue bulk update --project group/repo --ids 1,2 --add-labels triage --confirm <confirm_token> --compact"},
	"issue close":          {"gitlab-cli issue close 42 --dry-run --compact", "gitlab-cli issue close 42 --confirm <confirm_token> --compact"},
	"issue comment add":    {"gitlab-cli issue comment add 42 --body \"on it\" --dry-run --compact", "gitlab-cli issue comment add 42 --body \"on it\" --confirm <confirm_token> --compact"},
	"issue comment delete": {"gitlab-cli issue comment delete 42 1001 --dry-run --compact", "gitlab-cli issue comment delete 42 1001 --confirm <confirm_token> --compact"},
	"issue comment list":   {"gitlab-cli issue comment list 42 --compact"},
	"issue create":         {"gitlab-cli issue create --title \"Bug\" --dry-run --compact", "gitlab-cli issue create --title \"Bug\" --confirm <confirm_token> --compact"},
	"issue get":            {"gitlab-cli issue get 42 --compact"},
	"issue label":          {"gitlab-cli issue label 42 --add bug --dry-run --compact", "gitlab-cli issue label 42 --add bug --confirm <confirm_token> --compact"},
	"issue list":           {"gitlab-cli issue list --state opened --limit 20 --compact"},
	"issue reopen":         {"gitlab-cli issue reopen 42 --dry-run --compact", "gitlab-cli issue reopen 42 --confirm <confirm_token> --compact"},
	"issue update":         {"gitlab-cli issue update 42 --title \"New\" --dry-run --compact", "gitlab-cli issue update 42 --title \"New\" --confirm <confirm_token> --compact"},

	"mr approve":            {"gitlab-cli mr approve 7 --dry-run --compact", "gitlab-cli mr approve 7 --confirm <confirm_token> --compact"},
	"mr bulk approve":       {"gitlab-cli mr bulk approve --project group/repo --ids 7,8 --dry-run --compact", "gitlab-cli mr bulk approve --project group/repo --ids 7,8 --confirm <confirm_token> --compact"},
	"mr bulk close":         {"gitlab-cli mr bulk close --project group/repo --ids 7,8 --dry-run --compact", "gitlab-cli mr bulk close --project group/repo --ids 7,8 --confirm <confirm_token> --compact"},
	"mr bulk merge":         {"gitlab-cli mr bulk merge --project group/repo --ids 7,8 --dangerous --dry-run --compact", "gitlab-cli mr bulk merge --project group/repo --ids 7,8 --dangerous --confirm <confirm_token> --compact"},
	"mr bulk update":        {"gitlab-cli mr bulk update --project group/repo --ids 7,8 --add-labels ready --dry-run --compact", "gitlab-cli mr bulk update --project group/repo --ids 7,8 --add-labels ready --confirm <confirm_token> --compact"},
	"mr close":              {"gitlab-cli mr close 7 --dry-run --compact", "gitlab-cli mr close 7 --confirm <confirm_token> --compact"},
	"mr comment add":        {"gitlab-cli mr comment add 7 --body \"LGTM\" --dry-run --compact", "gitlab-cli mr comment add 7 --body \"LGTM\" --confirm <confirm_token> --compact"},
	"mr comment delete":     {"gitlab-cli mr comment delete 7 2002 --dry-run --compact", "gitlab-cli mr comment delete 7 2002 --confirm <confirm_token> --compact"},
	"mr comment list":       {"gitlab-cli mr comment list 7 --compact"},
	"mr discussion create":  {"gitlab-cli mr discussion create 7 --project group/repo --new-path src/app.go --new-line 42 --body \"nil deref here\" --dry-run --compact", "gitlab-cli mr discussion create 7 --project group/repo --new-path src/app.go --new-line 42 --body \"nil deref here\" --confirm <confirm_token> --compact"},
	"mr discussion list":    {"gitlab-cli mr discussion list 7 --project group/repo --compact"},
	"mr discussion reply":   {"gitlab-cli mr discussion reply 7 --project group/repo --discussion-id abc123 --body \"done\" --dry-run --compact", "gitlab-cli mr discussion reply 7 --project group/repo --discussion-id abc123 --body \"done\" --confirm <confirm_token> --compact"},
	"mr discussion resolve": {"gitlab-cli mr discussion resolve 7 --project group/repo --discussion-id abc123 --dry-run --compact", "gitlab-cli mr discussion resolve 7 --project group/repo --discussion-id abc123 --confirm <confirm_token> --compact"},
	"mr create":             {"gitlab-cli mr create --source feature --target main --title \"Add x\" --dry-run --compact", "gitlab-cli mr create --source feature --target main --title \"Add x\" --confirm <confirm_token> --compact"},
	"mr current":            {"gitlab-cli mr current --compact"},
	"mr diff":               {"gitlab-cli mr diff 7 --compact"},
	"mr get":                {"gitlab-cli mr get 7 --compact"},
	"mr list":               {"gitlab-cli mr list --state opened --limit 20 --compact"},
	"mr merge":              {"gitlab-cli mr merge 7 --dangerous --dry-run --compact", "gitlab-cli mr merge 7 --dangerous --confirm <confirm_token> --compact"},
	"mr reopen":             {"gitlab-cli mr reopen 7 --dry-run --compact", "gitlab-cli mr reopen 7 --confirm <confirm_token> --compact"},
	"mr unapprove":          {"gitlab-cli mr unapprove 7 --dry-run --compact", "gitlab-cli mr unapprove 7 --confirm <confirm_token> --compact"},
	"mr update":             {"gitlab-cli mr update 7 --title \"New\" --dry-run --compact", "gitlab-cli mr update 7 --title \"New\" --confirm <confirm_token> --compact"},

	"pipeline cancel":  {"gitlab-cli pipeline cancel 555 --dry-run --compact", "gitlab-cli pipeline cancel 555 --confirm <confirm_token> --compact"},
	"pipeline create":  {"gitlab-cli pipeline create --ref main --dry-run --compact", "gitlab-cli pipeline create --ref main --confirm <confirm_token> --compact"},
	"pipeline current": {"gitlab-cli pipeline current --compact"},
	"pipeline get":     {"gitlab-cli pipeline get 555 --compact"},
	"pipeline jobs":    {"gitlab-cli pipeline jobs 555 --compact"},
	"pipeline list":    {"gitlab-cli pipeline list --limit 20 --compact"},
	"pipeline retry":   {"gitlab-cli pipeline retry 555 --dry-run --compact", "gitlab-cli pipeline retry 555 --confirm <confirm_token> --compact"},
	"pipeline wait":    {"gitlab-cli pipeline wait 555 --compact"},

	"job artifacts": {"gitlab-cli job artifacts 999 --output artifacts.zip --compact"},
	"job cancel":    {"gitlab-cli job cancel 999 --dry-run --compact", "gitlab-cli job cancel 999 --confirm <confirm_token> --compact"},
	"job get":       {"gitlab-cli job get 999 --compact"},
	"job log":       {"gitlab-cli job log 999 --compact", "gitlab-cli job log 999 --tail 200 --grep \"error|failed\" --compact", "gitlab-cli job log 999 --follow --json   # NDJSON: {type:chunk} lines + final {type:summary}"},
	"job play":      {"gitlab-cli job play 999 --dry-run --compact", "gitlab-cli job play 999 --variable KEY=val --confirm <confirm_token> --compact"},
	"job retry":     {"gitlab-cli job retry 999 --dry-run --compact", "gitlab-cli job retry 999 --confirm <confirm_token> --compact"},
	"job wait":      {"gitlab-cli job wait 999 --compact"},

	"label create": {"gitlab-cli label create --name bug --color \"#d9534f\" --dry-run --compact", "gitlab-cli label create --name bug --color \"#d9534f\" --confirm <confirm_token> --compact"},
	"label delete": {"gitlab-cli label delete bug --dry-run --compact", "gitlab-cli label delete bug --confirm <confirm_token> --compact"},
	"label list":   {"gitlab-cli label list --compact"},
	"label update": {"gitlab-cli label update bug --color \"#e0e0e0\" --dry-run --compact", "gitlab-cli label update bug --color \"#e0e0e0\" --confirm <confirm_token> --compact"},

	"milestone close":  {"gitlab-cli milestone close 3 --dry-run --compact", "gitlab-cli milestone close 3 --confirm <confirm_token> --compact"},
	"milestone create": {"gitlab-cli milestone create --title \"v1\" --dry-run --compact", "gitlab-cli milestone create --title \"v1\" --confirm <confirm_token> --compact"},
	"milestone get":    {"gitlab-cli milestone get 3 --compact"},
	"milestone list":   {"gitlab-cli milestone list --compact"},
	"milestone update": {"gitlab-cli milestone update 3 --title \"v1.1\" --dry-run --compact", "gitlab-cli milestone update 3 --title \"v1.1\" --confirm <confirm_token> --compact"},

	"release create": {"gitlab-cli release create --tag v1.0.0 --dry-run --compact", "gitlab-cli release create --tag v1.0.0 --confirm <confirm_token> --compact"},
	"release delete": {"gitlab-cli release delete v1.0.0 --dangerous --dry-run --compact", "gitlab-cli release delete v1.0.0 --dangerous --confirm <confirm_token> --compact"},
	"release get":    {"gitlab-cli release get v1.0.0 --compact"},
	"release list":   {"gitlab-cli release list --compact"},
	"release update": {"gitlab-cli release update v1.0.0 --name \"GA\" --dry-run --compact", "gitlab-cli release update v1.0.0 --name \"GA\" --confirm <confirm_token> --compact"},

	"repo branch create": {"gitlab-cli repo branch create feature --ref main --dry-run --compact", "gitlab-cli repo branch create feature --ref main --confirm <confirm_token> --compact"},
	"repo branch delete": {"gitlab-cli repo branch delete feature --dangerous --dry-run --compact", "gitlab-cli repo branch delete feature --dangerous --confirm <confirm_token> --compact"},
	"repo branch list":   {"gitlab-cli repo branch list --compact"},
	"repo commit create": {"gitlab-cli repo commit create --project group/repo --branch main --message \"sync\" --action 'create:path=a.txt;content=hi' --action 'delete:path=old.txt' --dry-run --compact", "gitlab-cli repo commit create --project group/repo --branch main --message \"sync\" --action 'create:path=a.txt;content=hi' --action 'delete:path=old.txt' --confirm <confirm_token> --compact"},
	"repo commit diff":   {"gitlab-cli repo commit diff abc1234 --project group/repo --compact", "gitlab-cli repo commit diff abc1234 --project group/repo --fields newPath,additions,deletions --compact"},
	"repo commit get":    {"gitlab-cli repo commit get abc1234 --project group/repo --compact"},
	"repo commit list":   {"gitlab-cli repo commit list --project group/repo --author alice --since 2026-06-01T00:00:00Z --with-stats --compact", "gitlab-cli repo commit list --group my-team --author alice --since 2026-06-01T00:00:00Z --with-stats --compact"},
	"repo file create":   {"gitlab-cli repo file create README.md --branch main --content \"hi\" --message \"add\" --dry-run --compact", "gitlab-cli repo file create README.md --branch main --content \"hi\" --message \"add\" --confirm <confirm_token> --compact"},
	"repo file delete":   {"gitlab-cli repo file delete README.md --branch main --message \"rm\" --dangerous --dry-run --compact", "gitlab-cli repo file delete README.md --branch main --message \"rm\" --dangerous --confirm <confirm_token> --compact"},
	"repo file get":      {"gitlab-cli repo file get README.md --ref main --compact"},
	"repo file update":   {"gitlab-cli repo file update README.md --branch main --content \"hi\" --message \"edit\" --dry-run --compact", "gitlab-cli repo file update README.md --branch main --content \"hi\" --message \"edit\" --confirm <confirm_token> --compact"},
	"repo tree":          {"gitlab-cli repo tree --ref main --compact"},

	"search code":     {"gitlab-cli search code \"func main\" --compact"},
	"search commits":  {"gitlab-cli search commits \"fix bug\" --compact"},
	"search issues":   {"gitlab-cli search issues \"login\" --compact"},
	"search mrs":      {"gitlab-cli search mrs \"refactor\" --compact"},
	"search projects": {"gitlab-cli search projects \"gitlab\" --compact"},

	"project create":  {"gitlab-cli project create --name \"My App\" --visibility private --dry-run --compact", "gitlab-cli project create --name \"My App\" --visibility private --confirm <confirm_token> --compact"},
	"project get":     {"gitlab-cli project get group/repo --compact"},
	"project list":    {"gitlab-cli project list --limit 20 --compact"},
	"project members": {"gitlab-cli project members --compact"},

	"user get":    {"gitlab-cli user get alice --compact"},
	"user me":     {"gitlab-cli user me --compact"},
	"user search": {"gitlab-cli user search alice --compact"},

	"variable bulk-import": {"gitlab-cli variable bulk-import --project group/repo --file .env --dangerous --dry-run --compact", "gitlab-cli variable bulk-import --project group/repo --file .env --dangerous --confirm <confirm_token> --compact"},
	"variable create":      {"gitlab-cli variable create --key TOKEN --value s3cr3t --dangerous --dry-run --compact", "gitlab-cli variable create --key TOKEN --value s3cr3t --dangerous --confirm <confirm_token> --compact"},
	"variable delete":      {"gitlab-cli variable delete TOKEN --dangerous --dry-run --compact", "gitlab-cli variable delete TOKEN --dangerous --confirm <confirm_token> --compact"},
	"variable get":         {"gitlab-cli variable get TOKEN --compact"},
	"variable list":        {"gitlab-cli variable list --compact"},
	"variable update":      {"gitlab-cli variable update TOKEN --value new --dangerous --dry-run --compact", "gitlab-cli variable update TOKEN --value new --dangerous --confirm <confirm_token> --compact"},
}
