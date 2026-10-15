package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/gitctx"
)

// The merge request group. An MR is named by its ref (acme/web!5), its web URL, or
// its iid with --project; notes and discussions by the MR's ref, their collection
// and ID (acme/web!5/notes/12, acme/web!5/discussions/<id>; GitLab's #note_12 and
// #discussion_<id> anchors are accepted too), and a changed file by the MR's ref
// and its path (acme/web!5:src/app.go), as git names a file at a revision. This file holds the reads (list, get, current, diff) and what every MR
// command shares; the writes are in mr_write.go, mr_bulk.go, mr_comment.go and
// mr_discussion.go.

const mrEvidence = "simulated:cmd/mr_test.go"

var (
	mrFields = []string{"ref", "id", "iid", "title", "description", "state", "draft", "source_branch", "target_branch",
		"author", "assignees", "reviewers", "labels", "pipeline_status", "detailed_merge_status", "has_conflicts", "sha",
		"diff_refs", "web_url", "created_at", "updated_at", "merged_at"}
	// mrListDefaults answer the questions the 2026-10 eval asked of MR lists (who is
	// on it, its labels, whether its pipeline failed; CLI-040).
	mrListDefaults = []string{"ref", "title", "state", "assignees", "reviewers", "labels", "pipeline_status", "updated_at", "web_url"}
	// mrGetDefaults leave out the description and the diff refs: an MR description
	// can be long (CLI-042), so it is asked for with --fields +description.
	mrGetDefaults = []string{"ref", "title", "state", "draft", "source_branch", "target_branch", "author", "assignees", "reviewers",
		"labels", "pipeline_status", "detailed_merge_status", "has_conflicts", "sha", "web_url", "updated_at", "merged_at"}
	// mrUntrusted are the fields people wrote: titles, descriptions, branch and label
	// names, usernames (CLI-046).
	mrUntrusted = []string{"title", "description", "author", "assignees", "reviewers", "labels", "source_branch", "target_branch"}
	// mrResultFields are the fields of a write's result (mrResult): no description
	// and no diff refs.
	mrResultFields    = mrWithout(mrFields, "description", "diff_refs")
	mrResultUntrusted = mrWithout(mrUntrusted, "description")

	mrStates         = []string{"opened", "closed", "merged", "locked", "all"}
	mrPipelineStates = []string{"created", "waiting_for_resource", "preparing", "pending", "running", "success", "failed",
		"canceled", "skipped", "manual", "scheduled"}
)

// mrPipelineScan is how many merge requests mr list --pipeline-status checks: the
// list endpoint has no pipeline status, so each one costs a read (CLI-044).
const mrPipelineScan = 100

// registerMR registers the merge request group, its nested groups and their
// schemas.
func registerMR(app *aicli.App) {
	group := addGroup(app, "mr", "List, read, create, update, review and merge merge requests, with their comments and discussions", "mrs")
	app.Schema("merge_request", aicli.SchemaDef{Shape: "object", Fields: mrFields, DefaultFields: mrGetDefaults, Untrusted: mrUntrusted})
	app.Schema("merge_request_page", aicli.SchemaDef{Shape: "page", Fields: mrFields, DefaultFields: mrListDefaults,
		Untrusted: mrUntrusted})
	app.Schema("merge_request_result", aicli.SchemaDef{Shape: "object", Fields: mrResultFields, Untrusted: mrResultUntrusted})
	app.Schema("merge_request_approval", aicli.SchemaDef{Shape: "object",
		Fields: []string{"ref", "approved", "approved_by", "approvals_left"}, Untrusted: []string{"approved_by"}})
	app.Schema("merge_request_diff_page", aicli.SchemaDef{Shape: "page", Fields: mrDiffFields, DefaultFields: mrDiffDefaults,
		Untrusted: []string{"old_path", "new_path", "diff"}})
	addMRList(app, group)
	addMRGet(app, group)
	addMRCurrent(app, group)
	addMRDiff(app, group)
	addMRCreate(app, group)
	addMRUpdate(app, group)
	addMRMerge(app, group)
	addMRClose(app, group)
	addMRReopen(app, group)
	addMRApprove(app, group)
	addMRUnapprove(app, group)
	registerMRComment(app, group)
	registerMRDiscussion(app, group)
	registerMRBulk(app, group)
	// R02 wants a root shortcut for every nested group (CLI-022). comment, bulk and
	// discussion are nested under issues too, so the root words carry the group
	// (App.AddShortcut; docs/migration-2.0/notes.md, "Integration decisions").
	for _, w := range []string{"mr-comment", "mr-comments"} {
		app.AddShortcut(w, "mr", "comment")
	}
	for _, w := range []string{"mr-discussion", "mr-discussions"} {
		app.AddShortcut(w, "mr", "discussion")
	}
	app.AddShortcut("mr-bulk", "mr", "bulk")
}

