package cmd

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/gitctx"
)

// The pipeline group: list, get (with --wait), current, create, retry, cancel, jobs,
// and wait (deprecated for get --wait). A pipeline is named by its global ID, the
// number in its web URL (/-/pipelines/<id>).

var (
	pipelineFields = []string{"ref", "id", "iid", "ref_name", "sha", "status", "finished", "source", "name", "tag", "user",
		"created_at", "updated_at", "started_at", "finished_at", "duration_ms", "queued_duration_ms", "coverage",
		"yaml_errors", "web_url"}
	pipelineDefaultFields = []string{"ref", "ref_name", "status", "source", "sha", "created_at", "web_url"}
	// A branch or tag name, .gitlab-ci.yml's workflow name and GitLab's quote of its
	// errors are written by people (CLI-046).
	pipelineUntrusted = []string{"ref_name", "name", "yaml_errors"}
)

func registerPipeline(app *aicli.App) {
	group := addGroup(app, "pipeline", "List, read, run, retry and cancel CI/CD pipelines, and list their jobs", "pipelines", "ci")
	app.Schema("pipeline", aicli.SchemaDef{Shape: "object", Fields: pipelineFields, Untrusted: pipelineUntrusted})
	app.Schema("pipeline_page", aicli.SchemaDef{Shape: "page", Fields: pipelineFields, DefaultFields: pipelineDefaultFields,
		Untrusted: pipelineUntrusted})
	addPipelineList(app, group)
	addPipelineGet(app, group)
	addPipelineCurrent(app, group)
	addPipelineCreate(app, group)
	addPipelineRetry(app, group)
	addPipelineCancel(app, group)
	addPipelineJobs(app, group)
	addPipelineWait(app, group)
}

// pipelineOut is a pipeline as every pipeline command returns it. The list endpoint
// lacks the detail fields; they are left out rather than reported empty.
func pipelineOut(project string, p api.Pipeline, detail bool) map[string]any {
	out := map[string]any{
		"ref": pipelineRef(project, p.ID), "id": idString(p.ID), "iid": idString(p.IID), "ref_name": p.Ref, "sha": p.SHA,
		"status": p.Status, "finished": ciFinished(p.Status), "source": p.Source, "name": optString(p.Name),
		"web_url": p.WebURL, "created_at": utcTime(p.CreatedAt), "updated_at": utcTime(p.UpdatedAt),
	}
	if detail {
		out["tag"] = p.Tag
		out["user"] = ciUser(p.User)
		out["started_at"] = utcTime(p.StartedAt)
		out["finished_at"] = utcTime(p.FinishedAt)
		out["duration_ms"] = ciMillis(p.Duration)
		out["queued_duration_ms"] = ciMillis(p.QueuedDuration)
		out["coverage"] = nil
		if p.Coverage != nil {
			if f, err := strconv.ParseFloat(strings.TrimSpace(*p.Coverage), 64); err == nil {
				out["coverage"] = f
			}
		}
		out["yaml_errors"] = nil
		if p.YamlErrors != nil && *p.YamlErrors != "" {
			out["yaml_errors"] = *p.YamlErrors
		}
	}
	return out
}

// pipelineNotFound is the 404 of a pipeline read.
func pipelineNotFound(project string) errOpt {
	return onNotFound("The project has no pipeline with that ID, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli pipeline list --project %s` to find the pipeline.", project))
}

// pipelineTarget reads the pipeline positional (or --pipeline) and the project.
func pipelineTarget(cmd *cobra.Command, gl *lazyClient) (ciTarget, error) {
	return ciIDTarget(cmd, gl, "pipeline", "pipelines", "pipeline", "gitlab-cli pipeline list --project <group/project>")
}

func addPipelineTargetFlags(c *cobra.Command) {
	c.Flags().String("pipeline", "", "Pipeline ID, ref (acme/web/-/pipelines/123) or web URL")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the pipeline is a ref or URL")
}

