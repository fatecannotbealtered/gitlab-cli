package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The search noun of the project group: GitLab's search API for projects, issues,
// merge requests, code and commits. Every search takes --query (CLI-028), optionally
// --project (a path, URL, ID or name), and states its order as upstream (CLI-029):
// GitLab's own, which scope.ranking describes (searchRanking).

const searchEvidence = "simulated:cmd/search_test.go"

// searchRanking is what each search page says about its order, sort upstream
// (CLI-029). GitLab ranks by relevance only with advanced search; basic search lists
// issues and merge requests newest first and states no order for the other scopes,
// and an answer does not say which search served it (docs/migration-2.0/findings.md, PROJ-G1).
const searchRanking = "GitLab's order: by relevance where GitLab has advanced search; basic search lists issues and merge requests newest first and states no order for the others"

// searchDataChars caps the matched lines of a code hit, and searchCodeLimit is the
// code search's default page, so a page of hits stays within the success budget
// (CLI-042) even when the lines are full of characters JSON escapes; the scope says
// so.
const (
	searchDataChars = 200
	searchCodeLimit = 10
)

var (
	// The search API sends projects without visibility or archived, so search
	// projects has its own schema: the project fields less those two.
	searchProjectFields   = slices.DeleteFunc(slices.Clone(projectFields), func(f string) bool { return f == "visibility" || f == "archived" })
	searchProjectDefaults = []string{"ref", "name", "last_activity_at", "web_url"}

	searchIssueFields    = []string{"ref", "id", "iid", "title", "state", "author", "labels", "created_at", "updated_at", "web_url"}
	searchIssueDefaults  = []string{"ref", "title", "state", "updated_at", "web_url"}
	searchIssueUntrusted = []string{"title", "labels"}

	searchMRFields = []string{"ref", "id", "iid", "title", "state", "draft", "author", "labels", "source_branch",
		"target_branch", "created_at", "updated_at", "web_url"}
	searchMRDefaults  = []string{"ref", "title", "state", "draft", "updated_at", "web_url"}
	searchMRUntrusted = []string{"title", "labels", "source_branch", "target_branch"}

	searchBlobFields    = []string{"ref", "path", "git_ref", "start_line", "data"}
	searchBlobUntrusted = []string{"ref", "path", "git_ref", "data"}

	searchCommitFields    = []string{"ref", "id", "short_id", "title", "author_name", "created_at", "web_url"}
	searchCommitDefaults  = []string{"ref", "short_id", "title", "author_name", "created_at"}
	searchCommitUntrusted = []string{"title", "author_name"}
)

func registerSearch(app *aicli.App) {
	group := addGroup(app, "search", "Search projects, issues, merge requests, code and commits by text")
	app.Schema("search_project_page", aicli.SchemaDef{Shape: "page", Fields: searchProjectFields, DefaultFields: searchProjectDefaults,
		Untrusted: projectUntrusted})
	app.Schema("search_issue_page", aicli.SchemaDef{Shape: "page", Fields: searchIssueFields, DefaultFields: searchIssueDefaults,
		Untrusted: searchIssueUntrusted})
	app.Schema("search_mr_page", aicli.SchemaDef{Shape: "page", Fields: searchMRFields, DefaultFields: searchMRDefaults,
		Untrusted: searchMRUntrusted})
	app.Schema("search_blob_page", aicli.SchemaDef{Shape: "page", Fields: searchBlobFields, Untrusted: searchBlobUntrusted})
	app.Schema("search_commit_page", aicli.SchemaDef{Shape: "page", Fields: searchCommitFields, DefaultFields: searchCommitDefaults,
		Untrusted: searchCommitUntrusted})
	addSearchProjects(app, group)
	addSearchIssues(app, group)
	addSearchMRs(app, group)
	addSearchCode(app, group)
	addSearchCommits(app, group)
}

// searchInput is a search's checked --query, --project ("" searches every project
// this account can see) and page.
type searchInput struct {
	query, project string
	req            pageRequest
}