// ---------------------------------------------------------------- output

// mrOut is a merge request as every MR command returns it. pipeline tells whether
// m came from a single GET, which has the head pipeline: list items get
// pipeline_status from mrHeadPipelines instead.
func mrOut(project string, m api.MergeRequest, pipeline bool) map[string]any {
	out := map[string]any{
		"ref": mrRef(project, m.IID), "id": idString(m.ID), "iid": idString(m.IID), "title": m.Title,
		"description": m.Description, "state": m.State, "draft": m.Draft, "source_branch": m.SourceBranch,
		"target_branch": m.TargetBranch, "author": mrUsername(m.Author), "assignees": mrAssignees(m),
		"reviewers": mrUsernames(m.Reviewers), "labels": mrStrings(m.Labels),
		"detailed_merge_status": optString(m.DetailedMergeStatus), "has_conflicts": m.HasConflicts, "sha": optString(m.SHA),
		"web_url": m.WebURL, "created_at": utcTime(m.CreatedAt), "updated_at": utcTime(m.UpdatedAt), "merged_at": utcTime(m.MergedAt),
	}
	if pipeline {
		out["pipeline_status"] = mrPipelineStatus(m)
	}
	if m.DiffRefs != nil {
		out["diff_refs"] = map[string]any{"base_sha": m.DiffRefs.BaseSHA, "start_sha": m.DiffRefs.StartSHA, "head_sha": m.DiffRefs.HeadSHA}
	}
	return out
}

// mrResult is the data.result of a write (schema merge_request_result): the merge
// request without its description and diff refs, so a confirm stays small whatever
// the description holds (receipt.observed has the fields the write changed, the
// description included).
func mrResult(project string, m api.MergeRequest) map[string]any {
	out := mrOut(project, m, m.HeadPipeline != nil)
	delete(out, "description")
	delete(out, "diff_refs")
	return out
}

// mrWithout is list without the names given.
func mrWithout(list []string, names ...string) []string {
	var out []string
	for _, x := range list {
		if !hasStringIn(names, x) {
			out = append(out, x)
		}
	}
	return out
}

func mrPipelineStatus(m api.MergeRequest) any {
	if m.HeadPipeline == nil || m.HeadPipeline.Status == "" {
		return nil
	}
	return m.HeadPipeline.Status
}

// mrUsername is a user as the tool shows users: the user's ref, @carol (userRef,
// CLI-048), as every group does.
func mrUsername(u *api.User) any {
	if u == nil || u.Username == "" {
		return nil
	}
	return userRef(u.Username)
}

// mrUsernames lists users' refs in GitLab's order (never null).
func mrUsernames(users []api.User) []string {
	out := []string{}
	for _, u := range users {
		if u.Username != "" {
			out = append(out, userRef(u.Username))
		}
	}
	return out
}

// mrAssignees is the assignees list, or the single assignee older GitLab sends.
func mrAssignees(m api.MergeRequest) []string {
	out := mrUsernames(m.Assignees)
	if len(out) == 0 && m.Assignee != nil && m.Assignee.Username != "" {
		out = []string{userRef(m.Assignee.Username)}
	}
	return out
}

func mrStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return append([]string{}, in...)
}

// mrSorted is a sorted copy, for sets compared across reads (assignees, reviewers).
func mrSorted(in []string) []string {
	out := mrStrings(in)
	sort.Strings(out)
	return out
}

// mrExcerpt cuts external text to n characters for a bounded summary (CLI-043).
func mrExcerpt(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// ---------------------------------------------------------------- refs

// mrSel is the merge request a command acts on.
type mrSel struct {
	project string
	iid     int
}

func (s mrSel) ref() string { return mrRef(s.project, s.iid) }

// parseMRArg reads a merge request as an agent has it: "5", "!5", "acme/web!5", or
// the web URL (…/-/merge_requests/5, any suffix). rest is what follows the MR in a
// comment or thread ref, as GitLab's anchor ("/notes/12" and "#note_12" give
// "#note_12", "/discussions/<id>" and "#discussion_<id>" give "#discussion_<id>",
// a URL's fragment too), or in a changed file's ref (":src/app.go"); MR-level
// commands ignore it. project is "" when the input names none.
func parseMRArg(raw string) (project string, iid int, rest string, err error) {
	s := strings.TrimSpace(raw)
	if head, frag, ok := strings.Cut(s, "#"); ok {
		s, rest = head, "#"+frag
	}
	if !strings.Contains(s, "://") && !strings.Contains(s, "/-/merge_requests/") {
		start := strings.Index(s, "!") + 1 // project paths never hold "!"
		end := start
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end > start && end < len(s) && rest == "" {
			switch tail := s[end:]; {
			case tail[0] == ':':
				s, rest = s[:end], tail
			case strings.HasPrefix(tail, "/notes/"):
				s, rest = s[:end], "#note_"+strings.TrimPrefix(tail, "/notes/")
			case strings.HasPrefix(tail, "/discussions/"):
				s, rest = s[:end], "#discussion_"+strings.TrimPrefix(tail, "/discussions/")
			}
		}
	}
	project, iid, err = parseIIDRef(s, "!", "/-/merge_requests/")
	return project, iid, rest, err
}

// mrTarget reads the merge request positional (or --mr) and the project, and binds
// their canonical forms (CLI-054): the iid as --mr, the project path as --project,
// so "5 --project acme/web", "acme/web!5" and the web URL are one request. rest is
// parseMRArg's.
func mrTarget(cmd *cobra.Command, gl *lazyClient) (mrSel, string, error) {
	raw, _ := cmd.Flags().GetString("mr")
	if strings.TrimSpace(raw) == "" {
		e := aicli.Usage("missing_parameter", "--mr", fmt.Sprintf("`%s` needs the merge request: its iid with --project, its ref or its URL.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Pass the merge request as the argument, for example `%s acme/web!5`.", cmd.CommandPath())
		return mrSel{}, "", e
	}
	refProject, iid, rest, err := parseMRArg(raw)
	if err != nil {
		e := aicli.Validation("--mr", "an iid (5), a ref (acme/web!5) or the merge request's web URL", "That is not a merge request iid, ref or URL.")
		e.Details["next_action"] = "Run `gitlab-cli mr list --project <group/project>` to find the merge request."
		return mrSel{}, "", e
	}
	project, err := projectFlag(cmd, refProject, gl)
	if err != nil {
		return mrSel{}, "", err
	}
	if err := aicli.SetResolved(cmd, "mr", strconv.Itoa(iid)); err != nil {
		return mrSel{}, "", err
	}
	return mrSel{project: project, iid: iid}, rest, nil
}

// addMRTargetFlags declares the flags that name one merge request.
func addMRTargetFlags(c *cobra.Command) {
	c.Flags().String("mr", "", "Merge request: iid (5), ref (acme/web!5) or web URL")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the merge request is a ref or URL")
}

// mrNotFound is the 404 of a read of one merge request.
func mrNotFound(project string) errOpt {
	return onNotFound("The project has no merge request with that iid, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli mr list --project %s --state all` to find the merge request.", project))
}

// mrUser is a user filter or input as GitLab takes it: "carol" for "@carol"; "me"
// stays "me".
func mrUser(raw string) string { return strings.TrimPrefix(strings.TrimSpace(raw), "@") }

func hasStringIn(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// mrHeadPipelines reads the head pipeline of each merge request, four at a time:
// the list endpoint has none, a single GET has it. A failed read fails the list.
func mrHeadPipelines(ctx context.Context, c *api.Client, project string, mrs []api.MergeRequest) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	errs := make([]error, len(mrs))
	for i := range mrs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			full, err := c.MergeRequests.Get(ctx, project, mrs[i].IID)
			if err != nil {
				errs[i] = err
				return
			}
			mrs[i].HeadPipeline = full.HeadPipeline
			if mrs[i].HeadPipeline == nil {
				mrs[i].HeadPipeline = &api.MergeRequestPipeline{}
			}
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return readError(err, mrNotFound(project))
		}
	}
	return nil
}

