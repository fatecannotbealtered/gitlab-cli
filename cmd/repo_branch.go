package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// repo branch: list, get, create, delete. A branch is named by its name (feat/x),
// its ref (acme/web/-/branches/feat/x) or a web URL (/-/tree/feat/x or
// /-/branches/feat/x); the branch commands accept --branch for --name, the flag
// agents guess first (CLI-022). The root shortcut `branch` reaches them.

var repoBranchUntrusted = []string{"ref", "name", "commit_title"}

// repoBranchOut is a branch as every branch command returns it.
func repoBranchOut(project string, b api.Branch) map[string]any {
	out := map[string]any{"ref": repoBranchRef(project, b.Name), "name": b.Name, "default": b.Default, "protected": b.Protected,
		"merged": b.Merged, "developers_can_push": b.DevelopersCanPush, "developers_can_merge": b.DevelopersCanMerge,
		"can_push": b.CanPush, "web_url": b.WebURL, "commit_sha": nil, "commit_title": nil, "committed_date": nil}
	if b.Commit != nil {
		out["commit_sha"], out["commit_title"], out["committed_date"] = b.Commit.ID, b.Commit.Title, utcTime(b.Commit.CommittedDate)
	}
	return out
}

// repoParseBranchArg reads a branch as an agent has it: its name (feat/x), its ref
// (acme/web/-/branches/feat/x) or a web URL (/-/tree/feat/x, /-/branches/feat/x).
// project is "" when the input names none.
func repoParseBranchArg(raw string) (project, name string, err error) {
	arg, err := parseRepoWebArg(raw, "branches", "tree")
	if err != nil {
		return "", "", err
	}
	name, err = repoRefName(arg.rest, "--name", "branch")
	return arg.project, name, err
}

// repoBranchTarget reads the branch argument (or --name) and the project, and binds
// their canonical forms, so the name, the ref and the URL are one request (CLI-054).
func repoBranchTarget(cmd *cobra.Command, gl *lazyClient) (project, name string, err error) {
	raw, _ := cmd.Flags().GetString("name")
	if strings.TrimSpace(raw) == "" {
		e := aicli.Usage("missing_parameter", "--name", fmt.Sprintf("`%s` needs the branch: its name or ref.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Pass the branch as the argument, for example `%s feature-x --project acme/web`.", cmd.CommandPath())
		return "", "", e
	}
	refProject, name, err := repoParseBranchArg(raw)
	if err != nil {
		e := aicli.Validation("--name", "a branch name (feat/x), its ref (acme/web/-/branches/feat/x) or web URL",
			"That is not a branch name, ref or URL git accepts.")
		e.Details["next_action"] = "Run `gitlab-cli repo branch list --project <group/project>` to find the branch."
		return "", "", e
	}
	if project, err = projectFlag(cmd, refProject, gl); err != nil {
		return "", "", err
	}
	if err := aicli.SetResolved(cmd, "name", name); err != nil {
		return "", "", err
	}
	return project, name, nil
}

// addRepoBranchTargetFlags declares the flags that name one branch.
func addRepoBranchTargetFlags(c *cobra.Command) {
	c.Flags().String("name", "", "Branch name (feat/x), ref (acme/web/-/branches/feat/x) or web URL")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the branch is a ref")
}

func addRepoBranchList(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		query, err := repoQueryFlag(cmd)
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
		branches, pag, err := client.Repos.ListBranchesPage(cmd.Context(), project, api.BranchListOpts{Search: query,
			Page: req.Page, PerPage: req.Limit})
		if err != nil {
			return readError(err, projectNotFound())
		}
		items := make([]map[string]any, 0, len(branches))
		for _, b := range branches {
			items = append(items, repoBranchOut(project, b))
		}
		scope := map[string]any{"project": project}
		if query != "" {
			scope["query"] = query
		}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "name_asc"))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("query", "", "Only branches whose name contains this text (^ anchors the start, $ the end)")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List a project's branches with protection, merge state and last commit date",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "branch_page", ErrorCodes: gitlabReadCodes,
		RenamedFlags: map[string]string{"search": "query"},
		Maturity:     aicli.Verified, Evidence: []string{repoBranchEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo branch list --project acme/web",
			"gitlab-cli repo branch list --project acme/web --query feature --fields +commit_title"}})
}

// repoQueryFlag reads --query (1.x's --search too: Meta.RenamedFlags, CLI-028).
func repoQueryFlag(cmd *cobra.Command) (string, error) {
	query, _ := cmd.Flags().GetString("query")
	return strings.TrimSpace(query), nil
}

func addRepoBranchGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <name>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		project, name, err := repoBranchTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		b, err := client.Repos.GetBranch(cmd.Context(), project, name)
		if err != nil {
			return readError(err, repoBranchNotFound(project))
		}
		return app.OK(cmd, repoBranchOut(project, *b))
	}}
	addRepoBranchTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one branch: protection, merge state and its last commit",
		FlagAliases: map[string]string{"branch": "name"}, // the fleet's guess on branch commands (CLI-022, Q11)
		Aliases:     []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "branch", Positional: "name",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{repoBranchEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo branch get main --project acme/web", "gitlab-cli repo branch get acme/web/-/branches/feat/login"}})
}

func addRepoBranchCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create <name>", RunE: func(cmd *cobra.Command, _ []string) error {
		refRaw, _ := cmd.Flags().GetString("ref")
		from, err := repoRevision(refRaw, "--ref")
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		project, name, err := repoBranchTarget(cmd, gl)
		if err != nil {
			return err
		}
		ref := repoBranchRef(project, name)
		var sha string // the commit --ref names, as the latest Preview read it
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: ref, Kind: "branch"},
			ReadBack: "repo branch get " + shellArg(ref),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				// A repeat of an applied create with the same --idempotency-key never gets
				// here: the gate replays it first (CLI-062).
				var blocks []aicli.Block
				switch _, err := client.Repos.GetBranch(ctx, project, name); {
				case err == nil:
					blocks = append(blocks, aicli.Block{Kind: "already_exists", Message: "The project already has a branch with this name."})
				case !repoIsNotFound(err):
					return aicli.Preview{}, readError(err)
				}
				commit, err := client.Repos.GetCommit(ctx, project, from)
				if err != nil {
					if repoIsNotFound(err) {
						if perr := projectExists(ctx, client, project); perr != nil {
							return aicli.Preview{}, perr
						}
					}
					e := readError(err, onNotFound("--ref names no branch, tag or commit in the project.",
						fmt.Sprintf("Run `gitlab-cli repo branch list --project %s` to find the ref to branch from.", project)))
					if ae, ok := e.(*aicli.Error); ok && ae.Code == "E_NOT_FOUND" {
						ae.Details["parameter"] = "--ref"
					}
					return aicli.Preview{}, e
				}
				sha = commit.ID
				// The branch starts at the commit --ref names now; Apply sends that SHA, so
				// a --ref that moves after the dry-run is state_drift, not a surprise.
				return aicli.Preview{Action: fmt.Sprintf("Create branch %s in %s at commit %s", name, project, commit.ShortID),
					Changes:  []aicli.FieldChange{{Field: "name", After: name}, {Field: "commit_sha", After: commit.ID}},
					CanApply: len(blocks) == 0, BlockedBy: blocks}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				b, err := client.Repos.CreateBranch(ctx, project, name, sha)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"))
				}
				obs := map[string]any{"name": b.Name, "ref": repoBranchRef(project, b.Name)}
				if b.Commit != nil {
					obs["commit_sha"] = b.Commit.ID
				}
				return aicli.Result{Observed: obs, Data: repoBranchOut(project, *b), Untrusted: repoBranchUntrusted}, nil
			},
		})
	}}
	addRepoBranchTargetFlags(c)
	c.Flags().String("ref", "", "Branch, tag or commit SHA to start the branch from")
	_ = c.MarkFlagRequired("ref")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same create with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create a branch at the commit a branch, tag or SHA names",
		FlagAliases: map[string]string{"branch": "name"}, // the fleet's guess on branch commands (CLI-022, Q11)
		Aliases:     []string{"add", "new"}, Effects: []aicli.Effect{aicli.Change}, Output: "branch", Positional: "name",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{repoBranchEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo branch create feature-x --project acme/web --ref main --dry-run",
			"gitlab-cli repo branch create feature-x --project acme/web --ref main --confirm <confirm_token>"}})
}

func addRepoBranchDelete(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "delete <name>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		project, name, err := repoBranchTarget(cmd, gl)
		if err != nil {
			return err
		}
		ref := repoBranchRef(project, name)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: ref, Kind: "branch"},
			ReadBack: "repo branch get " + shellArg(ref),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				// A missing branch is E_NOT_FOUND at the dry-run, never a success (CLI-029).
				cur, err := client.Repos.GetBranch(ctx, project, name)
				if err != nil {
					return aicli.Preview{}, readError(err, repoBranchNotFound(project))
				}
				var blocks []aicli.Block
				if cur.Default {
					blocks = append(blocks, aicli.Block{Kind: "default_branch", Message: "This is the project's default branch; GitLab refuses to delete it."})
				}
				if cur.Protected {
					blocks = append(blocks, aicli.Block{Kind: "protected_branch",
						Message: "The branch is protected; GitLab refuses to delete it until a maintainer unprotects it."})
				}
				note := "deleted; commits on no other branch or tag stay reachable only by SHA"
				if cur.Merged {
					note = "deleted; its commits are already merged into the default branch"
				}
				// The branch's head is the base: a push after the dry-run is state_drift,
				// so work pushed since the plan is never deleted unseen.
				base := ""
				if cur.Commit != nil {
					base = cur.Commit.ID
				}
				return aicli.Preview{Action: fmt.Sprintf("Delete branch %s from %s", name, project),
					Changes:     []aicli.FieldChange{{Field: "exists", Before: true, After: false, Note: note}},
					BaseVersion: base, BaseSource: "commit_sha", CanApply: len(blocks) == 0, BlockedBy: blocks}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				if err := client.Repos.DeleteBranch(ctx, project, name); err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), repoBranchNotFound(project))
				}
				switch _, err := client.Repos.GetBranch(ctx, project, name); {
				case repoIsNotFound(err):
					return aicli.Result{Observed: map[string]any{"exists": false}, Data: map[string]any{"ref": ref, "deleted": true}}, nil
				case err == nil:
					return aicli.Result{Observed: map[string]any{"exists": true}, Data: map[string]any{"ref": ref, "deleted": false}}, nil
				}
				return aicli.Result{Data: map[string]any{"ref": ref, "deleted": true}}, nil
			},
		})
	}}
	addRepoBranchTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Delete a branch; protected and default branches are refused",
		FlagAliases: map[string]string{"branch": "name"}, // the fleet's guess on branch commands (CLI-022, Q11)
		Aliases:     []string{"rm", "remove"}, Effects: []aicli.Effect{aicli.Change}, Dangerous: true, Output: "branch_deleted",
		Positional: "name", ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{repoBranchEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo branch delete old-experiment --project acme/web --dangerous --dry-run",
			"gitlab-cli repo branch delete old-experiment --project acme/web --dangerous --confirm <confirm_token>"}})
}
