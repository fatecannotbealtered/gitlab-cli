package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/gitctx"
)

// The merge request writes: create, update, merge, close, reopen, approve and
// unapprove. Each write to one merge request is an mrPlan, which the single command
// runs through App.Write and its bulk form (mr_bulk.go) through App.WriteBatch as
// one item, so both preview, block and verify alike.

// mrPlan is one write to one merge request: its target and its gate functions.
type mrPlan struct {
	target  aicli.Target
	preview func(context.Context) (aicli.Preview, error)
	apply   func(context.Context, aicli.Op) (aicli.Result, error)
}

func (p mrPlan) item() aicli.BatchItem {
	return aicli.BatchItem{Target: p.target, Preview: p.preview, Apply: p.apply}
}

// mrWrite runs one plan through the gate; the read-back is `mr get <ref>`.
func mrWrite(app *aicli.App, cmd *cobra.Command, sel mrSel, p mrPlan) error {
	return app.Write(cmd, aicli.WriteSpec{Target: p.target, ReadBack: "mr get " + sel.ref(), Preview: p.preview, Apply: p.apply})
}

func mrTargetOf(sel mrSel) aicli.Target {
	return aicli.Target{Key: sel.ref(), Kind: "merge_request"}
}

// mrRead is the current state of the merge request, for a Preview.
func mrRead(ctx context.Context, gl *lazyClient, sel mrSel) (*api.Client, *api.MergeRequest, error) {
	c, err := gl.get()
	if err != nil {
		return nil, nil, err
	}
	m, err := c.MergeRequests.Get(ctx, sel.project, sel.iid)
	if err != nil {
		return nil, nil, readError(err, mrNotFound(sel.project))
	}
	return c, m, nil
}

// mrApplyClient is the client inside Apply: failing to build it sent nothing.
func mrApplyClient(gl *lazyClient) (*api.Client, error) {
	c, err := gl.get()
	if err != nil {
		return nil, aicli.NotDispatched(err)
	}
	return c, nil
}

func sortChanges(ch []aicli.FieldChange) []aicli.FieldChange {
	sort.SliceStable(ch, func(i, j int) bool { return ch[i].Field < ch[j].Field })
	return ch
}

// mrText is how a description or a comment body is sent and compared: line ends as
// \n and no trailing white space, which GitLab does not keep reliably.
func mrText(s string) string {
	return strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), " \t\r\n")
}

// mrResolveUsers resolves usernames (carol, @carol, me) to IDs, and returns the
// users' refs (@carol) sorted, as the read-back reports them.
func mrResolveUsers(ctx context.Context, c *api.Client, names []string, parameter string) ([]int, []string, error) {
	ids := []int{}
	usernames := []string{}
	for _, n := range names {
		u, err := resolveUser(ctx, c, n, parameter)
		if err != nil {
			return nil, nil, err
		}
		ids = append(ids, u.ID)
		usernames = append(usernames, userRef(u.Username))
	}
	sort.Strings(usernames)
	return ids, usernames, nil
}

// mrUserList reads a user list flag: usernames, comma-separated or repeated; set is
// false when the flag was not given, and an empty list clears (CLI-029).
func mrUserList(cmd *cobra.Command, name string) (names []string, set bool) {
	if !cmd.Flags().Changed(name) {
		return nil, false
	}
	raw, _ := cmd.Flags().GetStringSlice(name)
	names = []string{}
	for _, r := range raw {
		if n := mrUser(r); n != "" {
			names = append(names, n)
		}
	}
	return names, true
}

