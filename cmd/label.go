package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The label group: list, get, create, update, delete. It is the reference pattern
// for the other 2.0 groups (docs/migration-2.0/notes.md): one register function, a schema per
// output, Meta on every leaf, reads through app.OK and app.List, writes through
// app.Write with a read-only Preview and an Apply that sends once and reports what
// it observed.

const labelEvidence = "simulated:cmd/label_test.go"

var (
	labelFields = []string{"ref", "id", "name", "color", "text_color", "description", "priority", "is_project_label",
		"subscribed", "open_issues_count", "closed_issues_count", "open_merge_requests_count"}
	labelDefaultFields = []string{"ref", "name", "color", "description", "priority"}
	labelUntrusted     = []string{"name", "description"}
	labelCountFields   = []string{"open_issues_count", "closed_issues_count", "open_merge_requests_count"}
	labelHexColor      = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// registerLabel registers the label group. Leaf aliases (ls, view, add, edit, rm)
// are per parent (P1): every group may use the same ones.
func registerLabel(app *aicli.App) {
	group := addGroup(app, "label", "List, read, create, update and delete project labels", "labels")
	app.Schema("label", aicli.SchemaDef{Shape: "object", Fields: labelFields, Untrusted: labelUntrusted})
	app.Schema("label_page", aicli.SchemaDef{Shape: "page", Fields: labelFields, DefaultFields: labelDefaultFields,
		Untrusted: labelUntrusted})
	app.Schema("label_deleted", aicli.SchemaDef{Shape: "object", Fields: []string{"ref", "deleted"}})
	addLabelList(app, group)
	addLabelGet(app, group)
	addLabelCreate(app, group)
	addLabelUpdate(app, group)
	addLabelDelete(app, group)
}

// labelOut is a label as every label command returns it. The counts are present
// only when GitLab sent them (label list with them in --fields).
func labelOut(project string, l api.Label) map[string]any {
	out := map[string]any{
		"ref": labelRef(project, l.Name), "id": idString(l.ID), "name": l.Name, "color": l.Color,
		"text_color": l.TextColor, "description": l.Description, "priority": optInt(l.Priority),
		"is_project_label": l.IsProjectLabel, "subscribed": l.Subscribed,
	}
	if l.OpenIssuesCount != nil {
		out["open_issues_count"] = *l.OpenIssuesCount
	}
	if l.ClosedIssuesCount != nil {
		out["closed_issues_count"] = *l.ClosedIssuesCount
	}
	if l.OpenMergeRequestsCount != nil {
		out["open_merge_requests_count"] = *l.OpenMergeRequestsCount
	}
	return out
}

// parseLabelArg reads a label as an agent has it: its name ("bug", "~bug",
// `~"feature request"`), its ref ("acme/web/-/labels/bug"), GitLab's cross-project
// form ("acme/web~bug"), or a URL ending in /-/labels/<name>. project is "" when
// the input names none.
func parseLabelArg(raw string) (project, name string, err error) {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "://") {
		u, perr := url.Parse(s)
		if perr != nil || !strings.Contains(u.Path, "/-/labels/") {
			return "", "", errors.New("not a label URL")
		}
		s = u.Path
	}
	if i := strings.Index(s, "/-/labels/"); i >= 0 {
		p, _, perr := parseProject(s[:i])
		if perr != nil {
			return "", "", perr
		}
		if n, uerr := url.PathUnescape(s[i+len("/-/labels/"):]); uerr == nil {
			name = n
		} else {
			name = s[i+len("/-/labels/"):]
		}
		project = p
	} else if i := strings.Index(s, "~"); i > 0 && strings.Contains(s[:i], "/") {
		p, _, perr := parseProject(s[:i])
		if perr != nil {
			return "", "", perr
		}
		project, name = p, s[i+1:]
	} else {
		name = strings.TrimPrefix(s, "~")
	}
	if len(name) >= 2 && strings.HasPrefix(name, `"`) && strings.HasSuffix(name, `"`) {
		name = name[1 : len(name)-1]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", errors.New("empty label name")
	}
	return project, name, nil
}

// labelSel is the label a command acts on. ident is what GitLab's API takes (the
// name, or the ID given by the deprecated --label-id); name is the label's name,
// which its ref and scope key use. byID marks a name that came from GitLab, not from
// the agent's input.
type labelSel struct {
	project, ident, name string
	byID                 bool
}

func (s labelSel) ref() string { return labelRef(s.project, s.name) }

