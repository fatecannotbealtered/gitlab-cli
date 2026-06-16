package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// clearSliceFlag hard-resets a pflag StringSlice flag to empty. Set("") would
// append (not replace) once Changed is true, so we use the SliceValue.Replace
// API to guarantee a clean slate regardless of prior state.
func clearSliceFlag(t *testing.T, cmd *cobra.Command, name string) {
	t.Helper()
	f := cmd.Flags().Lookup(name)
	if f == nil {
		t.Fatalf("no flag %q", name)
	}
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		_ = sv.Replace([]string{})
	}
	f.Changed = false
}

// resetCommitQueryFlags clears the (sticky) flag state on the commit list/diff
// commands so each subtest starts clean AND leaves them clean on exit — pflag
// slice/bool flags otherwise leak across rootCmd.Execute calls in the same
// binary and would break sibling tests that assume default state.
func resetCommitQueryFlags(t *testing.T) {
	t.Helper()
	apply := func() {
		resetRootPersistentFlags(t)
		clearSliceFlag(t, repoCommitListCmd, "project")
		clearSliceFlag(t, repoCommitListCmd, "group")
		for _, kv := range []struct{ name, val string }{
			{"all-projects", "false"}, {"author", ""}, {"with-stats", "false"},
			{"all-branches", "false"}, {"ref-name", ""}, {"since", ""}, {"until", ""},
			{"path", ""}, {"limit", "20"}, {"fields", ""},
		} {
			resetCommandFlagForTest(t, repoCommitListCmd, kv.name, kv.val)
		}
		for _, kv := range []struct{ name, val string }{
			{"project", ""}, {"path", ""}, {"fields", ""},
		} {
			resetCommandFlagForTest(t, repoCommitDiffCmd, kv.name, kv.val)
		}
	}
	apply()
	t.Cleanup(apply)
}

func withJSONMode(t *testing.T) {
	t.Helper()
	orig := jsonMode
	t.Cleanup(func() { jsonMode = orig })
	jsonMode = true
}

// TestRepoCommitList_AuthorWithStats verifies the single-project path forwards
// author/with_stats to GitLab and surfaces the per-commit line counts.
func TestRepoCommitList_AuthorWithStats(t *testing.T) {
	var gotAuthor, gotWithStats string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthor = r.URL.Query().Get("author")
		gotWithStats = r.URL.Query().Get("with_stats")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"abc123","short_id":"abc","title":"fix","author_name":"Alice","authored_date":"2026-06-02","stats":{"additions":10,"deletions":2,"total":12}}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--project", "group/proj", "--author", "alice", "--with-stats", "--json"})
		_ = rootCmd.Execute()
	})
	if gotAuthor != "alice" {
		t.Errorf("author query = %q, want alice", gotAuthor)
	}
	if gotWithStats != "true" {
		t.Errorf("with_stats query = %q, want true", gotWithStats)
	}
	if !strings.Contains(out, `"additions": 10`) || !strings.Contains(out, `"total": 12`) {
		t.Errorf("expected flattened stats in output, got:\n%s", out)
	}
}

// TestRepoCommitList_GroupFanOut verifies --group expands to its projects and
// aggregates per-project commits with project annotation + scan report.
func TestRepoCommitList_GroupFanOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/groups/team/projects"):
			_, _ = w.Write([]byte(`[{"id":1,"path_with_namespace":"team/a"},{"id":2,"path_with_namespace":"team/b"}]`))
		case strings.Contains(r.URL.Path, "/repository/commits"):
			// Distinguish the two projects by the encoded path segment.
			if strings.Contains(r.URL.Path, "team/a") {
				_, _ = w.Write([]byte(`[{"id":"a1","short_id":"a1","title":"in a","author_name":"Alice"}]`))
			} else {
				_, _ = w.Write([]byte(`[{"id":"b1","short_id":"b1","title":"in b","author_name":"Alice"}]`))
			}
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--group", "team", "--author", "alice", "--json"})
		_ = rootCmd.Execute()
	})
	var env struct {
		Data struct {
			Items           []map[string]any `json:"items"`
			Scope           string           `json:"scope"`
			ProjectsScanned int              `json:"projectsScanned"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if env.Data.Scope != "group" || env.Data.ProjectsScanned != 2 {
		t.Errorf("scope=%q scanned=%d, want group/2", env.Data.Scope, env.Data.ProjectsScanned)
	}
	if len(env.Data.Items) != 2 {
		t.Fatalf("want 2 aggregated commits, got %d:\n%s", len(env.Data.Items), out)
	}
	for _, it := range env.Data.Items {
		if it["project"] == "" || it["project"] == nil {
			t.Errorf("commit item missing project annotation: %v", it)
		}
	}
}

// TestRepoCommitList_GroupPartialFailure verifies one project's failure does not
// abort the scan and is reported under projectErrors.
func TestRepoCommitList_GroupPartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/groups/team/projects"):
			_, _ = w.Write([]byte(`[{"id":1,"path_with_namespace":"team/ok"},{"id":2,"path_with_namespace":"team/bad"}]`))
		case strings.Contains(r.URL.Path, "/repository/commits"):
			if strings.Contains(r.URL.Path, "team/bad") {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":"o1","short_id":"o1","title":"ok","author_name":"Alice"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--group", "team", "--json"})
		_ = rootCmd.Execute()
	})
	var env struct {
		Data struct {
			Items           []map[string]any `json:"items"`
			ProjectsScanned int              `json:"projectsScanned"`
			ProjectErrors   []map[string]any `json:"projectErrors"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if env.Data.ProjectsScanned != 1 || len(env.Data.Items) != 1 {
		t.Errorf("want 1 scanned/1 item, got scanned=%d items=%d", env.Data.ProjectsScanned, len(env.Data.Items))
	}
	if len(env.Data.ProjectErrors) != 1 {
		t.Fatalf("want 1 projectError, got %d:\n%s", len(env.Data.ProjectErrors), out)
	}
}

// TestRepoCommitList_AllProjectsRequiresAuthor verifies the instance-wide scope
// is fail-closed without --author (E_VALIDATION / exit 2).
func TestRepoCommitList_AllProjectsRequiresAuthor(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "http://localhost:0")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--all-projects", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitBadArgs {
		t.Fatalf("exit=%d want=%d\n%s", lastExit, ExitBadArgs, out)
	}
	if !strings.Contains(out, "requires --author") {
		t.Errorf("expected author-required error, got:\n%s", out)
	}
}