// readSearch checks a search's flags; --project, when given, is read like any
// project, a name included (projectFlag).
func readSearch(cmd *cobra.Command, gl *lazyClient) (searchInput, error) {
	req, err := readPage(cmd)
	if err != nil {
		return searchInput{}, err
	}
	query, _ := cmd.Flags().GetString("query")
	if query = strings.TrimSpace(query); query == "" {
		return searchInput{}, aicli.Validation("--query", "the text to search for", "--query is empty.")
	}
	in := searchInput{query: query, req: req}
	if f := cmd.Flags().Lookup("project"); f != nil && f.Changed {
		if in.project, err = projectFlag(cmd, "", gl); err != nil {
			return searchInput{}, err
		}
	}
	return in, nil
}

func (in searchInput) opts() api.SearchOpts {
	return api.SearchOpts{Project: in.project, Query: in.query, Page: in.req.Page, PerPage: in.req.Limit}
}

// scope is what a search page covers (CLI-041): the query, the project or every
// project this account can see, and what its order means.
func (in searchInput) scope() map[string]any {
	s := map[string]any{"query": in.query, "ranking": searchRanking}
	if in.project != "" {
		s["project"] = in.project
	} else {
		s["covers"] = "every project this account can see"
	}
	return s
}

// searchError is readError for a search. A 404 of a search in a project is the
// project's; a search of every project that GitLab refuses for a scope only advanced
// search has (commits, code) points at --project.
func searchError(err error, in searchInput, needsAdvanced bool) error {
	if in.project != "" {
		return readError(err, projectNotFound())
	}
	e := readError(err)
	var ae *aicli.Error
	if needsAdvanced && errors.As(e, &ae) && ae.Code == "E_VALIDATION" {
		ae.Details["next_action"] = "Searching every project for this needs GitLab advanced search; add --project <group/project> to search one project."
	}
	return e
}

// searchProjectOf is the project path in a GitLab web URL
// (https://gitlab.example.com/acme/web/-/issues/3 gives acme/web), the configured
// host and its relative URL root left out; "" when there is none.
func searchProjectOf(host, webURL string) string {
	rest := ""
	switch {
	case webURL == "":
		return ""
	case host != "" && strings.HasPrefix(webURL, host+"/"):
		rest = webURL[len(host)+1:]
	default:
		u, err := url.Parse(webURL)
		if err != nil {
			return ""
		}
		rest = strings.TrimPrefix(u.Path, "/")
	}
	i := strings.Index(rest, "/-/")
	if i < 0 {
		return ""
	}
	p, numeric, err := parseProject(rest[:i])
	if err != nil || numeric {
		return ""
	}
	return p
}

// searchIIDRef is the ref of an issue ("#", issueRef) or merge request ("!", mrRef)
// a search found. Its project is the searched one, else the one GitLab's full
// reference names, else the one in its web URL; the web URL itself stands in when
// none is readable (commands accept it, CLI-048).
func searchIIDRef(host, project, full, webURL, sigil, urlPart string, iid int) string {
	if project == "" {
		if p, n, err := parseIIDRef(full, sigil, urlPart); err == nil && n == iid {
			project = p
		}
	}
	if project == "" {
		project = searchProjectOf(host, webURL)
	}
	if project == "" {
		return webURL
	}
	return project + sigil + strconv.Itoa(iid)
}

// searchAuthor is a found item's author as a user ref (@carol), or null.
func searchAuthor(u *api.User) any {
	if u == nil || u.Username == "" {
		return nil
	}
	return userRef(u.Username)
}

func searchLabels(labels []string) []string {
	if labels == nil {
		return []string{}
	}
	return labels
}