// readBack is the command that shows the label after a write, built from the
// agent's input only (SEC-002): a name GitLab returned never goes into it.
func (s labelSel) readBack(newName string) string {
	switch {
	case newName != "":
		return "label get " + shellArg(labelRef(s.project, newName))
	case s.byID:
		return "label list --project " + s.project
	}
	return "label get " + shellArg(s.ref())
}

// labelTarget reads the label positional (or --label), or the deprecated
// --label-id, and the project, and binds their canonical forms (CLI-054). A
// --label-id still needs its name, which the ref and the scope key use: resolve
// reads it, inside the gate for a write (WriteSpec.ResolveTarget, Q1), so a repeated
// confirm of a delete is token_used rather than the lookup's E_NOT_FOUND.
func labelTarget(cmd *cobra.Command, gl *lazyClient) (labelSel, error) {
	raw, _ := cmd.Flags().GetString("label")
	if cmd.Flags().Changed("label-id") {
		if strings.TrimSpace(raw) != "" {
			e := aicli.Usage("conflicting_flags", "--label-id", "Give the label once: as the argument (or --label), not also with --label-id.")
			e.Details["next_action"] = "Drop --label-id; pass the label's name or ref as the argument."
			return labelSel{}, e
		}
		rawID, _ := cmd.Flags().GetString("label-id") // a string flag, parsed here (an Int flag would do too, P2)
		id, _ := strconv.Atoi(strings.TrimSpace(rawID))
		if id <= 0 {
			return labelSel{}, aicli.Validation("--label-id", "a positive label ID", "--label-id must be a positive number.")
		}
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return labelSel{}, err
		}
		return labelSel{project: project, ident: strconv.Itoa(id), byID: true}, nil
	}
	if strings.TrimSpace(raw) == "" {
		e := aicli.Usage("missing_parameter", "--label", fmt.Sprintf("`%s` needs the label: its name or ref.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Pass the label as the argument, for example `%s bug --project acme/web`.", cmd.CommandPath())
		return labelSel{}, e
	}
	refProject, name, err := parseLabelArg(raw)
	if err != nil {
		e := aicli.Validation("--label", "a label name (bug), its ref (acme/web/-/labels/bug) or acme/web~bug", "That is not a label name or ref.")
		e.Details["next_action"] = "Run `gitlab-cli label list --project <group/project>` to find the label."
		return labelSel{}, e
	}
	project, err := projectFlag(cmd, refProject, gl)
	if err != nil {
		return labelSel{}, err
	}
	if err := aicli.SetResolved(cmd, "label", name); err != nil {
		return labelSel{}, err
	}
	return labelSel{project: project, ident: name, name: name}, nil
}

// resolve reads the name of a label given by --label-id (one read); a label named
// by its name needs none.
func (s *labelSel) resolve(ctx context.Context, gl *lazyClient) error {
	if s.name != "" {
		return nil
	}
	c, err := gl.get()
	if err != nil {
		return err
	}
	l, err := c.Labels.Get(ctx, s.project, s.ident)
	if err != nil {
		return readError(err, labelNotFound(s.project))
	}
	s.name = l.Name
	return nil
}

// target is the write target of the label: its ref, read first for a --label-id
// (WriteSpec.ResolveTarget, Q1).
func (s *labelSel) target(gl *lazyClient) func(ctx context.Context) (aicli.Target, error) {
	return func(ctx context.Context) (aicli.Target, error) {
		if err := s.resolve(ctx, gl); err != nil {
			return aicli.Target{}, err
		}
		return aicli.Target{Key: s.ref(), Kind: "label"}, nil
	}
}

// labelNotFound is the 404 of a label read.
func labelNotFound(project string) errOpt {
	return onNotFound("The project has no label with that name or ID, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli label list --project %s` to find the label.", project))
}

// labelColor checks --color: six-digit hex (#d73a4a) or one of the colour names
// GitLab stores as hex. The value is returned the way GitLab reports it, so the
// read-back can be compared with the request.
func labelColor(raw string) (string, error) {
	c := api.NormalizeColor(raw)
	if !labelHexColor.MatchString(c) {
		names := []string{"black", "blue", "gray", "green", "grey", "orange", "pink", "purple", "red", "white", "yellow"}
		return "", aicli.Validation("--color", "#RRGGBB or one of "+strings.Join(names, ", "), "--color must be a six-digit hex colour or a known colour name.")
	}
	return strings.ToLower(c), nil
}

