package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// repo commit: list (one project, or a fan-out over several, a group or every
// project of the account), get (with the message), diff (per-file entries, a heavy
// sub-resource of its own, CLI-043); create is in repo_commit_create.go.

const (
	repoFanOutConcurrency = 8
	repoFanOutMaxProjects = 100
	repoCommitMessageMax  = 4000 // characters of a message get returns in JSON
	repoDiffMaxFiles      = 1000 // files of one commit's diff read from GitLab
	repoDiffPatchMax      = 4000 // characters of one file's patch in JSON
	repoDiffPageBudget    = 6000 // characters of diff entries per page (CLI-042)
)

// repoCommitOut is a commit as commit list (without the message) and commit get return it.
func repoCommitOut(project string, c api.Commit, withMessage bool) map[string]any {
	parents := c.ParentIDs
	if parents == nil {
		parents = []string{}
	}
	out := map[string]any{"ref": repoCommitRef(project, c.ID), "sha": c.ID, "short_sha": c.ShortID, "title": c.Title,
		"author_name": c.AuthorName, "author_email": c.AuthorEmail, "authored_date": utcTime(c.AuthoredDate),
		"committer_name": c.CommitterName, "committer_email": c.CommitterEmail, "committed_date": utcTime(c.CommittedDate),
		"parent_shas": parents, "web_url": c.WebURL}
	if c.Stats != nil {
		out["additions"], out["deletions"], out["total"] = c.Stats.Additions, c.Stats.Deletions, c.Stats.Total
	}
	if withMessage {
		out["message"], out["message_truncated"] = repoClip(c.Message, repoCommitMessageMax)
	}
	return out
}

// repoParseCommitArg reads a commit as an agent has it: a SHA, a branch or tag name
// (its head commit), the ref (acme/web/-/commit/<sha>), GitLab's acme/web@<sha>, or
// a web URL (/-/commit/<sha>). project is "" when the input names none.
func repoParseCommitArg(raw string) (project, sha string, err error) {
	arg, err := parseRepoWebArg(raw, "commit", "commits")
	if err != nil {
		return "", "", err
	}
	if arg.web {
		sha = strings.SplitN(arg.rest, "/", 2)[0]
		if !repoHexSHA.MatchString(sha) {
			return "", "", fmt.Errorf("not a commit SHA")
		}
		return arg.project, sha, nil
	}
	s := strings.TrimSpace(raw)
	if i := strings.LastIndex(s, "@"); i > 0 && strings.Contains(s[:i], "/") && repoHexSHA.MatchString(s[i+1:]) {
		p, _, perr := parseProject(s[:i])
		if perr != nil {
			return "", "", perr
		}
		return p, s[i+1:], nil
	}
	sha, err = repoRevision(s, "--sha")
	return "", sha, err
}