func addSearchProjects(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "projects", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		in, err := readSearch(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		ps, pag, err := client.Search.ProjectsPage(cmd.Context(), in.query, in.req.Page, in.req.Limit)
		if err != nil {
			return searchError(err, in, false)
		}
		items := make([]map[string]any, 0, len(ps))
		for _, p := range ps {
			item := projectOut(p)
			delete(item, "visibility")
			delete(item, "archived")
			items = append(items, item)
		}
		return app.List(cmd, gitlabPage(items, len(items), in.req, pag, in.scope(), "upstream"))
	}}
	c.Flags().String("query", "", "Text in the project's name, full path or description")
	_ = c.MarkFlagRequired("query")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Search projects by name, full path or description",
		Aliases: []string{"project"}, Effects: []aicli.Effect{aicli.Observe}, Output: "search_project_page",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified,
		Evidence: []string{searchEvidence, liveEvidence("search")}, Examples: []string{"gitlab-cli search projects --query payments"}})
}

func addSearchIssues(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "issues", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		in, err := readSearch(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		is, pag, err := client.Search.IssuesPage(cmd.Context(), in.opts())
		if err != nil {
			return searchError(err, in, false)
		}
		items := make([]map[string]any, 0, len(is))
		for _, i := range is {
			items = append(items, map[string]any{
				"ref": searchIIDRef(client.Host(), in.project, i.References.Full, i.WebURL, "#", "/-/issues/", i.IID),
				"id":  idString(i.ID), "iid": idString(i.IID), "title": i.Title, "state": i.State, "author": searchAuthor(i.Author),
				"labels": searchLabels(i.Labels), "created_at": utcTime(i.CreatedAt), "updated_at": utcTime(i.UpdatedAt), "web_url": i.WebURL,
			})
		}
		return app.List(cmd, gitlabPage(items, len(items), in.req, pag, in.scope(), "upstream"))
	}}
	c.Flags().String("query", "", "Text in the issue's title or description")
	c.Flags().String("project", "", "Only this project: path (acme/web), web URL, numeric ID or name")
	_ = c.MarkFlagRequired("query")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Search issues by title and description, in one project or in all",
		Aliases: []string{"issue"}, Effects: []aicli.Effect{aicli.Observe}, Output: "search_issue_page",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{searchEvidence, liveEvidence("search")},
		Examples: []string{`gitlab-cli search issues --query "login bug" --project acme/web`, "gitlab-cli search issues --query timeout"}})
}

func addSearchMRs(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "mrs", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		in, err := readSearch(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		ms, pag, err := client.Search.MergeRequestsPage(cmd.Context(), in.opts())
		if err != nil {
			return searchError(err, in, false)
		}
		items := make([]map[string]any, 0, len(ms))
		for _, m := range ms {
			items = append(items, map[string]any{
				"ref": searchIIDRef(client.Host(), in.project, m.References.Full, m.WebURL, "!", "/-/merge_requests/", m.IID),
				"id":  idString(m.ID), "iid": idString(m.IID), "title": m.Title, "state": m.State, "draft": m.Draft,
				"author": searchAuthor(m.Author), "labels": searchLabels(m.Labels), "source_branch": m.SourceBranch,
				"target_branch": m.TargetBranch, "created_at": utcTime(m.CreatedAt), "updated_at": utcTime(m.UpdatedAt), "web_url": m.WebURL,
			})
		}
		return app.List(cmd, gitlabPage(items, len(items), in.req, pag, in.scope(), "upstream"))
	}}
	c.Flags().String("query", "", "Text in the merge request's title or description")
	c.Flags().String("project", "", "Only this project: path (acme/web), web URL, numeric ID or name")
	_ = c.MarkFlagRequired("query")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Search merge requests by title and description, in one project or in all",
		Aliases: []string{"mr", "merge-requests"}, Effects: []aicli.Effect{aicli.Observe}, Output: "search_mr_page",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{searchEvidence, liveEvidence("search")},
		Examples: []string{"gitlab-cli search mrs --query retry --project acme/web", "gitlab-cli search mrs --query retry"}})
}