func addPipelineList(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		state, err := ciStateFilter(cmd)
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
		username, err := pipelineUsername(cmd, client)
		if err != nil {
			return err
		}
		ref, _ := cmd.Flags().GetString("ref")
		ref = strings.TrimSpace(ref)
		opts := api.PipelineListOpts{Ref: ref, Username: username, Page: req.Page, PerPage: req.Limit}
		if state != "all" {
			opts.Status = state
		}
		ps, pag, err := client.Pipelines.ListPage(cmd.Context(), project, opts)
		if err != nil {
			return readError(err, projectNotFound())
		}
		items := make([]map[string]any, 0, len(ps))
		for _, p := range ps {
			items = append(items, pipelineOut(project, p, false))
		}
		scope := map[string]any{"project": project, "state": state}
		if ref != "" {
			scope["ref_name"] = ref
		}
		if username != "" {
			scope["username"] = username
		}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "id_desc"))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("ref", "", "Only pipelines for this branch or tag")
	c.Flags().String("state", "all", "Only pipelines in this state, or all")
	c.Flags().String("username", "", "Only pipelines this user triggered: carol, @carol or me")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List a project's pipelines, newest first, by branch, state or user",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "pipeline_page",
		Params:       map[string]aicli.ParamDoc{"state": {Enum: append(append([]string{}, pipelineStates...), "all")}},
		RenamedFlags: map[string]string{"status": "state"},
		ErrorCodes:   gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{pipelineEvidence, liveEvidence("pipeline")},
		Examples: []string{"gitlab-cli pipeline list --project acme/web --ref main --state failed --limit 1",
			"gitlab-cli pipeline list --project acme/web --username me"}})
}

// ciStateFilter reads --state (1.x's --status too: Meta.RenamedFlags, CLI-028).
func ciStateFilter(cmd *cobra.Command) (string, error) {
	state, _ := cmd.Flags().GetString("state")
	return state, nil
}

// pipelineUsername reads --username: carol, @carol, or me (this account). A name
// GitLab does not know filters to an empty list, as GitLab does (CLI-029).
func pipelineUsername(cmd *cobra.Command, c *api.Client) (string, error) {
	raw, _ := cmd.Flags().GetString("username")
	name := strings.TrimPrefix(strings.TrimSpace(raw), "@")
	if name != "me" {
		return name, nil
	}
	u, err := resolveUser(cmd.Context(), c, "me", "--username")
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func addPipelineGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <pipeline>", RunE: func(cmd *cobra.Command, _ []string) error {
		wait, err := ciWaitFlag(cmd)
		if err != nil {
			return err
		}
		return pipelineShow(app, cmd, wait, ciPollEvery)
	}}
	addPipelineTargetFlags(c)
	c.Flags().String("wait", "", "Wait until the pipeline finishes, at most this long (up to 90s): 60s, 90s")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one pipeline by ID, ref or URL; --wait waits up to 90s for it to finish",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "pipeline", Positional: "pipeline",
		Params:     map[string]aicli.ParamDoc{"wait": {Unit: "duration"}},
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{pipelineEvidence, liveEvidence("pipeline")},
		Examples: []string{"gitlab-cli pipeline get 123 --project acme/web",
			"gitlab-cli pipeline get acme/web/-/pipelines/123 --wait 90s"}})
}

// pipelineShow reads one pipeline and, with wait, reads it again until it finishes
// or the wait ends (CLI-049); a wait that ends first says so in a notice.
func pipelineShow(app *aicli.App, cmd *cobra.Command, wait, poll time.Duration) error {
	gl := gitlabFor(app)
	t, err := pipelineTarget(cmd, gl)
	if err != nil {
		return err
	}
	client, err := gl.get()
	if err != nil {
		return err
	}
	read := func(ctx context.Context) (*api.Pipeline, error) {
		p, err := client.Pipelines.Get(ctx, t.project, t.id)
		if err != nil {
			return nil, readError(err, pipelineNotFound(t.project))
		}
		return p, nil
	}
	p, err := read(cmd.Context())
	if err != nil {
		return err
	}
	if wait > 0 && !ciFinished(p.Status) {
		finished, err := ciWait(cmd.Context(), wait, poll, func(ctx context.Context) (string, error) {
			np, err := read(ctx)
			if err != nil {
				return "", err
			}
			p = np
			return np.Status, nil
		})
		if err != nil {
			return err
		}
		if !finished {
			ciWaitEnded(app, "pipeline", wait)
		}
	}
	return app.OK(cmd, pipelineOut(t.project, *p, true))
}

