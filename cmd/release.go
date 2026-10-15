package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The release group: list, get, create, update, delete. A release is named by its
// tag (v1.2.0), its ref (acme/web/-/releases/v1.2.0), the tag's ref
// (acme/web/-/tags/v1.2.0) or a web URL of either. Release names and notes are
// text people wrote (untrusted, CLI-046); get cuts long notes and prints them whole
// with --format raw (CLI-042).

const (
	releaseEvidence       = "simulated:cmd/release_test.go"
	releaseDescriptionMax = 4000 // characters of the notes get returns in JSON
)

var (
	releaseListFields = []string{"ref", "tag_name", "name", "created_at", "released_at", "upcoming_release", "author",
		"commit_sha", "milestones", "asset_count", "web_url"}
	releaseFields        = append(append([]string{}, releaseListFields...), "description", "description_truncated")
	releaseListUntrusted = []string{"ref", "tag_name", "name", "milestones"}
	releaseUntrusted     = append(append([]string{}, releaseListUntrusted...), "description")
)

func registerRelease(app *aicli.App) {
	group := addGroup(app, "release", "List, read, create, update and delete project releases", "releases")
	app.Schema("release", aicli.SchemaDef{Shape: "object", Fields: releaseFields, Untrusted: releaseUntrusted})
	app.Schema("release_page", aicli.SchemaDef{Shape: "page", Fields: releaseListFields,
		DefaultFields: []string{"ref", "tag_name", "name", "released_at", "author"}, Untrusted: releaseListUntrusted})
	app.Schema("release_deleted", aicli.SchemaDef{Shape: "object", Fields: []string{"ref", "deleted"}, Untrusted: []string{"ref"}})
	addReleaseList(app, group)
	addReleaseGet(app, group)
	addReleaseCreate(app, group)
	addReleaseUpdate(app, group)
	addReleaseDelete(app, group)
}

func releaseRef(project, tag string) string { return project + "/-/releases/" + tag }

// releaseMilestones is a release's milestone titles in order, so previews and
// read-backs compare the same list.
func releaseMilestones(r api.Release) []string {
	out := []string{}
	for _, m := range r.Milestones {
		out = append(out, m.Title)
	}
	sort.Strings(out)
	return out
}

// releaseOut is a release as the release commands return it; the notes only from get
// and the writes (CLI-043: lists carry no bodies).
func releaseOut(project string, r api.Release, withDescription bool) map[string]any {
	out := map[string]any{"ref": releaseRef(project, r.TagName), "tag_name": r.TagName, "name": r.Name,
		"created_at": utcTime(r.CreatedAt), "released_at": utcTime(r.ReleasedAt), "upcoming_release": r.UpcomingRelease,
		"author": nil, "commit_sha": nil, "milestones": releaseMilestones(r), "asset_count": 0, "web_url": r.Links.Self}
	if r.Author != nil && r.Author.Username != "" {
		out["author"] = userRef(r.Author.Username)
	}
	if r.Commit != nil {
		out["commit_sha"] = r.Commit.ID
	}
	if r.Assets != nil {
		out["asset_count"] = r.Assets.Count
	}
	if withDescription {
		out["description"], out["description_truncated"] = repoClip(r.Description, releaseDescriptionMax)
	}
	return out
}

// parseReleaseArg reads a release as an agent has it: its tag, its ref, the tag's
// ref, or a web URL of either. project is "" when the input names none.
func parseReleaseArg(raw string) (project, tag string, err error) {
	arg, err := parseRepoWebArg(raw, "releases", "tags")
	if err != nil {
		return "", "", err
	}
	tag, err = repoRefName(arg.rest, "--tag", "tag")
	return arg.project, tag, err
}

// releaseTarget reads the release argument (or --tag) and the project, and binds
// their canonical forms (CLI-054).
func releaseTarget(cmd *cobra.Command, gl *lazyClient) (project, tag string, err error) {
	raw, _ := cmd.Flags().GetString("tag")
	if strings.TrimSpace(raw) == "" {
		e := aicli.Usage("missing_parameter", "--tag", fmt.Sprintf("`%s` needs the release's tag, or its ref.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Pass the tag as the argument, for example `%s v1.2.0 --project acme/web`.", cmd.CommandPath())
		return "", "", e
	}
	refProject, tag, err := parseReleaseArg(raw)
	if err != nil {
		e := aicli.Validation("--tag", "a tag (v1.2.0), a release ref (acme/web/-/releases/v1.2.0) or web URL", "That is not a tag, release ref or URL.")
		e.Details["next_action"] = "Run `gitlab-cli release list --project <group/project>` to find the release."
		return "", "", e
	}
	if project, err = projectFlag(cmd, refProject, gl); err != nil {
		return "", "", err
	}
	if err := aicli.SetResolved(cmd, "tag", tag); err != nil {
		return "", "", err
	}
	return project, tag, nil
}

func addReleaseTargetFlags(c *cobra.Command) {
	c.Flags().String("tag", "", "Tag (v1.2.0), release ref (acme/web/-/releases/v1.2.0) or web URL")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the release is a ref")
}

func releaseNotFound(project string) errOpt {
	return onNotFound("The project has no release for that tag, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli release list --project %s` to find the release.", project))
}

