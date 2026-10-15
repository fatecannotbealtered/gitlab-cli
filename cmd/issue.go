package cmd

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The issue group: list, get, create, update, close, reopen, assign and label one
// issue; the same changes for many issues under `issue bulk` (issue_bulk.go); and
// an issue's comments under `issue comment` (issue_comment.go). An issue is named
// by its ref (acme/web#12), its web URL, or its iid with --project; every form
// binds as the same iid and project (CLI-048, CLI-054), and the ref is the write's
// target key.

const issueEvidence = "simulated:cmd/issue_test.go"

var (
	issueFields = []string{"ref", "id", "iid", "title", "description", "state", "author", "assignees", "labels",
		"milestone", "confidential", "due_date", "user_notes_count", "web_url", "created_at", "updated_at", "closed_at"}
	// What agents ask of issue lists: which ones (ref, title, web_url), open or
	// closed, who opened them, who has them, how they are labelled, how fresh.
	issueDefaultFields = []string{"ref", "title", "state", "author", "assignees", "labels", "updated_at", "web_url"}
	// Text people wrote: titles, descriptions, label names and milestone titles.
	issueUntrusted = []string{"title", "description", "labels", "milestone"}
	// A write's result leaves the description out (issueResult).
	issueResultUntrusted = []string{"title", "labels", "milestone"}
	// GitLab usernames: letters, digits, _, - and . (a leading @ is dropped first).
	issueUsernameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
)

func registerIssue(app *aicli.App) {
	group := addGroup(app, "issue", "List, read, create and change issues, one at a time or in bulk, and their comments", "issues")
	app.Schema("issue", aicli.SchemaDef{Shape: "object", Fields: issueFields, Untrusted: issueUntrusted})
	app.Schema("issue_page", aicli.SchemaDef{Shape: "page", Fields: issueFields, DefaultFields: issueDefaultFields,
		Untrusted: issueUntrusted})
	addIssueList(app, group)
	addIssueGet(app, group)
	addIssueCreate(app, group)
	addIssueUpdate(app, group)
	addIssueClose(app, group)
	addIssueReopen(app, group)
	addIssueAssign(app, group)
	addIssueLabel(app, group)
	registerIssueComment(app, group)
	registerIssueBulk(app, group)
	// R02 wants a root shortcut for every nested group (CLI-022). comment and bulk
	// are nested under merge requests too, so the root words carry the group
	// (App.AddShortcut; docs/migration-2.0/notes.md, "Integration decisions").
	for _, w := range []string{"issue-comment", "issue-comments"} {
		app.AddShortcut(w, "issue", "comment")
	}
	app.AddShortcut("issue-bulk", "issue", "bulk")
}

// ---------------------------------------------------------------- output

// issueOut is an issue as issue list and issue get return it.
func issueOut(project string, iss api.Issue) map[string]any {
	return map[string]any{
		"ref": issueRef(project, iss.IID), "id": idString(iss.ID), "iid": idString(iss.IID), "title": iss.Title,
		"description": iss.Description, "state": iss.State, "author": issueUser(iss.Author),
		"assignees": issueAssignees(iss.Assignees), "labels": issueLabels(iss.Labels),
		"milestone": issueMilestoneTitle(iss.Milestone), "confidential": iss.Confidential, "due_date": optString(iss.DueDate),
		"user_notes_count": iss.UserNotesCount, "web_url": iss.WebURL, "created_at": utcTime(iss.CreatedAt),
		"updated_at": utcTime(iss.UpdatedAt), "closed_at": utcTime(iss.ClosedAt),
	}
}

// issueResult is the result of a write on one issue: the issue without its
// description, which can be long and which the write's receipt already reports
// when the write changed it (CLI-042, CLI-059).
func issueResult(project string, iss api.Issue) map[string]any {
	out := issueOut(project, iss)
	delete(out, "description")
	return out
}

// issueBrief is the per-item result of a bulk write: enough to tell the issues
// apart and see their new state, small enough for a batch of many (CLI-063).
func issueBrief(project string, iss api.Issue) map[string]any {
	return map[string]any{"ref": issueRef(project, iss.IID), "title": iss.Title, "state": iss.State,
		"assignees": issueAssignees(iss.Assignees), "labels": issueLabels(iss.Labels), "updated_at": utcTime(iss.UpdatedAt)}
}

// issueUser is a user as issues name them: the user's ref, @username (CLI-048).
func issueUser(u *api.User) any {
	if u == nil || u.Username == "" {
		return nil
	}
	return userRef(u.Username)
}

// issueAssignees is the sorted list of the assignees' refs ([] for none).
func issueAssignees(users []api.User) []string {
	out := []string{}
	for _, u := range users {
		if u.Username != "" {
			out = append(out, userRef(u.Username))
		}
	}
	sort.Strings(out)
	return out
}

// issueLabels is the sorted list of label names ([] for none).
func issueLabels(labels []string) []string {
	out := append([]string{}, labels...)
	sort.Strings(out)
	return out
}

func issueMilestoneTitle(m *api.IssueMilestone) any {
	if m == nil {
		return nil
	}
	return m.Title
}

// ---------------------------------------------------------------- input