func addSearchCode(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "code", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		in, err := readSearch(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		bs, pag, err := client.Search.BlobsPage(cmd.Context(), in.opts())
		if err != nil {
			return searchError(err, in, true)
		}
		items := make([]map[string]any, 0, len(bs))
		for _, b := range bs {
			path, gitRef := b.Path, b.Ref
			if path == "" {
				path = b.Filename
			}
			if gitRef == "" {
				gitRef = "HEAD"
			}
			items = append(items, map[string]any{
				"ref":  fmt.Sprintf("%s/-/blob/%s/%s#L%d", in.project, gitRef, path, b.StartLine),
				"path": path, "git_ref": optString(b.Ref), "start_line": b.StartLine, "data": projectClip(b.Data, searchDataChars),
			})
		}
		scope := in.scope()
		scope["data"] = fmt.Sprintf("the matched lines, at most %d characters per item", searchDataChars)
		return app.List(cmd, gitlabPage(items, len(items), in.req, pag, scope, "upstream"))
	}}
	c.Flags().String("query", "", "Text in the files")
	c.Flags().String("project", "", "The project to search: path (acme/web), web URL, numeric ID or name")
	_ = c.MarkFlagRequired("query")
	_ = c.MarkFlagRequired("project")
	c.Flags().Int("limit", searchCodeLimit, "Items per page, 1-100")
	c.Flags().Int("offset", 0, "Items to skip: next_offset from the previous page")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Search the files of one project's default branch",
		Aliases: []string{"blobs", "blob"}, Effects: []aicli.Effect{aicli.Observe}, Output: "search_blob_page",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{searchEvidence, liveEvidence("search")},
		Examples: []string{`gitlab-cli search code --query "func Login" --project acme/web`}})
}

func addSearchCommits(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "commits", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		in, err := readSearch(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		cs, pag, err := client.Search.CommitsPage(cmd.Context(), in.opts())
		if err != nil {
			return searchError(err, in, true)
		}
		projectOf := searchCommitProjects(cmd.Context(), client, in.project)
		items := make([]map[string]any, 0, len(cs))
		for _, sc := range cs {
			ref := sc.ID
			if p := projectOf(sc); p != "" {
				ref = p + "/-/commit/" + sc.ID
			}
			items = append(items, map[string]any{"ref": ref, "id": sc.ID, "short_id": sc.ShortID, "title": sc.Title,
				"author_name": sc.AuthorName, "created_at": utcTime(sc.CreatedAt), "web_url": optString(sc.WebURL)})
		}
		return app.List(cmd, gitlabPage(items, len(items), in.req, pag, in.scope(), "upstream"))
	}}
	c.Flags().String("query", "", "Text in the commit message")
	c.Flags().String("project", "", "Only this project: path (acme/web), web URL, numeric ID or name; all projects need GitLab advanced search")
	_ = c.MarkFlagRequired("query")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Search commit messages in one project, or in all with GitLab advanced search",
		Aliases: []string{"commit"}, Effects: []aicli.Effect{aicli.Observe}, Output: "search_commit_page",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{searchEvidence, liveEvidence("search")},
		Examples: []string{"gitlab-cli search commits --query hotfix --project acme/web"}})
}

// searchCommitProjects finds the project of each commit a search found, for its ref
// (acme/web/-/commit/<sha>): the searched project, else the one in its web URL, else
// its project ID read once per project. A project that cannot be read leaves the
// commit's ref its bare SHA.
func searchCommitProjects(ctx context.Context, c *api.Client, project string) func(api.SearchCommit) string {
	byID := map[int]string{}
	return func(sc api.SearchCommit) string {
		if project != "" {
			return project
		}
		if p := searchProjectOf(c.Host(), sc.WebURL); p != "" {
			return p
		}
		if sc.ProjectID <= 0 {
			return ""
		}
		if p, ok := byID[sc.ProjectID]; ok {
			return p
		}
		p, err := resolveProjectPath(ctx, c, strconv.Itoa(sc.ProjectID))
		if err != nil {
			p = ""
		}
		byID[sc.ProjectID] = p
		return p
	}
}
