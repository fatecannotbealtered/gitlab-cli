package cmd

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------- fake GitLab

// The fake's repositories: a history of commits per project, each holding the whole
// tree, with branches, tags and releases on top. Writes add commits the way GitLab
// does (one per file write, one per actions[] commit).

type fakeRepoCommit struct {
	ID, Title, Message, Author, Email, Date string
	Parents                                 []string
	Files                                   map[string][]byte // the tree at this commit
	Last                                    map[string]string // path -> the commit that last changed it
	Diff                                    []fakeRepoDiff
}

type fakeRepoDiff struct {
	Old, New                  string
	NewFile, Deleted, Renamed bool
	Patch                     string
}

type fakeRepoBranch struct {
	Head               string
	Protected, CanPush bool
}

type fakeRepoRelease struct {
	Tag, Name, Description, Author, ReleasedAt string
	Milestones                                 []string
}

type fakeRepo struct {
	Default  string
	Branches map[string]*fakeRepoBranch
	Commits  map[string]*fakeRepoCommit
	Order    []string // commit SHAs, oldest first
	Tags     map[string]string
	Releases []*fakeRepoRelease // newest first
}

// fakeRepos is the repository state of the fake, by project path.
func fakeRepos(gl *fakeGitLab) map[string]*fakeRepo { return gl.state["repos"].(map[string]*fakeRepo) }

// fakeRepoIsEmpty is GitLab's empty_repo: the project's repository has no commit.
func fakeRepoIsEmpty(gl *fakeGitLab, project string) bool {
	repos, _ := gl.state["repos"].(map[string]*fakeRepo)
	rp := repos[project]
	return rp != nil && len(rp.Commits) == 0
}