// mrList reads a list flag (labels): trimmed, empty entries and repeats dropped.
func mrList(cmd *cobra.Command, name string) []string {
	raw, _ := cmd.Flags().GetStringSlice(name)
	var out []string
	seen := map[string]bool{}
	for _, r := range raw {
		if v := strings.TrimSpace(r); v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// mrTextInput reads a text that may come from --<name> or --<name>-file (stdin with
// -); ok is false when neither was given.
func mrTextInput(app *aicli.App, cmd *cobra.Command, name string) (text string, ok bool, err error) {
	if cmd.Flags().Changed(name + "-file") {
		b, err := app.ReadFileInput(cmd, name+"-file")
		if err != nil {
			return "", false, err
		}
		return mrText(string(b)), true, nil
	}
	if cmd.Flags().Changed(name) {
		v, _ := cmd.Flags().GetString(name)
		return mrText(v), true, nil
	}
	return "", false, nil
}

// ---------------------------------------------------------------- create

func addMRCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create", RunE: func(cmd *cobra.Command, _ []string) error {
		if auto, _ := cmd.Flags().GetBool("auto"); auto {
			if err := mrAutoFill(cmd); err != nil {
				return err
			}
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		want, err := readMRCreateFlags(app, cmd)
		if err != nil {
			return err
		}
		return mrCreate(app, cmd, gl, project, want)
	}}
	c.Flags().String("project", "", "Project path, URL or ID")
	c.Flags().String("title", "", "Title")
	c.Flags().String("source-branch", "", "Branch with the changes")
	c.Flags().String("target-branch", "", "Branch to merge into (default: the default branch)")
	c.Flags().String("description", "", "Description (Markdown)")
	c.Flags().String("description-file", "", "Description from a file, - for stdin")
	c.Flags().StringSlice("assignee", nil, "Assignees: carol, @carol or me; comma-separated")
	c.Flags().StringSlice("reviewer", nil, "Reviewers: carol, @carol or me; comma-separated")
	c.Flags().StringSlice("label", nil, "Labels, comma-separated")
	c.Flags().Bool("draft", false, "Create as a draft (Draft: title prefix)")
	c.Flags().Bool("squash", false, "Squash the commits on merge")
	c.Flags().Bool("remove-source-branch", false, "Delete the source branch on merge")
	c.Flags().Bool("auto", false, "Fill the project, branch and title from the git checkout")
	c.Flags().Bool("find-existing", false, "No effect")
	c.Flags().String("idempotency-key", "", "Repeat-safe key for this create")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create a merge request from a branch",
		Aliases: []string{"add", "new"}, Effects: []aicli.Effect{aicli.Change}, Output: "merge_request_result",
		FileInputs:      map[string]string{"description-file": "description"},
		DeprecatedFlags: map[string]string{"find-existing": "duplicates are blocked anyway"},
		ErrorCodes:      gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{mrEvidence, liveEvidence("mr")},
		Examples: []string{`gitlab-cli mr create --project acme/web --source-branch feat/x --title "Add x" --dry-run`,
			`gitlab-cli mr create --project acme/web --source-branch feat/x --title "Add x" --confirm <confirm_token>`}})
}

