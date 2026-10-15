package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The milestone group: list, get, create, update, close. A milestone is named by
// its iid (the number in its web URL, /-/milestones/<iid>) or its title; GitLab's
// API wants the global ID, which the commands look up.

const milestoneEvidence = "simulated:cmd/milestone_test.go"

var (
	milestoneFields = []string{"ref", "id", "iid", "title", "description", "state", "start_date", "due_date",
		"expired", "web_url", "created_at", "updated_at"}
	milestoneDefaultFields = []string{"ref", "title", "state", "due_date", "expired", "web_url"}
	milestoneUntrusted     = []string{"title", "description"}
)

func registerMilestone(app *aicli.App) {
	group := addGroup(app, "milestone", "List, read, create, update and close project milestones", "milestones")
	app.Schema("milestone", aicli.SchemaDef{Shape: "object", Fields: milestoneFields, Untrusted: milestoneUntrusted})
	app.Schema("milestone_page", aicli.SchemaDef{Shape: "page", Fields: milestoneFields,
		DefaultFields: milestoneDefaultFields, Untrusted: milestoneUntrusted})
	addMilestoneList(app, group)
	addMilestoneGet(app, group)
	addMilestoneCreate(app, group)
	addMilestoneUpdate(app, group)
	addMilestoneClose(app, group)
}

// milestoneOut is a milestone as every milestone command returns it.
func milestoneOut(project string, m api.Milestone) map[string]any {
	out := map[string]any{
		"ref": milestoneRef(project, m.IID), "id": idString(m.ID), "iid": idString(m.IID), "title": m.Title,
		"description": m.Description, "state": m.State, "start_date": optString(m.StartDate), "due_date": optString(m.DueDate),
		"web_url": m.WebURL, "created_at": utcTime(m.CreatedAt), "updated_at": utcTime(m.UpdatedAt),
	}
	if m.Expired != nil {
		out["expired"] = *m.Expired
	} else {
		out["expired"] = nil
	}
	return out
}

// parseMilestoneArg reads a milestone as an agent has it: its iid ("3", "%3"), its
// ref ("acme/web/-/milestones/3") or web URL, GitLab's cross-project form
// ("acme/web%3", `acme/web%"v1.0"`), or its title ("v1.0", `%"Sprint 12"`). A title
// that is only digits needs the `%"2026"` form. project is "" when the input names
// none.
func parseMilestoneArg(raw string) (project string, iid int, title string, err error) {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "://") {
		u, perr := url.Parse(s)
		if perr != nil || !strings.Contains(u.Path, "/-/milestones/") {
			return "", 0, "", errors.New("not a milestone URL")
		}
		s = u.Path
	}
	if i := strings.Index(s, "/-/milestones/"); i >= 0 {
		p, _, perr := parseProject(s[:i])
		n, nerr := strconv.Atoi(strings.SplitN(s[i+len("/-/milestones/"):], "/", 2)[0])
		if perr != nil || nerr != nil || n <= 0 {
			return "", 0, "", errors.New("not a milestone ref")
		}
		return p, n, "", nil
	}
	if i := strings.Index(s, "%"); i == 0 || (i > 0 && strings.Contains(s[:i], "/")) {
		if i > 0 {
			p, _, perr := parseProject(s[:i])
			if perr != nil {
				return "", 0, "", perr
			}
			project = p
		}
		s = s[i+1:]
		if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
			title = strings.TrimSpace(s[1 : len(s)-1])
			if title == "" {
				return "", 0, "", errors.New("empty title")
			}
			return project, 0, title, nil
		}
	}
	if digits.MatchString(s) {
		n, nerr := strconv.Atoi(s)
		if nerr != nil || n <= 0 {
			return "", 0, "", errors.New("not an iid")
		}
		return project, n, "", nil
	}
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		s = s[1 : len(s)-1]
	}
	if s = strings.TrimSpace(s); s == "" {
		return "", 0, "", errors.New("empty milestone")
	}
	return project, 0, s, nil
}

// milestoneSel is the milestone a command acts on: its project and iid (the ref
// and the scope key use them).
type milestoneSel struct {
	project string
	iid     int
}

func (s milestoneSel) ref() string { return milestoneRef(s.project, s.iid) }

// milestoneNotFound is the 404 of a milestone read.
func milestoneNotFound(project string) errOpt {
	return onNotFound("The project has no milestone with that iid or title, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli milestone list --project %s --state all` to find the milestone.", project))
}