// ---------------------------------------------------------------- list

func addMRList(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		f := cmd.Flags()
		state, _ := f.GetString("state")
		labels, _ := f.GetStringSlice("label")
		query, _ := f.GetString("query")
		source, _ := f.GetString("source-branch")
		target, _ := f.GetString("target-branch")
		pipeline, _ := f.GetString("pipeline-status")
		opts := api.MergeRequestListOpts{State: state, Labels: labels, Search: strings.TrimSpace(query),
			SourceBranch: strings.TrimSpace(source), TargetBranch: strings.TrimSpace(target), OrderBy: "updated_at", Sort: "desc"}
		scope := map[string]any{"project": project, "state": state}
		client, err := gl.get()
		if err != nil {
			return err
		}
		me := ""
		for _, uf := range []struct {
			flag string
			dst  *string
		}{{"author", &opts.AuthorUsername}, {"assignee", &opts.AssigneeUsername}, {"reviewer", &opts.ReviewerUsername}} {
			raw, _ := f.GetString(uf.flag)
			name := mrUser(raw)
			if name == "me" {
				if me == "" {
					u, err := client.Users.Me(cmd.Context())
					if err != nil {
						return readError(err)
					}
					me = u.Username
				}
				name = me
			}
			if name != "" {
				*uf.dst = name
				scope[uf.flag] = userRef(name)
			}
		}
		for key, v := range map[string]string{"query": opts.Search, "source_branch": opts.SourceBranch, "target_branch": opts.TargetBranch} {
			if v != "" {
				scope[key] = v
			}
		}
		if len(labels) > 0 {
			scope["labels"] = labels
		}
		if pipeline != "" {
			return mrListByPipeline(app, cmd, client, project, opts, req, scope, pipeline)
		}
		opts.Page, opts.PerPage = req.Page, req.Limit
		mrs, pag, err := client.MergeRequests.ListPage(cmd.Context(), project, opts)
		if err != nil {
			return readError(err, projectNotFound())
		}
		withPipeline := wantsField(cmd, "pipeline_status")
		if withPipeline {
			if err := mrHeadPipelines(cmd.Context(), client, project, mrs); err != nil {
				return err
			}
		}
		items := make([]map[string]any, 0, len(mrs))
		for _, m := range mrs {
			items = append(items, mrOut(project, m, withPipeline))
		}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "updated_at_desc"))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("state", "opened", "State, or all")
	c.Flags().String("author", "", "Author: carol, @carol or me")
	c.Flags().String("assignee", "", "Assignee: carol, @carol or me")
	c.Flags().String("reviewer", "", "Reviewer: carol, @carol or me")
	c.Flags().StringSlice("label", nil, "Has all of these labels (comma-separated)")
	c.Flags().String("query", "", "Text in the title or description")
	c.Flags().String("source-branch", "", "From this branch")
	c.Flags().String("target-branch", "", "Into this branch")
	c.Flags().String("pipeline-status", "", "Head pipeline status; checks the 100 most recently updated")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List a project's merge requests, most recently updated first, with assignees, reviewers, labels and pipeline status",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "merge_request_page",
		Params:       map[string]aicli.ParamDoc{"state": {Enum: mrStates}, "pipeline-status": {Enum: mrPipelineStates}},
		RenamedFlags: map[string]string{"search": "query"},
		ErrorCodes:   gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{mrEvidence, liveEvidence("mr")},
		Examples: []string{"gitlab-cli mr list --project acme/web",
			"gitlab-cli mr list --project acme/web --pipeline-status failed",
			"gitlab-cli mr list --project acme/web --reviewer me --fields +source_branch"}})
}

