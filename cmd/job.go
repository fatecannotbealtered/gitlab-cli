package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

// test hook for artifacts output file close
var closeOutputFile = func(f *os.File) error { return f.Close() }

var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "Manage CI/CD jobs",
}

var jobGetCmd = &cobra.Command{
	Use:   "get <job_id>",
	Short: "Get a job by ID",
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
		jobID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("job_id must be an integer")
		}
		j, err := client.Jobs.Get(apiCtx(), project, jobID)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.JobToMap(toFlatJob(j)), fields))
			return nil
		}
		fmt.Printf("Job #%d: %s\n", j.ID, j.Name)
		fmt.Printf("  Stage:  %s\n", j.Stage)
		fmt.Printf("  Status: %s\n", output.StatusBadge(j.Status))
		fmt.Printf("  Ref:    %s\n", j.Ref)
		if j.WebURL != "" {
			fmt.Printf("  URL:    %s\n", j.WebURL)
		}
		return nil
	},
}

var jobLogCmd = &cobra.Command{
	Use:   "log <job_id>",
	Short: "Print the job trace log",
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
		jobID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("job_id must be an integer")
		}
		follow, _ := cmd.Flags().GetBool("follow")
		timeoutSec, _ := cmd.Flags().GetInt("timeout")

		if !follow {
			data, err := client.Jobs.Log(apiCtx(), project, jobID)
			if err != nil {
				return handleAPIError(err, jsonMode)
			}
			if jsonMode {
				output.PrintJSON(map[string]any{"jobId": jobID, "log": string(data)})
				return nil
			}
			_, _ = os.Stdout.Write(data)
			return nil
		}

		// --follow path
		ctx := cmd.Context()
		var cancel context.CancelFunc
		if timeoutSec > 0 {
			ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
			defer cancel()
		}

		if jsonMode {
			var buf bytes.Buffer
			if err := client.Jobs.LogStream(ctx, project, jobID, &buf, 0); err != nil {
				return handleAPIError(err, jsonMode)
			}
			j, err := client.Jobs.Get(apiCtx(), project, jobID)
			if err != nil {
				return handleAPIError(err, jsonMode)
			}
			output.PrintJSON(map[string]any{
				"id":     j.ID,
				"status": j.Status,
				"log":    buf.String(),
			})
			return nil
		}

		return client.Jobs.LogStream(ctx, project, jobID, os.Stdout, 0)
	},
}

var jobRetryCmd = &cobra.Command{
	Use:   "retry <job_id>",
	Short: "Retry a job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		jobID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("job_id must be an integer")
		}
		if dryRunOutput("retry job", map[string]any{"project": project, "job_id": jobID}) {
			return nil
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		j, err := client.Jobs.Retry(apiCtx(), project, jobID)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.JobToMap(toFlatJob(j)))
			return nil
		}
		output.Success(fmt.Sprintf("Job #%d retried (status: %s)", j.ID, j.Status))
		return nil
	},
}

var jobCancelCmd = &cobra.Command{
	Use:   "cancel <job_id>",
	Short: "Cancel a job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		jobID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("job_id must be an integer")
		}
		if dryRunOutput("cancel job", map[string]any{"project": project, "job_id": jobID}) {
			return nil
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		j, err := client.Jobs.Cancel(apiCtx(), project, jobID)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.JobToMap(toFlatJob(j)))
			return nil
		}
		output.Success(fmt.Sprintf("Job #%d canceled (status: %s)", j.ID, j.Status))
		return nil
	},
}

