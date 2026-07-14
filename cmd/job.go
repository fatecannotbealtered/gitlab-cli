package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
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
			tail, _ := cmd.Flags().GetInt("tail")
			grep, _ := cmd.Flags().GetString("grep")
			maxBytes, _ := cmd.Flags().GetInt("max-bytes")
			filtered, meta, ferr := filterTrace(data, grep, tail, maxBytes)
			if ferr != nil {
				return ferr
			}
			if jsonMode {
				out := map[string]any{"jobId": output.ID(jobID), "log": string(filtered)}
				if meta.filtered {
					out["totalBytes"] = meta.totalBytes
					out["returnedBytes"] = meta.returnedBytes
					out["truncated"] = meta.truncated
				}
				output.PrintJSON(output.MarkUntrusted(out, "log"))
				return nil
			}
			_, _ = os.Stdout.Write(filtered)
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
			// NDJSON stream (CLI-SPEC §5): one {type:"chunk"} line per new trace
			// range, then a final {type:"summary"} line. GitLab has no native
			// trace streaming, so LogStreamStatus polls with a byte offset.
			totalBytes := 0
			chunks := 0
			status, streamErr := client.Jobs.LogStreamStatus(ctx, project, jobID, 0, func(b []byte, offset int) error {
				chunks++
				totalBytes += len(b)
				return output.PrintNDJSON("chunk", output.MarkUntrusted(map[string]any{
					"jobId":  output.ID(jobID),
					"offset": offset,
					"bytes":  len(b),
					"data":   string(b),
				}, "data"))
			})
			if streamErr != nil {
				return handleAPIError(streamErr, jsonMode)
			}
			return output.PrintNDJSON("summary", map[string]any{
				"jobId":      output.ID(jobID),
				"status":     status,
				"chunks":     chunks,
				"totalBytes": totalBytes,
			})
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
		client, _, err := newClient()
		if err != nil {
			return err
		}
		// A manual or scheduled (delayed) job is not retryable — it must be
		// played/run, not retried. GitLab answers a raw retry with a 403 that
		// reads like a permission error, so inspect the status first (in both the
		// dry-run and confirm steps) and fail with an accurate hint pointing at
		// `job play`, instead of leaking a misleading auth suggestion.
		j, err := client.Jobs.Get(apiCtx(), project, jobID)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if j.Status == "manual" || j.Status == "scheduled" {
			return failArgf("job #%d is a %s job and cannot be retried; start it with 'gitlab-cli job play %d' instead", jobID, j.Status, jobID)
		}
		if done, err := prepareWrite(cmd, "retry job", map[string]any{"project": project, "job_id": jobID}); done || err != nil {
			return err
		}
		j, err = client.Jobs.Retry(apiCtx(), project, jobID)
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