// mrAutoFill is --auto: the project from the checkout's GitLab remote, the source
// branch checked out, the title from the last commit, each only when not given.
// The values are bound as if given (CLI-054), so a commit made between the dry-run
// and the confirm changes the request.
func mrAutoFill(cmd *cobra.Command) error {
	g, _ := gitctx.Detect("")
	if g == nil || g.Remote == nil || g.CurrentBranch == "" {
		e := aicli.Usage("missing_parameter", "--project", "--auto found no git checkout with a GitLab remote and a branch in the working directory.")
		e.Details["next_action"] = "Run it in the project's checkout, or drop --auto and pass --project, --source-branch and --title."
		return e
	}
	fill := map[string]string{"project": g.Remote.ProjectPath, "source-branch": g.CurrentBranch}
	if !cmd.Flags().Changed("title") {
		if out, err := exec.Command("git", "log", "-1", "--pretty=%s").Output(); err == nil {
			fill["title"] = strings.TrimSpace(string(out))
		}
	}
	for _, name := range []string{"project", "source-branch", "title"} {
		if v := fill[name]; v != "" && !cmd.Flags().Changed(name) {
			if err := aicli.SetResolved(cmd, name, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// mrCreateWant is the requested merge request.
type mrCreateWant struct {
	title, source, target, description string
	hasDescription, draft              bool
	assignees, reviewers               []string
	setAssignees, setReviewers         bool
	labels                             []string
	squash, removeSource               bool
}

func readMRCreateFlags(app *aicli.App, cmd *cobra.Command) (mrCreateWant, error) {
	f := cmd.Flags()
	var w mrCreateWant
	w.title, _ = f.GetString("title")
	w.title = strings.TrimSpace(w.title)
	w.source, _ = f.GetString("source-branch")
	w.source = strings.TrimSpace(w.source)
	w.target, _ = f.GetString("target-branch")
	w.target = strings.TrimSpace(w.target)
	for _, req := range [][2]string{{"title", w.title}, {"source-branch", w.source}} {
		if flag, v := req[0], req[1]; v == "" {
			e := aicli.Usage("missing_parameter", "--"+flag, fmt.Sprintf("`mr create` needs --%s (or --auto in the project's checkout).", flag))
			e.Details["next_action"] = "Add --title and --source-branch, for example `gitlab-cli mr create --project acme/web --source-branch feature/x --title \"Add x\"`."
			return w, e
		}
	}
	if f.Changed("target-branch") && w.target == "" {
		return w, aicli.Validation("--target-branch", "a branch name", "--target-branch is empty.")
	}
	if w.target != "" && w.target == w.source {
		return w, aicli.Validation("--target-branch", "a branch other than --source-branch", "The source and target branches are the same.")
	}
	var err error
	if w.description, w.hasDescription, err = mrTextInput(app, cmd, "description"); err != nil {
		return w, err
	}
	w.assignees, w.setAssignees = mrUserList(cmd, "assignee")
	w.reviewers, w.setReviewers = mrUserList(cmd, "reviewer")
	w.labels = mrList(cmd, "label")
	w.draft, _ = f.GetBool("draft")
	w.squash, _ = f.GetBool("squash")
	w.removeSource, _ = f.GetBool("remove-source-branch")
	if w.draft && !mrIsDraftTitle(w.title) {
		w.title = "Draft: " + w.title
	}
	return w, nil
}

// mrIsDraftTitle reports a title GitLab reads as a draft.
func mrIsDraftTitle(t string) bool {
	l := strings.ToLower(strings.TrimSpace(t))
	return strings.HasPrefix(l, "draft:") || strings.HasPrefix(l, "[draft]") || strings.HasPrefix(l, "(draft)")
}

// mrDefaultBranch reads the project's default branch ("" for an empty repository).
func mrDefaultBranch(ctx context.Context, c *api.Client, project string) (string, error) {
	data, err := c.Get(ctx, c.APIPath("/projects/"+api.EncodeProjectPath(project)))
	if err != nil {
		return "", readError(err, projectNotFound())
	}
	var p struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := jsonUnmarshal(data, &p); err != nil {
		return "", fmt.Errorf("parsing project %s: unexpected response", project)
	}
	return p.DefaultBranch, nil
}

func mrCreate(app *aicli.App, cmd *cobra.Command, gl *lazyClient, project string, w mrCreateWant) error {
	target := w.target
	var assigneeIDs, reviewerIDs []int
	return app.Write(cmd, aicli.WriteSpec{
		Target:   aicli.Target{Key: project + "/-/merge_requests", Kind: "merge_requests", Label: "new merge request"},
		ReadBack: fmt.Sprintf("mr list --project %s --source-branch %s --state all", project, shellArg(w.source)),
		Preview: func(ctx context.Context) (aicli.Preview, error) {
			c, err := gl.get()
			if err != nil {
				return aicli.Preview{}, err
			}
			pv := aicli.Preview{Action: "Create a merge request in " + project, CanApply: true}
			if w.target == "" {
				if target, err = mrDefaultBranch(ctx, c, project); err != nil {
					return aicli.Preview{}, err
				}
			}
			if w.target == "" || len(w.assignees)+len(w.reviewers) > 0 {
				pv.Untrusted = []string{"changes[].after"} // a branch name or usernames GitLab returned
			}
			var assignees, reviewers []string
			if assigneeIDs, assignees, err = mrResolveUsers(ctx, c, w.assignees, "--assignee"); err != nil {
				return aicli.Preview{}, err
			}
			if reviewerIDs, reviewers, err = mrResolveUsers(ctx, c, w.reviewers, "--reviewer"); err != nil {
				return aicli.Preview{}, err
			}
			pv.Changes = mrCreateChanges(w, target, assignees, reviewers)
			var blocks []aicli.Block
			switch target {
			case "":
				blocks = append(blocks, aicli.Block{Kind: "no_target_branch", Message: "The project has no default branch; pass --target-branch."})
			case w.source:
				blocks = append(blocks, aicli.Block{Kind: "same_branch", Message: "The project's default branch is the source branch; pass another --target-branch."})
			default:
				// GitLab refuses a second open merge request between the same branches;
				// a repeat with the same --idempotency-key never gets here (P4).
				open, _, err := c.MergeRequests.ListPage(ctx, project, api.MergeRequestListOpts{State: "opened",
					SourceBranch: w.source, TargetBranch: target, Page: 1, PerPage: 1})
				if err != nil {
					return aicli.Preview{}, readError(err, projectNotFound())
				}
				if len(open) > 0 {
					blocks = append(blocks, aicli.Block{Kind: "already_exists", Message: fmt.Sprintf(
						"Merge request %s is already open between these branches; update it with `gitlab-cli mr update %s` instead.",
						mrRef(project, open[0].IID), mrRef(project, open[0].IID))})
				}
			}
			if w.hasDescription && hasQuickAction(w.description) {
				blocks = append(blocks, quickActionBlock("description"))
			}
			if len(blocks) > 0 {
				pv.CanApply, pv.BlockedBy = false, blocks
			}
			return pv, nil
		},
		Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
			c, err := mrApplyClient(gl)
			if err != nil {
				return aicli.Result{}, err
			}
			req := &api.MergeRequestCreateRequest{Title: w.title, SourceBranch: w.source, TargetBranch: target,
				Description: w.description, AssigneeIDs: assigneeIDs, ReviewerIDs: reviewerIDs, Labels: strings.Join(w.labels, ","),
				Squash: w.squash, RemoveSourceBranch: w.removeSource}
			m, err := c.MergeRequests.Create(ctx, project, req)
			if err != nil {
				return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), projectNotFound())
			}
			// The new ref is observed too, so `operation show` finds it after a lost
			// reply (CLI-059).
			obs := map[string]any{"ref": mrRef(project, m.IID), "title": m.Title, "source_branch": m.SourceBranch,
				"target_branch": m.TargetBranch}
			if w.hasDescription {
				obs["description"] = mrText(m.Description)
			}
			if w.setAssignees {
				obs["assignees"] = mrSorted(mrAssignees(*m))
			}
			if w.setReviewers {
				obs["reviewers"] = mrSorted(mrUsernames(m.Reviewers))
			}
			if len(w.labels) > 0 {
				obs["labels"] = mrStrings(m.Labels)
			}
			if w.draft {
				obs["draft"] = m.Draft
			}
			return aicli.Result{Observed: obs, Data: mrResult(project, *m), Untrusted: mrResultUntrusted}, nil
		},
	})
}

func mrCreateChanges(w mrCreateWant, target string, assignees, reviewers []string) []aicli.FieldChange {
	ch := []aicli.FieldChange{{Field: "title", After: w.title}, {Field: "source_branch", After: w.source},
		{Field: "target_branch", After: target}}
	if w.hasDescription {
		ch = append(ch, aicli.FieldChange{Field: "description", After: w.description})
	}
	if w.setAssignees {
		ch = append(ch, aicli.FieldChange{Field: "assignees", After: assignees})
	}
	if w.setReviewers {
		ch = append(ch, aicli.FieldChange{Field: "reviewers", After: reviewers})
	}
	if len(w.labels) > 0 {
		ch = append(ch, aicli.FieldChange{Field: "labels", Add: w.labels})
	}
	if w.draft {
		ch = append(ch, aicli.FieldChange{Field: "draft", After: true})
	}
	if w.squash {
		ch = append(ch, aicli.FieldChange{Field: "squash", Note: "the commits are squashed when it is merged"})
	}
	if w.removeSource {
		ch = append(ch, aicli.FieldChange{Field: "remove_source_branch", Note: "the source branch is deleted when it is merged"})
	}
	return sortChanges(ch)
}

// ---------------------------------------------------------------- update

// mrUpdateWant is the requested change of mr update and mr bulk update; nil and
// unset fields stay as they are.
type mrUpdateWant struct {
	title, description, target *string
	assignees, reviewers       []string
	setAssignees, setReviewers bool
	addLabels, removeLabels    []string
}

func readMRUpdateFlags(app *aicli.App, cmd *cobra.Command) (mrUpdateWant, error) {
	var w mrUpdateWant
	f := cmd.Flags()
	if f.Changed("title") {
		t, _ := f.GetString("title")
		if t = strings.TrimSpace(t); t == "" {
			return w, aicli.Validation("--title", "a title", "The title is empty.")
		}
		w.title = &t
	}
	if d, ok, err := mrTextInput(app, cmd, "description"); err != nil {
		return w, err
	} else if ok {
		w.description = &d
	}
	if f.Lookup("assignee") != nil {
		w.assignees, w.setAssignees = mrUserList(cmd, "assignee")
		w.reviewers, w.setReviewers = mrUserList(cmd, "reviewer")
	}
	w.addLabels, w.removeLabels = mrList(cmd, "add-labels"), mrList(cmd, "remove-labels")
	for _, a := range w.addLabels {
		for _, r := range w.removeLabels {
			if a == r {
				return w, aicli.Usage("conflicting_flags", "--remove-labels", "A label is both added and removed.")
			}
		}
	}
	if f.Changed("target-branch") {
		t, _ := f.GetString("target-branch")
		if t = strings.TrimSpace(t); t == "" {
			return w, aicli.Validation("--target-branch", "a branch name", "--target-branch is empty.")
		}
		w.target = &t
	}
	return w, nil
}

// mrUpdatePlan updates one merge request.
func mrUpdatePlan(gl *lazyClient, sel mrSel, w mrUpdateWant) mrPlan {
	var assigneeIDs, reviewerIDs []int
	return mrPlan{
		target: mrTargetOf(sel),
		preview: func(ctx context.Context) (aicli.Preview, error) {
			c, cur, err := mrRead(ctx, gl, sel)
			if err != nil {
				return aicli.Preview{}, err
			}
			var ch []aicli.FieldChange
			if w.title != nil {
				ch = append(ch, aicli.FieldChange{Field: "title", Before: cur.Title, After: *w.title})
			}
			if w.description != nil {
				ch = append(ch, aicli.FieldChange{Field: "description", Before: mrText(cur.Description), After: *w.description})
			}
			if w.setAssignees {
				var names []string
				if assigneeIDs, names, err = mrResolveUsers(ctx, c, w.assignees, "--assignee"); err != nil {
					return aicli.Preview{}, err
				}
				ch = append(ch, aicli.FieldChange{Field: "assignees", Before: mrSorted(mrAssignees(*cur)), After: names})
			}
			if w.setReviewers {
				var names []string
				if reviewerIDs, names, err = mrResolveUsers(ctx, c, w.reviewers, "--reviewer"); err != nil {
					return aicli.Preview{}, err
				}
				ch = append(ch, aicli.FieldChange{Field: "reviewers", Before: mrSorted(mrUsernames(cur.Reviewers)), After: names})
			}
			if len(w.addLabels) > 0 || len(w.removeLabels) > 0 {
				lc := aicli.FieldChange{Field: "labels", Before: mrStrings(cur.Labels)}
				if len(w.addLabels) > 0 {
					lc.Add = w.addLabels
				}
				if len(w.removeLabels) > 0 {
					lc.Remove = w.removeLabels
				}
				ch = append(ch, lc)
			}
			if w.target != nil {
				ch = append(ch, aicli.FieldChange{Field: "target_branch", Before: cur.TargetBranch, After: *w.target})
			}
			pv := aicli.Preview{Action: "Update the merge request", Changes: sortChanges(ch), CanApply: true,
				BaseVersion: cur.UpdatedAt, BaseSource: "updated_at", Untrusted: []string{"changes[].before"}}
			if w.setAssignees || w.setReviewers {
				pv.Untrusted = append(pv.Untrusted, "changes[].after") // usernames GitLab returned
			}
			var blocks []aicli.Block
			if w.target != nil && cur.State == "merged" {
				blocks = append(blocks, aicli.Block{Kind: "merged", Message: "The merge request is merged; its target branch can no longer change."})
			}
			if w.description != nil && hasQuickAction(*w.description) {
				blocks = append(blocks, quickActionBlock("description"))
			}
			if len(blocks) > 0 {
				pv.CanApply, pv.BlockedBy = false, blocks
			}
			return pv, nil
		},
		apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
			c, err := mrApplyClient(gl)
			if err != nil {
				return aicli.Result{}, err
			}
			opts := api.MergeRequestUpdateOpts{Title: w.title, Description: w.description, TargetBranch: w.target,
				AddLabels: strings.Join(w.addLabels, ","), RemoveLabels: strings.Join(w.removeLabels, ",")}
			if w.setAssignees {
				opts.AssigneeIDs = &assigneeIDs
			}
			if w.setReviewers {
				opts.ReviewerIDs = &reviewerIDs
			}
			m, err := c.MergeRequests.Update(ctx, sel.project, sel.iid, opts)
			if err != nil {
				return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), mrNotFound(sel.project))
			}
			obs := map[string]any{}
			if w.title != nil {
				obs["title"] = m.Title
			}
			if w.description != nil {
				obs["description"] = mrText(m.Description)
			}
			if w.setAssignees {
				obs["assignees"] = mrSorted(mrAssignees(*m))
			}
			if w.setReviewers {
				obs["reviewers"] = mrSorted(mrUsernames(m.Reviewers))
			}
			if len(w.addLabels) > 0 || len(w.removeLabels) > 0 {
				obs["labels"] = mrStrings(m.Labels)
			}
			if w.target != nil {
				obs["target_branch"] = m.TargetBranch
			}
			return aicli.Result{Observed: obs, Data: mrResult(sel.project, *m), Untrusted: mrResultUntrusted}, nil
		},
	}
}