// parseIssueArg reads an issue as an agent has it: its iid ("12", "#12"), its ref
// ("acme/web#12"), or its web URL (…/acme/web/-/issues/12, or /-/work_items/12 as
// newer GitLab versions show it). project is "" when the input names none.
func parseIssueArg(raw string) (project string, iid int, err error) {
	if strings.Contains(raw, "/-/work_items/") {
		return parseIIDRef(raw, "#", "/-/work_items/")
	}
	return parseIIDRef(raw, "#", "/-/issues/")
}

// issueSel is the issue a command acts on.
type issueSel struct {
	project string
	iid     int
}

func (s issueSel) ref() string { return issueRef(s.project, s.iid) }

// issueTarget reads the issue positional (or --issue) and the project and binds
// the iid and the project path (CLI-054): 12 with --project acme/web, #12,
// acme/web#12 and the web URL are one request. The iid is bound alone, not as the
// ref: a ref would bind the project twice, and G03 expects a changed --project to
// be named --project (docs/migration-2.0/findings.md, ISSUE-G2). Nothing is read from GitLab unless
// --project is a numeric ID.
func issueTarget(cmd *cobra.Command, gl *lazyClient) (issueSel, error) {
	raw, _ := cmd.Flags().GetString("issue")
	if strings.TrimSpace(raw) == "" {
		e := aicli.Usage("missing_parameter", "--issue", fmt.Sprintf("`%s` needs the issue: its number, ref or web URL.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Pass the issue as the argument, for example `%s acme/web#12`.", cmd.CommandPath())
		return issueSel{}, e
	}
	refProject, iid, err := parseIssueArg(raw)
	if err != nil {
		e := aicli.Validation("--issue", "an issue number (12), ref (acme/web#12) or web URL", "That is not an issue number, ref or web URL.")
		e.Details["next_action"] = "Run `gitlab-cli issue list --project <group/project>` to find the issue."
		return issueSel{}, e
	}
	project, err := projectFlag(cmd, refProject, gl)
	if err != nil {
		return issueSel{}, err
	}
	if err := aicli.SetResolved(cmd, "issue", strconv.Itoa(iid)); err != nil {
		return issueSel{}, err
	}
	return issueSel{project: project, iid: iid}, nil
}

// addIssueTargetFlags declares the flags that name one issue.
func addIssueTargetFlags(c *cobra.Command) {
	c.Flags().String("issue", "", "Issue number (12), ref (acme/web#12) or web URL")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the issue is a ref or URL")
}

// issueNotFound is the 404 of an issue read.
func issueNotFound(project string) errOpt {
	return onNotFound("The project has no issue with that number, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli issue list --project %s --state all` to find the issue.", project))
}

// issueUsernames reads a user-list flag (--assignee): "carol", "@carol" and "me",
// comma-separated or repeated. Names are bound without the @ (CLI-054). given is
// false when the flag was not used; an empty value gives no names (a request to
// clear).
func issueUsernames(cmd *cobra.Command, flag string) (names []string, given bool, err error) {
	f := cmd.Flags().Lookup(flag)
	if f == nil || !f.Changed {
		return nil, false, nil
	}
	raw, _ := cmd.Flags().GetStringSlice(flag)
	seen := map[string]bool{}
	for _, v := range raw {
		name := strings.TrimPrefix(strings.TrimSpace(v), "@")
		switch {
		case name == "":
			continue
		case strings.EqualFold(name, "me"):
			name = "me"
		case !issueUsernameRE.MatchString(name):
			return nil, true, aicli.Validation("--"+flag, "GitLab usernames (carol, @carol) or me",
				fmt.Sprintf("--%s takes usernames, not display names.", flag))
		}
		if !seen[strings.ToLower(name)] {
			seen[strings.ToLower(name)] = true
			names = append(names, name)
		}
	}
	if len(names) > 0 {
		if err := aicli.SetResolved(cmd, flag, names...); err != nil {
			return nil, true, err
		}
	}
	return names, true, nil
}