// repoCommitTarget reads the commit argument (or --sha) and the project.
func repoCommitTarget(cmd *cobra.Command, gl *lazyClient) (project, sha string, err error) {
	raw, _ := cmd.Flags().GetString("sha")
	if strings.TrimSpace(raw) == "" {
		e := aicli.Usage("missing_parameter", "--sha", fmt.Sprintf("`%s` needs the commit: its SHA, ref, URL, or a branch or tag name.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Pass the commit as the argument, for example `%s 4f1c2a9 --project acme/web`.", cmd.CommandPath())
		return "", "", e
	}
	refProject, sha, err := repoParseCommitArg(raw)
	if err != nil {
		e := aicli.Validation("--sha", "a commit SHA, its ref (acme/web/-/commit/<sha>), acme/web@<sha>, a web URL, or a branch or tag name",
			"That is not a commit SHA, ref, URL or name.")
		e.Details["next_action"] = "Run `gitlab-cli repo commit list --project <group/project>` to find the commit."
		return "", "", e
	}
	if project, err = projectFlag(cmd, refProject, gl); err != nil {
		return "", "", err
	}
	if err := aicli.SetResolved(cmd, "sha", sha); err != nil {
		return "", "", err
	}
	return project, sha, nil
}

func addRepoCommitTargetFlags(c *cobra.Command) {
	c.Flags().String("sha", "", "Commit SHA (full or short), ref (acme/web/-/commit/<sha>), acme/web@<sha>, web URL, or a branch or tag name")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the commit is a ref")
}

// ---------------------------------------------------------------- commit list

func addRepoCommitList(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		opts, scope, err := repoCommitFilters(app, cmd)
		if err != nil {
			return err
		}
		projects, _ := cmd.Flags().GetStringSlice("project")
		groups, _ := cmd.Flags().GetStringSlice("group")
		allProjects, _ := cmd.Flags().GetBool("all-projects")
		projects, groups = repoNonEmpty(projects), repoNonEmpty(groups)
		given := 0
		for _, b := range []bool{len(projects) > 0, len(groups) > 0, allProjects} {
			if b {
				given++
			}
		}
		switch {
		case given == 0:
			e := aicli.Usage("missing_parameter", "--project", "`repo commit list` needs where to look: --project, --group or --all-projects.")
			e.Details["next_action"] = "Add --project <group/project>, for example `gitlab-cli repo commit list --project acme/web`."
			return e
		case given > 1:
			e := aicli.Usage("conflicting_flags", "--group", "Give one of --project, --group and --all-projects.")
			e.Details["next_action"] = "Keep the one that says where to look and drop the others."
			return e
		case allProjects && opts.Author == "":
			e := aicli.Usage("missing_parameter", "--author", "--all-projects reads every project of the account, so it needs --author.")
			e.Details["next_action"] = "Add --author <name or email>, or list one project with --project."
			return e
		}
		gl := gitlabFor(app)
		if len(projects) == 1 {
			project, err := repoCommitProject(cmd.Context(), projects[0], gl)
			if err != nil {
				return err
			}
			client, err := gl.get()
			if err != nil {
				return err
			}
			opts.Page, opts.PerPage = req.Page, req.Limit
			commits, pag, err := client.Repos.ListCommitsPage(cmd.Context(), project, opts)
			if err != nil {
				return readError(err, onNotFound("No project at that path, or no ref by that name in it, or this account cannot see it.",
					"Check --project, and --ref against `gitlab-cli repo branch list --project <group/project>`."))
			}
			items := make([]map[string]any, 0, len(commits))
			for _, c := range commits {
				items = append(items, repoCommitOut(project, c, false))
			}
			scope["project"] = project
			return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "committed_date_desc"))
		}
		if req.Offset > 0 {
			e := aicli.Usage("invalid_value", "--offset", "--offset pages one project's commits; a list over several projects is not paged.")
			e.Details["next_action"] = "Narrow --since and --until, or list one project with --project and page it with --offset."
			return e
		}
		return repoCommitFanOut(app, cmd, gl, req, opts, scope, projects, groups)
	}}
	c.Flags().StringSlice("project", nil, "Project path (acme/web), web URL or numeric ID; repeat or comma-separate for several")
	c.Flags().StringSlice("group", nil, "Every project of these groups and their subgroups (archived ones left out)")
	c.Flags().Bool("all-projects", false, "Every project this account is a member of; needs --author")
	c.Flags().String("author", "", "Only commits by this author: a name or email (GitLab 15.10 or later)")
	c.Flags().String("since", "", "Only commits after this time: ISO 8601 (2026-06-01) or a time ago (7d, 24h)")
	c.Flags().String("until", "", "Only commits before this time: ISO 8601 (2026-06-30) or a time ago (7d, 24h)")
	c.Flags().String("ref", "", "Branch, tag or SHA whose history to list (default: the default branch)")
	c.Flags().String("path", "", "Only commits that changed this file or directory")
	c.Flags().Bool("with-stats", false, "Add each commit's additions, deletions and total line counts")
	c.Flags().Bool("all-branches", false, "List the commits of every branch, not one ref's history")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List commits newest first, in one project or across projects or a group",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "commit_page", ErrorCodes: gitlabReadCodes,
		RenamedFlags: map[string]string{"ref-name": "ref"},
		Maturity:     aicli.Verified, Evidence: []string{repoCommitEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo commit list --project acme/web --ref main --limit 10",
			"gitlab-cli repo commit list --group acme --author alice --since 2026-06-01 --until 2026-06-30 --with-stats"}})
}