// findMilestone reads one milestone of a project by iid or by exact title
// (E_NOT_FOUND when none matches).
func findMilestone(ctx context.Context, c *api.Client, project string, iid int, title string) (*api.Milestone, error) {
	opts := api.MilestoneListOpts{State: "all", Page: 1, PerPage: 20}
	if title != "" {
		opts.Title = title
	} else {
		opts.IIDs = []int{iid}
	}
	ms, _, err := c.Milestones.ListPage(ctx, project, opts)
	if err != nil {
		return nil, readError(err, projectNotFound())
	}
	for i := range ms {
		if (title != "" && ms[i].Title == title) || (title == "" && ms[i].IID == iid) {
			return &ms[i], nil
		}
	}
	e := aicli.NotFound("The project has no milestone with that iid or title, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli milestone list --project %s --state all` to find the milestone.", project))
	e.Details["dispatch_state"] = "not_started"
	return nil, e
}

// milestoneTarget reads the milestone positional (or --milestone), or the
// deprecated --milestone-id (the global ID), and the project. A title or a global ID
// is resolved to the iid with one read before the gate, and the iid is bound with
// aicli.SetResolved, so `3`, `%3`, the ref, the URL and the title are one request
// (CLI-054).
func milestoneTarget(cmd *cobra.Command, gl *lazyClient) (milestoneSel, error) {
	raw, _ := cmd.Flags().GetString("milestone")
	if cmd.Flags().Changed("milestone-id") {
		if strings.TrimSpace(raw) != "" {
			e := aicli.Usage("conflicting_flags", "--milestone-id", "Give the milestone once: as the argument (or --milestone), not also with --milestone-id.")
			e.Details["next_action"] = "Drop --milestone-id; pass the milestone's iid, ref or title as the argument."
			return milestoneSel{}, e
		}
		rawID, _ := cmd.Flags().GetString("milestone-id") // a string flag, parsed here (an Int flag would do too, P2)
		id, _ := strconv.Atoi(strings.TrimSpace(rawID))
		if id <= 0 {
			return milestoneSel{}, aicli.Validation("--milestone-id", "a positive milestone ID", "--milestone-id must be a positive number.")
		}
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return milestoneSel{}, err
		}
		c, err := gl.get()
		if err != nil {
			return milestoneSel{}, err
		}
		m, err := c.Milestones.GetByID(cmd.Context(), project, id)
		if err != nil {
			return milestoneSel{}, readError(err, milestoneNotFound(project))
		}
		return milestoneSel{project: project, iid: m.IID}, nil
	}
	if strings.TrimSpace(raw) == "" {
		e := aicli.Usage("missing_parameter", "--milestone", fmt.Sprintf("`%s` needs the milestone: its iid, ref or title.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Pass the milestone as the argument, for example `%s 3 --project acme/web`.", cmd.CommandPath())
		return milestoneSel{}, e
	}
	refProject, iid, title, err := parseMilestoneArg(raw)
	if err != nil {
		e := aicli.Validation("--milestone", "an iid (3), a ref (acme/web/-/milestones/3) or a title (v1.0)", "That is not a milestone iid, ref or title.")
		e.Details["next_action"] = "Run `gitlab-cli milestone list --project <group/project> --state all` to find the milestone."
		return milestoneSel{}, e
	}
	project, err := projectFlag(cmd, refProject, gl)
	if err != nil {
		return milestoneSel{}, err
	}
	if title != "" {
		c, err := gl.get()
		if err != nil {
			return milestoneSel{}, err
		}
		m, err := findMilestone(cmd.Context(), c, project, 0, title)
		if err != nil {
			return milestoneSel{}, err
		}
		iid = m.IID
	}
	if err := aicli.SetResolved(cmd, "milestone", strconv.Itoa(iid)); err != nil {
		return milestoneSel{}, err
	}
	return milestoneSel{project: project, iid: iid}, nil
}

// milestoneDate checks a YYYY-MM-DD flag.
func milestoneDate(cmd *cobra.Command, name string) (string, error) {
	v, _ := cmd.Flags().GetString(name)
	v = strings.TrimSpace(v)
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return "", aicli.Validation("--"+name, "a date as YYYY-MM-DD", "--"+name+" must be a date like 2026-11-30.")
	}
	return v, nil
}