// TestRepoCommitList_AllProjects verifies --all-projects --author enumerates
// membership projects and aggregates.
func TestRepoCommitList_AllProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/projects":
			_, _ = w.Write([]byte(`[{"id":1,"path_with_namespace":"x/one"}]`))
		case strings.Contains(r.URL.Path, "/repository/commits"):
			_, _ = w.Write([]byte(`[{"id":"c1","short_id":"c1","title":"t","author_name":"Alice"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--all-projects", "--author", "alice", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"scope": "all-projects"`) || !strings.Contains(out, `"project": "x/one"`) {
		t.Errorf("expected all-projects aggregation, got:\n%s", out)
	}
}

// TestRepoCommitList_NoScope verifies a missing scope is a usage error.
func TestRepoCommitList_NoScope(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "http://localhost:0")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitBadArgs {
		t.Fatalf("exit=%d want=%d\n%s", lastExit, ExitBadArgs, out)
	}
	if !strings.Contains(out, "provide a scope") {
		t.Errorf("expected scope error, got:\n%s", out)
	}
}

// TestRepoCommitDiff_Full verifies the per-file diff shape.
func TestRepoCommitDiff_Full(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/diff") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"old_path":"a.go","new_path":"a.go","new_file":false,"deleted_file":false,"renamed_file":false,"diff":"@@ -1 +1,2 @@\n line\n+added\n-removed\n"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "diff", "abc1234", "--project", "group/proj", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"filesChanged": 1`) || !strings.Contains(out, `"newPath": "a.go"`) {
		t.Errorf("expected diff files shape, got:\n%s", out)
	}
	if !strings.Contains(out, `"additions": 1`) || !strings.Contains(out, `"deletions": 1`) {
		t.Errorf("expected computed per-file line counts, got:\n%s", out)
	}
}

// TestRepoCommitDiff_FieldsProjection verifies --fields drops the patch text for
// a cheap inventory.
func TestRepoCommitDiff_FieldsProjection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"old_path":"a.go","new_path":"a.go","diff":"@@\n+x\n"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "diff", "abc1234", "--project", "group/proj", "--fields", "newPath,additions,deletions", "--json"})
		_ = rootCmd.Execute()
	})
	if strings.Contains(out, `"diff"`) {
		t.Errorf("--fields should have dropped patch text, got:\n%s", out)
	}
	if !strings.Contains(out, `"newPath": "a.go"`) {
		t.Errorf("expected newPath retained, got:\n%s", out)
	}
}

// TestRepoCommitDiff_PathFilter verifies --path narrows to one file.
func TestRepoCommitDiff_PathFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"old_path":"a.go","new_path":"a.go","diff":"@@\n+x\n"},{"old_path":"b.go","new_path":"b.go","diff":"@@\n+y\n"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "diff", "abc1234", "--project", "group/proj", "--path", "b.go", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"filesChanged": 1`) || !strings.Contains(out, "b.go") || strings.Contains(out, "a.go") {
		t.Errorf("expected only b.go, got:\n%s", out)
	}
}

// TestRepoCommitDiff_RequiresProject verifies --project is required.
func TestRepoCommitDiff_RequiresProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "http://localhost:0")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	resetCommitQueryFlags(t)
	withJSONMode(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "diff", "abc1234", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitBadArgs {
		t.Fatalf("exit=%d want=%d\n%s", lastExit, ExitBadArgs, out)
	}
}