// issueLabelNames reads a label-list flag: names as GitLab shows them ("bug"), with
// GitLab's ~ ("~bug"), or label refs ("acme/web/-/labels/bug"), comma-separated or
// repeated. A label ref of another project is refused. The names are bound as names
// (CLI-054). An empty list is refused: a flag given without a label is a mistake.
func issueLabelNames(cmd *cobra.Command, flag, project string) ([]string, error) {
	if f := cmd.Flags().Lookup(flag); f == nil || !f.Changed {
		return nil, nil
	}
	raw, _ := cmd.Flags().GetStringSlice(flag)
	var out []string
	for _, v := range raw {
		if strings.TrimSpace(v) == "" {
			continue
		}
		p, name, err := parseLabelArg(v)
		if err != nil {
			return nil, aicli.Validation("--"+flag, "label names (bug), ~bug or label refs (acme/web/-/labels/bug)",
				fmt.Sprintf("A --%s value is not a label name or ref.", flag))
		}
		if p != "" && !strings.EqualFold(p, project) {
			e := aicli.Usage("conflicting_flags", "--"+flag, fmt.Sprintf("A --%s label ref names another project than the issue's.", flag))
			e.Details["next_action"] = "Pass the label by its name; labels of the project's groups apply too."
			return nil, e
		}
		if !issueHas(out, name) {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, aicli.Validation("--"+flag, "at least one label name", fmt.Sprintf("--%s names no label.", flag))
	}
	if err := aicli.SetResolved(cmd, flag, out...); err != nil {
		return nil, err
	}
	return out, nil
}

// issueMilestoneArg is a requested milestone: --milestone (an iid, ref, URL or
// title, as milestone get takes it; an empty value removes the milestone) or the
// deprecated --milestone-id (the global ID, as 1.x took it).
type issueMilestoneArg struct {
	given, clear bool
	id, iid      int
	title        string
}

// readIssueMilestone reads --milestone or --milestone-id and binds an iid as the
// milestone's ref, so 2, %2 and the ref are one request. A title is bound as given:
// it is resolved in Preview, with the issue's other inputs.
func readIssueMilestone(cmd *cobra.Command, project string) (issueMilestoneArg, error) {
	var m issueMilestoneArg
	if f := cmd.Flags().Lookup("milestone-id"); f != nil && f.Changed {
		raw, _ := cmd.Flags().GetString("milestone-id")
		id, _ := strconv.Atoi(strings.TrimSpace(raw))
		if id <= 0 {
			return m, aicli.Validation("--milestone-id", "a positive milestone ID", "--milestone-id must be a positive number.")
		}
		return issueMilestoneArg{given: true, id: id}, nil
	}
	f := cmd.Flags().Lookup("milestone")
	if f == nil || !f.Changed {
		return m, nil
	}
	raw := strings.TrimSpace(f.Value.String())
	if raw == "" {
		return issueMilestoneArg{given: true, clear: true}, nil
	}
	p, iid, title, err := parseMilestoneArg(raw)
	if err != nil {
		return m, aicli.Validation("--milestone", "a milestone iid (3), ref (acme/web/-/milestones/3) or title (v1.0)",
			"That is not a milestone iid, ref or title.")
	}
	if p != "" && !strings.EqualFold(p, project) {
		return m, aicli.Usage("conflicting_flags", "--milestone", "The milestone ref names another project than the issue's.")
	}
	if iid > 0 {
		if err := aicli.SetResolved(cmd, "milestone", milestoneRef(project, iid)); err != nil {
			return m, err
		}
	}
	return issueMilestoneArg{given: true, iid: iid, title: title}, nil
}

// readIssueText reads a text flag and its --<flag>-file twin (Meta.FileInputs). A
// file's trailing line breaks are dropped. given is false when neither was used.
func readIssueText(app *aicli.App, cmd *cobra.Command, flag string) (text string, given bool, err error) {
	if f := cmd.Flags().Lookup(flag + "-file"); f != nil && f.Changed {
		b, err := app.ReadFileInput(cmd, flag+"-file")
		if err != nil {
			return "", true, err
		}
		return strings.TrimRight(string(b), "\r\n"), true, nil
	}
	if f := cmd.Flags().Lookup(flag); f != nil && f.Changed {
		return f.Value.String(), true, nil
	}
	return "", false, nil
}

// ---------------------------------------------------------------- planned changes

// issueChange is a requested change of an issue; nil and empty fields stay as they
// are (CLI-029).
type issueChange struct {
	title, description      *string
	assignees               []string // usernames or "me"
	setAssignees            bool     // --assignee given; no names unassigns everyone
	addLabels, removeLabels []string
	milestone               issueMilestoneArg
	confidential            *bool  // create only
	stateEvent              string // close, reopen
}

// readIssueChange reads the change flags a command declares.
func readIssueChange(app *aicli.App, cmd *cobra.Command, project string) (issueChange, error) {
	var w issueChange
	if f := cmd.Flags().Lookup("title"); f != nil && f.Changed {
		t := strings.TrimSpace(f.Value.String())
		if t == "" {
			return w, aicli.Validation("--title", "a title", "The title is empty.")
		}
		w.title = &t
	}
	if d, given, err := readIssueText(app, cmd, "description"); err != nil {
		return w, err
	} else if given {
		w.description = &d
	}
	var err error
	if w.assignees, w.setAssignees, err = issueUsernames(cmd, "assignee"); err != nil {
		return w, err
	}
	// A command declares one pair: --add-labels/--remove-labels (update), --add/--remove
	// (label), or --label (create).
	removeFlag := ""
	for _, lf := range []struct {
		flag string
		dst  *[]string
	}{{"add-labels", &w.addLabels}, {"add", &w.addLabels}, {"label", &w.addLabels},
		{"remove-labels", &w.removeLabels}, {"remove", &w.removeLabels}} {
		names, err := issueLabelNames(cmd, lf.flag, project)
		if err != nil {
			return w, err
		}
		if names != nil {
			*lf.dst = names
			if lf.dst == &w.removeLabels {
				removeFlag = "--" + lf.flag
			}
		}
	}
	for _, l := range w.addLabels {
		if issueHas(w.removeLabels, l) {
			e := aicli.Usage("conflicting_flags", removeFlag, "A label cannot be added and removed in one call.")
			e.Details["next_action"] = "Name each label once: to add it or to remove it."
			return w, e
		}
	}
	if w.milestone, err = readIssueMilestone(cmd, project); err != nil {
		return w, err
	}
	if f := cmd.Flags().Lookup("confidential"); f != nil && f.Changed {
		v, _ := cmd.Flags().GetBool("confidential")
		w.confidential = &v
	}
	return w, nil
}

// issuePlan is what a Preview resolved from an issueChange: the users and the
// milestone that will be sent.
type issuePlan struct {
	assigneeIDs   []int
	assigneeNames []string // @username, sorted
	milestoneID   int
	milestone     any // title, or nil when the milestone is removed
}

// issueResolver resolves usernames and the milestone inside Preview, once per run
// (a bulk preview asks for every item). The confirm is a new run, so it reads again.
type issueResolver struct {
	gl      *lazyClient
	project string
	plan    *issuePlan
	err     error
}

func newIssueResolver(gl *lazyClient, project string) *issueResolver {
	return &issueResolver{gl: gl, project: project}
}

// resolve turns the change's users and milestone into IDs (E_NOT_FOUND names the
// flag that has no match).
func (r *issueResolver) resolve(ctx context.Context, want issueChange) (issuePlan, error) {
	if r.plan != nil || r.err != nil {
		if r.err != nil {
			return issuePlan{}, r.err
		}
		return *r.plan, nil
	}
	p, err := r.resolveOnce(ctx, want)
	if err != nil {
		r.err = err
		return issuePlan{}, err
	}
	r.plan = &p
	return p, nil
}

func (r *issueResolver) resolveOnce(ctx context.Context, want issueChange) (issuePlan, error) {
	var p issuePlan
	if !want.setAssignees && !want.milestone.given {
		return p, nil
	}
	client, err := r.gl.get()
	if err != nil {
		return p, err
	}
	if want.setAssignees {
		p.assigneeIDs, p.assigneeNames = []int{}, []string{}
		for _, name := range want.assignees {
			u, err := resolveUser(ctx, client, name, "--assignee")
			if err != nil {
				return p, err
			}
			p.assigneeIDs = append(p.assigneeIDs, u.ID)
			p.assigneeNames = append(p.assigneeNames, userRef(u.Username))
		}
		sort.Strings(p.assigneeNames)
	}
	if m := want.milestone; m.given {
		switch {
		case m.clear:
		case m.id > 0:
			ms, err := client.Milestones.GetByID(ctx, r.project, m.id)
			if err != nil {
				return p, readError(err, milestoneNotFound(r.project))
			}
			p.milestoneID, p.milestone = ms.ID, ms.Title
		default:
			ms, err := findMilestone(ctx, client, r.project, m.iid, m.title)
			if err != nil {
				var ae *aicli.Error
				if errors.As(err, &ae) && ae.Code == "E_NOT_FOUND" {
					ae.Details["parameter"] = "--milestone"
				}
				return p, err
			}
			p.milestoneID, p.milestone = ms.ID, ms.Title
		}
	}
	return p, nil
}

// issueChanges lists the requested fields, with their current values when cur is
// known. Field names are the output's, so the gate compares them with the
// read-back (Result.Observed). A removed milestone has no after: a FieldChange
// cannot say null (docs/migration-2.0/findings.md, ISSUE-L2), so its note says so and Apply checks it.
func issueChanges(cur *api.Issue, want issueChange, plan issuePlan) []aicli.FieldChange {
	var out []aicli.FieldChange
	add := func(ch aicli.FieldChange, before func(c *api.Issue) any) {
		if cur != nil {
			ch.Before = before(cur)
		}
		out = append(out, ch)
	}
	if want.title != nil {
		add(aicli.FieldChange{Field: "title", After: *want.title}, func(c *api.Issue) any { return c.Title })
	}
	if want.description != nil {
		add(aicli.FieldChange{Field: "description", After: *want.description}, func(c *api.Issue) any { return c.Description })
	}
	if want.setAssignees {
		add(aicli.FieldChange{Field: "assignees", After: plan.assigneeNames},
			func(c *api.Issue) any { return issueAssignees(c.Assignees) })
	}
	if len(want.addLabels) > 0 || len(want.removeLabels) > 0 {
		ch := aicli.FieldChange{Field: "labels"}
		if len(want.addLabels) > 0 {
			ch.Add = want.addLabels
		}
		if len(want.removeLabels) > 0 {
			ch.Remove = want.removeLabels
		}
		out = append(out, ch)
	}
	if want.milestone.given {
		ch := aicli.FieldChange{Field: "milestone", After: plan.milestone}
		if want.milestone.clear {
			ch.Note = "removed"
		}
		add(ch, func(c *api.Issue) any { return issueMilestoneTitle(c.Milestone) })
	}
	if want.confidential != nil {
		out = append(out, aicli.FieldChange{Field: "confidential", After: *want.confidential})
	}
	if want.stateEvent != "" {
		add(aicli.FieldChange{Field: "state", After: map[string]string{"close": "closed", "reopen": "opened"}[want.stateEvent]},
			func(c *api.Issue) any { return c.State })
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// issueChangeUntrusted is the preview's untrusted paths: the current values hold
// titles, descriptions and milestone titles people wrote, and a milestone named by
// its iid gets its title from GitLab.
func issueChangeUntrusted(cur bool, want issueChange) []string {
	var paths []string
	if cur {
		paths = append(paths, "changes[].before")
	}
	if want.milestone.given {
		paths = append(paths, "changes[].after")
	}
	return paths
}

// issueObserved is the read-back of the requested fields from GitLab's answer, in
// the form of issueChanges.
func issueObserved(iss api.Issue, want issueChange) map[string]any {
	obs := map[string]any{}
	if want.title != nil {
		obs["title"] = iss.Title
	}
	if want.description != nil {
		obs["description"] = iss.Description
	}
	if want.setAssignees {
		obs["assignees"] = issueAssignees(iss.Assignees)
	}
	if len(want.addLabels) > 0 || len(want.removeLabels) > 0 {
		obs["labels"] = issueLabels(iss.Labels)
	}
	if want.milestone.given {
		obs["milestone"] = issueMilestoneTitle(iss.Milestone)
	}
	if want.confidential != nil {
		obs["confidential"] = iss.Confidential
	}
	if want.stateEvent != "" {
		obs["state"] = iss.State
	}
	return obs
}

// issueApplied is the Result of a write on one issue. The gate compares every
// planned after, add and remove with the read-back; a removed milestone has no
// after, so it is compared here.
func issueApplied(iss api.Issue, want issueChange, data map[string]any, untrusted []string) aicli.Result {
	res := aicli.Result{Observed: issueObserved(iss, want), Data: data, Untrusted: untrusted}
	if want.milestone.clear {
		if iss.Milestone != nil {
			res.Mismatched = []string{"milestone"}
			res.Outcome.Verification = "failed"
		} else if issueOnlyClearsMilestone(want) {
			res.Outcome.Verification = "passed" // nothing else for the gate to compare
		}
	}
	return res
}

func issueOnlyClearsMilestone(w issueChange) bool {
	return w.title == nil && w.description == nil && !w.setAssignees && len(w.addLabels) == 0 && len(w.removeLabels) == 0 &&
		w.confidential == nil && w.stateEvent == ""
}

// issueUpdateOpts is the request body of an update.
func issueUpdateOpts(want issueChange, plan issuePlan) api.IssueUpdateOpts {
	o := api.IssueUpdateOpts{Title: want.title, Description: want.description, StateEvent: want.stateEvent,
		AddLabels: strings.Join(want.addLabels, ","), RemoveLabels: strings.Join(want.removeLabels, ",")}
	if want.setAssignees {
		ids := append([]int{}, plan.assigneeIDs...)
		if len(ids) == 0 {
			ids = []int{0} // GitLab: 0 unassigns everyone
		}
		o.AssigneeIDs = &ids
	}
	if want.milestone.given {
		id := plan.milestoneID // 0 removes the milestone
		o.MilestoneID = &id
	}
	return o
}

// issueStateBlocks refuses a close of a closed issue and a reopen of an open one.
func issueStateBlocks(cur *api.Issue, event string) []aicli.Block {
	switch {
	case event == "close" && cur.State == "closed":
		return []aicli.Block{{Kind: "already_closed", Message: "The issue is already closed."}}
	case event == "reopen" && cur.State != "closed":
		return []aicli.Block{{Kind: "already_open", Message: "The issue is already open."}}
	}
	return nil
}

// issueBlocks are why a change cannot apply: a close of a closed issue, a reopen of
// an open one (cur is nil for a create), and a description with a line GitLab would
// run as a quick action (/close, /move).
func issueBlocks(cur *api.Issue, want issueChange) []aicli.Block {
	var blocks []aicli.Block
	if cur != nil {
		blocks = issueStateBlocks(cur, want.stateEvent)
	}
	if want.description != nil && hasQuickAction(*want.description) {
		blocks = append(blocks, quickActionBlock("description"))
	}
	return blocks
}

// issueWrite runs a change of one issue through the gate: update, close, reopen,
// assign and label share it. role is the GitLab role a refusal names (CLI-014).
func issueWrite(app *aicli.App, cmd *cobra.Command, gl *lazyClient, sel issueSel, want issueChange, verb, role string) error {
	r := newIssueResolver(gl, sel.project)
	var plan issuePlan
	return app.Write(cmd, aicli.WriteSpec{
		Target:   aicli.Target{Key: sel.ref(), Kind: "issue"},
		ReadBack: "issue get " + sel.ref(),
		Preview: func(ctx context.Context) (aicli.Preview, error) {
			client, err := gl.get()
			if err != nil {
				return aicli.Preview{}, err
			}
			cur, err := client.Issues.Get(ctx, sel.project, sel.iid)
			if err != nil {
				return aicli.Preview{}, readError(err, issueNotFound(sel.project))
			}
			if plan, err = r.resolve(ctx, want); err != nil {
				return aicli.Preview{}, err
			}
			pv := aicli.Preview{Action: fmt.Sprintf("%s issue %s", verb, sel.ref()), CanApply: true,
				Changes: issueChanges(cur, want, plan), BaseVersion: cur.UpdatedAt, BaseSource: "updated_at",
				Untrusted: issueChangeUntrusted(true, want)}
			if b := issueBlocks(cur, want); len(b) > 0 {
				pv.CanApply, pv.BlockedBy = false, b
			}
			return pv, nil
		},
		Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
			client, err := gl.get()
			if err != nil {
				return aicli.Result{}, aicli.NotDispatched(err)
			}
			iss, err := client.Issues.Update(ctx, sel.project, sel.iid, issueUpdateOpts(want, plan))
			if err != nil {
				return aicli.Result{}, classifyWriteError(err, requiresRole(role), issueNotFound(sel.project))
			}
			return issueApplied(*iss, want, issueResult(sel.project, *iss), issueResultUntrusted), nil
		},
	})
}

// ---------------------------------------------------------------- commands

func addIssueList(app *aicli.App, parent *cobra.Command) {
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
		f, err := readIssueFilters(cmd, project)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		opts, scope, err := f.resolve(cmd.Context(), client, project)
		if err != nil {
			return err
		}
		opts.Page, opts.PerPage = req.Page, req.Limit
		issues, pag, err := client.Issues.ListPage(cmd.Context(), project, opts)
		if err != nil {
			return readError(err, projectNotFound())
		}
		items := make([]map[string]any, 0, len(issues))
		for _, iss := range issues {
			items = append(items, issueOut(project, iss))
		}
		page := gitlabPage(items, len(items), req, pag, scope, "created_at_desc")
		if f.milestoneIID > 0 {
			page.Untrusted = []string{"scope.milestone"} // the title GitLab gave for the iid
		}
		return app.List(cmd, page)
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("state", "opened", "Only issues in this state: opened, closed or all")
	c.Flags().String("query", "", "Only issues whose title or description contains this text (see --in)")
	c.Flags().StringSlice("in", nil, "Where --query looks: title, description, or both (the default)")
	c.Flags().StringSlice("label", nil, "Only issues with every one of these labels (none: no label; any: some label)")
	c.Flags().String("author", "", "Only issues opened by this user: carol, @carol or me")
	c.Flags().String("assignee", "", "Only issues assigned to this user (carol, @carol or me), or none, or any")
	c.Flags().String("milestone", "", "Only issues in this milestone: its title, iid or ref; none or any")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{
		Summary: "List a project's issues, newest first (open ones unless --state says otherwise), with the total GitLab counts",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "issue_page",
		Params: map[string]aicli.ParamDoc{"state": {Enum: []string{"opened", "closed", "all"}},
			"in": {Enum: []string{"title", "description"}}},
		RenamedFlags: map[string]string{"search": "query"},
		ErrorCodes:   gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{issueEvidence, liveEvidence("issue")},
		Examples: []string{"gitlab-cli issue list --project acme/web --label bug",
			"gitlab-cli issue list --project acme/web --query timeout --in title",
			"gitlab-cli issue list --project acme/web --state all --limit 100 --fields ref,author"}})
}

// issueFilters are issue list's filters as given, checked without GitLab.
type issueFilters struct {
	state, query     string
	in, labels       []string
	author, assignee string // a username, me; for the assignee also none or any
	milestone        string // a title, None or Any
	milestoneIID     int    // a milestone given by iid or ref, resolved to its title
}

func readIssueFilters(cmd *cobra.Command, project string) (issueFilters, error) {
	var f issueFilters
	f.state, _ = cmd.Flags().GetString("state")
	f.query, _ = cmd.Flags().GetString("query") // 1.x's --search too (Meta.RenamedFlags)
	f.query = strings.TrimSpace(f.query)
	f.in, _ = cmd.Flags().GetStringSlice("in")
	if len(f.in) > 0 && f.query == "" {
		e := aicli.Usage("missing_parameter", "--query", "--in says where --query looks, so it needs --query.")
		e.Details["next_action"] = "Add --query <text>, or drop --in."
		return f, e
	}
	if cmd.Flags().Changed("label") {
		raw, _ := cmd.Flags().GetStringSlice("label")
		if len(raw) == 1 && (strings.EqualFold(raw[0], "none") || strings.EqualFold(raw[0], "any")) {
			f.labels = []string{strings.ToUpper(raw[0][:1]) + strings.ToLower(raw[0][1:])} // GitLab's None and Any
		} else {
			labels, err := issueLabelNames(cmd, "label", project)
			if err != nil {
				return f, err
			}
			f.labels = labels
		}
	}
	for _, uf := range []struct {
		flag  string
		dst   *string
		extra []string
	}{{"author", &f.author, nil}, {"assignee", &f.assignee, []string{"none", "any"}}} {
		raw, _ := cmd.Flags().GetString(uf.flag)
		if raw = strings.TrimSpace(raw); raw == "" {
			continue
		}
		name := strings.TrimPrefix(raw, "@")
		lower := strings.ToLower(name)
		switch {
		case lower == "me" || issueHas(uf.extra, lower):
			*uf.dst = lower
		case issueUsernameRE.MatchString(name):
			*uf.dst = name
		default:
			expected := "a GitLab username (carol, @carol) or me"
			if len(uf.extra) > 0 {
				expected += ", none or any"
			}
			return f, aicli.Validation("--"+uf.flag, expected, fmt.Sprintf("--%s takes a username, not a display name.", uf.flag))
		}
	}
	if raw, _ := cmd.Flags().GetString("milestone"); strings.TrimSpace(raw) != "" {
		raw = strings.TrimSpace(raw)
		switch strings.ToLower(raw) {
		case "none", "any":
			f.milestone = strings.ToUpper(raw[:1]) + strings.ToLower(raw[1:])
		default:
			p, iid, title, err := parseMilestoneArg(raw)
			if err != nil {
				return f, aicli.Validation("--milestone", "a milestone title (v1.0), iid (3) or ref, or none or any",
					"That is not a milestone title, iid or ref.")
			}
			if p != "" && !strings.EqualFold(p, project) {
				return f, aicli.Usage("conflicting_flags", "--milestone", "The milestone ref names another project than --project.")
			}
			f.milestone, f.milestoneIID = title, iid
		}
	}
	return f, nil
}

// resolve turns the filters into GitLab's query (me becomes the account's
// username, a milestone iid its title) and the page's scope: what the list covers.
func (f issueFilters) resolve(ctx context.Context, c *api.Client, project string) (api.IssueListOpts, map[string]any, error) {
	opts := api.IssueListOpts{State: f.state, Labels: f.labels, Search: f.query, In: f.in, OrderBy: "created_at", Sort: "desc"}
	scope := map[string]any{"project": project, "state": f.state}
	if f.query != "" {
		scope["query"] = f.query
		scope["in"] = []string{"title", "description"}
		if len(f.in) > 0 {
			scope["in"] = f.in
		}
	}
	if len(f.labels) > 0 {
		scope["labels"] = f.labels
	}
	me := ""
	for _, name := range []string{f.author, f.assignee} {
		if name == "me" && me == "" {
			u, err := c.Users.Me(ctx)
			if err != nil {
				return opts, nil, readError(err)
			}
			me = u.Username
		}
	}
	username := func(name string) string {
		if name == "me" {
			return me
		}
		return name
	}
	if f.author != "" {
		opts.AuthorUsername = username(f.author)
		scope["author"] = userRef(opts.AuthorUsername)
	}
	switch f.assignee {
	case "":
	case "none", "any":
		opts.AssigneeID = strings.ToUpper(f.assignee[:1]) + f.assignee[1:]
		scope["assignee"] = f.assignee
	default:
		opts.AssigneeUsername = username(f.assignee)
		scope["assignee"] = userRef(opts.AssigneeUsername)
	}
	if f.milestoneIID > 0 {
		m, err := findMilestone(ctx, c, project, f.milestoneIID, "")
		if err != nil {
			return opts, nil, err
		}
		opts.Milestone = m.Title
	} else if f.milestone != "" {
		opts.Milestone = f.milestone
	}
	if opts.Milestone != "" {
		scope["milestone"] = opts.Milestone
	}
	return opts, scope, nil
}

func addIssueGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <issue>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := issueTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		iss, err := client.Issues.Get(cmd.Context(), sel.project, sel.iid)
		if err != nil {
			return readError(err, issueNotFound(sel.project))
		}
		return app.OK(cmd, issueOut(sel.project, *iss))
	}}
	addIssueTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one issue with its description, by ref, web URL or number",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "issue", Positional: "issue",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{issueEvidence, liveEvidence("issue")},
		Examples: []string{"gitlab-cli issue get acme/web#12", "gitlab-cli issue get 12 --project acme/web"}})
}

func addIssueCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		want, err := readIssueChange(app, cmd, project)
		if err != nil {
			return err
		}
		r := newIssueResolver(gl, project)
		var plan issuePlan
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: project + "/-/issues", Kind: "issues"},
			ReadBack: fmt.Sprintf("issue list --project %s --state all --query %s --in title", project, shellArg(*want.title)),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				if err := projectExists(ctx, client, project); err != nil {
					return aicli.Preview{}, err
				}
				if plan, err = r.resolve(ctx, want); err != nil {
					return aicli.Preview{}, err
				}
				// No duplicate check: GitLab allows issues with the same title. A repeat
				// with the same --idempotency-key is replayed by the gate (CLI-062).
				pv := aicli.Preview{Action: "Create an issue in " + project, CanApply: true,
					Changes: issueChanges(nil, want, plan), Untrusted: issueChangeUntrusted(false, want)}
				if b := issueBlocks(nil, want); len(b) > 0 {
					pv.CanApply, pv.BlockedBy = false, b
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				opts := api.IssueCreateOpts{Title: *want.title, AssigneeIDs: plan.assigneeIDs,
					Labels: strings.Join(want.addLabels, ","), MilestoneID: plan.milestoneID}
				if want.description != nil {
					opts.Description = *want.description
				}
				if want.confidential != nil {
					opts.Confidential = *want.confidential
				}
				iss, err := client.Issues.Create(ctx, project, opts)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("guest"), projectNotFound())
				}
				res := issueApplied(*iss, want, issueResult(project, *iss), issueResultUntrusted)
				// The new issue's ref is observed too, so `operation show` finds it after
				// a lost reply (CLI-059).
				res.Observed["ref"] = issueRef(project, iss.IID)
				return res, nil
			},
		})
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("title", "", "Title")
	_ = c.MarkFlagRequired("title")
	c.Flags().String("description", "", "Description (Markdown)")
	c.Flags().String("description-file", "", "Read the description from a file, or - for stdin")
	c.Flags().StringSlice("assignee", nil, "Assignees: carol, @carol or me (comma-separated or repeated)")
	c.Flags().StringSlice("label", nil, "Labels (comma-separated or repeated)")
	c.Flags().String("milestone", "", "Milestone: its iid, ref or title (the project's own milestones)")
	c.Flags().String("milestone-id", "", "Milestone ID (the global one)")
	c.Flags().Bool("confidential", false, "Make the issue confidential")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same create with the same key returns the first result")
	c.MarkFlagsMutuallyExclusive("milestone", "milestone-id")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create an issue", Aliases: []string{"add", "new"},
		Effects: []aicli.Effect{aicli.Change}, Output: "issue", FileInputs: map[string]string{"description-file": "description"},
		DeprecatedFlags: map[string]string{"milestone-id": "use --milestone with the milestone's iid, ref or title"},
		ErrorCodes:      gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{issueEvidence, liveEvidence("issue")},
		Examples: []string{"gitlab-cli issue create --project acme/web --title \"Checkout times out\" --label bug --dry-run",
			"gitlab-cli issue create --project acme/web --title \"Checkout times out\" --label bug --confirm <confirm_token>"}})
}