// addMRUpdateFields declares the fields mr update and mr bulk update change.
func addMRUpdateFields(c *cobra.Command, users bool) {
	c.Flags().String("title", "", "New title")
	c.Flags().String("description", "", "New description (Markdown; empty clears it)")
	c.Flags().String("description-file", "", "New description from a file, - for stdin")
	if users {
		c.Flags().StringSlice("assignee", nil, "New assignees: carol, @carol or me; empty unassigns all")
		c.Flags().StringSlice("reviewer", nil, "New reviewers: carol, @carol or me; empty removes all")
	}
	c.Flags().StringSlice("add-labels", nil, "Labels to add, comma-separated")
	c.Flags().StringSlice("remove-labels", nil, "Labels to remove, comma-separated")
	c.Flags().String("target-branch", "", "New target branch")
}

func addMRUpdate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "update <mr>", RunE: func(cmd *cobra.Command, _ []string) error {
		w, err := readMRUpdateFlags(app, cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, _, err := mrTarget(cmd, gl)
		if err != nil {
			return err
		}
		return mrWrite(app, cmd, sel, mrUpdatePlan(gl, sel, w))
	}}
	addMRTargetFlags(c)
	addMRUpdateFields(c, true)
	// An update with nothing to change is a usage error before the gate (CLI-029).
	c.MarkFlagsOneRequired("title", "description", "description-file", "assignee", "reviewer", "add-labels", "remove-labels", "target-branch")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Change a merge request's title, description, assignees, reviewers, labels or target branch",
		Aliases: []string{"edit"}, Effects: []aicli.Effect{aicli.Change}, Output: "merge_request_result", Positional: "mr",
		FileInputs: map[string]string{"description-file": "description"},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{mrEvidence, liveEvidence("mr")},
		Examples: []string{"gitlab-cli mr update acme/web!5 --assignee carol --add-labels needs-review --dry-run",
			"gitlab-cli mr update acme/web!5 --assignee carol --add-labels needs-review --confirm <confirm_token>"}})
}