// mrListByPipeline is mr list --pipeline-status: GitLab cannot filter merge
// requests by pipeline, so the tool reads the head pipeline of the mrPipelineScan
// most recently updated merge requests that match the other filters and filters
// them itself (CLI-044). --limit and --offset page through the matches; scope says
// what was checked, and a list whose check stopped short is truncated (CLI-041).
func mrListByPipeline(app *aicli.App, cmd *cobra.Command, c *api.Client, project string, opts api.MergeRequestListOpts,
	req pageRequest, scope map[string]any, status string) error {
	opts.Page, opts.PerPage = 1, mrPipelineScan
	mrs, pag, err := c.MergeRequests.ListPage(cmd.Context(), project, opts)
	if err != nil {
		return readError(err, projectNotFound())
	}
	if err := mrHeadPipelines(cmd.Context(), c, project, mrs); err != nil {
		return err
	}
	var matches []map[string]any
	for _, m := range mrs {
		if m.HeadPipeline != nil && m.HeadPipeline.Status == status {
			matches = append(matches, mrOut(project, m, true))
		}
	}
	scope["pipeline_status"] = status
	scope["checked"] = len(mrs)
	scope["covers"] = fmt.Sprintf("the head pipeline of the %d most recently updated merge requests matching the other filters", mrPipelineScan)
	scope["limit"] = req.Limit
	if req.Offset > 0 {
		scope["offset"] = req.Offset
	}
	start := min(req.Offset, len(matches))
	end := min(start+req.Limit, len(matches))
	items := matches[start:end]
	if items == nil {
		items = []map[string]any{}
	}
	p := aicli.Page{Items: items, Count: len(items), Scope: scope, Sort: "updated_at_desc"}
	if end < len(matches) {
		next := req.Offset + req.Limit
		p.HasMore, p.NextOffset = true, &next
	}
	if pag.NextPage > 0 {
		// More merge requests match than were checked.
		p.HasMore, p.Truncated = true, true
		p.TruncatedReason = fmt.Sprintf("Only the %d most recently updated merge requests that match the other filters were checked for their pipeline status.", mrPipelineScan)
		if p.NextOffset == nil {
			p.ContinuationReason = "Narrow the list with --author, --assignee, --label, --source-branch or --target-branch to check older merge requests."
		}
	} else {
		total := len(matches)
		p.Total = &total
	}
	return app.List(cmd, p)
}

// ---------------------------------------------------------------- get

func addMRGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <mr>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, _, err := mrTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		m, err := client.MergeRequests.Get(cmd.Context(), sel.project, sel.iid)
		if err != nil {
			return readError(err, mrNotFound(sel.project))
		}
		return app.OK(cmd, mrOut(sel.project, *m, true))
	}}
	addMRTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one merge request with its pipeline and merge status; --fields +description adds the description",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "merge_request", Positional: "mr",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{mrEvidence, liveEvidence("mr")},
		Examples: []string{"gitlab-cli mr get acme/web!5", "gitlab-cli mr get 5 --project acme/web --fields +description,+diff_refs"}})
}

// ---------------------------------------------------------------- current