var jobArtifactsCmd = &cobra.Command{
	Use:   "artifacts <job_id>",
	Short: "Download job artifacts to a file",
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
		outPath, _ := cmd.Flags().GetString("output")
		if outPath == "" {
			return failArg("--output is required")
		}
		jobID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("job_id must be an integer")
		}
		if err := validateOutputPath(outPath); err != nil {
			output.Error(err.Error())
			setExitCode(ExitBadArgs)
			return ErrSilent
		}
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			output.Error("creating output file: " + err.Error())
			setExitCode(ExitNetwork)
			return ErrSilent
		}
		if err := client.Jobs.ArtifactsTo(apiCtx(), project, jobID, f); err != nil {
			_ = f.Close()
			_ = os.Remove(outPath)
			return handleAPIError(err, jsonMode)
		}
		if err := closeOutputFile(f); err != nil {
			output.Error("closing output file: " + err.Error())
			setExitCode(ExitNetwork)
			return ErrSilent
		}
		fi, _ := os.Stat(outPath)
		var nbytes int64
		if fi != nil {
			nbytes = fi.Size()
		}
		if !jsonMode {
			output.Success(fmt.Sprintf("Artifacts saved to %s (%d bytes)", outPath, nbytes))
		} else {
			output.PrintJSON(map[string]any{"path": outPath, "bytes": nbytes})
		}
		return nil
	},
}

var jobWaitCmd = &cobra.Command{
	Use:   "wait <job_id>",
	Short: "Wait for a job to reach a terminal status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		jobID, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("job_id must be an integer")
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
			j, err := client.Jobs.Get(apiCtx(), project, jobID)
			if err != nil {
				return handleAPIError(err, jsonMode)
			}
			elapsed := time.Since(start)
			switch j.Status {
			case "success", "failed", "canceled", "skipped", "manual":
				if jsonMode {
					output.PrintJSON(output.JobToMap(toFlatJob(j)))
				} else {
					fmt.Printf("Job #%d finished: %s\n", j.ID, output.StatusBadge(j.Status))
				}
				if j.Status != "success" {
					setExitCode(ExitCIFailed)
					return ErrSilent
				}
				return nil
			}
			if timeoutSec > 0 && elapsed >= time.Duration(timeoutSec)*time.Second {
				output.Error(fmt.Sprintf("timed out waiting for job #%d", jobID))
				setExitCode(ExitTimeout)
				return ErrSilent
			}
			if !jsonMode && !quietMode {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Waiting... status=%s (%.0fs elapsed)\n", j.Status, elapsed.Seconds())
			}
			if err := sleepContext(cmd.Context(), interval); err != nil {
				output.Error(err.Error())
				setExitCode(ExitNetwork)
				return ErrSilent
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(jobCmd)

	// get
	jobGetCmd.Flags().String("project", "", "Project ID or path (required)")
	jobGetCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	jobCmd.AddCommand(jobGetCmd)

	// log
	jobLogCmd.Flags().String("project", "", "Project ID or path (required)")
	jobLogCmd.Flags().BoolP("follow", "f", false, "Stream log in real time until job completes")
	jobLogCmd.Flags().Int("timeout", 0, "Timeout in seconds for --follow (0 = unlimited)")
	jobCmd.AddCommand(jobLogCmd)
	markOutputType(jobLogCmd, "json")
	markOutputFormats(jobLogCmd, formatJSON, formatText, formatRaw)

	// retry
	jobRetryCmd.Flags().String("project", "", "Project ID or path (required)")
	jobCmd.AddCommand(jobRetryCmd)
	markWrite(jobRetryCmd)

	// cancel
	jobCancelCmd.Flags().String("project", "", "Project ID or path (required)")
	jobCmd.AddCommand(jobCancelCmd)
	markWrite(jobCancelCmd)

	// artifacts
	jobArtifactsCmd.Flags().String("project", "", "Project ID or path (required)")
	jobArtifactsCmd.Flags().String("output", "", "Output file path (required)")
	jobCmd.AddCommand(jobArtifactsCmd)

	// wait
	jobWaitCmd.Flags().String("project", "", "Project ID or path (required)")
	jobWaitCmd.Flags().Int("timeout", 0, "Timeout in seconds (0 = unlimited)")
	jobWaitCmd.Flags().Int("interval", 5, "Poll interval in seconds")
	jobCmd.AddCommand(jobWaitCmd)
}