// ---------------------------------------------------------------- merge

// mrMergeWant is the requested merge.
type mrMergeWant struct {
	squash, removeSource *bool
	message, sha         string
}

func readMRMergeFlags(cmd *cobra.Command) mrMergeWant {
	var w mrMergeWant
	f := cmd.Flags()
	if f.Changed("squash") {
		v, _ := f.GetBool("squash")
		w.squash = &v
	}
	if f.Changed("should-remove-source-branch") {
		v, _ := f.GetBool("should-remove-source-branch")
		w.removeSource = &v
	}
	if f.Lookup("merge-commit-message") != nil {
		w.message, _ = f.GetString("merge-commit-message")
	}
	if f.Lookup("sha") != nil {
		w.sha, _ = f.GetString("sha")
		w.sha = strings.TrimSpace(w.sha)
	}
	return w
}

// mrMergeBlocks says why GitLab would refuse the merge now. Statuses GitLab is
// still computing (unchecked, checking, preparing) do not block: GitLab decides.
func mrMergeBlocks(cur *api.MergeRequest, w mrMergeWant) []aicli.Block {
	var b []aicli.Block
	switch cur.State {
	case "opened":
	case "merged":
		return []aicli.Block{{Kind: "merged", Message: "The merge request is already merged."}}
	case "closed":
		return []aicli.Block{{Kind: "closed", Message: "The merge request is closed; reopen it first (mr reopen)."}}
	default:
		return []aicli.Block{{Kind: "not_open", Message: "The merge request is not open, so it cannot be merged."}}
	}
	if cur.Draft {
		b = append(b, aicli.Block{Kind: "draft", Message: "The merge request is a draft; drop the Draft: prefix from its title first (mr update --title)."})
	}
	if cur.HasConflicts {
		b = append(b, aicli.Block{Kind: "conflict", Message: "The merge request has conflicts with its target branch; they must be resolved first."})
	}
	if w.sha != "" && w.sha != cur.SHA {
		b = append(b, aicli.Block{Kind: "sha_mismatch", Message: "The source branch's head is no longer the --sha given; review the new commits first."})
	}
	reasons := map[string]string{
		"not_approved":             "The merge request still needs approvals.",
		"discussions_not_resolved": "Unresolved discussions block the merge.",
		"ci_must_pass":             "The project requires a successful pipeline first.",
		"ci_still_running":         "The pipeline is still running.",
		"need_rebase":              "The source branch must be rebased onto the target branch first.",
		"blocked_status":           "Another merge request blocks this one.",
		"requested_changes":        "A reviewer requested changes.",
		"merge_request_blocked":    "GitLab blocks this merge request.",
		"external_status_checks":   "External status checks have not passed.",
		"jira_association_missing": "The title or description must name a Jira issue.",
	}
	if msg, ok := reasons[cur.DetailedMergeStatus]; ok {
		b = append(b, aicli.Block{Kind: cur.DetailedMergeStatus, Message: msg})
	}
	return b
}