func addReleaseList(app *aicli.App, parent *cobra.Command) {
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
		releases, pag, err := client.Releases.ListPage(cmd.Context(), project, req.Page, req.Limit)
		if err != nil {
			return readError(err, projectNotFound())
		}
		items := make([]map[string]any, 0, len(releases))
		for _, r := range releases {
			items = append(items, releaseOut(project, r, false))
		}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, map[string]any{"project": project}, "released_at_desc"))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List a project's releases, newest release date first (notes: release get)",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "release_page", ErrorCodes: gitlabReadCodes,
		Maturity: aicli.Verified, Evidence: []string{releaseEvidence, liveEvidence("release")},
		Examples: []string{"gitlab-cli release list --project acme/web", "gitlab-cli release list --project acme/web --fields +milestones,commit_sha"}})
}

func addReleaseGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <tag>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		project, tag, err := releaseTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		r, err := client.Releases.Get(cmd.Context(), project, tag)
		if err != nil {
			return readError(err, releaseNotFound(project))
		}
		if repoRawFormat(cmd) {
			return app.OK(cmd, r.Description)
		}
		out := releaseOut(project, *r, true)
		if out["description_truncated"] == true {
			app.Notice(aicli.Notice{Type: "description_truncated", Severity: "warning",
				Message: "The release notes are longer than one response carries and were cut; add --format raw for all of them."})
		}
		return app.OK(cmd, out)
	}}
	addReleaseTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one release with its notes; --format raw prints the notes",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "release", Positional: "tag",
		RawOutput: true, ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{releaseEvidence, liveEvidence("release")},
		Examples: []string{"gitlab-cli release get v1.2.0 --project acme/web", "gitlab-cli release get acme/web/-/releases/v1.2.0 --format raw"}})
}

// releaseFromFlags is the requested content of release create and update; set holds
// the flags given (description for --description or --description-file).
type releaseFromFlags struct {
	name, description string
	milestones        []string
	set               map[string]bool
}

func readReleaseFlags(app *aicli.App, cmd *cobra.Command) (releaseFromFlags, error) {
	f := releaseFromFlags{set: map[string]bool{}}
	f.set["name"] = cmd.Flags().Changed("name")
	f.name, _ = cmd.Flags().GetString("name")
	if f.set["name"] && strings.TrimSpace(f.name) == "" {
		return f, aicli.Validation("--name", "a release name", "The release name is empty.")
	}
	switch {
	case cmd.Flags().Changed("description-file"):
		b, err := app.ReadFileInput(cmd, "description-file")
		if err != nil {
			return f, err
		}
		f.description, f.set["description"] = string(b), true
	case cmd.Flags().Changed("description"):
		f.description, _ = cmd.Flags().GetString("description")
		f.set["description"] = true
	}
	if cmd.Flags().Changed("milestone") {
		f.set["milestones"] = true
		raw, _ := cmd.Flags().GetStringSlice("milestone")
		seen := map[string]bool{}
		f.milestones = []string{}
		for _, m := range raw {
			if m = strings.TrimSpace(m); m != "" && !seen[m] {
				seen[m] = true
				f.milestones = append(f.milestones, m)
			}
		}
		sort.Strings(f.milestones)
	}
	return f, nil
}

// checkMilestones looks every requested milestone title up (GitLab refuses unknown
// ones), so the preview names a missing one before anything is sent.
func (f releaseFromFlags) checkMilestones(ctx context.Context, c *api.Client, project string) error {
	for _, title := range f.milestones {
		if _, err := findMilestone(ctx, c, project, 0, title); err != nil {
			if ae, ok := err.(*aicli.Error); ok && ae.Code == "E_NOT_FOUND" {
				ae.Details["parameter"] = "--milestone"
			}
			return err
		}
	}
	return nil
}

// changes lists the requested fields with their current values when cur is known
// (update). Long notes show as their digest (repoTextValue), as the read-back does.
func (f releaseFromFlags) changes(cur *api.Release) []aicli.FieldChange {
	var out []aicli.FieldChange
	add := func(field string, before, after any) {
		ch := aicli.FieldChange{Field: field, After: after}
		if cur != nil {
			ch.Before = before
		}
		out = append(out, ch)
	}
	var before api.Release
	if cur != nil {
		before = *cur
	}
	if f.set["description"] {
		add("description", repoTextValue(before.Description), repoTextValue(f.description))
	}
	if f.set["milestones"] {
		add("milestones", releaseMilestones(before), f.milestones)
	}
	if f.set["name"] {
		add("name", before.Name, f.name)
	}
	return out
}