// milestoneFromFlags is the requested change of milestone create and update; set
// holds the flags given.
type milestoneFromFlags struct {
	title, description, startDate, dueDate, stateEvent string
	set                                                map[string]bool
}

func readMilestoneFlags(cmd *cobra.Command) (milestoneFromFlags, error) {
	f := milestoneFromFlags{set: map[string]bool{}}
	for _, n := range []string{"title", "description", "start-date", "due-date", "state-event"} {
		f.set[n] = cmd.Flags().Lookup(n) != nil && cmd.Flags().Changed(n)
	}
	f.title, _ = cmd.Flags().GetString("title")
	f.title = strings.TrimSpace(f.title)
	if f.set["title"] && f.title == "" {
		return f, aicli.Validation("--title", "a milestone title", "The title is empty.")
	}
	f.description, _ = cmd.Flags().GetString("description")
	var err error
	if f.set["start-date"] {
		if f.startDate, err = milestoneDate(cmd, "start-date"); err != nil {
			return f, err
		}
	}
	if f.set["due-date"] {
		if f.dueDate, err = milestoneDate(cmd, "due-date"); err != nil {
			return f, err
		}
	}
	if f.startDate != "" && f.dueDate != "" && f.dueDate < f.startDate {
		return f, aicli.Validation("--due-date", "a date on or after --start-date", "--due-date is before --start-date.")
	}
	f.stateEvent, _ = cmd.Flags().GetString("state-event")
	return f, nil
}

// milestoneChanges lists the requested fields, with their current values when cur
// is known. Field names are the output's, so the gate compares them with the
// read-back.
func milestoneChanges(cur *api.Milestone, want milestoneFromFlags) []aicli.FieldChange {
	var before api.Milestone
	if cur != nil {
		before = *cur
	}
	var out []aicli.FieldChange
	add := func(field string, b, a any) {
		ch := aicli.FieldChange{Field: field, After: a}
		if cur != nil {
			ch.Before = b
		}
		out = append(out, ch)
	}
	if want.set["title"] {
		add("title", before.Title, want.title)
	}
	if want.set["description"] {
		add("description", before.Description, want.description)
	}
	if want.set["start-date"] {
		add("start_date", optString(before.StartDate), want.startDate)
	}
	if want.set["due-date"] {
		add("due_date", optString(before.DueDate), want.dueDate)
	}
	if want.set["state-event"] {
		add("state", before.State, map[string]string{"close": "closed", "activate": "active"}[want.stateEvent])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// milestoneObserved is the read-back of the requested fields, in the form of
// milestoneChanges.
func milestoneObserved(m api.Milestone, want milestoneFromFlags) map[string]any {
	obs := map[string]any{}
	if want.set["title"] {
		obs["title"] = m.Title
	}
	if want.set["description"] {
		obs["description"] = m.Description
	}
	if want.set["start-date"] {
		obs["start_date"] = optString(m.StartDate)
	}
	if want.set["due-date"] {
		obs["due_date"] = optString(m.DueDate)
	}
	if want.set["state-event"] {
		obs["state"] = m.State
	}
	return obs
}

func addMilestoneList(app *aicli.App, parent *cobra.Command) {
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
		client, err := gl.get()
		if err != nil {
			return err
		}
		state, _ := cmd.Flags().GetString("state")
		query, _ := cmd.Flags().GetString("query")
		query = strings.TrimSpace(query)
		ms, pag, err := client.Milestones.ListPage(cmd.Context(), project, api.MilestoneListOpts{State: state, Search: query,
			Page: req.Page, PerPage: req.Limit})
		if err != nil {
			return readError(err, projectNotFound())
		}
		items := make([]map[string]any, 0, len(ms))
		for _, m := range ms {
			items = append(items, milestoneOut(project, m))
		}
		scope := map[string]any{"project": project, "state": state,
			"covers": "the project's own milestones; milestones of its groups are not included"}
		if query != "" {
			scope["query"] = query
		}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "id_desc"))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("state", "active", "Only milestones in this state: active, closed or all")
	c.Flags().String("query", "", "Only milestones whose title or description contains this text")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List a project's milestones, newest first (active ones unless --state says otherwise)",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "milestone_page",
		Params:     map[string]aicli.ParamDoc{"state": {Enum: []string{"active", "closed", "all"}}},
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{milestoneEvidence, liveEvidence("milestone")},
		Examples: []string{"gitlab-cli milestone list --project acme/web",
			"gitlab-cli milestone list --project acme/web --state all --query release"}})
}

func addMilestoneGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <milestone>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := milestoneTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		m, err := findMilestone(cmd.Context(), client, sel.project, sel.iid, "")
		if err != nil {
			return err
		}
		return app.OK(cmd, milestoneOut(sel.project, *m))
	}}
	addMilestoneTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one milestone by iid, ref, URL or title", Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe},
		Output: "milestone", Positional: "milestone", ErrorCodes: gitlabReadCodes,
		DeprecatedFlags: map[string]string{"milestone-id": "pass the milestone's iid, ref or title as the argument"},
		Maturity:        aicli.Verified, Evidence: []string{milestoneEvidence, liveEvidence("milestone")},
		Examples: []string{"gitlab-cli milestone get 3 --project acme/web", "gitlab-cli milestone get acme/web/-/milestones/3"}})
}

// addMilestoneTargetFlags declares the flags that name one milestone.
func addMilestoneTargetFlags(c *cobra.Command) {
	c.Flags().String("milestone", "", "Milestone iid (3), ref (acme/web/-/milestones/3), web URL or title")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the milestone is a ref")
	c.Flags().String("milestone-id", "", "Milestone ID (the global one, not the iid)")
}

func addMilestoneCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create", RunE: func(cmd *cobra.Command, _ []string) error {
		want, err := readMilestoneFlags(cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: project + "/-/milestones", Kind: "milestones"},
			ReadBack: "milestone get " + shellArg(want.title) + " --project " + project,
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				pv := aicli.Preview{Action: fmt.Sprintf("Create milestone %s in %s", want.title, project), CanApply: true,
					Changes: milestoneChanges(nil, want)}
				// GitLab refuses a second milestone with the same title in a project. A
				// repeat of an applied create with the same --idempotency-key never gets
				// here: the gate replays it before Preview (CLI-062, P4).
				var nf *aicli.Error
				switch _, err := findMilestone(ctx, client, project, 0, want.title); {
				case err == nil:
					pv.CanApply = false
					pv.BlockedBy = []aicli.Block{{Kind: "already_exists", Message: "The project already has a milestone with this title."}}
				case errors.As(err, &nf) && nf.Code == "E_NOT_FOUND":
				default:
					return aicli.Preview{}, err
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				m, err := client.Milestones.Create(ctx, project, api.MilestoneCreateOpts{Title: want.title,
					Description: want.description, StartDate: want.startDate, DueDate: want.dueDate})
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("reporter"), projectNotFound())
				}
				// The new milestone's ref is observed too, so `operation show` finds it after
				// a lost reply (CLI-059).
				obs := milestoneObserved(*m, want)
				obs["ref"] = milestoneRef(project, m.IID)
				return aicli.Result{Observed: obs, Data: milestoneOut(project, *m), Untrusted: milestoneUntrusted}, nil
			},
		})
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("title", "", "Milestone title")
	c.Flags().String("description", "", "Description")
	c.Flags().String("start-date", "", "Start date, YYYY-MM-DD")
	c.Flags().String("due-date", "", "Due date, YYYY-MM-DD")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same create with the same key returns the first result")
	_ = c.MarkFlagRequired("title")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create a project milestone", Aliases: []string{"add", "new"}, Effects: []aicli.Effect{aicli.Change}, Output: "milestone",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{milestoneEvidence, liveEvidence("milestone")},
		Examples: []string{"gitlab-cli milestone create --project acme/web --title v2.1 --due-date 2026-11-30 --dry-run",
			"gitlab-cli milestone create --project acme/web --title v2.1 --due-date 2026-11-30 --confirm <confirm_token>"}})
}