// mrMergePlan merges one merge request. The base version is the source branch's
// head: new commits pushed after the dry-run are state_drift (CLI-057), and the
// head SHA the preview saw is sent, so GitLab refuses a merge of anything else.
func mrMergePlan(gl *lazyClient, sel mrSel, w mrMergeWant) mrPlan {
	var sha string
	return mrPlan{
		target: mrTargetOf(sel),
		preview: func(ctx context.Context) (aicli.Preview, error) {
			_, cur, err := mrRead(ctx, gl, sel)
			if err != nil {
				return aicli.Preview{}, err
			}
			sha = cur.SHA
			ch := []aicli.FieldChange{{Field: "state", Before: cur.State, After: "merged",
				Note: "merged into its target branch; cannot be undone"}}
			if w.squash != nil && *w.squash {
				ch = append(ch, aicli.FieldChange{Field: "squash", Note: "the commits are squashed into one"})
			}
			if w.removeSource != nil && *w.removeSource {
				ch = append(ch, aicli.FieldChange{Field: "source_branch", Note: "deleted after the merge"})
			}
			pv := aicli.Preview{Action: "Merge the merge request into its target branch", Changes: sortChanges(ch), CanApply: true,
				BaseVersion: cur.SHA, BaseSource: "sha"}
			if b := mrMergeBlocks(cur, w); len(b) > 0 {
				pv.CanApply, pv.BlockedBy = false, b
			}
			return pv, nil
		},
		apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
			c, err := mrApplyClient(gl)
			if err != nil {
				return aicli.Result{}, err
			}
			m, err := c.MergeRequests.Merge(ctx, sel.project, sel.iid, api.MergeRequestMergeOpts{SHA: sha, Squash: w.squash,
				ShouldRemoveSourceBranch: w.removeSource, MergeCommitMessage: w.message})
			if err != nil {
				return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), mrNotFound(sel.project))
			}
			return aicli.Result{Observed: map[string]any{"state": m.State}, Data: mrResult(sel.project, *m), Untrusted: mrResultUntrusted}, nil
		},
	}
}