func addPipelineCurrent(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "current", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		project, ref, err := pipelineCurrentInputs(app, cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		ps, _, err := client.Pipelines.ListPage(cmd.Context(), project, api.PipelineListOpts{Ref: ref, Page: 1, PerPage: 1})
		if err != nil {
			return readError(err, projectNotFound())
		}
		if len(ps) == 0 {
			return aicli.NotFound("No pipeline has run for this branch or tag yet.",
				fmt.Sprintf("Run `gitlab-cli pipeline create --project %s --ref %s --dry-run` to start one.", project, shellArg(ref)))
		}
		p, err := client.Pipelines.Get(cmd.Context(), project, ps[0].ID)
		if err != nil {
			return readError(err, pipelineNotFound(project))
		}
		return app.OK(cmd, pipelineOut(project, *p, true))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; default: the current directory's GitLab remote")
	c.Flags().String("ref", "", "Branch or tag; default: the current directory's git branch")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show the latest pipeline of a branch, by default the current directory's git branch",
		Effects: []aicli.Effect{aicli.Observe}, Output: "pipeline",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Experimental, Evidence: []string{pipelineEvidence},
		Examples: []string{"gitlab-cli pipeline current", "gitlab-cli pipeline current --project acme/web --ref main"}})
}

// pipelineCurrentInputs is the project and branch of pipeline current: the flags,
// else the git remote and branch of the current directory (local only). A remote on
// another host than the configured GitLab is not used: its path names a project of
// another server.
func pipelineCurrentInputs(app *aicli.App, cmd *cobra.Command, gl *lazyClient) (string, string, error) {
	ref, _ := cmd.Flags().GetString("ref")
	ref = strings.TrimSpace(ref)
	projectGiven := cmd.Flags().Changed("project")
	var git *gitctx.Context
	if !projectGiven || ref == "" {
		git, _ = gitctx.Detect("")
	}
	gitProject := ""
	if !projectGiven {
		p, err := pipelineGitProject(app, cmd, git)
		if err != nil {
			return "", "", err
		}
		gitProject = p
	}
	project, err := projectFlag(cmd, gitProject, gl)
	if err != nil {
		return "", "", err
	}
	if ref == "" {
		if git == nil || git.CurrentBranch == "" {
			e := aicli.Usage("missing_parameter", "--ref", "The current directory has no current git branch, so pass --ref.")
			e.Details["next_action"] = fmt.Sprintf("Add --ref <branch>, for example `gitlab-cli pipeline current --project %s --ref main`.", project)
			return "", "", e
		}
		ref = git.CurrentBranch
	}
	return project, ref, nil
}

// pipelineGitProject is the project path of the current directory's GitLab remote,
// when the remote is on the configured host.
func pipelineGitProject(app *aicli.App, cmd *cobra.Command, git *gitctx.Context) (string, error) {
	missing := func(why string) error {
		e := aicli.Usage("missing_parameter", "--project", why)
		e.Details["next_action"] = "Add --project <group/project>, for example `gitlab-cli pipeline current --project acme/web --ref main`."
		return e
	}
	if git == nil || git.Remote == nil {
		return "", missing("The current directory has no GitLab git remote, so pass --project.")
	}
	host, _, e := app.Setting(cmd, "host")
	if e != nil {
		return "", e
	}
	u, err := url.Parse(strings.TrimSpace(host))
	if err != nil || !strings.EqualFold(u.Hostname(), git.Remote.Host) {
		return "", missing("The current directory's git remote is on another host than the configured GitLab, so pass --project.")
	}
	p, numeric, err := parseProject(git.Remote.ProjectPath)
	if err != nil || numeric {
		return "", missing("The current directory's git remote does not name a project path, so pass --project.")
	}
	return p, nil
}

func addPipelineCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create", RunE: func(cmd *cobra.Command, _ []string) error {
		ref, _ := cmd.Flags().GetString("ref")
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return aicli.Validation("--ref", "a branch or tag name", "--ref is empty.")
		}
		keys, values, err := ciKeyValues(cmd, "variable")
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		vars := make([]api.PipelineVariable, len(keys))
		for i := range keys {
			vars[i] = api.PipelineVariable{Key: keys[i], Value: values[i]}
		}
		// The variables' values may be secrets: the preview names the keys only (the
		// flag is in Meta.Secrets, so the inputs show *** and bind a keyed digest).
		changes := []aicli.FieldChange{{Field: "ref_name", After: ref}}
		if len(keys) > 0 {
			changes = append(changes, aicli.FieldChange{Field: "variables", After: ciSortedKeys(keys), Note: "the values are not shown"})
		}
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: project + "/-/pipelines", Kind: "pipelines"},
			ReadBack: "pipeline list --project " + project + " --ref " + shellArg(ref) + " --limit 1",
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				if err := projectExists(ctx, client, project); err != nil {
					return aicli.Preview{}, err
				}
				return aicli.Preview{Action: fmt.Sprintf("Run a new pipeline for %s in %s", ref, project), CanApply: true,
					Changes: changes}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				p, err := client.Pipelines.Create(ctx, project, api.PipelineCreateBody{Ref: ref, Variables: vars})
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), projectNotFound())
				}
				// The new pipeline's ref is observed too, so `operation show` finds it
				// after a lost reply (CLI-059).
				obs := map[string]any{"ref_name": p.Ref, "ref": pipelineRef(project, p.ID), "status": p.Status}
				return aicli.Result{Observed: obs, Data: pipelineOut(project, *p, true), Untrusted: pipelineUntrusted}, nil
			},
		})
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("ref", "", "Branch or tag to run the pipeline for")
	_ = c.MarkFlagRequired("ref")
	c.Flags().StringArray("variable", nil, "Pipeline variable KEY=VALUE, repeatable; values are secret: never shown or stored")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same create with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Run a new pipeline for a branch or tag, optionally with pipeline variables",
		Aliases: []string{"add", "new", "run"}, Effects: []aicli.Effect{aicli.Change}, Output: "pipeline",
		Secrets: []string{"variable"}, ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{pipelineEvidence, liveEvidence("pipeline")},
		Examples: []string{"gitlab-cli pipeline create --project acme/web --ref main --dry-run",
			"gitlab-cli pipeline create --project acme/web --ref main --confirm <confirm_token>"}})
}

// pipelineAction is a retry or a cancel: the statuses it applies to, why it is
// blocked otherwise, and the statuses GitLab's answer shows when it worked.
type pipelineAction struct {
	verb, note       string
	applies, worked  []string
	blockKind, block string
	send             func(ctx context.Context, c *api.Client, project string, id int) (*api.Pipeline, error)
}

// pipelineWrite runs a retry or a cancel through the gate. The base version is the
// status: a running pipeline's updated_at moves with every job, which would make
// every confirm a state_drift, while a changed status changes the plan.
func pipelineWrite(app *aicli.App, cmd *cobra.Command, a pipelineAction) error {
	gl := gitlabFor(app)
	t, err := pipelineTarget(cmd, gl)
	if err != nil {
		return err
	}
	ref := pipelineRef(t.project, t.id)
	return app.Write(cmd, aicli.WriteSpec{
		Target:   aicli.Target{Key: ref, Kind: "pipeline"},
		ReadBack: "pipeline get " + ref,
		Preview: func(ctx context.Context) (aicli.Preview, error) {
			client, err := gl.get()
			if err != nil {
				return aicli.Preview{}, err
			}
			cur, err := client.Pipelines.Get(ctx, t.project, t.id)
			if err != nil {
				return aicli.Preview{}, readError(err, pipelineNotFound(t.project))
			}
			pv := aicli.Preview{Action: fmt.Sprintf("%s pipeline %s", a.verb, ref), CanApply: true,
				Changes:     []aicli.FieldChange{{Field: "status", Before: cur.Status, Note: a.note}},
				BaseVersion: cur.Status, BaseSource: "status"}
			if !slices.Contains(a.applies, cur.Status) {
				pv.CanApply, pv.BlockedBy = false, []aicli.Block{{Kind: a.blockKind, Message: a.block}}
			}
			return pv, nil
		},
		Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
			client, err := gl.get()
			if err != nil {
				return aicli.Result{}, aicli.NotDispatched(err)
			}
			p, err := a.send(ctx, client, t.project, t.id)
			if err != nil {
				return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), pipelineNotFound(t.project))
			}
			res := aicli.Result{Observed: map[string]any{"status": p.Status}, Data: pipelineOut(t.project, *p, true),
				Untrusted: pipelineUntrusted}
			res.Outcome.Verification, res.Mismatched = ciVerifyStatus(p.Status, a.worked)
			return res, nil
		},
	})
}

func addPipelineRetry(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "retry <pipeline>", RunE: func(cmd *cobra.Command, _ []string) error {
		return pipelineWrite(app, cmd, pipelineAction{verb: "Retry", note: "its failed and canceled jobs run again",
			applies: []string{"failed", "canceled"}, worked: ciActiveStates,
			blockKind: "not_retryable", block: "Only a failed or canceled pipeline can be retried.",
			send: func(ctx context.Context, c *api.Client, project string, id int) (*api.Pipeline, error) {
				return c.Pipelines.Retry(ctx, project, id)
			}})
	}}
	addPipelineTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Run the failed and canceled jobs of a pipeline again",
		Effects: []aicli.Effect{aicli.Change}, Output: "pipeline", Positional: "pipeline", ErrorCodes: gitlabWriteCodes,
		Maturity: aicli.Verified, Evidence: []string{pipelineEvidence, liveEvidence("pipeline")},
		Examples: []string{"gitlab-cli pipeline retry 123 --project acme/web --dry-run",
			"gitlab-cli pipeline retry 123 --project acme/web --confirm <confirm_token>"}})
}