// milestoneWrite runs an update of one milestone through the gate: update and
// close share it.
func milestoneWrite(app *aicli.App, cmd *cobra.Command, want milestoneFromFlags, action string, blocked func(*api.Milestone) []aicli.Block) error {
	gl := gitlabFor(app)
	sel, err := milestoneTarget(cmd, gl)
	if err != nil {
		return err
	}
	var id int
	return app.Write(cmd, aicli.WriteSpec{
		Target:   aicli.Target{Key: sel.ref(), Kind: "milestone"},
		ReadBack: "milestone get " + sel.ref(),
		Preview: func(ctx context.Context) (aicli.Preview, error) {
			client, err := gl.get()
			if err != nil {
				return aicli.Preview{}, err
			}
			cur, err := findMilestone(ctx, client, sel.project, sel.iid, "")
			if err != nil {
				return aicli.Preview{}, err
			}
			id = cur.ID
			pv := aicli.Preview{Action: fmt.Sprintf("%s milestone %s", action, sel.ref()), CanApply: true,
				Changes: milestoneChanges(cur, want), BaseVersion: cur.UpdatedAt, BaseSource: "updated_at",
				Untrusted: []string{"changes[].before"}}
			if b := blocked(cur); len(b) > 0 {
				pv.CanApply, pv.BlockedBy = false, b
			}
			return pv, nil
		},
		Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
			client, err := gl.get()
			if err != nil {
				return aicli.Result{}, aicli.NotDispatched(err)
			}
			var opts api.MilestoneUpdateOpts
			if want.set["title"] {
				opts.Title = ptr(want.title)
			}
			if want.set["description"] {
				opts.Description = ptr(want.description)
			}
			if want.set["start-date"] {
				opts.StartDate = ptr(want.startDate)
			}
			if want.set["due-date"] {
				opts.DueDate = ptr(want.dueDate)
			}
			if want.set["state-event"] {
				opts.StateEvent = ptr(want.stateEvent)
			}
			m, err := client.Milestones.Update(ctx, sel.project, id, opts)
			if err != nil {
				return aicli.Result{}, classifyWriteError(err, requiresRole("reporter"), milestoneNotFound(sel.project))
			}
			return aicli.Result{Observed: milestoneObserved(*m, want), Data: milestoneOut(sel.project, *m), Untrusted: milestoneUntrusted}, nil
		},
	})
}

func addMilestoneUpdate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "update <milestone>", RunE: func(cmd *cobra.Command, _ []string) error {
		want, err := readMilestoneFlags(cmd)
		if err != nil {
			return err
		}
		return milestoneWrite(app, cmd, want, "Update", func(*api.Milestone) []aicli.Block { return nil })
	}}
	addMilestoneTargetFlags(c)
	c.Flags().String("title", "", "New title")
	c.Flags().String("description", "", "New description (an empty value clears it)")
	c.Flags().String("start-date", "", "New start date, YYYY-MM-DD")
	c.Flags().String("due-date", "", "New due date, YYYY-MM-DD")
	c.Flags().String("state-event", "", "close or activate")
	// An update with nothing to change is a usage error before the gate (CLI-029).
	c.MarkFlagsOneRequired("title", "description", "start-date", "due-date", "state-event")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Change a milestone's title, description, dates or state",
		Aliases: []string{"edit"}, Effects: []aicli.Effect{aicli.Change}, Output: "milestone", Positional: "milestone",
		Params:          map[string]aicli.ParamDoc{"state-event": {Enum: []string{"close", "activate"}}},
		ErrorCodes:      gitlabWriteCodes,
		DeprecatedFlags: map[string]string{"milestone-id": "pass the milestone's iid, ref or title as the argument"},
		Maturity:        aicli.Verified, Evidence: []string{milestoneEvidence, liveEvidence("milestone")},
		Examples: []string{"gitlab-cli milestone update 3 --project acme/web --due-date 2026-12-15 --dry-run",
			"gitlab-cli milestone update 3 --project acme/web --due-date 2026-12-15 --confirm <confirm_token>"}})
}

func addMilestoneClose(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "close <milestone>", RunE: func(cmd *cobra.Command, _ []string) error {
		want := milestoneFromFlags{stateEvent: "close", set: map[string]bool{"state-event": true}}
		return milestoneWrite(app, cmd, want, "Close", func(m *api.Milestone) []aicli.Block {
			if m.State == "closed" {
				return []aicli.Block{{Kind: "already_closed", Message: "The milestone is already closed."}}
			}
			return nil
		})
	}}
	addMilestoneTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Close a milestone (milestone update --state-event activate reopens it)",
		Effects: []aicli.Effect{aicli.Change}, Output: "milestone", Positional: "milestone", ErrorCodes: gitlabWriteCodes,
		DeprecatedFlags: map[string]string{"milestone-id": "pass the milestone's iid, ref or title as the argument"},
		Maturity:        aicli.Verified, Evidence: []string{milestoneEvidence, liveEvidence("milestone")},
		Examples: []string{"gitlab-cli milestone close 3 --project acme/web --dry-run",
			"gitlab-cli milestone close 3 --project acme/web --confirm <confirm_token>"}})
}