func addMRMerge(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "merge <mr>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, _, err := mrTarget(cmd, gl)
		if err != nil {
			return err
		}
		return mrWrite(app, cmd, sel, mrMergePlan(gl, sel, readMRMergeFlags(cmd)))
	}}
	addMRTargetFlags(c)
	addMRMergeFlags(c)
	c.Flags().String("merge-commit-message", "", "Merge commit message")
	c.Flags().String("sha", "", "Merge only if this is still the source branch's head commit")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Merge a merge request into its target branch (cannot be undone)",
		Effects: []aicli.Effect{aicli.Change}, Dangerous: true, Output: "merge_request_result", Positional: "mr",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{mrEvidence, liveEvidence("mr")},
		Examples: []string{"gitlab-cli mr merge acme/web!5 --dangerous --dry-run",
			"gitlab-cli mr merge acme/web!5 --dangerous --confirm <confirm_token>"}})
}

func addMRMergeFlags(c *cobra.Command) {
	c.Flags().Bool("squash", false, "Squash the commits into one")
	c.Flags().Bool("should-remove-source-branch", false, "Delete the source branch after the merge")
}

// ---------------------------------------------------------------- close, reopen

// mrStatePlan closes (event "close") or reopens (event "reopen") one merge request.
func mrStatePlan(gl *lazyClient, sel mrSel, event string) mrPlan {
	after := map[string]string{"close": "closed", "reopen": "opened"}[event]
	return mrPlan{
		target: mrTargetOf(sel),
		preview: func(ctx context.Context) (aicli.Preview, error) {
			_, cur, err := mrRead(ctx, gl, sel)
			if err != nil {
				return aicli.Preview{}, err
			}
			verb := map[string]string{"close": "Close", "reopen": "Reopen"}[event]
			pv := aicli.Preview{Action: verb + " the merge request", CanApply: true,
				Changes:     []aicli.FieldChange{{Field: "state", Before: cur.State, After: after}},
				BaseVersion: cur.UpdatedAt, BaseSource: "updated_at"}
			var block *aicli.Block
			switch {
			case cur.State == "merged":
				block = &aicli.Block{Kind: "merged", Message: "The merge request is merged, so it can be neither closed nor reopened."}
			case event == "close" && cur.State == "closed":
				block = &aicli.Block{Kind: "already_closed", Message: "The merge request is already closed."}
			case event == "reopen" && cur.State == "opened":
				block = &aicli.Block{Kind: "already_open", Message: "The merge request is already open."}
			case cur.State != "opened" && cur.State != "closed":
				block = &aicli.Block{Kind: "locked", Message: "The merge request is locked."}
			}
			if block != nil {
				pv.CanApply, pv.BlockedBy = false, []aicli.Block{*block}
			}
			return pv, nil
		},
		apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
			c, err := mrApplyClient(gl)
			if err != nil {
				return aicli.Result{}, err
			}
			m, err := c.MergeRequests.Update(ctx, sel.project, sel.iid, api.MergeRequestUpdateOpts{StateEvent: &event})
			if err != nil {
				return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), mrNotFound(sel.project))
			}
			return aicli.Result{Observed: map[string]any{"state": m.State}, Data: mrResult(sel.project, *m), Untrusted: mrResultUntrusted}, nil
		},
	}
}

func addMRClose(app *aicli.App, parent *cobra.Command) {
	addMRStateCommand(app, parent, "close", "Close a merge request without merging it (mr reopen undoes it)")
}

func addMRReopen(app *aicli.App, parent *cobra.Command) {
	addMRStateCommand(app, parent, "reopen", "Reopen a closed merge request")
}

func addMRStateCommand(app *aicli.App, parent *cobra.Command, event, summary string) {
	c := &cobra.Command{Use: event + " <mr>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, _, err := mrTarget(cmd, gl)
		if err != nil {
			return err
		}
		return mrWrite(app, cmd, sel, mrStatePlan(gl, sel, event))
	}}
	addMRTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: summary, Effects: []aicli.Effect{aicli.Change}, Output: "merge_request_result", Positional: "mr",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{mrEvidence},
		Examples: []string{fmt.Sprintf("gitlab-cli mr %s acme/web!5 --dry-run", event),
			fmt.Sprintf("gitlab-cli mr %s acme/web!5 --confirm <confirm_token>", event)}})
}

// ---------------------------------------------------------------- approve, unapprove

// mrApprovalOut is an MR's approval state as approve and unapprove return it.
func mrApprovalOut(sel mrSel, ap *api.MergeRequestApprovals) map[string]any {
	return map[string]any{"ref": sel.ref(), "approved": ap.Approved, "approved_by": mrUserRefs(ap.Usernames()), "approvals_left": ap.ApprovalsLeft}
}