func addIssueUpdate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "update <issue>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := issueTarget(cmd, gl)
		if err != nil {
			return err
		}
		want, err := readIssueChange(app, cmd, sel.project)
		if err != nil {
			return err
		}
		return issueWrite(app, cmd, gl, sel, want, "Update", "reporter")
	}}
	addIssueTargetFlags(c)
	c.Flags().String("title", "", "New title")
	c.Flags().String("description", "", "New description (Markdown; an empty value clears it)")
	c.Flags().String("description-file", "", "Read the new description from a file, or - for stdin")
	c.Flags().StringSlice("assignee", nil, "New assignees, replacing the current ones: carol, @carol or me (an empty value unassigns everyone)")
	c.Flags().StringSlice("add-labels", nil, "Labels to add (comma-separated or repeated)")
	c.Flags().StringSlice("remove-labels", nil, "Labels to remove (comma-separated or repeated)")
	c.Flags().String("milestone", "", "New milestone: its iid, ref or title (an empty value removes it)")
	c.Flags().String("milestone-id", "", "New milestone ID (the global one)")
	// An update with nothing to change is a usage error before the gate (CLI-029).
	c.MarkFlagsOneRequired("title", "description", "description-file", "assignee", "add-labels", "remove-labels",
		"milestone", "milestone-id")
	c.MarkFlagsMutuallyExclusive("milestone", "milestone-id")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Change an issue's title, description, assignees, labels or milestone (only the fields given)",
		Aliases: []string{"edit"}, Effects: []aicli.Effect{aicli.Change}, Output: "issue", Positional: "issue",
		FileInputs:      map[string]string{"description-file": "description"},
		DeprecatedFlags: map[string]string{"milestone-id": "use --milestone with the milestone's iid, ref or title"},
		ErrorCodes:      gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{issueEvidence, liveEvidence("issue")},
		Examples: []string{"gitlab-cli issue update acme/web#12 --add-labels needs-triage --milestone v1.1 --dry-run",
			"gitlab-cli issue update acme/web#12 --add-labels needs-triage --milestone v1.1 --confirm <confirm_token>"}})
}