func repoNonEmpty(list []string) []string {
	var out []string
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// repoCommitFilters reads the filters of commit list into the API options and the
// list's scope (resolved times in UTC, CLI-028).
func repoCommitFilters(app *aicli.App, cmd *cobra.Command) (api.CommitListOpts, map[string]any, error) {
	var opts api.CommitListOpts
	scope := map[string]any{}
	ref, _ := cmd.Flags().GetString("ref") // 1.x's --ref-name too (Meta.RenamedFlags)
	if strings.TrimSpace(ref) != "" {
		r, err := repoRevision(ref, "--ref")
		if err != nil {
			return opts, nil, err
		}
		opts.RefName, scope["ref_name"] = r, r
	}
	now := repoNow(app)
	var err error
	if opts.Since, err = repoTimeFlag(cmd, "since", now); err != nil {
		return opts, nil, err
	}
	if opts.Until, err = repoTimeFlag(cmd, "until", now); err != nil {
		return opts, nil, err
	}
	if opts.Since != "" && opts.Until != "" && opts.Until < opts.Since {
		return opts, nil, aicli.Validation("--until", "a time after --since", "--until is before --since.")
	}
	for k, v := range map[string]string{"since": opts.Since, "until": opts.Until} {
		if v != "" {
			scope[k] = v
		}
	}
	if p, _ := cmd.Flags().GetString("path"); strings.TrimSpace(p) != "" {
		if opts.Path, err = repoFilePath(p, "--path", true); err != nil {
			return opts, nil, err
		}
		scope["path"] = opts.Path
	}
	author, _ := cmd.Flags().GetString("author")
	if opts.Author = strings.TrimSpace(author); opts.Author != "" {
		scope["author"] = opts.Author
	}
	opts.WithStats, _ = cmd.Flags().GetBool("with-stats")
	opts.WithStats = opts.WithStats || wantsField(cmd, "additions", "deletions", "total")
	if opts.AllBranches, _ = cmd.Flags().GetBool("all-branches"); opts.AllBranches {
		scope["all_branches"] = true
	}
	return opts, scope, nil
}

// repoGroupPath canonicalises a --group value: a path (acme, acme/team), a web URL
// (https://host/groups/acme or https://host/acme) or a numeric ID.
func repoGroupPath(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			s = ""
		} else {
			s = strings.TrimPrefix(u.Path, "/groups/")
		}
	}
	s = strings.Trim(s, "/")
	ok := s != ""
	for _, seg := range strings.Split(s, "/") {
		if !projectSegment.MatchString(seg) || strings.HasSuffix(seg, ".") {
			ok = false
		}
	}
	if !ok {
		return "", aicli.Validation("--group", "a group path such as acme or acme/team, its web URL or its numeric ID", "--group needs a group path.")
	}
	return s, nil
}

// repoCommitProject canonicalises one --project value of commit list; a numeric
// ID is resolved to its path.
func repoCommitProject(ctx context.Context, raw string, gl *lazyClient) (string, error) {
	p, numeric, err := parseProject(raw)
	if err != nil {
		return "", projectInvalid(raw)
	}
	if !numeric {
		return p, nil
	}
	c, err := gl.get()
	if err != nil {
		return "", err
	}
	return resolveProjectPath(ctx, c, p)
}

// repoCommitFanOut lists commits of several projects (CLI-041, CLI-044): each project's
// newest --limit commits are read (at most 8 at a time), merged newest first and cut
// to --limit. A project that cannot be read is named in scope.projects_failed; the
// call fails only when none could be read.
func repoCommitFanOut(app *aicli.App, cmd *cobra.Command, gl *lazyClient, req pageRequest, opts api.CommitListOpts,
	scope map[string]any, projects, groups []string) error {
	ctx := cmd.Context()
	client, err := gl.get()
	if err != nil {
		return err
	}
	var paths []string
	cut := false
	seen := map[string]bool{}
	addPath := func(p string) {
		if !seen[p] && len(paths) < repoFanOutMaxProjects {
			seen[p] = true
			paths = append(paths, p)
		} else if !seen[p] {
			cut = true
		}
	}
	switch {
	case len(projects) > 0:
		for _, raw := range projects {
			p, err := repoCommitProject(ctx, raw, gl)
			if err != nil {
				return err
			}
			addPath(p)
		}
		scope["projects"] = paths
	case len(groups) > 0:
		var clean []string
		for _, raw := range groups {
			g, err := repoGroupPath(raw)
			if err != nil {
				return err
			}
			clean = append(clean, g)
			list, more, err := client.Repos.GroupProjectPaths(ctx, g, repoFanOutMaxProjects)
			if err != nil {
				return readError(err, onNotFound("No group at that path, or this account cannot see it.", "Check --group: it is the path in the group's URL, like acme."))
			}
			cut = cut || more
			for _, p := range list {
				addPath(p)
			}
		}
		scope["group"] = clean
	default:
		list, more, err := client.Repos.MemberProjectPaths(ctx, repoFanOutMaxProjects)
		if err != nil {
			return readError(err)
		}
		cut = more
		for _, p := range list {
			addPath(p)
		}
		scope["all_projects"] = true
	}
	type result struct {
		commits []api.Commit
		pag     api.Pagination
		err     error
	}
	results := make([]result, len(paths))
	sem := make(chan struct{}, repoFanOutConcurrency)
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			o := opts
			o.Page, o.PerPage = 1, req.Limit
			commits, pag, err := client.Repos.ListCommitsPage(ctx, p, o)
			results[i] = result{commits, pag, err}
		}(i, p)
	}
	wg.Wait()
	type dated struct {
		at  time.Time
		out map[string]any
	}
	var all []dated
	var failed []map[string]any
	var firstErr error
	more, total, totalKnown := cut, 0, !cut
	for i, r := range results {
		if r.err != nil {
			e := readError(r.err)
			code := "E_UNKNOWN"
			if ae, ok := e.(*aicli.Error); ok {
				code = ae.Code
			}
			failed = append(failed, map[string]any{"project": paths[i], "code": code})
			if firstErr == nil {
				firstErr = e
			}
			continue
		}
		more = more || r.pag.NextPage > 0
		totalKnown = totalKnown && r.pag.HasTotal
		total += r.pag.Total
		for _, c := range r.commits {
			at, _ := time.Parse(time.RFC3339Nano, c.CommittedDate)
			all = append(all, dated{at, repoCommitOut(paths[i], c, false)})
		}
	}
	if len(paths) > 0 && len(failed) == len(paths) {
		return firstErr
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].at.After(all[j].at) })
	if len(all) > req.Limit {
		all, more = all[:req.Limit], true
	}
	items := make([]map[string]any, 0, len(all))
	for _, d := range all {
		items = append(items, d.out)
	}
	scope["limit"] = req.Limit
	scope["projects_scanned"] = len(paths) - len(failed)
	if len(failed) > 0 {
		scope["projects_failed"] = failed
	}
	page := aicli.Page{Items: items, Count: len(items), Scope: scope, Sort: "committed_date_desc"}
	if totalKnown && len(failed) == 0 && total >= len(items) {
		page.Total = &total
	}
	if cut {
		page.Truncated = true
		page.TruncatedReason = fmt.Sprintf("only the first %d projects were read", repoFanOutMaxProjects)
	}
	if more {
		page.HasMore = true
		page.ContinuationReason = "A list over several projects is not paged: narrow --since and --until, or list one project with --project and page it with --offset."
	}
	return app.List(cmd, page)
}