var jobPlayCmd = &cobra.Command{
	Use:   "play <job_id>",
	Short: "Play (start) a manual job",
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
		vars, err := parseJobVariables(cmd)
		if err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		// Only a manual or scheduled (delayed) job can be played — GitLab's play
		// endpoint accepts both. Before a confirm token is issued (dry-run or a
		// first bare call, i.e. no --confirm yet) reject a non-playable job with an
		// accurate message. Once --confirm is supplied we skip this check and let
		// the confirm-token comparison below surface any state drift: the token was
		// only ever issued while the job was playable, so a mismatch means the job
		// moved on and confirm fails closed with E_CONFLICT.
		j, err := client.Jobs.Get(apiCtx(), project, jobID)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		playable := j.Status == "manual" || j.Status == "scheduled"
		if confirmFlag == "" && !playable {
			return failArgf("job #%d is not startable (status: %s); only manual or scheduled jobs can be played", jobID, j.Status)
		}
		// Bind the confirm token to the job identity/state plus the SET of
		// variable KEYS only — never the values, which would leak into the
		// dry-run preview (see variable create). The caller supplies the same
		// values again at confirm time, so binding keys is sufficient.
		detail := map[string]any{
			"project": project,
			"job_id":  jobID,
			"ref":     j.Ref,
			"status":  j.Status,
		}
		if j.Pipeline != nil {
			detail["pipeline_id"] = j.Pipeline.ID
		}
		if len(vars) > 0 {
			detail["variables"] = variableKeys(vars)
		}
		if done, err := prepareWrite(cmd, "play job", detail); done || err != nil {
			return err
		}
		j, err = client.Jobs.Play(apiCtx(), project, jobID, vars)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.JobToMap(toFlatJob(j)))
			return nil
		}
		output.Success(fmt.Sprintf("Job #%d started (status: %s)", j.ID, j.Status))
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
		if done, err := prepareWrite(cmd, "cancel job", map[string]any{"project": project, "job_id": jobID}); done || err != nil {
			return err
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
			return failArg(err.Error())
		}
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return failWithCode("creating output file: "+err.Error(), ExitNetwork, output.ErrNetwork)
		}
		if err := client.Jobs.ArtifactsTo(apiCtx(), project, jobID, f); err != nil {
			_ = f.Close()
			_ = os.Remove(outPath)
			return handleAPIError(err, jsonMode)
		}
		if err := closeOutputFile(f); err != nil {
			return failWithCode("closing output file: "+err.Error(), ExitNetwork, output.ErrNetwork)
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
				return failWithCode(fmt.Sprintf("timed out waiting for job #%d", jobID), ExitTimeout, output.ErrTimeout)
			}
			if !jsonMode && !quietMode {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Waiting... status=%s (%.0fs elapsed)\n", j.Status, elapsed.Seconds())
			}
			if err := sleepContext(cmd.Context(), interval); err != nil {
				return failWithCode(err.Error(), ExitNetwork, output.ErrNetwork)
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
	jobLogCmd.Flags().Int("tail", 0, "Return only the last N lines of the trace")
	jobLogCmd.Flags().String("grep", "", "Return only trace lines matching this regular expression")
	jobLogCmd.Flags().Int("max-bytes", 0, "Cap the returned trace to at most N bytes (keeps the tail)")
	jobCmd.AddCommand(jobLogCmd)
	markOutputType(jobLogCmd, "json")
	markOutputFormats(jobLogCmd, formatJSON, formatText, formatRaw)

	// retry
	jobRetryCmd.Flags().String("project", "", "Project ID or path (required)")
	jobCmd.AddCommand(jobRetryCmd)
	markWrite(jobRetryCmd)

	// play
	jobPlayCmd.Flags().String("project", "", "Project ID or path (required)")
	jobPlayCmd.Flags().StringArray("variable", nil, "Job variable in KEY=VAL format (repeatable)")
	jobCmd.AddCommand(jobPlayCmd)
	markWrite(jobPlayCmd)

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

// parseJobVariables parses repeatable --variable KEY=VAL flags for `job play`.
func parseJobVariables(cmd *cobra.Command) ([]api.JobVariable, error) {
	raw, _ := cmd.Flags().GetStringArray("variable")
	out := make([]api.JobVariable, 0, len(raw))
	for _, v := range raw {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, failArgf("invalid --variable %q: expected KEY=VALUE", v)
		}
		out = append(out, api.JobVariable{Key: parts[0], Value: parts[1]})
	}
	return out, nil
}

// variableKeys returns the sorted variable keys, used to bind the confirm token
// to the variable set without exposing the (possibly secret) values.
func variableKeys(vars []api.JobVariable) []string {
	keys := make([]string, 0, len(vars))
	for _, v := range vars {
		keys = append(keys, v.Key)
	}
	sort.Strings(keys)
	return keys
}

// traceMeta describes how a job trace was filtered for token-efficient reads.
type traceMeta struct {
	filtered      bool
	totalBytes    int
	returnedBytes int
	truncated     bool
}

// filterTrace applies the token-efficient read modes to a raw job trace: --grep
// keeps matching lines, --tail keeps the last N lines, and --max-bytes caps the
// payload to its final N bytes. Filters compose in that order (grep, then tail,
// then max-bytes) and all keep the tail, where the actionable end of a CI log
// usually is. With no filter flags the trace is returned unchanged.
func filterTrace(data []byte, grep string, tail, maxBytes int) ([]byte, traceMeta, error) {
	meta := traceMeta{totalBytes: len(data), returnedBytes: len(data)}
	if grep == "" && tail <= 0 && maxBytes <= 0 {
		return data, meta, nil
	}
	meta.filtered = true
	out := data

	if grep != "" {
		re, err := regexp.Compile(grep)
		if err != nil {
			return nil, meta, failArgf("invalid --grep pattern: %v", err)
		}
		var b []byte
		for _, line := range bytes.SplitAfter(out, []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			if re.Match(bytes.TrimRight(line, "\n")) {
				b = append(b, line...)
			}
		}
		out = b
	}

	if tail > 0 {
		out = lastNLines(out, tail)
	}

	if maxBytes > 0 && len(out) > maxBytes {
		out = out[len(out)-maxBytes:]
		// Byte-slicing can land inside a multi-byte UTF-8 rune; advance past any
		// leading continuation bytes so we never emit a partial rune (drops ≤3
		// bytes, still within the maxBytes budget).
		for len(out) > 0 && !utf8.RuneStart(out[0]) {
			out = out[1:]
		}
	}

	meta.returnedBytes = len(out)
	meta.truncated = len(out) < meta.totalBytes
	return out, meta, nil
}

// lastNLines returns the final n lines of data, preserving line endings.
func lastNLines(data []byte, n int) []byte {
	if n <= 0 || len(data) == 0 {
		return data
	}
	lines := bytes.SplitAfter(data, []byte("\n"))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return data
	}
	return bytes.Join(lines[len(lines)-n:], nil)
}