// addIssueStateCommand adds issue close or issue reopen.
func addIssueStateCommand(app *aicli.App, parent *cobra.Command, verb, event, summary string) {
	c := &cobra.Command{Use: verb + " <issue>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := issueTarget(cmd, gl)
		if err != nil {
			return err
		}
		return issueWrite(app, cmd, gl, sel, issueChange{stateEvent: event}, strings.ToUpper(verb[:1])+verb[1:], "reporter")
	}}
	addIssueTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: summary, Effects: []aicli.Effect{aicli.Change}, Output: "issue", Positional: "issue",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{issueEvidence, liveEvidence("issue")}, // close and reopen both ran live
		Examples: []string{"gitlab-cli issue " + verb + " acme/web#12 --dry-run",
			"gitlab-cli issue " + verb + " acme/web#12 --confirm <confirm_token>"}})
}

func addIssueClose(app *aicli.App, parent *cobra.Command) {
	addIssueStateCommand(app, parent, "close", "close", "Close an issue (issue reopen undoes it)")
}

func addIssueReopen(app *aicli.App, parent *cobra.Command) {
	addIssueStateCommand(app, parent, "reopen", "reopen", "Reopen a closed issue")
}

func addIssueAssign(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "assign <issue>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := issueTarget(cmd, gl)
		if err != nil {
			return err
		}
		var want issueChange
		if want.assignees, want.setAssignees, err = issueUsernames(cmd, "assignee"); err != nil {
			return err
		}
		return issueWrite(app, cmd, gl, sel, want, "Assign", "reporter")
	}}
	addIssueTargetFlags(c)
	c.Flags().StringSlice("assignee", nil, "Users to assign, replacing the current ones: carol, @carol or me (an empty value unassigns everyone)")
	_ = c.MarkFlagRequired("assignee")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Assign an issue to users, replacing its assignees",
		Effects: []aicli.Effect{aicli.Change}, Output: "issue", Positional: "issue",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{issueEvidence, liveEvidence("issue")},
		Examples: []string{"gitlab-cli issue assign acme/web#12 --assignee carol --dry-run",
			"gitlab-cli issue assign acme/web#12 --assignee carol --confirm <confirm_token>"}})
}

func addIssueLabel(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "label <issue>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := issueTarget(cmd, gl)
		if err != nil {
			return err
		}
		want, err := readIssueChange(app, cmd, sel.project)
		if err != nil {
			return err
		}
		return issueWrite(app, cmd, gl, sel, want, "Label", "reporter")
	}}
	addIssueTargetFlags(c)
	c.Flags().StringSlice("add", nil, "Labels to add (comma-separated or repeated)")
	c.Flags().StringSlice("remove", nil, "Labels to remove (comma-separated or repeated)")
	c.MarkFlagsOneRequired("add", "remove")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Add labels to an issue or remove them; its other labels stay",
		Effects: []aicli.Effect{aicli.Change}, Output: "issue", Positional: "issue",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{issueEvidence},
		Examples: []string{"gitlab-cli issue label acme/web#12 --add needs-triage --remove stale --dry-run",
			"gitlab-cli issue label acme/web#12 --add needs-triage --remove stale --confirm <confirm_token>"}})
}

// issueHas reports whether list holds s.
func issueHas(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