// labelFromFlags is the requested change of label create and update. set holds
// the flags given.
type labelFromFlags struct {
	name, color, description string
	priority                 int
	set                      map[string]bool
}

func readLabelFlags(cmd *cobra.Command, nameFlag string) (labelFromFlags, error) {
	f := labelFromFlags{set: map[string]bool{}}
	for _, n := range []string{nameFlag, "color", "description", "priority"} {
		f.set[n] = cmd.Flags().Changed(n)
	}
	f.name, _ = cmd.Flags().GetString(nameFlag)
	f.name = strings.TrimSpace(f.name)
	if f.set[nameFlag] && f.name == "" {
		return f, aicli.Validation("--"+nameFlag, "a label name", "The label name is empty.")
	}
	if f.set["color"] {
		raw, _ := cmd.Flags().GetString("color")
		c, err := labelColor(raw)
		if err != nil {
			return f, err
		}
		f.color = c
	}
	f.description, _ = cmd.Flags().GetString("description")
	f.priority, _ = cmd.Flags().GetInt("priority")
	if f.set["priority"] && f.priority < 0 {
		return f, aicli.Validation("--priority", "0 or more", "--priority must not be negative.")
	}
	return f, nil
}

func addLabelList(app *aicli.App, parent *cobra.Command) {
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
		query, _ := cmd.Flags().GetString("query")
		query = strings.TrimSpace(query)
		labels, pag, err := client.Labels.ListPage(cmd.Context(), project, api.LabelListOpts{Search: query,
			WithCounts: wantsField(cmd, labelCountFields...), Page: req.Page, PerPage: req.Limit})
		if err != nil {
			return readError(err, projectNotFound())
		}
		items := make([]map[string]any, 0, len(labels))
		for _, l := range labels {
			items = append(items, labelOut(project, l))
		}
		scope := map[string]any{"project": project, "covers": "the project's labels and the labels it inherits from its groups"}
		if query != "" {
			scope["query"] = query
		}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "name_asc"))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("query", "", "Only labels whose name contains this text")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List a project's labels, its own and those of its groups, by name",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "label_page",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{labelEvidence, liveEvidence("label")},
		Examples: []string{"gitlab-cli label list --project acme/web",
			"gitlab-cli label list --project acme/web --query bug --fields +open_issues_count"}})
}

func addLabelGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <label>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := labelTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		l, err := client.Labels.Get(cmd.Context(), sel.project, sel.ident)
		if err != nil {
			return readError(err, labelNotFound(sel.project))
		}
		return app.OK(cmd, labelOut(sel.project, *l))
	}}
	c.Flags().String("label", "", "Label name, ref (acme/web/-/labels/bug) or acme/web~bug")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the label is a ref")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one label", Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "label",
		Positional: "label", ErrorCodes: gitlabReadCodes,
		Maturity: aicli.Verified, Evidence: []string{labelEvidence, liveEvidence("label")},
		Examples: []string{"gitlab-cli label get bug --project acme/web", "gitlab-cli label get acme/web/-/labels/bug"}})
}

func addLabelCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create", RunE: func(cmd *cobra.Command, _ []string) error {
		want, err := readLabelFlags(cmd, "name")
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		ref := labelRef(project, want.name)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: ref, Kind: "label"},
			ReadBack: "label get " + shellArg(ref),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				// GitLab refuses a second label with the same name (in the project or
				// its groups): block the plan instead of sending a doomed request. A
				// repeat of an applied create with the same --idempotency-key never gets
				// here: the gate replays it before Preview (CLI-062, P4).
				pv := aicli.Preview{Action: fmt.Sprintf("Create label %s in %s", want.name, project), CanApply: true,
					Changes: labelChanges(nil, want, "name")}
				var apiErr *api.APIError
				switch _, err := client.Labels.Get(ctx, project, want.name); {
				case err == nil:
					pv.CanApply = false
					pv.BlockedBy = []aicli.Block{{Kind: "already_exists",
						Message: "The project or one of its groups already has a label with this name; update that label instead."}}
				case errors.As(err, &apiErr) && apiErr.StatusCode == 404:
					// No such label, or no such project: GitLab answers 404 to both.
					if err := projectExists(ctx, client, project); err != nil {
						return aicli.Preview{}, err
					}
				default:
					return aicli.Preview{}, readError(err, projectNotFound())
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				opts := api.LabelCreateOpts{Name: want.name, Color: want.color, Description: want.description}
				if want.set["priority"] {
					opts.Priority = ptr(want.priority)
				}
				l, err := client.Labels.Create(ctx, project, opts)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("reporter"))
				}
				return aicli.Result{Observed: labelObserved(*l, want, "name"), Data: labelOut(project, *l), Untrusted: labelUntrusted}, nil
			},
		})
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("name", "", "Label name")
	c.Flags().String("color", "", "Colour: #RRGGBB or a name such as red")
	c.Flags().String("description", "", "Description")
	c.Flags().Int("priority", 0, "Priority (0 or more; lower sorts first)")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same create with the same key returns the first result")
	_ = c.MarkFlagRequired("name")
	_ = c.MarkFlagRequired("color")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create a project label", Aliases: []string{"add", "new"}, Effects: []aicli.Effect{aicli.Change}, Output: "label",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{labelEvidence, liveEvidence("label")},
		Examples: []string{"gitlab-cli label create --project acme/web --name needs-review --color #d73a4a --dry-run",
			"gitlab-cli label create --project acme/web --name needs-review --color #d73a4a --confirm <confirm_token>"}})
}