func addMRCurrent(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "current", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		branch, _ := cmd.Flags().GetString("branch")
		branch = strings.TrimSpace(branch)
		refProject := ""
		if !cmd.Flags().Changed("project") || branch == "" {
			// The checkout in the working directory: its GitLab remote and branch.
			g, _ := gitctx.Detect("")
			if g != nil && g.Remote != nil && !cmd.Flags().Changed("project") {
				if p, _, err := parseProject(g.Remote.ProjectPath); err == nil {
					refProject = p
				}
			}
			if branch == "" && g != nil {
				branch = g.CurrentBranch
			}
		}
		if refProject == "" && !cmd.Flags().Changed("project") {
			e := aicli.Usage("missing_parameter", "--project", "The working directory is not a clone of a GitLab project, so `mr current` needs --project.")
			e.Details["next_action"] = "Run it in the project's checkout, or pass --project <group/project> and --branch <branch>."
			return e
		}
		if branch == "" {
			e := aicli.Usage("missing_parameter", "--branch", "No branch is checked out in the working directory, so `mr current` needs --branch.")
			e.Details["next_action"] = "Pass --branch <source branch>."
			return e
		}
		project, err := projectFlag(cmd, refProject, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		mrs, _, err := client.MergeRequests.ListPage(cmd.Context(), project, api.MergeRequestListOpts{State: "opened",
			SourceBranch: branch, OrderBy: "updated_at", Sort: "desc", Page: 1, PerPage: 20})
		if err != nil {
			return readError(err, projectNotFound())
		}
		switch len(mrs) {
		case 0:
			return aicli.NotFound("No open merge request has this branch as its source.",
				fmt.Sprintf("Run `gitlab-cli mr list --project %s --source-branch %s --state all` to see closed and merged ones, or create one with `mr create`.",
					project, shellArg(branch)))
		case 1:
		default:
			cands := make([]aicli.Candidate, 0, len(mrs))
			for _, m := range mrs {
				cands = append(cands, aicli.Candidate{Ref: mrRef(project, m.IID), Label: m.TargetBranch})
			}
			return aicli.Ambiguous("More than one open merge request has this branch as its source (into different target branches).", cands)
		}
		m, err := client.MergeRequests.Get(cmd.Context(), project, mrs[0].IID)
		if err != nil {
			return readError(err, mrNotFound(project))
		}
		return app.OK(cmd, mrOut(project, *m, true))
	}}
	c.Flags().String("project", "", "Project path (acme/web); default: the GitLab remote of the git checkout in the working directory")
	c.Flags().String("branch", "", "Source branch; default: the branch checked out in the working directory")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show the open merge request of the current git branch (or of --branch)",
		Effects: []aicli.Effect{aicli.Observe}, Output: "merge_request",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Experimental, Evidence: []string{mrEvidence},
		Examples: []string{"gitlab-cli mr current", "gitlab-cli mr current --project acme/web --branch feature/retry"}})
}

// ---------------------------------------------------------------- diff

var (
	mrDiffFields   = []string{"ref", "old_path", "new_path", "status", "additions", "deletions", "too_large", "diff"}
	mrDiffDefaults = []string{"ref", "new_path", "status", "additions", "deletions"}
)

// mrDiffScanPages bounds the pages mr diff --path reads to find its file.
const mrDiffScanPages = 10

// mrDiffOut is one changed file. Its ref is the MR's and the path, as git names a
// file at a revision (acme/web!5:src/app.go).
func mrDiffOut(sel mrSel, d api.MergeRequestDiff) map[string]any {
	status := "modified"
	switch {
	case d.NewFile:
		status = "added"
	case d.DeletedFile:
		status = "deleted"
	case d.RenamedFile:
		status = "renamed"
	}
	add, del := 0, 0
	for _, line := range strings.Split(d.Diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			add++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			del++
		}
	}
	return map[string]any{"ref": sel.ref() + ":" + d.NewPath, "old_path": d.OldPath, "new_path": d.NewPath, "status": status,
		"additions": add, "deletions": del, "too_large": d.TooLarge, "diff": d.Diff}
}

