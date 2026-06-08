package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/gitctx"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var pipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Manage CI/CD pipelines",
}

var pipelineListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pipelines for a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		ref, _ := cmd.Flags().GetString("ref")
		status, _ := cmd.Flags().GetString("status")
		username, _ := cmd.Flags().GetString("username")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		pipelines, err := client.Pipelines.List(apiCtx(), project, &api.PipelineListOpts{
			Ref:      ref,
			Status:   status,
			Username: username,
			Limit:    limit,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			flat := make([]map[string]any, len(pipelines))
			for i, p := range pipelines {
				fp := toFlatPipeline(&p)
				flat[i] = output.FilterMap(output.PipelineToMap(fp), fields)
			}
			printSimpleListJSON(cmd, flat, limit)
			return nil
		}
		if len(pipelines) == 0 {
			output.Info("No pipelines found.")
			return nil
		}
		headers := []string{"ID", "IID", "REF", "STATUS", "SOURCE", "CREATED"}
		rows := make([][]string, len(pipelines))
		for i, p := range pipelines {
			rows[i] = []string{
				strconv.Itoa(p.ID),
				strconv.Itoa(p.IID),
				p.Ref,
				output.StatusBadge(p.Status),
				p.Source,
				p.CreatedAt,
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

var pipelineGetCmd = &cobra.Command{
	Use:   "get <pipeline_id>",
	Short: "Get a pipeline by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		pipelineID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("pipeline_id must be an integer")
		}
		p, err := client.Pipelines.Get(apiCtx(), project, pipelineID)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.PipelineToMap(toFlatPipeline(p)), fields))
			return nil
		}
		printPipelineDetail(p)
		return nil
	},
}

var pipelineCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Get the latest pipeline for the current branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, _ := gitctx.Detect("")
		if ctx.Remote == nil || ctx.CurrentBranch == "" {
			return failNotFound("not in a GitLab git repository or no current branch")
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		pipelines, err := client.Pipelines.List(apiCtx(), ctx.Remote.ProjectPath, &api.PipelineListOpts{
			Ref:   ctx.CurrentBranch,
			Limit: 1,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if len(pipelines) == 0 {
			if jsonMode {
				return failNotFound(fmt.Sprintf("no pipelines found for branch %q", ctx.CurrentBranch))
			}
			output.Info(fmt.Sprintf("No pipelines found for branch %q.", ctx.CurrentBranch))
			setExitCode(ExitNotFound)
			return ErrSilent
		}
		p := &pipelines[0]
		if jsonMode {
			output.PrintJSON(output.PipelineToMap(toFlatPipeline(p)))
			return nil
		}
		printPipelineDetail(p)
		return nil
	},
}

var pipelineCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create (trigger) a new pipeline",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		ref, _ := cmd.Flags().GetString("ref")
		if ref == "" {
			return failArg("--ref is required")
		}
		varFlags, _ := cmd.Flags().GetStringArray("variable")
		vars, err := parseVariables(varFlags)
		if err != nil {
			return err
		}

		confirmPayload := map[string]any{"project": project, "ref": ref, "variables": varFlags}
		if done, err := prepareWrite(cmd, "create pipeline", confirmPayload); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		p, err := client.Pipelines.Create(apiCtx(), project, api.PipelineCreateBody{Ref: ref, Variables: vars})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.PipelineToMap(toFlatPipeline(p)))
			return nil
		}
		output.Success(fmt.Sprintf("Pipeline #%d created (status: %s)", p.ID, p.Status))
		output.Info(p.WebURL)
		return nil
	},
}

var pipelineRetryCmd = &cobra.Command{
	Use:   "retry <pipeline_id>",
	Short: "Retry a pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		pipelineID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("pipeline_id must be an integer")
		}
		if done, err := prepareWrite(cmd, "retry pipeline", map[string]any{"project": project, "pipeline_id": pipelineID}); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		p, err := client.Pipelines.Retry(apiCtx(), project, pipelineID)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.PipelineToMap(toFlatPipeline(p)))
			return nil
		}
		output.Success(fmt.Sprintf("Pipeline #%d retried (status: %s)", p.ID, p.Status))
		return nil
	},
}