func addLabelUpdate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "update <label>", RunE: func(cmd *cobra.Command, _ []string) error {
		want, err := readLabelFlags(cmd, "name")
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, err := labelTarget(cmd, gl)
		if err != nil {
			return err
		}
		project, ident := sel.project, sel.ident
		newName := ""
		if want.set["name"] {
			newName = want.name
		}
		return app.Write(cmd, aicli.WriteSpec{
			ResolveTarget: sel.target(gl),
			ReadBack:      sel.readBack(newName),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				cur, err := client.Labels.Get(ctx, project, ident)
				if err != nil {
					return aicli.Preview{}, readError(err, labelNotFound(project))
				}
				pv := aicli.Preview{Action: fmt.Sprintf("Update label %s in %s", sel.name, project), CanApply: true,
					Changes: labelChanges(cur, want, "name"), Untrusted: sel.untrusted("changes[].before")}
				if !cur.IsProjectLabel {
					pv.CanApply = false
					pv.BlockedBy = []aicli.Block{{Kind: "group_label", Message: "This label belongs to a parent group; change it in the group."}}
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				var opts api.LabelUpdateOpts
				if want.set["name"] {
					opts.NewName = ptr(want.name)
				}
				if want.set["color"] {
					opts.Color = ptr(want.color)
				}
				if want.set["description"] {
					opts.Description = ptr(want.description)
				}
				if want.set["priority"] {
					opts.Priority = ptr(want.priority)
				}
				l, err := client.Labels.Update(ctx, project, ident, opts)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("reporter"), labelNotFound(project))
				}
				return aicli.Result{Observed: labelObserved(*l, want, "name"), Data: labelOut(project, *l), Untrusted: labelUntrusted}, nil
			},
		})
	}}
	c.Flags().String("label", "", "Label name, ref (acme/web/-/labels/bug) or acme/web~bug")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the label is a ref")
	c.Flags().String("label-id", "", "Label ID")
	c.Flags().String("name", "", "New name")
	c.Flags().String("color", "", "New colour: #RRGGBB or a name such as red")
	c.Flags().String("description", "", "New description (an empty value clears it)")
	c.Flags().Int("priority", 0, "New priority (0 or more)")
	// An update with nothing to change is a usage error before the gate (CLI-029).
	c.MarkFlagsOneRequired("name", "color", "description", "priority")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Change a project label's name, colour, description or priority",
		Aliases: []string{"edit"}, Effects: []aicli.Effect{aicli.Change}, Output: "label", Positional: "label",
		ErrorCodes:      gitlabWriteCodes,
		DeprecatedFlags: map[string]string{"label-id": "pass the label's name or ref as the argument"},
		Maturity:        aicli.Verified, Evidence: []string{labelEvidence, liveEvidence("label")},
		Examples: []string{"gitlab-cli label update bug --project acme/web --color #fbca04 --dry-run",
			"gitlab-cli label update bug --project acme/web --color #fbca04 --confirm <confirm_token>"}})
}