// observed is the read-back of the requested fields, in the form of changes.
func (f releaseFromFlags) observed(r api.Release) map[string]any {
	obs := map[string]any{}
	if f.set["description"] {
		obs["description"] = repoTextValue(r.Description)
	}
	if f.set["milestones"] {
		obs["milestones"] = releaseMilestones(r)
	}
	if f.set["name"] {
		obs["name"] = r.Name
	}
	return obs
}

// addReleaseContentFlags declares the flags of release create and update.
func addReleaseContentFlags(c *cobra.Command, verb string) {
	c.Flags().String("name", "", verb+"release name")
	c.Flags().String("description", "", verb+"release notes (Markdown); an empty value clears them")
	c.Flags().String("description-file", "", verb+"release notes from a file (- reads stdin)")
	c.Flags().StringSlice("milestone", nil, verb+"milestone titles, repeated or comma-separated; an empty value clears them")
}

func addReleaseCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create <tag>", RunE: func(cmd *cobra.Command, _ []string) error {
		want, err := readReleaseFlags(app, cmd)
		if err != nil {
			return err
		}
		from := ""
		if raw, _ := cmd.Flags().GetString("ref"); strings.TrimSpace(raw) != "" {
			if from, err = repoRevision(raw, "--ref"); err != nil {
				return err
			}
		}
		gl := gitlabFor(app)
		project, tag, err := releaseTarget(cmd, gl)
		if err != nil {
			return err
		}
		key := releaseRef(project, tag)
		var createFrom string // the commit a new tag starts at, as the latest Preview read it
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: key, Kind: "release"},
			ReadBack: "release get " + shellArg(key),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				// A repeat of an applied create with the same --idempotency-key never gets
				// here: the gate replays it first (CLI-062).
				var blocks []aicli.Block
				switch _, err := client.Releases.Get(ctx, project, tag); {
				case err == nil:
					blocks = append(blocks, aicli.Block{Kind: "already_exists", Message: "The tag already has a release; update that release instead."})
				case !repoIsNotFound(err):
					return aicli.Preview{}, readError(err)
				}
				sha, note := "", "the release uses the existing tag"
				createFrom = ""
				t, err := client.Repos.GetTag(ctx, project, tag)
				switch {
				case err == nil && t.Commit != nil:
					sha = t.Commit.ID
				case err == nil:
					sha = t.Target
				case !repoIsNotFound(err):
					return aicli.Preview{}, readError(err)
				default:
					if perr := projectExists(ctx, client, project); perr != nil {
						return aicli.Preview{}, perr
					}
					if from == "" {
						blocks = append(blocks, aicli.Block{Kind: "tag_missing",
							Message: "The tag does not exist; pass --ref with the branch or commit to create it from."})
						break
					}
					commit, err := client.Repos.GetCommit(ctx, project, from)
					if err != nil {
						e := readError(err, onNotFound("--ref names no branch, tag or commit in the project.",
							fmt.Sprintf("Run `gitlab-cli repo branch list --project %s` to find the ref.", project)))
						if ae, ok := e.(*aicli.Error); ok && ae.Code == "E_NOT_FOUND" {
							ae.Details["parameter"] = "--ref"
						}
						return aicli.Preview{}, e
					}
					sha, createFrom, note = commit.ID, commit.ID, "creates the tag at this commit"
				}
				if err := want.checkMilestones(ctx, client, project); err != nil {
					return aicli.Preview{}, err
				}
				changes := append([]aicli.FieldChange{{Field: "commit_sha", After: sha, Note: note}, {Field: "tag_name", After: tag}},
					want.changes(nil)...)
				sort.SliceStable(changes, func(i, j int) bool { return changes[i].Field < changes[j].Field })
				return aicli.Preview{Action: fmt.Sprintf("Create release %s in %s", tag, project), Changes: changes,
					CanApply: len(blocks) == 0, BlockedBy: blocks}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				body := api.ReleaseCreateBody{TagName: tag, Ref: createFrom, Milestones: want.milestones}
				if want.set["name"] {
					body.Name = want.name
				}
				if want.set["description"] {
					body.Description = want.description
				}
				r, err := client.Releases.Create(ctx, project, body)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), projectNotFound())
				}
				obs := want.observed(*r)
				obs["tag_name"], obs["ref"] = r.TagName, releaseRef(project, r.TagName)
				if r.Commit != nil {
					obs["commit_sha"] = r.Commit.ID
				}
				return aicli.Result{Observed: obs, Data: releaseOut(project, *r, true), Untrusted: releaseUntrusted}, nil
			},
		})
	}}
	addReleaseTargetFlags(c)
	addReleaseContentFlags(c, "The ")
	c.Flags().String("ref", "", "Branch, tag or commit to create the tag from when it does not exist yet")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same create with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create a release for a tag, creating the tag from --ref when it does not exist",
		Aliases: []string{"add", "new"}, Effects: []aicli.Effect{aicli.Change}, Output: "release", Positional: "tag",
		FileInputs: map[string]string{"description-file": "description"},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{releaseEvidence, liveEvidence("release")},
		Examples: []string{"gitlab-cli release create v1.2.0 --project acme/web --ref main --name \"v1.2.0\" --description-file notes.md --dry-run",
			"gitlab-cli release create v1.2.0 --project acme/web --ref main --name \"v1.2.0\" --description-file notes.md --confirm <confirm_token>"}})
}