func addPipelineCancel(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "cancel <pipeline>", RunE: func(cmd *cobra.Command, _ []string) error {
		return pipelineWrite(app, cmd, pipelineAction{verb: "Cancel", note: "its pending and running jobs stop",
			applies: ciActiveStates, worked: ciCanceledStates,
			blockKind: "not_cancelable", block: "The pipeline has finished; only a pending or running pipeline can be canceled.",
			send: func(ctx context.Context, c *api.Client, project string, id int) (*api.Pipeline, error) {
				return c.Pipelines.Cancel(ctx, project, id)
			}})
	}}
	addPipelineTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Cancel a pending or running pipeline (pipeline retry runs it again)",
		Effects: []aicli.Effect{aicli.Change}, Output: "pipeline", Positional: "pipeline", ErrorCodes: gitlabWriteCodes,
		Maturity: aicli.Experimental, Evidence: []string{pipelineEvidence},
		Examples: []string{"gitlab-cli pipeline cancel 123 --project acme/web --dry-run",
			"gitlab-cli pipeline cancel 123 --project acme/web --confirm <confirm_token>"}})
}

func addPipelineJobs(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "jobs <pipeline>", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		states, err := pipelineJobStates(cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		t, err := pipelineTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		jobs, pag, err := client.Pipelines.JobsPage(cmd.Context(), t.project, t.id,
			api.PipelineJobsOpts{Scope: states, Page: req.Page, PerPage: req.Limit})
		if err != nil {
			return readError(err, pipelineNotFound(t.project))
		}
		items := make([]map[string]any, 0, len(jobs))
		for _, j := range jobs {
			items = append(items, jobOut(t.project, j))
		}
		scope := map[string]any{"project": t.project, "pipeline": pipelineRef(t.project, t.id), "state": "all",
			"covers": "the latest attempt of each job; trigger jobs and earlier attempts of retried jobs are not included"}
		if len(states) > 0 {
			scope["state"] = states
		}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, ""))
	}}
	addPipelineTargetFlags(c)
	c.Flags().StringSlice("state", nil, "Only jobs in these states, comma-separated (failed,canceled), or all")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List the jobs of a pipeline, with each failed job's failure_reason",
		Effects: []aicli.Effect{aicli.Observe}, Output: "job_page", Positional: "pipeline",
		Params:       map[string]aicli.ParamDoc{"state": {Enum: append(append([]string{}, jobStates...), "all")}},
		RenamedFlags: map[string]string{"scope": "state"},
		ErrorCodes:   gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{pipelineEvidence, liveEvidence("pipeline")},
		Examples: []string{"gitlab-cli pipeline jobs 123 --project acme/web --state failed",
			"gitlab-cli pipeline jobs acme/web/-/pipelines/123"}})
}

// pipelineJobStates reads --state (1.x's --scope too: Meta.RenamedFlags); all (or
// none) is no filter.
func pipelineJobStates(cmd *cobra.Command) ([]string, error) {
	states, _ := cmd.Flags().GetStringSlice("state")
	if slices.Contains(states, "all") {
		return nil, nil
	}
	return states, nil
}

func addPipelineWait(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "wait <pipeline>", RunE: func(cmd *cobra.Command, _ []string) error {
		wait, poll := ciDeprecatedWait(app, cmd)
		return pipelineShow(app, cmd, wait, poll)
	}}
	addPipelineTargetFlags(c)
	c.Flags().Int("timeout", 0, "Seconds to wait, at most 90 (0: 90)")
	c.Flags().Int("interval", 0, "Seconds between status reads")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Wait up to 90s for a pipeline to finish and show it",
		Deprecated: "use `pipeline get <pipeline> --wait <duration>`",
		Effects:    []aicli.Effect{aicli.Observe}, Output: "pipeline", Positional: "pipeline",
		Params:     map[string]aicli.ParamDoc{"timeout": {Unit: "seconds"}, "interval": {Unit: "seconds"}},
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Experimental, Evidence: []string{pipelineEvidence},
		Examples: []string{"gitlab-cli pipeline wait 123 --project acme/web --timeout 60"}})
}