func addLabelDelete(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "delete <label>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := labelTarget(cmd, gl)
		if err != nil {
			return err
		}
		project, ident := sel.project, sel.ident
		return app.Write(cmd, aicli.WriteSpec{
			ResolveTarget: sel.target(gl),
			ReadBack:      sel.readBack(""),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				cur, err := client.Labels.Get(ctx, project, ident)
				if err != nil {
					return aicli.Preview{}, readError(err, labelNotFound(project))
				}
				pv := aicli.Preview{Action: fmt.Sprintf("Delete label %s from %s permanently", sel.name, project), CanApply: true,
					Changes: []aicli.FieldChange{{Field: "label", Before: cur.Name,
						Note: "removed from every issue and merge request that has it; cannot be undone"}},
					Untrusted: sel.untrusted("changes[].before")}
				if !cur.IsProjectLabel {
					pv.CanApply = false
					pv.BlockedBy = []aicli.Block{{Kind: "group_label", Message: "This label belongs to a parent group; delete it in the group."}}
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				if err := client.Labels.Delete(ctx, project, ident); err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("reporter"), labelNotFound(project))
				}
				// Read back: the label must be gone.
				var apiErr *api.APIError
				switch _, err := client.Labels.Get(ctx, project, ident); {
				case errors.As(err, &apiErr) && apiErr.StatusCode == 404:
					return aicli.Result{Outcome: aicli.Outcome{Verification: "passed"}, Observed: map[string]any{"exists": false},
						Data: map[string]any{"ref": sel.ref(), "deleted": true}}, nil
				case err == nil:
					return aicli.Result{Outcome: aicli.Outcome{Verification: "failed"}, Observed: map[string]any{"exists": true},
						Mismatched: []string{"exists"}, Data: map[string]any{"ref": sel.ref(), "deleted": false}}, nil
				}
				return aicli.Result{Data: map[string]any{"ref": sel.ref(), "deleted": true}}, nil
			},
		})
	}}
	c.Flags().String("label", "", "Label name, ref (acme/web/-/labels/bug) or acme/web~bug")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the label is a ref")
	c.Flags().String("label-id", "", "Label ID")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Delete a project label, removing it from every issue and merge request",
		Aliases: []string{"rm", "remove"}, Effects: []aicli.Effect{aicli.Change}, Dangerous: true, Output: "label_deleted", Positional: "label",
		ErrorCodes:      gitlabWriteCodes,
		DeprecatedFlags: map[string]string{"label-id": "pass the label's name or ref as the argument"},
		Maturity:        aicli.Verified, Evidence: []string{labelEvidence, liveEvidence("label")},
		Examples: []string{"gitlab-cli label delete stale --project acme/web --dangerous --dry-run",
			"gitlab-cli label delete stale --project acme/web --dangerous --confirm <confirm_token>"}})
}

// labelChanges lists the requested fields, with their current values when cur is
// known (update). Field names are the output's, so the gate can compare them with
// the read-back (Result.Observed).
func labelChanges(cur *api.Label, want labelFromFlags, nameFlag string) []aicli.FieldChange {
	var out []aicli.FieldChange
	add := func(field string, before, after any) {
		ch := aicli.FieldChange{Field: field, After: after}
		if cur != nil {
			ch.Before = before
		}
		out = append(out, ch)
	}
	var before api.Label
	if cur != nil {
		before = *cur
	}
	if want.set[nameFlag] {
		add("name", before.Name, want.name)
	}
	if want.set["color"] {
		add("color", strings.ToLower(before.Color), want.color)
	}
	if want.set["description"] {
		add("description", before.Description, want.description)
	}
	if want.set["priority"] {
		add("priority", optInt(before.Priority), want.priority)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// labelObserved is the read-back of the requested fields from GitLab's answer,
// in the same form as labelChanges.
func labelObserved(l api.Label, want labelFromFlags, nameFlag string) map[string]any {
	obs := map[string]any{}
	if want.set[nameFlag] {
		obs["name"] = l.Name
	}
	if want.set["color"] {
		obs["color"] = strings.ToLower(l.Color)
	}
	if want.set["description"] {
		obs["description"] = l.Description
	}
	if want.set["priority"] {
		obs["priority"] = optInt(l.Priority)
	}
	return obs
}

// untrusted adds the preview's action to paths when the action names a label
// GitLab returned (the --label-id path), which is external text (CLI-046).
func (s labelSel) untrusted(paths ...string) []string {
	if s.byID {
		return append(paths, "action")
	}
	return paths
}

// projectNotFound is the 404 of a request on a project path.
func projectNotFound() errOpt {
	return onNotFound("No project at that path, or this account cannot see it.",
		"Check --project: it is the path in the project's URL, like acme/web.")
}