var pipelineCancelCmd = &cobra.Command{
	Use:   "cancel <pipeline_id>",
	Short: "Cancel a pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		pipelineID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("pipeline_id must be an integer")
		}
		confirmPayload := map[string]any{"project": project, "pipeline_id": pipelineID}
		if done, err := prepareWrite(cmd, "cancel pipeline", confirmPayload); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		p, err := client.Pipelines.Cancel(apiCtx(), project, pipelineID)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.PipelineToMap(toFlatPipeline(p)))
			return nil
		}
		output.Success(fmt.Sprintf("Pipeline #%d canceled (status: %s)", p.ID, p.Status))
		return nil
	},
}

var pipelineJobsCmd = &cobra.Command{
	Use:   "jobs <pipeline_id>",
	Short: "List jobs for a pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		pipelineID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("pipeline_id must be an integer")
		}
		scopeStr, _ := cmd.Flags().GetString("scope")
		var scope []string
		if scopeStr != "" {
			scope = strings.Split(scopeStr, ",")
		}

		jobs, err := client.Pipelines.Jobs(apiCtx(), project, pipelineID, scope)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			flat := make([]map[string]any, len(jobs))
			for i, j := range jobs {
				flat[i] = output.FilterMap(output.JobToMap(toFlatJob(&j)), fields)
			}
			printSimpleListJSON(cmd, flat, len(flat))
			return nil
		}
		if len(jobs) == 0 {
			output.Info("No jobs found.")
			return nil
		}
		headers := []string{"ID", "NAME", "STAGE", "STATUS", "DURATION"}
		rows := make([][]string, len(jobs))
		for i, j := range jobs {
			rows[i] = []string{
				strconv.Itoa(j.ID),
				j.Name,
				j.Stage,
				output.StatusBadge(j.Status),
				fmt.Sprintf("%.1fs", j.Duration),
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

var pipelineWaitCmd = &cobra.Command{
	Use:   "wait <pipeline_id>",
	Short: "Wait for a pipeline to reach a terminal status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		pipelineID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("pipeline_id must be an integer")
		}
		timeoutSec, _ := cmd.Flags().GetInt("timeout")
		intervalSec, _ := cmd.Flags().GetInt("interval")
		interval := time.Duration(intervalSec) * time.Second

		client, _, err := newClient()
		if err != nil {
			return err
		}

		start := time.Now()
		for {
			p, err := client.Pipelines.Get(apiCtx(), project, pipelineID)
			if err != nil {
				return handleAPIError(err, jsonMode)
			}
			elapsed := time.Since(start)
			switch p.Status {
			case "success", "failed", "canceled", "skipped", "manual":
				if jsonMode {
					output.PrintJSON(output.PipelineToMap(toFlatPipeline(p)))
				} else {
					fmt.Printf("Pipeline #%d finished: %s\n", p.ID, output.StatusBadge(p.Status))
				}
				if p.Status != "success" {
					setExitCode(ExitCIFailed)
					return ErrSilent
				}
				return nil
			}
			if timeoutSec > 0 && elapsed >= time.Duration(timeoutSec)*time.Second {
				return failWithCode(fmt.Sprintf("timed out waiting for pipeline #%d", pipelineID), ExitTimeout, output.ErrTimeout)
			}
			if !jsonMode && !quietMode {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Waiting... status=%s (%.0fs elapsed)\n", p.Status, elapsed.Seconds())
			}
			if err := sleepContext(cmd.Context(), interval); err != nil {
				return failWithCode(err.Error(), ExitNetwork, output.ErrNetwork)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(pipelineCmd)

	// list
	pipelineListCmd.Flags().String("project", "", "Project ID or path (required)")
	pipelineListCmd.Flags().String("ref", "", "Filter by branch/tag")
	pipelineListCmd.Flags().String("status", "", "Filter by status: created|pending|running|success|failed|canceled|skipped|manual")
	pipelineListCmd.Flags().String("username", "", "Filter by triggering username")
	pipelineListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	pipelineListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	pipelineCmd.AddCommand(pipelineListCmd)

	// get
	pipelineGetCmd.Flags().String("project", "", "Project ID or path (required)")
	pipelineGetCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	pipelineCmd.AddCommand(pipelineGetCmd)

	// current
	pipelineCmd.AddCommand(pipelineCurrentCmd)

	// create
	pipelineCreateCmd.Flags().String("project", "", "Project ID or path (required)")
	pipelineCreateCmd.Flags().String("ref", "", "Branch or tag to run pipeline on (required)")
	pipelineCreateCmd.Flags().StringArray("variable", nil, "Pipeline variable in KEY=VAL format (repeatable)")
	pipelineCmd.AddCommand(pipelineCreateCmd)
	markWrite(pipelineCreateCmd)
	markConfirm(pipelineCreateCmd)
	markRiskLevel(pipelineCreateCmd, "high")

	// retry
	pipelineRetryCmd.Flags().String("project", "", "Project ID or path (required)")
	pipelineCmd.AddCommand(pipelineRetryCmd)
	markWrite(pipelineRetryCmd)

	// cancel
	pipelineCancelCmd.Flags().String("project", "", "Project ID or path (required)")
	pipelineCmd.AddCommand(pipelineCancelCmd)
	markWrite(pipelineCancelCmd)
	markConfirm(pipelineCancelCmd)
	markRiskLevel(pipelineCancelCmd, "high")

	// jobs
	pipelineJobsCmd.Flags().String("project", "", "Project ID or path (required)")
	pipelineJobsCmd.Flags().String("scope", "", "Comma-separated job scopes: created|pending|running|failed|success|canceled|skipped|manual")
	pipelineJobsCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	pipelineCmd.AddCommand(pipelineJobsCmd)

	// wait
	pipelineWaitCmd.Flags().String("project", "", "Project ID or path (required)")
	pipelineWaitCmd.Flags().Int("timeout", 0, "Timeout in seconds (0 = unlimited)")
	pipelineWaitCmd.Flags().Int("interval", 10, "Poll interval in seconds")
	pipelineCmd.AddCommand(pipelineWaitCmd)
}

// toFlatPipeline converts an api.Pipeline to output.FlatPipeline.
func toFlatPipeline(p *api.Pipeline) output.FlatPipeline {
	fp := output.FlatPipeline{
		ID:        p.ID,
		IID:       p.IID,
		ProjectID: p.ProjectID,
		Ref:       p.Ref,
		SHA:       p.SHA,
		Status:    p.Status,
		Source:    p.Source,
		WebURL:    p.WebURL,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
	if p.User != nil {
		fp.Username = p.User.Username
	}
	return fp
}

// toFlatJob converts an api.Job to output.FlatJob.
func toFlatJob(j *api.Job) output.FlatJob {
	fj := output.FlatJob{
		ID:         j.ID,
		Name:       j.Name,
		Status:     j.Status,
		Stage:      j.Stage,
		Ref:        j.Ref,
		WebURL:     j.WebURL,
		CreatedAt:  j.CreatedAt,
		StartedAt:  j.StartedAt,
		FinishedAt: j.FinishedAt,
		Duration:   j.Duration,
	}
	if j.User != nil {
		fj.Username = j.User.Username
	}
	if j.Pipeline != nil {
		fj.PipelineID = j.Pipeline.ID
	}
	return fj
}

// printPipelineDetail prints a single pipeline in human-readable form.
func printPipelineDetail(p *api.Pipeline) {
	fmt.Printf("Pipeline #%d (IID: %d)\n", p.ID, p.IID)
	fmt.Printf("  Ref:    %s\n", p.Ref)
	fmt.Printf("  Status: %s\n", output.StatusBadge(p.Status))
	fmt.Printf("  Source: %s\n", p.Source)
	if p.WebURL != "" {
		fmt.Printf("  URL:    %s\n", p.WebURL)
	}
}

// parseVariables parses KEY=VAL strings into PipelineVariable slice.
func parseVariables(vars []string) ([]api.PipelineVariable, error) {
	out := make([]api.PipelineVariable, 0, len(vars))
	for _, v := range vars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return nil, failArg(fmt.Sprintf("invalid --variable %q: expected KEY=VALUE", v))
		}
		out = append(out, api.PipelineVariable{Key: parts[0], Value: parts[1]})
	}
	return out, nil
}
