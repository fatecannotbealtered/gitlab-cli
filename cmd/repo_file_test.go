package cmd

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- behaviour matrix

func TestRepoFileMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "repo file get", Args: []string{"README.md", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--path"},
				{[]string{"a/../b", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--path"},
				{[]string{"README.md"}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"README.md", "--project", "acme/web", "--output", "x.md"}, "E_USAGE", "invalid_value", "--output"},
			}},
		{Path: "repo file content", Args: []string{"README.md", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--path"},
				{[]string{"README.md", "--project", "acme/web", "--limit", "0"}, "E_USAGE", "invalid_value", "--limit"},
				{[]string{"README.md", "--project", "acme/web", "--offset", "-1"}, "E_USAGE", "invalid_value", "--offset"},
				{[]string{"README.md", "--project", "acme/web", "--offset", "5", "--format", "raw"}, "E_USAGE", "conflicting_flags", "--offset"},
			}},
		{Path: "repo file create", Args: []string{"docs/new.md", "--project", "acme/web", "--branch", "main", "--content", "hello",
			"--commit-message", "Add a doc"}, Role: "developer", PreviewReads: 2,
			Invalid: []invalidCase{
				{[]string{"docs/new.md", "--project", "acme/web", "--branch", "main", "--content", "x"}, "E_USAGE", "missing_parameter", "--commit-message"},
				{[]string{"docs/new.md", "--project", "acme/web", "--branch", "main", "--commit-message", "m"}, "E_USAGE", "missing_parameter", "--content"},
				{[]string{"docs/new.md", "--project", "acme/web", "--content", "x", "--commit-message", "m"}, "E_USAGE", "missing_parameter", "--branch"},
				{[]string{"docs//new.md", "--project", "acme/web", "--branch", "main", "--content", "x", "--commit-message", "m"}, "E_VALIDATION", "invalid_value", "--path"},
				{[]string{"x.md", "--project", "acme/web", "--branch", "main", "--content", "x", "--commit-message", "m", "--encoding", "hex"}, "E_USAGE", "invalid_value", "--encoding"},
				{[]string{"x.md", "--project", "acme/web", "--branch", "main", "--content", "!!", "--commit-message", "m", "--encoding", "base64"}, "E_VALIDATION", "invalid_value", "--encoding"},
				{[]string{"x.md", "--project", "acme/web", "--branch", "a b", "--content", "x", "--commit-message", "m"}, "E_VALIDATION", "invalid_value", "--branch"},
			}},
		{Path: "repo file update", Args: []string{"README.md", "--project", "acme/web", "--branch", "main", "--content", "# Web 2\n",
			"--commit-message", "Update the readme"}, Role: "developer", PreviewReads: 2,
			Invalid: []invalidCase{
				{[]string{"README.md", "--project", "acme/web", "--branch", "main", "--commit-message", "m"}, "E_USAGE", "missing_parameter", "--content"},
				{[]string{"acme/web/-/blob/main/README.md", "--branch", "develop", "--content", "x", "--commit-message", "m"}, "E_USAGE", "conflicting_flags", "--branch"},
			}},
		{Path: "repo file delete", Args: []string{"docs/guide.md", "--project", "acme/web", "--branch", "main", "--commit-message", "Remove the guide"},
			Role: "developer", PreviewReads: 2,
			Invalid: []invalidCase{
				{[]string{"docs/guide.md", "--project", "acme/web", "--branch", "main", "--dangerous"}, "E_USAGE", "missing_parameter", "--commit-message"},
				{[]string{"--project", "acme/web", "--branch", "main", "--commit-message", "m", "--dangerous"}, "E_USAGE", "missing_parameter", "--path"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestRepoFileFormsAreOneTarget: a path with --ref, the file's ref, its blob and raw
// URLs read the same file; a ref whose branch holds a "/" is found; a write's ref
// form and its path with --branch bind as one request (CLI-048, CLI-054).
func TestRepoFileFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"repo", "file", "get", "README.md", "--project", "acme/web", "--ref", "release/1.0"},
		{"repo", "file", "get", "acme/web/-/blob/release/1.0/README.md"},
		{"repo", "file", "get", e.gl.URL + "/acme/web/-/blob/release/1.0/README.md"},
		{"repo", "file", "get", e.gl.URL + "/acme/web/-/raw/release/1.0/README.md"},
		{"file", "get", "--path", "/README.md", "--project", "1", "--ref", "release/1.0"},
	} {
		r := e.run(args...)
		d := r.data()
		if r.exit != 0 || d["ref"] != "acme/web/-/blob/release/1.0/README.md" || d["ref_name"] != "release/1.0" ||
			d["last_commit_id"] != repoSeedSHA(2) || d["web_url"] != e.gl.URL+"/acme/web/-/blob/release/1.0/README.md" {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	r := e.run("repo", "file", "get", "README.md", "--project", "acme/web")
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if r.data()["ref_name"] != "HEAD" || len(notices) != 1 || notices[0].(map[string]any)["type"] != "content_not_included" {
		t.Fatalf("the default ref and the content notice: %s", r.out)
	}
	if _, has := r.data()["content"]; has {
		t.Fatalf("get carries no content (CLI-043): %s", r.out)
	}
	if r := e.run("repo", "file", "get", "acme/web/-/blob/main/nope.md"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing file: %s", r.out)
	}
	args := []string{"repo", "file", "update", "acme/web/-/blob/release/1.0/README.md", "--content", "x", "--commit-message", "m", "--dry-run"}
	if r := e.run(args...); r.preview()["can_apply"] != false || !strings.Contains(repoJSON(r.preview()["blocked_by"]), "cannot_push") {
		t.Fatalf("a branch this account cannot push to blocks the plan: %s", r.out)
	}
	tok := e.dryRun("repo", "file", "update", "acme/web/-/blob/develop/docs/guide.md", "--content", "new", "--commit-message", "m")
	r = e.confirm(tok, "repo", "file", "update", "docs/guide.md", "--project", "acme/web", "--branch", "develop", "--content", "new", "--commit-message", "m")
	if r.exit != 0 || r.data()["result"].(map[string]any)["ref"] != "acme/web/-/blob/develop/docs/guide.md" {
		t.Fatalf("the ref and the path with --branch bind alike: %s", r.out)
	}
}

// TestRepoFileGetRawAndOutput: --format raw prints the file's bytes, binary ones too;
// 1.x's --output is refused with the replacement named.
func TestRepoFileGetRawAndOutput(t *testing.T) {
	e := newTestEnv(t)
	if r := e.run("repo", "file", "get", "assets/logo.png", "--project", "acme/web", "--format", "raw"); r.exit != 0 || r.out != string(fakeRepoLogo) {
		t.Fatalf("raw bytes: %q", r.out)
	}
	before := len(e.gl.Requests())
	r := e.run("repo", "file", "content", "README.md", "--project", "acme/web", "--output", "x.md")
	if r.code() != "E_USAGE" || !strings.Contains(r.details()["next_action"].(string), "--format raw") || len(e.gl.Requests()) != before {
		t.Fatalf("--output: %s", r.out)
	}
}

// TestRepoFileContentWindows: the content comes a window of lines at a time with what
// is needed to go on (CLI-042); binary files point at --format raw; a line longer
// than a response is cut and flagged.
func TestRepoFileContentWindows(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("repo", "file", "content", "README.md", "--project", "acme/web")
	d := r.data()
	if r.exit != 0 || d["content"] != "# Web\n\nThe acme web app, with login.\n" || d["total_lines"] != repoNumber(3) || d["has_more"] != false ||
		d["start_line"] != repoNumber(1) || d["binary"] != false {
		t.Fatalf("content: %s", r.out)
	}
	r = e.run("repo", "file", "content", "README.md", "--project", "acme/web", "--limit", "2")
	if r.data()["content"] != "# Web\n\n" || r.data()["has_more"] != true || r.data()["next_offset"] != repoNumber(2) {
		t.Fatalf("first window: %s", r.out)
	}
	r = e.run("repo", "file", "content", "README.md", "--project", "acme/web", "--limit", "2", "--offset", "2")
	if r.data()["content"] != "The acme web app, with login.\n" || r.data()["has_more"] != false || r.data()["start_line"] != repoNumber(3) {
		t.Fatalf("second window: %s", r.out)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.content") {
		t.Fatalf("content is untrusted: %s", r.out)
	}
	r = e.run("repo", "file", "content", "assets/logo.png", "--project", "acme/web")
	if r.exit != 0 || r.data()["binary"] != true || r.data()["content"] != nil || r.data()["size"] != repoNumber(len(fakeRepoLogo)) {
		t.Fatalf("binary: %s", r.out)
	}
	long := strings.Repeat("x", 9000) + "\nend\n"
	e.repoPush("acme/web", "main", "min.js", long)
	r = e.run("repo", "file", "content", "min.js", "--project", "acme/web", "--ref", "main")
	if r.exit != 0 || r.data()["truncated"] != true || r.data()["next_offset"] != repoNumber(1) || len(r.out) > 8000 {
		t.Fatalf("a long line: %d %v", len(r.out), r.data()["next_offset"])
	}
	if raw := e.run("repo", "file", "content", "min.js", "--project", "acme/web", "--ref", "main", "--format", "raw"); raw.out != long {
		t.Fatal("raw prints the whole file")
	}
}

// TestRepoFileCreateChecksAndBindsContent: an existing path, a missing branch and a
// branch this account cannot push to are refused before anything is sent; the
// content from a file binds like the same text inline, and a binary file goes as
// base64 (CLI-054).
func TestRepoFileCreateChecksAndBindsContent(t *testing.T) {
	e := newTestEnv(t)
	base := []string{"--project", "acme/web", "--content", "x", "--commit-message", "m", "--dry-run"}
	if r := e.run(append([]string{"repo", "file", "create", "README.md", "--branch", "main"}, base...)...); r.preview()["can_apply"] != false ||
		!strings.Contains(repoJSON(r.preview()["blocked_by"]), "already_exists") {
		t.Fatalf("an existing file: %s", r.out)
	}
	if r := e.run(append([]string{"repo", "file", "create", "x.md", "--branch", "nope"}, base...)...); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing branch: %s", r.out)
	}
	file := filepath.Join(e.dir, "doc.md")
	if err := os.WriteFile(file, []byte("Hello, docs.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tok := e.dryRun("repo", "file", "create", "docs/hello.md", "-p", "acme/web", "--branch", "main", "--content-file", file, "--commit-message", "Add hello")
	r := e.confirm(tok, "repo", "file", "create", "docs/hello.md", "-p", "acme/web", "--branch", "main", "--content", "Hello, docs.\n", "--commit-message", "Add hello")
	if r.exit != 0 || r.outcome()["verification"] != "passed" || r.receipt()["observed"].(map[string]any)["content_sha256"] != fakeRepoSHA256([]byte("Hello, docs.\n")) {
		t.Fatalf("--content-file and --content are one request: %s", r.out)
	}
	if got, _ := e.repoFileAt("acme/web", "main", "docs/hello.md"); got != "Hello, docs.\n" {
		t.Fatalf("content: %q", got)
	}
	bin := filepath.Join(e.dir, "logo.bin")
	if err := os.WriteFile(bin, fakeRepoLogo, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"repo", "file", "create", "assets/copy.png", "-p", "acme/web", "--branch", "main", "--content-file", bin, "--commit-message", "Copy"}
	r = e.confirm(e.dryRun(args...), args...)
	if req, _ := e.gl.lastWrite(); r.exit != 0 || req.Body["encoding"] != "base64" {
		t.Fatalf("a binary file goes as base64: %s %v", r.out, req.Body)
	}
	if got, _ := e.repoFileAt("acme/web", "main", "assets/copy.png"); got != string(fakeRepoLogo) {
		t.Fatal("the binary content changed on the way")
	}
	args = []string{"repo", "file", "create", "b64.txt", "-p", "acme/web", "--branch", "main", "--content", base64.StdEncoding.EncodeToString([]byte("decoded")),
		"--encoding", "base64", "--commit-message", "B64"}
	if r := e.confirm(e.dryRun(args...), args...); r.exit != 0 {
		t.Fatalf("--encoding base64: %s", r.out)
	}
	if got, _ := e.repoFileAt("acme/web", "main", "b64.txt"); got != "decoded" {
		t.Fatalf("base64 content is decoded: %q", got)
	}
}

// TestRepoFirstCommitInEmptyRepository: a new project's repository has no branch
// yet, and GitLab's first commit creates the one it names, so repo file create and
// a commit create of new files go through there (found live: `project create`, then
// the first file, was E_NOT_FOUND at the dry-run). An update in the empty repository
// is still E_NOT_FOUND, and so is a missing branch once the repository has a commit.
func TestRepoFirstCommitInEmptyRepository(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"repo", "file", "create", "README.md", "-p", "acme/empty", "--branch", "main", "--content", "# Empty no more\n", "--commit-message", "Add README"}
	r := e.run(append(append([]string{}, args...), "--dry-run")...)
	if r.exit != 0 || r.preview()["can_apply"] != true || !strings.Contains(repoJSON(r.preview()["changes"]), "creates the branch") {
		t.Fatalf("the first file of an empty repository: %s", r.out)
	}
	if r = e.confirm(r.token(), args...); r.exit != 0 || r.outcome()["verification"] != "passed" {
		t.Fatalf("the first commit: %s", r.out)
	}
	if got, _ := e.repoFileAt("acme/empty", "main", "README.md"); got != "# Empty no more\n" {
		t.Fatalf("content: %q", got)
	}
	if r := e.run("repo", "file", "create", "x.md", "-p", "acme/empty", "--branch", "nope", "--content", "x", "--commit-message", "m", "--dry-run"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing branch once the repository has a commit: %s", r.out)
	}

	e = newTestEnv(t)
	if r := e.run("repo", "commit", "create", "-p", "acme/empty", "--branch", "main", "--message", "m", "--action", "update:path=a.md;content=x", "--dry-run"); r.preview()["can_apply"] != false ||
		!strings.Contains(repoJSON(r.preview()["blocked_by"]), "not_found") {
		t.Fatalf("an update in an empty repository: %s", r.out)
	}
	args = []string{"repo", "commit", "create", "-p", "acme/empty", "--branch", "main", "--message", "Start",
		"--action", "create:path=a.md;content=A", "--action", "create:path=docs/b.md;content=B"}
	if r := e.confirm(e.dryRun(args...), args...); r.exit != 0 {
		t.Fatalf("the first commit of new files: %s", r.out)
	}
	for path, want := range map[string]string{"a.md": "A", "docs/b.md": "B"} {
		if got, _ := e.repoFileAt("acme/empty", "main", path); got != want {
			t.Fatalf("%s: %q", path, got)
		}
	}
}

// TestRepoFileUpdateRefusesDrift: the file's last commit is the base; a commit to the
// file after the dry-run is state_drift and nothing is sent; content the file
// already has blocks the plan; GitLab gets last_commit_id.
func TestRepoFileUpdateRefusesDrift(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"repo", "file", "update", "docs/guide.md", "-p", "acme/web", "--branch", "main", "--content", "Mine\n", "--commit-message", "m"}
	tok := e.dryRun(args...)
	e.repoPush("acme/web", "main", "docs/guide.md", "Theirs\n")
	if r := e.confirm(tok, args...); r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" || e.gl.Writes() != 0 {
		t.Fatalf("drift: %s", r.out)
	}
	pushed := e.repoBranchHead("acme/web", "main") // the file's last commit now
	r := e.confirm(e.dryRun(args...), args...)
	if req, _ := e.gl.lastWrite(); r.exit != 0 || req.Body["last_commit_id"] != pushed {
		t.Fatalf("update with last_commit_id %s: %s %v", pushed, r.out, req.Body)
	}
	if r := e.run(append(args, "--dry-run")...); r.preview()["can_apply"] != false || !strings.Contains(repoJSON(r.preview()["blocked_by"]), "no_change") {
		t.Fatalf("the same content again: %s", r.out)
	}
}

// TestRepoFileDeleteReadsBack: a missing file is E_NOT_FOUND at the dry-run (CLI-029);
// the delete needs --dangerous and is read back as gone.
func TestRepoFileDeleteReadsBack(t *testing.T) {
	e := newTestEnv(t)
	if r := e.run("repo", "file", "delete", "nope.md", "-p", "acme/web", "--branch", "main", "--commit-message", "m", "--dangerous", "--dry-run"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing file: %s", r.out)
	}
	args := []string{"file", "rm", "docs/guide.md", "-p", "acme/web", "--branch", "develop", "--message", "Drop the guide", "--dangerous"}
	r := e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.outcome()["verification"] != "passed" || r.data()["result"].(map[string]any)["deleted"] != true {
		t.Fatalf("delete: %s", r.out)
	}
	if _, ok := e.repoFileAt("acme/web", "develop", "docs/guide.md"); ok {
		t.Fatal("the file is still there")
	}
	if _, ok := e.repoFileAt("acme/web", "main", "docs/guide.md"); !ok {
		t.Fatal("another branch lost the file")
	}
}