func addReleaseUpdate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "update <tag>", RunE: func(cmd *cobra.Command, _ []string) error {
		want, err := readReleaseFlags(app, cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		project, tag, err := releaseTarget(cmd, gl)
		if err != nil {
			return err
		}
		key := releaseRef(project, tag)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: key, Kind: "release"},
			ReadBack: "release get " + shellArg(key),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				cur, err := client.Releases.Get(ctx, project, tag)
				if err != nil {
					return aicli.Preview{}, readError(err, releaseNotFound(project))
				}
				if err := want.checkMilestones(ctx, client, project); err != nil {
					return aicli.Preview{}, err
				}
				return aicli.Preview{Action: fmt.Sprintf("Update release %s in %s", tag, project), Changes: want.changes(cur),
					CanApply: true, Untrusted: []string{"changes[].before"}}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				var body api.ReleaseUpdateBody
				if want.set["name"] {
					body.Name = ptr(want.name)
				}
				if want.set["description"] {
					body.Description = ptr(want.description)
				}
				if want.set["milestones"] {
					body.Milestones = ptr(want.milestones)
				}
				r, err := client.Releases.Update(ctx, project, tag, body)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), releaseNotFound(project))
				}
				return aicli.Result{Observed: want.observed(*r), Data: releaseOut(project, *r, true), Untrusted: releaseUntrusted}, nil
			},
		})
	}}
	addReleaseTargetFlags(c)
	addReleaseContentFlags(c, "New ")
	// An update with nothing to change is a usage error before the gate (CLI-029).
	c.MarkFlagsOneRequired("name", "description", "description-file", "milestone")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Change a release's name, notes or milestones",
		Aliases: []string{"edit"}, Effects: []aicli.Effect{aicli.Change}, Output: "release", Positional: "tag",
		FileInputs: map[string]string{"description-file": "description"},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{releaseEvidence, liveEvidence("release")},
		Examples: []string{"gitlab-cli release update v1.2.0 --project acme/web --description-file notes.md --dry-run",
			"gitlab-cli release update v1.2.0 --project acme/web --description-file notes.md --confirm <confirm_token>"}})
}

func addReleaseDelete(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "delete <tag>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		project, tag, err := releaseTarget(cmd, gl)
		if err != nil {
			return err
		}
		key := releaseRef(project, tag)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: key, Kind: "release"},
			ReadBack: "release get " + shellArg(key),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				// A missing release is E_NOT_FOUND at the dry-run, never a success (CLI-029).
				if _, err := client.Releases.Get(ctx, project, tag); err != nil {
					return aicli.Preview{}, readError(err, releaseNotFound(project))
				}
				return aicli.Preview{Action: fmt.Sprintf("Delete release %s from %s", tag, project), CanApply: true,
					Changes: []aicli.FieldChange{{Field: "exists", Before: true, After: false,
						Note: "the release, its notes and asset links are removed; the tag stays"}}}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				if err := client.Releases.Delete(ctx, project, tag); err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), releaseNotFound(project))
				}
				switch _, err := client.Releases.Get(ctx, project, tag); {
				case repoIsNotFound(err):
					return aicli.Result{Observed: map[string]any{"exists": false}, Data: map[string]any{"ref": key, "deleted": true}}, nil
				case err == nil:
					return aicli.Result{Observed: map[string]any{"exists": true}, Data: map[string]any{"ref": key, "deleted": false}}, nil
				}
				return aicli.Result{Data: map[string]any{"ref": key, "deleted": true}}, nil
			},
		})
	}}
	addReleaseTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Delete a release and its notes; the tag stays",
		Aliases: []string{"rm", "remove"}, Effects: []aicli.Effect{aicli.Change}, Dangerous: true, Output: "release_deleted",
		Positional: "tag", ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{releaseEvidence},
		Examples: []string{"gitlab-cli release delete v1.2.0 --project acme/web --dangerous --dry-run",
			"gitlab-cli release delete v1.2.0 --project acme/web --dangerous --confirm <confirm_token>"}})
}