func fakeRepoSHA(seed string) string {
	sum := sha1.Sum([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func fakeRepoSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// repoSeedSHA is the SHA of the n-th seeded commit of acme/web.
func repoSeedSHA(n int) string { return fakeRepoSHA("acme/web commit " + strconv.Itoa(n)) }

// fakeRepoLogo is a binary file of the seed.
var fakeRepoLogo = []byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3, 0xff}

// commit adds a commit on top of parent ("" for the first); change edits the tree
// and returns the diff entries.
func (rp *fakeRepo) commit(id, parent, title, author, date string, change func(files map[string][]byte) []fakeRepoDiff) *fakeRepoCommit {
	c := &fakeRepoCommit{ID: id, Title: title, Message: title, Author: author, Email: strings.ToLower(strings.Fields(author)[0]) + "@example.com",
		Date: date, Files: map[string][]byte{}, Last: map[string]string{}}
	if p := rp.Commits[parent]; p != nil {
		c.Parents = []string{parent}
		for k, v := range p.Files {
			c.Files[k] = v
		}
		for k, v := range p.Last {
			c.Last[k] = v
		}
	}
	c.Diff = change(c.Files)
	for _, d := range c.Diff {
		if d.Deleted || d.Renamed {
			delete(c.Last, d.Old)
		}
		if !d.Deleted {
			c.Last[d.New] = id
		}
	}
	rp.Commits[id] = c
	rp.Order = append(rp.Order, id)
	return c
}

// fakeRepoPatch is a unified diff hunk from old to new (nil for an added or deleted file).
func fakeRepoPatch(old, new []byte) string {
	lines := func(b []byte) []string {
		if b == nil {
			return nil
		}
		return strings.SplitAfter(strings.TrimSuffix(string(b), "\n"), "\n")
	}
	o, n := lines(old), lines(new)
	var b strings.Builder
	b.WriteString("@@ -1," + strconv.Itoa(len(o)) + " +1," + strconv.Itoa(len(n)) + " @@\n")
	for _, l := range o {
		b.WriteString("-" + strings.TrimSuffix(l, "\n") + "\n")
	}
	for _, l := range n {
		b.WriteString("+" + strings.TrimSuffix(l, "\n") + "\n")
	}
	return b.String()
}

func fakeRepoAdd(files map[string][]byte, path string, content []byte) fakeRepoDiff {
	files[path] = content
	return fakeRepoDiff{Old: path, New: path, NewFile: true, Patch: fakeRepoPatch(nil, content)}
}

func fakeRepoModify(files map[string][]byte, path string, content []byte) fakeRepoDiff {
	old := files[path]
	files[path] = content
	return fakeRepoDiff{Old: path, New: path, Patch: fakeRepoPatch(old, content)}
}

func fakeRepoRemove(files map[string][]byte, path string) fakeRepoDiff {
	old := files[path]
	delete(files, path)
	return fakeRepoDiff{Old: path, New: path, Deleted: true, Patch: fakeRepoPatch(old, nil)}
}

// seedRepos builds acme/web's history: three commits on main, branches (main
// protected, release/1.0 protected without push rights, old-experiment merged),
// tags v0.9, v1.0 and v1.1, and releases for v0.9 and v1.0. acme/empty has an empty
// repository.
func seedRepos(gl *fakeGitLab) map[string]*fakeRepo {
	web := &fakeRepo{Default: "main", Branches: map[string]*fakeRepoBranch{}, Commits: map[string]*fakeRepoCommit{}, Tags: map[string]string{}}
	c1 := web.commit(repoSeedSHA(1), "", "Initial commit", "Alice Smith", "2026-06-01T10:00:00.000+02:00", func(f map[string][]byte) []fakeRepoDiff {
		return []fakeRepoDiff{fakeRepoAdd(f, "README.md", []byte("# Web\n\nThe acme web app.\n")),
			fakeRepoAdd(f, "src/app.go", []byte("package main\n\nfunc main() {}\n")),
			fakeRepoAdd(f, "docs/guide.md", []byte("Guide\n")), fakeRepoAdd(f, "assets/logo.png", fakeRepoLogo)}
	})
	c2 := web.commit(repoSeedSHA(2), c1.ID, "Add login", "Bob Jones", "2026-06-15T10:00:00.000+02:00", func(f map[string][]byte) []fakeRepoDiff {
		return []fakeRepoDiff{fakeRepoAdd(f, "src/login.go", []byte("package main\n\nfunc login() {}\n")),
			fakeRepoModify(f, "README.md", []byte("# Web\n\nThe acme web app, with login.\n"))}
	})
	c3 := web.commit(repoSeedSHA(3), c2.ID, "Fix typo in guide", "Alice Smith", "2026-07-01T10:00:00.000+02:00", func(f map[string][]byte) []fakeRepoDiff {
		return []fakeRepoDiff{fakeRepoModify(f, "docs/guide.md", []byte("The guide\n"))}
	})
	c3.Message = "Fix typo in guide\n\nThe title said Guide."
	web.Branches["main"] = &fakeRepoBranch{Head: c3.ID, Protected: true, CanPush: true}
	web.Branches["develop"] = &fakeRepoBranch{Head: c2.ID, CanPush: true}
	web.Branches["feature-login"] = &fakeRepoBranch{Head: c2.ID, CanPush: true}
	web.Branches["old-experiment"] = &fakeRepoBranch{Head: c1.ID, CanPush: true}
	web.Branches["release/1.0"] = &fakeRepoBranch{Head: c2.ID, Protected: true}
	web.Tags = map[string]string{"v0.9": c1.ID, "v1.0": c2.ID, "v1.1": c3.ID}
	web.Releases = []*fakeRepoRelease{
		{Tag: "v1.0", Name: "Version 1.0", Description: "First stable release.", Author: "carol", ReleasedAt: "2026-06-20T08:00:00.000Z",
			Milestones: []string{"v1.0"}},
		{Tag: "v0.9", Name: "Version 0.9", Description: "Beta.", Author: "dave", ReleasedAt: "2026-06-02T08:00:00.000Z"},
	}
	empty := &fakeRepo{Default: "main", Branches: map[string]*fakeRepoBranch{}, Commits: map[string]*fakeRepoCommit{}, Tags: map[string]string{}}
	return map[string]*fakeRepo{"acme/web": web, "acme/empty": empty}
}

func init() {
	addFakeSeed(func(gl *fakeGitLab) { gl.state["repos"] = seedRepos(gl) })
	addFakeRoutes(
		fakeRoute{"GET /api/v4/projects/{project}/repository/tree", fakeRepoTree},
		fakeRoute{"GET /api/v4/projects/{project}/repository/files/{file}", fakeRepoFileGet},
		fakeRoute{"POST /api/v4/projects/{project}/repository/files/{file}", fakeRepoFileWrite},
		fakeRoute{"PUT /api/v4/projects/{project}/repository/files/{file}", fakeRepoFileWrite},
		fakeRoute{"DELETE /api/v4/projects/{project}/repository/files/{file}", fakeRepoFileWrite},
		fakeRoute{"GET /api/v4/projects/{project}/repository/branches", fakeRepoBranchList},
		fakeRoute{"GET /api/v4/projects/{project}/repository/branches/{branch}", fakeRepoBranchGet},
		fakeRoute{"POST /api/v4/projects/{project}/repository/branches", fakeRepoBranchCreate},
		fakeRoute{"DELETE /api/v4/projects/{project}/repository/branches/{branch}", fakeRepoBranchDelete},
		fakeRoute{"GET /api/v4/projects/{project}/repository/tags/{tag}", fakeRepoTagGet},
		fakeRoute{"GET /api/v4/projects/{project}/repository/commits", fakeRepoCommitList},
		fakeRoute{"GET /api/v4/projects/{project}/repository/commits/{sha}", fakeRepoCommitGet},
		fakeRoute{"GET /api/v4/projects/{project}/repository/commits/{sha}/diff", fakeRepoCommitDiff},
		fakeRoute{"POST /api/v4/projects/{project}/repository/commits", fakeRepoCommitCreate},
		fakeRoute{"GET /api/v4/groups/{group}/projects", fakeRepoGroupProjects},
	)
	addConformanceSamples(map[string][]string{
		"repo file create":   {"docs/new.md", "--project", "acme/web", "--branch", "main", "--content", "hello", "--commit-message", "Add a doc"},
		"repo file update":   {"README.md", "--project", "acme/web", "--branch", "main", "--content", "# Web 2\n", "--commit-message", "Update the readme"},
		"repo file delete":   {"docs/guide.md", "--project", "acme/web", "--branch", "main", "--commit-message", "Remove the guide"},
		"repo branch create": {"feature-x", "--project", "acme/web", "--ref", "main"},
		"repo branch delete": {"old-experiment", "--project", "acme/web"},
		"repo commit create": {"--project", "acme/web", "--branch", "main", "--message", "Sync docs",
			"--action", "create:path=docs/a.md;content=hello", "--action", "update:path=README.md;content=new readme"},
	})
}

// repoOf is the {project}'s repository; it answers 404 itself when there is none.
func (gl *fakeGitLab) repoOf(w http.ResponseWriter, r *http.Request) (*fakeProject, *fakeRepo) {
	p := gl.project(w, r)
	if p == nil {
		return nil, nil
	}
	return p, fakeRepos(gl)[p.Path]
}

// resolve finds the commit a ref names: HEAD, a branch, a tag, a SHA or its prefix.
func (rp *fakeRepo) resolve(ref string) *fakeRepoCommit {
	if ref == "" || ref == "HEAD" {
		ref = rp.Default
	}
	if b := rp.Branches[ref]; b != nil {
		return rp.Commits[b.Head]
	}
	if sha, ok := rp.Tags[ref]; ok {
		return rp.Commits[sha]
	}
	if c := rp.Commits[ref]; c != nil {
		return c
	}
	if len(ref) >= 7 {
		for id, c := range rp.Commits {
			if strings.HasPrefix(id, ref) {
				return c
			}
		}
	}
	return nil
}

// ancestors is every commit reachable from sha.
func (rp *fakeRepo) ancestors(sha string) map[string]bool {
	seen := map[string]bool{}
	for stack := []string{sha}; len(stack) > 0; {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if c := rp.Commits[id]; c != nil && !seen[id] {
			seen[id] = true
			stack = append(stack, c.Parents...)
		}
	}
	return seen
}

func (c *fakeRepoCommit) stats() map[string]any {
	add, del := 0, 0
	for _, d := range c.Diff {
		a, dl := repoCountDiffLines(d.Patch)
		add, del = add+a, del+dl
	}
	return map[string]any{"additions": add, "deletions": del, "total": add + del}
}

func (c *fakeRepoCommit) json(gl *fakeGitLab, project string, stats bool) map[string]any {
	parents := c.Parents
	if parents == nil {
		parents = []string{}
	}
	out := map[string]any{"id": c.ID, "short_id": c.ID[:8], "title": c.Title, "message": c.Message, "author_name": c.Author,
		"author_email": c.Email, "authored_date": c.Date, "committer_name": c.Author, "committer_email": c.Email,
		"committed_date": c.Date, "parent_ids": parents, "web_url": gl.URL + "/" + project + "/-/commit/" + c.ID}
	if stats {
		out["stats"] = c.stats()
	}
	return out
}

func fakeRepoTree(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	q := r.URL.Query()
	if len(rp.Commits) == 0 {
		fakePage(w, r, []map[string]any{}) // an empty repository
		return
	}
	c := rp.resolve(q.Get("ref"))
	if c == nil {
		fakeJSON(w, 404, map[string]any{"message": "404 Tree Not Found"})
		return
	}
	dir := strings.Trim(q.Get("path"), "/")
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	entries := map[string]string{} // path -> type
	for path := range c.Files {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		rest := strings.Split(path[len(prefix):], "/")
		for i := range rest {
			if i > 0 && q.Get("recursive") != "true" {
				break
			}
			typ := "tree"
			if i == len(rest)-1 {
				typ = "blob"
			}
			entries[prefix+strings.Join(rest[:i+1], "/")] = typ
		}
	}
	if len(entries) == 0 {
		fakeJSON(w, 404, map[string]any{"message": "404 Tree Not Found"})
		return
	}
	paths := sortedBy(entries)
	sort.SliceStable(paths, func(i, j int) bool { return entries[paths[i]] == "tree" && entries[paths[j]] == "blob" })
	var out []map[string]any
	for _, path := range paths {
		mode := "100644"
		if entries[path] == "tree" {
			mode = "040000"
		}
		out = append(out, map[string]any{"id": fakeRepoSHA(path + string(c.Files[path])), "name": path[strings.LastIndex(path, "/")+1:],
			"type": entries[path], "path": path, "mode": mode})
	}
	fakePage(w, r, out)
}

func fakeRepoFileGet(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	ref := r.URL.Query().Get("ref")
	c := rp.resolve(ref)
	path := r.PathValue("file")
	content, ok := []byte(nil), false
	if c != nil {
		content, ok = c.Files[path]
	}
	if !ok {
		fakeJSON(w, 404, map[string]any{"message": "404 File Not Found"})
		return
	}
	fakeJSON(w, 200, map[string]any{"file_name": path[strings.LastIndex(path, "/")+1:], "file_path": path, "size": len(content),
		"encoding": "base64", "content": base64.StdEncoding.EncodeToString(content), "content_sha256": fakeRepoSHA256(content),
		"ref": ref, "blob_id": fakeRepoSHA(string(content)), "commit_id": c.ID, "last_commit_id": c.Last[path], "execute_filemode": false})
}

// fakeRepoContent decodes the content of a file write or a commit action.
func fakeRepoContent(b map[string]any) []byte {
	s, _ := b["content"].(string)
	if b["encoding"] == "base64" {
		out, _ := base64.StdEncoding.DecodeString(s)
		return out
	}
	return []byte(s)
}

// repoUser is the user a request's token signs in as.
func (gl *fakeGitLab) repoUser(r *http.Request) fakeUser {
	return gl.users[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
}

func fakeRepoFileWrite(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	b := fakeBody(r)
	branch, _ := b["branch"].(string)
	br := rp.Branches[branch]
	if br == nil && r.Method == http.MethodPost && len(rp.Commits) == 0 {
		// An empty repository: GitLab's first commit creates the branch.
		br = &fakeRepoBranch{CanPush: true}
		rp.Branches[branch] = br
	}
	if br == nil {
		fakeJSON(w, 400, map[string]any{"message": "You can only create or edit files when you are on a branch"})
		return
	}
	if !br.CanPush {
		fakeJSON(w, 403, map[string]any{"message": "403 Forbidden - You are not allowed to push into this branch"})
		return
	}
	head := rp.Commits[br.Head]
	path := r.PathValue("file")
	exists, parent := false, ""
	if head != nil {
		_, exists = head.Files[path]
		parent = head.ID
	}
	switch {
	case r.Method == http.MethodPost && exists:
		fakeJSON(w, 400, map[string]any{"message": "A file with this name already exists"})
		return
	case r.Method != http.MethodPost && !exists:
		fakeJSON(w, 400, map[string]any{"message": "A file with this name doesn't exist"})
		return
	case r.Method != http.MethodPost && b["last_commit_id"] != nil && b["last_commit_id"] != head.Last[path]:
		fakeJSON(w, 400, map[string]any{"message": "You are attempting to update a file that has changed since you started editing it."})
		return
	}
	msg, _ := b["commit_message"].(string)
	c := rp.commit(fakeRepoSHA(gl.now()+path), parent, strings.SplitN(msg, "\n", 2)[0], gl.repoUser(r).Name, gl.now(), func(f map[string][]byte) []fakeRepoDiff {
		switch r.Method {
		case http.MethodPost:
			return []fakeRepoDiff{fakeRepoAdd(f, path, fakeRepoContent(b))}
		case http.MethodPut:
			return []fakeRepoDiff{fakeRepoModify(f, path, fakeRepoContent(b))}
		}
		return []fakeRepoDiff{fakeRepoRemove(f, path)}
	})
	c.Message = msg
	br.Head = c.ID
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	fakeJSON(w, map[string]int{http.MethodPost: 201, http.MethodPut: 200}[r.Method], map[string]any{"file_path": path, "branch": branch})
}

func (rp *fakeRepo) branchJSON(gl *fakeGitLab, project, name string) map[string]any {
	b := rp.Branches[name]
	merged := name != rp.Default && rp.ancestors(rp.Branches[rp.Default].Head)[b.Head]
	return map[string]any{"name": name, "merged": merged, "protected": b.Protected, "default": name == rp.Default,
		"developers_can_push": b.CanPush && !b.Protected, "developers_can_merge": !b.Protected, "can_push": b.CanPush,
		"web_url": gl.URL + "/" + project + "/-/tree/" + name, "commit": rp.Commits[b.Head].json(gl, project, false)}
}

func fakeRepoBranchList(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	search := r.URL.Query().Get("search")
	var out []map[string]any
	for _, name := range sortedBy(rp.Branches) {
		s := strings.TrimSuffix(strings.TrimPrefix(search, "^"), "$")
		switch {
		case strings.HasPrefix(search, "^") && !strings.HasPrefix(name, s),
			strings.HasSuffix(search, "$") && !strings.HasSuffix(name, s),
			!strings.Contains(name, s):
			continue
		}
		out = append(out, rp.branchJSON(gl, p.Path, name))
	}
	fakePage(w, r, out)
}

func fakeRepoBranchGet(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	if rp.Branches[r.PathValue("branch")] == nil {
		fakeJSON(w, 404, map[string]any{"message": "404 Branch Not Found"})
		return
	}
	fakeJSON(w, 200, rp.branchJSON(gl, p.Path, r.PathValue("branch")))
}

func fakeRepoBranchCreate(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	name, ref := r.URL.Query().Get("branch"), r.URL.Query().Get("ref")
	if rp.Branches[name] != nil {
		fakeJSON(w, 400, map[string]any{"message": "Branch already exists"})
		return
	}
	c := rp.resolve(ref)
	if c == nil {
		fakeJSON(w, 400, map[string]any{"message": "Invalid reference name: " + ref})
		return
	}
	rp.Branches[name] = &fakeRepoBranch{Head: c.ID, CanPush: true}
	fakeJSON(w, 201, rp.branchJSON(gl, p.Path, name))
}

func fakeRepoBranchDelete(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	name := r.PathValue("branch")
	switch b := rp.Branches[name]; {
	case b == nil:
		fakeJSON(w, 404, map[string]any{"message": "404 Branch Not Found"})
	case b.Protected:
		fakeJSON(w, 403, map[string]any{"message": "403 Forbidden"})
	default:
		delete(rp.Branches, name)
		w.WriteHeader(http.StatusNoContent)
	}
}

func fakeRepoTagGet(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	sha, ok := rp.Tags[r.PathValue("tag")]
	if !ok {
		fakeJSON(w, 404, map[string]any{"message": "404 Tag Not Found"})
		return
	}
	fakeJSON(w, 200, map[string]any{"name": r.PathValue("tag"), "message": "", "target": sha, "protected": false,
		"commit": rp.Commits[sha].json(gl, p.Path, false)})
}

func fakeRepoCommitList(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	q := r.URL.Query()
	if len(rp.Commits) == 0 {
		fakePage(w, r, []map[string]any{})
		return
	}
	reach := map[string]bool{}
	if q.Get("all") == "true" {
		for id := range rp.Commits {
			reach[id] = true
		}
	} else {
		c := rp.resolve(q.Get("ref_name"))
		if c == nil {
			fakeJSON(w, 404, map[string]any{"message": "404 Commit Not Found"})
			return
		}
		reach = rp.ancestors(c.ID)
	}
	var out []map[string]any
	for i := len(rp.Order) - 1; i >= 0; i-- { // newest first
		c := rp.Commits[rp.Order[i]]
		at, _ := time.Parse(time.RFC3339Nano, c.Date)
		since, _ := time.Parse(time.RFC3339, q.Get("since"))
		until, _ := time.Parse(time.RFC3339, q.Get("until"))
		touches := q.Get("path") == ""
		for _, d := range c.Diff {
			if strings.HasPrefix(d.New, q.Get("path")) || strings.HasPrefix(d.Old, q.Get("path")) {
				touches = true
			}
		}
		author := strings.ToLower(q.Get("author"))
		switch {
		case !reach[c.ID], !touches,
			q.Get("since") != "" && at.Before(since),
			q.Get("until") != "" && at.After(until),
			author != "" && !strings.Contains(strings.ToLower(c.Author+" "+c.Email), author):
			continue
		}
		out = append(out, c.json(gl, p.Path, q.Get("with_stats") == "true"))
	}
	fakePage(w, r, out)
}

func fakeRepoCommitGet(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	c := rp.resolve(r.PathValue("sha"))
	if c == nil {
		fakeJSON(w, 404, map[string]any{"message": "404 Commit Not Found"})
		return
	}
	fakeJSON(w, 200, c.json(gl, p.Path, r.URL.Query().Get("stats") != "false"))
}

func fakeRepoCommitDiff(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	c := rp.resolve(r.PathValue("sha"))
	if c == nil {
		fakeJSON(w, 404, map[string]any{"message": "404 Commit Not Found"})
		return
	}
	var out []map[string]any
	for _, d := range c.Diff {
		out = append(out, map[string]any{"old_path": d.Old, "new_path": d.New, "new_file": d.NewFile, "deleted_file": d.Deleted,
			"renamed_file": d.Renamed, "diff": d.Patch, "a_mode": "100644", "b_mode": "100644"})
	}
	fakePage(w, r, out)
}

func fakeRepoCommitCreate(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return
	}
	b := fakeBody(r)
	branch, _ := b["branch"].(string)
	start, _ := b["start_branch"].(string)
	base := ""
	switch {
	case rp.Branches[branch] != nil:
		if !rp.Branches[branch].CanPush {
			fakeJSON(w, 403, map[string]any{"message": "403 Forbidden - You are not allowed to push into this branch"})
			return
		}
		base = rp.Branches[branch].Head
	case start != "" && rp.Branches[start] != nil:
		base = rp.Branches[start].Head
	case start == "" && len(rp.Commits) == 0:
		// An empty repository: GitLab's first commit creates the branch.
	default:
		fakeJSON(w, 400, map[string]any{"message": "You can only create or edit files when you are on a branch"})
		return
	}
	head := rp.Commits[base]
	if head == nil {
		head = &fakeRepoCommit{Files: map[string][]byte{}, Last: map[string]string{}}
	}
	actions, _ := b["actions"].([]any)
	files := map[string][]byte{}
	for k, v := range head.Files {
		files[k] = v
	}
	var diff []fakeRepoDiff
	for _, raw := range actions {
		a, _ := raw.(map[string]any)
		path, _ := a["file_path"].(string)
		prev, _ := a["previous_path"].(string)
		lastOf := func(p string) bool { return a["last_commit_id"] == nil || a["last_commit_id"] == head.Last[p] }
		_, exists := files[path]
		fail := ""
		switch a["action"] {
		case "create":
			if exists {
				fail = "A file with this name already exists"
			} else {
				diff = append(diff, fakeRepoAdd(files, path, fakeRepoContent(a)))
			}
		case "update":
			if !exists || !lastOf(path) {
				fail = "A file with this name doesn't exist, or it changed"
			} else {
				diff = append(diff, fakeRepoModify(files, path, fakeRepoContent(a)))
			}
		case "delete":
			if !exists || !lastOf(path) {
				fail = "A file with this name doesn't exist, or it changed"
			} else {
				diff = append(diff, fakeRepoRemove(files, path))
			}
		case "move":
			old, had := files[prev]
			if !had || exists || !lastOf(prev) {
				fail = "The file to move doesn't exist, or the new path is taken"
				break
			}
			content := old
			if _, has := a["content"]; has {
				content = fakeRepoContent(a)
			}
			delete(files, prev)
			files[path] = content
			diff = append(diff, fakeRepoDiff{Old: prev, New: path, Renamed: true, Patch: fakeRepoPatch(old, content)})
		default:
			fail = "Unknown action"
		}
		if fail != "" {
			fakeJSON(w, 400, map[string]any{"message": fail})
			return
		}
	}
	msg, _ := b["commit_message"].(string)
	c := rp.commit(fakeRepoSHA(gl.now()+msg), head.ID, strings.SplitN(msg, "\n", 2)[0], gl.repoUser(r).Name, gl.now(), func(f map[string][]byte) []fakeRepoDiff {
		for k := range f {
			delete(f, k)
		}
		for k, v := range files {
			f[k] = v
		}
		return diff
	})
	c.Message = msg
	if rp.Branches[branch] == nil {
		rp.Branches[branch] = &fakeRepoBranch{CanPush: true}
	}
	rp.Branches[branch].Head = c.ID
	fakeJSON(w, 201, c.json(gl, p.Path, true))
}

func fakeRepoGroupProjects(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	group := r.PathValue("group")
	var out []map[string]any
	for _, p := range gl.projects {
		if strings.HasPrefix(p.Path, group+"/") {
			out = append(out, map[string]any{"id": p.ID, "path_with_namespace": p.Path})
		}
	}
	if out == nil && group != "acme" {
		fakeJSON(w, 404, map[string]any{"message": "404 Group Not Found"})
		return
	}
	fakePage(w, r, out)
}

// repoJSON is the compact JSON of a value from a decoded response, for comparisons.
func repoJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// repoNumber is a number as a decoded response holds it (UseNumber).
func repoNumber(n int) json.Number { return json.Number(strconv.Itoa(n)) }

// repoBranchHead is a branch's head commit (read under the fake's lock).
func (e *testEnv) repoBranchHead(project, branch string) string {
	var head string
	e.gl.locked(func() {
		if b := fakeRepos(e.gl)[project].Branches[branch]; b != nil {
			head = b.Head
		}
	})
	return head
}

// repoFileAt is a file's content at a branch ("" and false when it has none).
func (e *testEnv) repoFileAt(project, branch, path string) (string, bool) {
	var content []byte
	var ok bool
	e.gl.locked(func() {
		rp := fakeRepos(e.gl)[project]
		if c := rp.resolve(branch); c != nil {
			content, ok = c.Files[path]
		}
	})
	return string(content), ok
}

// repoPush adds a commit to a branch behind the CLI's back (drift tests).
func (e *testEnv) repoPush(project, branch, path, content string) {
	e.gl.locked(func() {
		rp := fakeRepos(e.gl)[project]
		b := rp.Branches[branch]
		c := rp.commit(fakeRepoSHA("push "+path+content), b.Head, "Pushed elsewhere", "Erin Hannon", e.gl.now(), func(f map[string][]byte) []fakeRepoDiff {
			if _, ok := f[path]; ok {
				return []fakeRepoDiff{fakeRepoModify(f, path, []byte(content))}
			}
			return []fakeRepoDiff{fakeRepoAdd(f, path, []byte(content))}
		})
		b.Head = c.ID
	})
}

// ---------------------------------------------------------------- behaviour matrix

func TestRepoTreeMatrix(t *testing.T) {
	runMatrix(t, matrixCase{Path: "repo tree", Args: []string{"--project", "acme/web"}, EmptyArgs: []string{"--project", "acme/empty"},
		Invalid: []invalidCase{
			{[]string{}, "E_USAGE", "missing_parameter", "--project"},
			{[]string{"../etc", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--path"},
			{[]string{"--project", "acme/web", "--limit", "0"}, "E_USAGE", "invalid_value", "--limit"},
			{[]string{"acme/web/-/tree/main/src", "--project", "acme/other"}, "E_USAGE", "conflicting_flags", "--project"},
		}})
}

// ---------------------------------------------------------------- focused tests

// TestRepoTreeRefsAndForms: a directory's path with --ref, its ref and its web URL
// list the same entries; a ref whose branch holds a "/" is found; blobs carry file
// refs that repo file content takes, trees directory refs (CLI-048).
func TestRepoTreeRefsAndForms(t *testing.T) {
	e := newTestEnv(t)
	var first []map[string]any
	for _, args := range [][]string{
		{"repo", "tree", "src", "--project", "acme/web", "--ref", "main"},
		{"repo", "tree", "acme/web/-/tree/main/src"},
		{"repo", "tree", e.gl.URL + "/acme/web/-/tree/main/src"},
		{"tree", "--path", "src", "-p", "acme/web", "--ref", "main"},
	} {
		r := e.run(args...)
		if r.exit != 0 || len(r.items()) != 2 {
			t.Fatalf("%v: %s", args, r.out)
		}
		if first == nil {
			first = r.items()
		} else if r.items()[0]["ref"] != first[0]["ref"] {
			t.Fatalf("%v: refs differ: %v vs %v", args, r.items()[0]["ref"], first[0]["ref"])
		}
	}
	if first[0]["ref"] != "acme/web/-/blob/main/src/app.go" || first[0]["type"] != "blob" {
		t.Fatalf("a blob's ref: %v", first[0])
	}
	r := e.run("repo", "tree", "--project", "acme/web")
	if items := r.items(); items[0]["type"] != "tree" || items[0]["ref"] != "acme/web/-/tree/HEAD/assets" {
		t.Fatalf("trees first, with directory refs: %s", r.out)
	}
	if scope := r.data()["scope"].(map[string]any); scope["ref_name"] != "HEAD" || scope["recursive"] != false {
		t.Fatalf("scope: %v", scope)
	}
	r = e.run("repo", "tree", "acme/web/-/tree/release/1.0/src")
	if r.exit != 0 || r.data()["scope"].(map[string]any)["ref_name"] != "release/1.0" || len(r.items()) != 2 {
		t.Fatalf("a branch with a slash: %s", r.out)
	}
	r = e.run("repo", "tree", "--project", "acme/web", "--recursive", "--limit", "100")
	if r.exit != 0 || len(r.items()) != 8 {
		t.Fatalf("recursive: %s", r.out)
	}
	if r := e.run("repo", "tree", "nowhere", "--project", "acme/web"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing directory: %s", r.out)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].path") {
		t.Fatalf("paths are untrusted: %s", r.out)
	}
}

// TestRepoCommandMapLines pins the repo and release lines of the Skill's command
// map (SKL-002): the verbs in registration order.
func TestRepoCommandMapLines(t *testing.T) {
	got := newTestApp(map[string]string{}).CommandMap()
	for _, line := range []string{
		"- `repo`: tree; `repo file` get|content|create|update|delete (dangerous); `repo branch` list|get|create|delete (dangerous); `repo commit` list|get|diff|create\n",
		"- `release`: list, get, create, update, delete (dangerous)\n",
	} {
		if !strings.Contains(got, line) {
			t.Fatalf("command map:\n%s\nwant the line:\n%s", got, line)
		}
	}
	if strings.Index(got, "- `repo`") > strings.Index(got, "- `release`") {
		t.Fatalf("repo comes before release:\n%s", got)
	}
}

// TestRepoAgentGuesses: the root nouns for the nested groups (branch, file, commit,
// tree and their plurals), the leaf aliases, --branch for --name on branch commands
// and --message for --commit-message are accepted (CLI-022).
func TestRepoAgentGuesses(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"branch", "list", "--project", "acme/web"},
		{"branches", "ls", "-R", "acme/web"},
		{"repos", "branch", "view", "main", "--repo", "acme/web"},
		{"branch", "get", "--branch", "main", "-p", "acme/web"},
		{"file", "get", "README.md", "-p", "acme/web"},
		{"files", "cat", "README.md", "-p", "acme/web"},
		{"commit", "list", "-p", "acme/web"},
		{"commits", "show", "main", "-p", "acme/web"},
		{"tree", "-p", "acme/web"},
		{"releases", "ls", "-p", "acme/web"},
		{"release", "view", "v1.0", "-p", "acme/web"},
	} {
		if r := e.run(args...); r.exit != 0 {
			t.Fatalf("%v should be accepted: %s", args, r.out)
		}
	}
	for _, args := range [][]string{
		{"branch", "delete", "--branch", "old-experiment", "-p", "acme/web", "--dangerous", "--dry-run"},
		{"branch", "rm", "old-experiment", "-p", "acme/web", "--dangerous", "--dry-run"},
		{"file", "add", "x.md", "-p", "acme/web", "--branch", "main", "--content", "x", "--message", "Add x", "--dry-run"},
		// A commit that deletes a file is dangerous (Preview.Dangerous, Q13).
		{"commit", "create", "-p", "acme/web", "--branch", "main", "--commit-message", "x", "--action", "delete:path=docs/guide.md", "--dangerous", "--dry-run"},
	} {
		if r := e.run(args...); r.exit != 0 || r.token() == "" {
			t.Fatalf("%v should be accepted: %s", args, r.out)
		}
	}
	// --branch means --name only on branch commands: elsewhere it stays itself.
	if r := e.run("release", "create", "v1.1", "-p", "acme/web", "--branch", "main", "--dry-run"); r.details()["problem"] != "unknown_flag" {
		t.Fatalf("--branch on release create: %s", r.out)
	}
}

// TestRepoScopeKeys: file writes are keyed by the file's ref on the branch, so a
// grant for one branch's docs/ covers files there and nothing else; branch writes by
// the branch's ref (SEC-012).
func TestRepoScopeKeys(t *testing.T) {
	e := newTestEnv(t)
	scope := fullScope()
	scope["targets"] = []map[string]any{{"key": "acme/web/-/blob/develop/docs"}, {"key": "acme/web/-/branches/feature-x"}}
	e.setScope(scope)
	ok := [][]string{
		{"repo", "file", "create", "docs/new.md", "-p", "acme/web", "--branch", "develop", "--content", "x", "--commit-message", "m", "--dry-run"},
		{"repo", "branch", "create", "feature-x", "-p", "acme/web", "--ref", "main", "--dry-run"},
		{"repo", "commit", "create", "-p", "acme/web", "--branch", "develop", "--message", "m", "--action", "create:path=docs/b.md;content=b", "--dry-run"},
	}
	for _, args := range ok {
		if r := e.run(args...); r.exit != 0 || r.token() == "" {
			t.Fatalf("%v is in the scope: %s", args, r.out)
		}
	}
	refused := [][]string{
		{"repo", "file", "create", "src/new.go", "-p", "acme/web", "--branch", "develop", "--content", "x", "--commit-message", "m", "--dry-run"},
		{"repo", "file", "create", "docs/new.md", "-p", "acme/web", "--branch", "main", "--content", "x", "--commit-message", "m", "--dry-run"},
		{"repo", "file", "create", "docsite.md", "-p", "acme/web", "--branch", "develop", "--content", "x", "--commit-message", "m", "--dry-run"},
		{"repo", "branch", "create", "feature-xy", "-p", "acme/web", "--ref", "main", "--dry-run"},
		{"repo", "commit", "create", "-p", "acme/web", "--branch", "develop", "--message", "m", "--action", "create:path=docs/b.md;content=b",
			"--action", "create:path=src/b.go;content=b", "--dry-run"},
	}
	for _, args := range refused {
		if r := e.run(args...); r.code() != "E_FORBIDDEN" || r.details()["scope"] == nil {
			t.Fatalf("%v is outside the scope: %s", args, r.out)
		}
	}
	if e.gl.Writes() != 0 {
		t.Fatal("a dry-run wrote")
	}
}

func TestParseRepoWebArg(t *testing.T) {
	for in, want := range map[string]repoWebArg{
		"docs/guide.md":                       {rest: "docs/guide.md"},
		"acme/web/-/blob/main/docs/guide.md":  {project: "acme/web", part: "blob", rest: "main/docs/guide.md", web: true},
		"https://h/acme/web/-/raw/v1/a%20b.c": {project: "acme/web", part: "raw", rest: "v1/a b.c", web: true},
		"g/s/p/-/tree/feat/x":                 {project: "g/s/p", part: "tree", rest: "feat/x", web: true},
	} {
		got, err := parseRepoWebArg(in, "blob", "raw", "tree")
		if err != nil || got != want {
			t.Errorf("parseRepoWebArg(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"https://h/acme/web", "acme/web/-/blob/", "https://h/acme/../web/-/blob/main/x"} {
		if _, err := parseRepoWebArg(bad, "blob"); err == nil {
			t.Errorf("parseRepoWebArg(%q) should fail", bad)
		}
	}
	if got := refPathSplits("feat/x/src/a.go", false); len(got) != 3 || got[0] != (repoRefPath{"feat", "x/src/a.go"}) ||
		got[2] != (repoRefPath{"feat/x/src", "a.go"}) {
		t.Errorf("splits: %v", got)
	}
	if got := refPathSplits("main", true); len(got) != 1 || got[0] != (repoRefPath{"main", ""}) {
		t.Errorf("a tree root: %v", got)
	}
}

func TestRepoNameAndPathRules(t *testing.T) {
	for _, good := range []string{"main", "feat/login", "release-1.0", "v1.2.0", "user@feature"} {
		if _, err := repoRefName(good, "--name", "branch"); err != nil {
			t.Errorf("%q is a valid name: %v", good, err)
		}
	}
	for _, bad := range []string{"", "a b", "a..b", "a~1", "x^", "a:b", "a?", "a*", "a[b", `a\b`, "-x", "/x", "x/", "x.", "a/.b", "x.lock", "a@{1}", "a//b", "@"} {
		if _, err := repoRefName(bad, "--name", "branch"); err == nil {
			t.Errorf("%q is not a valid name", bad)
		}
	}
	for _, c := range [][2]string{{"docs/a.md", "docs/a.md"}, {"/docs/a.md", "docs/a.md"}, {"./a.md", "a.md"}, {"  a b.md\t", "a b.md"}} {
		if got, err := repoFilePath(c[0], "--path", false); err != nil || got != c[1] {
			t.Errorf("repoFilePath(%q) = %q, %v", c[0], got, err)
		}
	}
	for _, bad := range []string{"", "a/../b", "a/./b", "a//b", "..", "a\\b", "a/"} {
		if _, err := repoFilePath(bad, "--path", false); err == nil {
			t.Errorf("repoFilePath(%q) should fail", bad)
		}
	}
	if got, err := repoFilePath("/", "--path", true); err != nil || got != "" {
		t.Errorf("the root directory: %q %v", got, err)
	}
	if got, cut := repoClip("one\ntwo\nthree\n", 9); !cut || got != "one\ntwo\n" {
		t.Errorf("repoClip at a line end: %q %v", got, cut)
	}
	if got := repoTextValue(strings.Repeat("x", 300)).(map[string]any); got["bytes"] != 300 {
		t.Errorf("a long text is its digest: %v", got)
	}
}

func TestRepoTimeFlag(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	flag := func(raw string) *cobra.Command {
		c := &cobra.Command{Use: "x"}
		c.Flags().String("since", "", "")
		_ = c.Flags().Set("since", raw)
		return c
	}
	for raw, want := range map[string]string{
		"7d": "2026-10-07T12:00:00Z", "24h": "2026-10-13T12:00:00Z", "30m": "2026-10-14T11:30:00Z",
		"2026-06-01": "2026-06-01T00:00:00Z", "2026-06-01T10:00:00+02:00": "2026-06-01T08:00:00Z", "": "",
	} {
		if got, err := repoTimeFlag(flag(raw), "since", now); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", raw, got, err, want)
		}
	}
	if _, err := repoTimeFlag(flag("last week"), "since", now); err == nil {
		t.Error("a sentence is not a time")
	}
}