// ---------------------------------------------------------------- commit get

func addRepoCommitGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <sha>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		project, sha, err := repoCommitTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		commit, err := client.Repos.GetCommit(cmd.Context(), project, sha)
		if err != nil {
			return readError(err, repoCommitNotFound(project))
		}
		if repoRawFormat(cmd) {
			return app.OK(cmd, commit.Message)
		}
		out := repoCommitOut(project, *commit, true)
		if out["message_truncated"] == true {
			app.Notice(aicli.Notice{Type: "message_truncated", Severity: "warning",
				Message: "The commit message is longer than one response carries and was cut; add --format raw for all of it."})
		}
		return app.OK(cmd, out)
	}}
	addRepoCommitTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one commit with its message and line counts",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "commit", Positional: "sha",
		RawOutput: true, ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{repoCommitEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo commit get 4f1c2a9 --project acme/web", "gitlab-cli repo commit get acme/web@4f1c2a9e"}})
}

// ---------------------------------------------------------------- commit diff

// repoDiffChange names what a commit did to a file.
func repoDiffChange(f api.CommitDiffFile) string {
	switch {
	case f.NewFile:
		return "added"
	case f.DeletedFile:
		return "deleted"
	case f.RenamedFile:
		return "renamed"
	}
	return "modified"
}

// repoCountDiffLines counts added and removed lines of a patch, without its headers.
func repoCountDiffLines(diff string) (additions, deletions int) {
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			additions++
		case strings.HasPrefix(line, "-"):
			deletions++
		}
	}
	return additions, deletions
}

// repoDiffItem is one file of a commit's diff; its ref is the file as the commit left
// it, null for a file the commit deleted.
func repoDiffItem(project, sha string, f api.CommitDiffFile) map[string]any {
	path := f.NewPath
	var ref any = repoBlobRef(project, sha, f.NewPath)
	if f.DeletedFile || path == "" {
		path, ref = f.OldPath, nil
	}
	add, del := repoCountDiffLines(f.Diff)
	patch, cut := repoClip(f.Diff, repoDiffPatchMax)
	return map[string]any{"ref": ref, "path": path, "old_path": f.OldPath, "new_path": f.NewPath, "change": repoDiffChange(f),
		"additions": add, "deletions": del, "diff": patch, "diff_truncated": cut || f.TooLarge || f.Collapsed}
}