// mrApprovePlan approves (approve true) or unapproves one merge request as the
// token's user. An approval is bound to the head commit the preview saw: GitLab
// refuses an approval sent for another SHA.
func mrApprovePlan(gl *lazyClient, sel mrSel, approve bool) mrPlan {
	var sha string
	return mrPlan{
		target: mrTargetOf(sel),
		preview: func(ctx context.Context) (aicli.Preview, error) {
			c, cur, err := mrRead(ctx, gl, sel)
			if err != nil {
				return aicli.Preview{}, err
			}
			me, err := c.Users.Me(ctx)
			if err != nil {
				return aicli.Preview{}, readError(err)
			}
			ap, err := c.MergeRequests.Approvals(ctx, sel.project, sel.iid)
			if err != nil {
				return aicli.Preview{}, readError(err, mrNotFound(sel.project))
			}
			approved := hasStringIn(ap.Usernames(), me.Username)
			sha = cur.SHA
			// The username is GitLab's answer to GET /user.
			pv := aicli.Preview{CanApply: true, Untrusted: []string{"changes[].add", "changes[].remove"}}
			var block *aicli.Block
			if approve {
				pv.Action = "Approve the merge request as this account"
				pv.Changes = []aicli.FieldChange{{Field: "approved_by", Add: []string{userRef(me.Username)}}}
				pv.BaseVersion, pv.BaseSource = cur.SHA, "sha"
				switch {
				case cur.State != "opened":
					block = &aicli.Block{Kind: "not_open", Message: "Only an open merge request can be approved."}
				case approved:
					block = &aicli.Block{Kind: "already_approved", Message: "This account has already approved the merge request."}
				}
			} else {
				pv.Action = "Withdraw this account's approval of the merge request"
				pv.Changes = []aicli.FieldChange{{Field: "approved_by", Remove: []string{userRef(me.Username)}}}
				if !approved {
					block = &aicli.Block{Kind: "not_approved", Message: "This account has not approved the merge request."}
				}
			}
			if block != nil {
				pv.CanApply, pv.BlockedBy = false, []aicli.Block{*block}
			}
			return pv, nil
		},
		apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
			c, err := mrApplyClient(gl)
			if err != nil {
				return aicli.Result{}, err
			}
			var ap *api.MergeRequestApprovals
			if approve {
				if ap, err = c.MergeRequests.Approve(ctx, sel.project, sel.iid, sha); err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), mrNotFound(sel.project))
				}
			} else {
				if err := c.MergeRequests.Unapprove(ctx, sel.project, sel.iid); err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), mrNotFound(sel.project))
				}
				// GitLab's answer to unapprove has no approval state: read it back.
				if ap, err = c.MergeRequests.Approvals(ctx, sel.project, sel.iid); err != nil {
					return aicli.Result{Data: map[string]any{"ref": sel.ref()}}, nil
				}
			}
			return aicli.Result{Observed: map[string]any{"approved_by": mrUserRefs(ap.Usernames())}, Data: mrApprovalOut(sel, ap),
				Untrusted: []string{"approved_by"}}, nil
		},
	}
}

func addMRApprove(app *aicli.App, parent *cobra.Command) {
	addMRApprovalCommand(app, parent, true)
}

func addMRUnapprove(app *aicli.App, parent *cobra.Command) {
	addMRApprovalCommand(app, parent, false)
}

func addMRApprovalCommand(app *aicli.App, parent *cobra.Command, approve bool) {
	verb, summary := "approve", "Approve a merge request as this account, for the head commit the preview shows"
	if !approve {
		verb, summary = "unapprove", "Withdraw this account's approval of a merge request"
	}
	c := &cobra.Command{Use: verb + " <mr>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, _, err := mrTarget(cmd, gl)
		if err != nil {
			return err
		}
		return mrWrite(app, cmd, sel, mrApprovePlan(gl, sel, approve))
	}}
	addMRTargetFlags(c)
	parent.AddCommand(c)
	maturity, evidence := aicli.Experimental, []string{mrEvidence}
	if approve { // approve ran live; unapprove did not
		maturity, evidence = aicli.Verified, append(evidence, liveEvidence("mr"))
	}
	app.Register(c, aicli.Meta{Summary: summary, Effects: []aicli.Effect{aicli.Change}, Output: "merge_request_approval", Positional: "mr",
		ErrorCodes: gitlabWriteCodes, Maturity: maturity, Evidence: evidence,
		Examples: []string{fmt.Sprintf("gitlab-cli mr %s acme/web!5 --dry-run", verb),
			fmt.Sprintf("gitlab-cli mr %s acme/web!5 --confirm <confirm_token>", verb)}})
}

// mrUserRefs turns usernames into users' refs (@carol).
func mrUserRefs(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, userRef(n))
	}
	return out
}