func addMRDiff(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "diff <mr>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, rest, err := mrTarget(cmd, gl)
		if err != nil {
			return err
		}
		path, _ := cmd.Flags().GetString("path")
		path = strings.TrimSpace(path)
		if file, ok := strings.CutPrefix(rest, ":"); ok && file != "" {
			if path != "" && path != file {
				e := aicli.Usage("conflicting_flags", "--path", "--path names another file than the file ref given as the argument.")
				e.Details["next_action"] = "Give the file once: as acme/web!5:<path>, or with --path."
				return e
			}
			path = file
			if err := aicli.SetResolved(cmd, "path", path); err != nil {
				return err
			}
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		if aicli.OutputFormat(cmd) == "raw" {
			raw, err := client.MergeRequests.RawDiff(cmd.Context(), sel.project, sel.iid)
			if err != nil {
				return readError(err, mrNotFound(sel.project))
			}
			if path != "" {
				raw = mrRawDiffFile(raw, path)
			}
			return app.OK(cmd, raw)
		}
		scope := map[string]any{"mr": sel.ref()}
		if path == "" {
			req, err := readPage(cmd)
			if err != nil {
				return err
			}
			diffs, pag, err := client.MergeRequests.DiffsPage(cmd.Context(), sel.project, sel.iid, req.Page, req.Limit)
			if err != nil {
				return readError(err, mrNotFound(sel.project))
			}
			items := make([]map[string]any, 0, len(diffs))
			for _, d := range diffs {
				items = append(items, mrDiffOut(sel, d))
			}
			return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, ""))
		}
		// One file: GitLab cannot filter the diff by path, so read pages until it shows.
		scope["path"] = path
		items := []map[string]any{}
		for page := 1; page <= mrDiffScanPages; page++ {
			diffs, pag, err := client.MergeRequests.DiffsPage(cmd.Context(), sel.project, sel.iid, page, maxLimit)
			if err != nil {
				return readError(err, mrNotFound(sel.project))
			}
			for _, d := range diffs {
				if d.NewPath == path || d.OldPath == path {
					items = append(items, mrDiffOut(sel, d))
				}
			}
			if len(items) > 0 || pag.NextPage == 0 {
				return app.List(cmd, aicli.Page{Items: items, Count: len(items), Scope: scope})
			}
		}
		return app.List(cmd, aicli.Page{Items: items, Count: 0, Scope: scope, HasMore: true, Truncated: true,
			TruncatedReason:    fmt.Sprintf("Only the first %d changed files were searched for the path.", mrDiffScanPages*maxLimit),
			ContinuationReason: "Run `gitlab-cli mr diff` without --path and page with --offset to reach the later files."})
	}}
	addMRTargetFlags(c)
	c.Flags().String("path", "", "Only this file (its new or old path); the argument acme/web!5:<path> does the same")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List the files a merge request changes with line counts; --fields +diff adds each file's diff, --format raw prints the unified diff",
		Effects: []aicli.Effect{aicli.Observe}, Output: "merge_request_diff_page", Positional: "mr", RawOutput: true,
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{mrEvidence, liveEvidence("mr")},
		Examples: []string{"gitlab-cli mr diff acme/web!5", "gitlab-cli mr diff acme/web!5:src/app.go --fields +diff",
			"gitlab-cli mr diff acme/web!5 --format raw"}})
}

// mrRawDiffFile keeps the part of a unified diff that changes path.
func mrRawDiffFile(raw, path string) string {
	var out strings.Builder
	keep := false
	for _, line := range strings.SplitAfter(raw, "\n") {
		if header, ok := strings.CutPrefix(line, "diff --git "); ok {
			header = strings.TrimRight(header, "\r\n")
			keep = strings.HasPrefix(header, "a/"+path+" ") || strings.HasSuffix(header, " b/"+path)
		}
		if keep {
			out.WriteString(line)
		}
	}
	return out.String()
}

// mrIs reports whether err is a GitLab answer with this status.
func mrIs(err error, status int) bool {
	var apiErr *api.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}