// repoUnifiedDiff is the commit's diff as one unified diff, for --format raw.
func repoUnifiedDiff(files []api.CommitDiffFile) string {
	var b strings.Builder
	for _, f := range files {
		from, to := "a/"+f.OldPath, "b/"+f.NewPath
		if f.NewFile {
			from = "/dev/null"
		}
		if f.DeletedFile {
			to = "/dev/null"
		}
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- %s\n+++ %s\n", f.OldPath, f.NewPath, from, to)
		b.WriteString(f.Diff)
		if f.Diff != "" && !strings.HasSuffix(f.Diff, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func repoJSONSize(v any) int {
	b, _ := json.Marshal(v)
	return len(b)
}

func addRepoCommitDiff(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "diff <sha>", RunE: func(cmd *cobra.Command, _ []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")
		if limit < 1 || limit > maxLimit {
			e := aicli.Usage("invalid_value", "--limit", "--limit must be between 1 and 100 files.")
			e.Details["expected"] = "1-100"
			return e
		}
		if offset < 0 {
			e := aicli.Usage("invalid_value", "--offset", "--offset must be next_offset from the previous page.")
			e.Details["expected"] = "0 or more"
			return e
		}
		if repoRawFormat(cmd) && (cmd.Flags().Changed("offset") || cmd.Flags().Changed("limit")) {
			e := aicli.Usage("conflicting_flags", "--offset", "--format raw prints the whole diff; --offset and --limit page the JSON form.")
			e.Details["next_action"] = "Drop --offset and --limit, or drop --format raw."
			return e
		}
		pathFilter := ""
		if p, _ := cmd.Flags().GetString("path"); strings.TrimSpace(p) != "" {
			var err error
			if pathFilter, err = repoFilePath(p, "--path", false); err != nil {
				return err
			}
		}
		gl := gitlabFor(app)
		project, sha, err := repoCommitTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		// The commit first: a branch or tag name becomes the SHA the entries' refs
		// and the scope name, and a missing commit is one clear E_NOT_FOUND.
		commit, err := client.Repos.GetCommit(cmd.Context(), project, sha)
		if err != nil {
			return readError(err, repoCommitNotFound(project))
		}
		files, unread, err := client.Repos.ListCommitDiff(cmd.Context(), project, commit.ID, repoDiffMaxFiles)
		if err != nil {
			return readError(err, repoCommitNotFound(project))
		}
		if pathFilter != "" {
			kept := files[:0]
			for _, f := range files {
				if f.NewPath == pathFilter || f.OldPath == pathFilter {
					kept = append(kept, f)
				}
			}
			files = kept
		}
		if repoRawFormat(cmd) {
			return app.OK(cmd, repoUnifiedDiff(files))
		}
		// A page holds the entries that fit the budget, at least one: one file's patch
		// is cut to repoDiffPatchMax, so it always fits.
		start := min(offset, len(files))
		items := []map[string]any{}
		used := 0
		for i := start; i < len(files) && len(items) < limit; i++ {
			it := repoDiffItem(project, commit.ID, files[i])
			size := repoJSONSize(it)
			if len(items) > 0 && used+size > repoDiffPageBudget {
				break
			}
			items, used = append(items, it), used+size
		}
		end := start + len(items)
		scope := map[string]any{"project": project, "commit": commit.ID, "limit": limit}
		if pathFilter != "" {
			scope["path"] = pathFilter
		}
		if offset > 0 {
			scope["offset"] = offset
		}
		page := aicli.Page{Items: items, Count: len(items), Scope: scope}
		switch {
		case end < len(files):
			page.HasMore, page.NextOffset = true, &end
		case unread:
			page.HasMore, page.Truncated = true, true
			page.TruncatedReason = fmt.Sprintf("GitLab's diff was read up to %d files", repoDiffMaxFiles)
			page.ContinuationReason = "Files past the first 1000 are not read; pass --path to read one of them."
		}
		if !unread {
			total := len(files)
			page.Total = &total
		}
		return app.List(cmd, page)
	}}
	addRepoCommitTargetFlags(c)
	c.Flags().String("path", "", "Only this file's entry (its new or old path)")
	c.Flags().Int("limit", defaultLimit, "Files per page, 1-100 (fewer when the patches are long)")
	c.Flags().Int("offset", 0, "Files to skip: next_offset from the previous page")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show a commit's changes per file with line counts and patches",
		Effects: []aicli.Effect{aicli.Observe}, Output: "commit_diff_page", Positional: "sha", RawOutput: true,
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{repoCommitEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo commit diff 4f1c2a9 --project acme/web --fields path,change,additions,deletions",
			"gitlab-cli repo commit diff 4f1c2a9 --project acme/web --path src/app.go"}})
}
