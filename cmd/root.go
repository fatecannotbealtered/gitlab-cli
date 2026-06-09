package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/audit"
	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

// Exit codes for machine-readable error classification.
const (
	ExitOK        = 0
	ExitError     = 1
	ExitBadArgs   = 2
	ExitNotFound  = 3
	ExitAuth      = 4
	ExitForbidden = 4
	ExitConfirm   = 5
	ExitCancelled = 5
	ExitConflict  = 6
	ExitRateLimit = 7
	ExitNetwork   = 7
	ExitTimeout   = 8
	ExitCIFailed  = 6 // pipeline/job wait finished in a non-success terminal state
)

// ErrSilent indicates the error has been printed; cobra should not print again.
var ErrSilent = errors.New("")

// version is injected by goreleaser ldflags.
var version = "dev"

// Global flags.
var (
	jsonMode    = true
	jsonAlias   bool
	compactJSON bool
	forceMode   bool
	quietMode   bool
	dryRun      bool
	formatMode  = formatJSON
)

const (
	formatJSON = "json"
	formatText = "text"
	formatRaw  = "raw"
)

const (
	toolRiskTier    = "T1"
	toolBlastRadius = "Can read and mutate GitLab project state including issues, merge requests, CI jobs, releases, repository files, branches, and CI/CD variables with the configured token permissions."
	skillMinVersion = "1.2.0"
)

// fieldsFlag is set by commands that opt into the global --fields flag.
// It's a comma-separated list of field names to include in JSON output.
//
// Each command that wants --fields registers it on its own flag set
// (this keeps it from leaking onto commands that don't have a flat schema).
//
// Helper: getFieldsFlag(cmd) returns the parsed list.

// lastExit tracks the exit code for the current command execution.
var lastExit int

// cmdStartTime records when the current command began (for audit logging).
var cmdStartTime time.Time

// activeCmd is the innermost command currently running (for context propagation).
var activeCmd *cobra.Command

// LastExitCode returns the exit code from the last command execution.
func LastExitCode() int { return lastExit }

// apiCtx returns the context for API calls from CLI commands (honours SIGINT when set).
func apiCtx() context.Context {
	if activeCmd != nil {
		if ctx := activeCmd.Context(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

// setExitCode sets the exit code (only increases severity, never decreases).
func setExitCode(code int) {
	if code > lastExit {
		lastExit = code
	}
}

var rootCmd = &cobra.Command{
	Use:           "gitlab-cli",
	Short:         "Full GitLab CLI scaffold for AI Agents",
	Version:       version,
	SilenceErrors: true,
	SilenceUsage:  true,
	Long: fmt.Sprintf("\n  %s\n  %s",
		output.FormatCyanBold("gitlab-cli"),
		output.FormatGray("Agent-native GitLab control")),
}

func init() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			version = info.Main.Version
		}
	}
	rootCmd.Version = version
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.PersistentFlags().StringVar(&formatMode, "format", formatJSON, "Output format: json|text|raw")
	rootCmd.PersistentFlags().BoolVar(&jsonAlias, "json", false, "Compatibility alias for --format json")
	rootCmd.PersistentFlags().BoolVar(&compactJSON, "compact", false, "Compact JSON (no indentation; only affects --format json)")
	rootCmd.PersistentFlags().BoolVar(&forceMode, "force", false, "Deprecated; use --dry-run and --confirm <confirm_token>")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress non-JSON stdout output (for scripts and AI Agents)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without executing")
	initConfirmFlag()

	cobra.OnInitialize(func() {
		output.Quiet = quietMode
		output.Compact = compactJSON
	})

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cmdStartTime = time.Now()
		activeCmd = cmd
		if err := applyFormatFlags(cmd); err != nil {
			return err
		}
		return nil
	}

	rootCmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		if !isWriteCommand(cmd) {
			return nil
		}
		duration := time.Since(cmdStartTime)
		audit.Log(cmd.CommandPath(), os.Args[1:], lastExit, duration.Milliseconds())
		return nil
	}
}

// Execute runs the root command with a background context.
func Execute() error {
	return ExecuteContext(context.Background())
}

// ExecuteContext runs the root command with the given context (e.g. signal.NotifyContext).
func ExecuteContext(ctx context.Context) error {
	lastExit = 0
	cmdStartTime = time.Now()
	activeCmd = nil
	defer func() { activeCmd = nil }()
	output.DurationMS = func() int64 {
		if cmdStartTime.IsZero() {
			return 0
		}
		return time.Since(cmdStartTime).Milliseconds()
	}
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, ErrSilent) {
			return err
		}
		emitError(err.Error(), ExitBadArgs, output.ErrValidation)
		return ErrSilent
	}
	return nil
}

// handleAPIError handles API errors with JSON mode support.
func handleAPIError(err error, jsonMode bool) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		msg := apiErr.Error()
		code := output.ErrorCodeFromStatus(apiErr.StatusCode)
		if jsonMode {
			output.PrintErrorJSONWithCode(msg, apiErr.StatusCode, code)
		} else {
			output.Error(msg)
		}
		setExitCode(exitCodeForStatus(apiErr.StatusCode))
		return ErrSilent
	}
	msg := err.Error()
	if jsonMode {
		output.PrintErrorJSONWithCode(msg, 0, output.ErrNetwork)
	} else {
		output.Error(msg)
	}
	setExitCode(ExitNetwork)
	return ErrSilent
}

// exitCodeForStatus maps HTTP status codes to semantic exit codes.
func exitCodeForStatus(status int) int {
	switch {
	case status == 401:
		return ExitAuth
	case status == 403:
		return ExitForbidden
	case status == 404:
		return ExitNotFound
	case status == 409:
		return ExitConflict
	case status == 429:
		return ExitRateLimit
	case status >= 500:
		return ExitNetwork
	default:
		return ExitBadArgs
	}
}

// dryRunOutput outputs a dry-run message and returns true if --dry-run is set.
func dryRunOutput(action string, detail map[string]any) bool {
	if !dryRun {
		return false
	}
	if jsonMode {
		if detail == nil {
			detail = map[string]any{}
		}
		normalized, _ := normalizeConfirmPayload(detail).(map[string]any)
		if normalized == nil {
			normalized = detail
		}
		changes := []map[string]any{}
		if len(normalized) > 0 {
			changes = append(changes, normalized)
		}
		token, expires := newConfirmToken(action, detail)
		output.PrintJSON(map[string]any{
			"preview": map[string]any{
				"action":  action,
				"changes": changes,
			},
			"confirm_token": token,
			"expires_at":    expires.Format(time.RFC3339),
		})
	} else {
		output.Info("[dry-run] " + action)
	}
	return true
}

func prepareWrite(cmd *cobra.Command, action string, detail map[string]any) (bool, error) {
	if dryRunOutput(action, detail) {
		return true, nil
	}
	if err := requireConfirm(cmd, action, detail); err != nil {
		return false, err
	}
	return false, nil
}

func applyFormatFlags(cmd *cobra.Command) error {
	requested := strings.ToLower(strings.TrimSpace(formatMode))
	if requested == "" {
		requested = formatJSON
	}
	formatFlag := rootCmd.PersistentFlags().Lookup("format")
	jsonFlag := rootCmd.PersistentFlags().Lookup("json")
	forceFlag := rootCmd.PersistentFlags().Lookup("force")
	jsonChanged := jsonFlag != nil && jsonFlag.Changed
	formatChanged := formatFlag != nil && formatFlag.Changed
	forceRequested := forceMode || (forceFlag != nil && forceFlag.Changed)
	defer func() {
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
			forceFlag.Changed = false
		}
		forceMode = false
	}()

	if !jsonChanged && !formatChanged && !jsonMode {
		requested = formatText
	}

	if jsonChanged && jsonAlias && formatChanged && requested != formatJSON {
		jsonMode = true
		return failArg("--json cannot be used with --format " + requested)
	}
	if jsonChanged && jsonAlias {
		requested = formatJSON
	}
	if requested != formatJSON && requested != formatText && requested != formatRaw {
		jsonMode = true
		return failArg("--format must be one of: json, text, raw")
	}

	formatMode = requested
	jsonMode = formatMode == formatJSON

	if formatMode != formatJSON {
		if f := cmd.Flags().Lookup("fields"); f != nil && f.Changed {
			return failArg("--fields is only supported with --format json")
		}
	}
	if !commandSupportsFormat(cmd, formatMode) {
		return failArg(fmt.Sprintf("%s does not support --format %s", cmd.CommandPath(), formatMode))
	}
	if forceRequested && isWriteCommand(cmd) {
		return failArg("--force is not supported for write commands; use --dry-run and --confirm <confirm_token>")
	}
	return nil
}

func commandSupportsFormat(cmd *cobra.Command, format string) bool {
	for _, allowed := range commandOutputFormats(cmd) {
		if allowed == format {
			return true
		}
	}
	return false
}

func commandOutputFormats(cmd *cobra.Command) []string {
	if cmd == nil || cmd.Annotations == nil {
		return []string{formatJSON, formatText}
	}
	raw := strings.TrimSpace(cmd.Annotations["formats"])
	if raw == "" {
		return []string{formatJSON, formatText}
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{formatJSON, formatText}
	}
	return out
}

// isWriteCommand returns true if the command has the "write" annotation.
func isWriteCommand(cmd *cobra.Command) bool {
	return cmd.Annotations["write"] == "true"
}

// markWrite sets the "write" annotation on a command for audit logging.
func markWrite(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["write"] = "true"
	cmd.Annotations["confirm"] = "true"
}

// markConfirm marks commands that prompt for typed confirmation unless --force is set.
func markConfirm(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["confirm"] = "true"
}

// markRiskLevel sets agent risk metadata: low, medium, high, critical.
func markRiskLevel(cmd *cobra.Command, level string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["riskLevel"] = level
}

// markOutputType sets the default stdout type for agent reference (json, text, raw).
func markOutputType(cmd *cobra.Command, outputType string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["outputType"] = outputType
}

func markOutputFormats(cmd *cobra.Command, formats ...string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["formats"] = strings.Join(formats, ",")
}

// newClient loads config and creates an API client.
func newClient() (*api.Client, *config.Config, error) {
	cfg, err := config.MustLoad()
	if err != nil {
		code := output.ErrConfig
		exit := ExitAuth
		if strings.Contains(err.Error(), "not logged in") {
			code = output.ErrAuth
		}
		if jsonMode {
			output.PrintErrorJSONWithCode(err.Error(), 0, code)
		} else {
			output.Error(err.Error())
		}
		setExitCode(exit)
		return nil, nil, ErrSilent
	}
	return api.NewClient(cfg), cfg, nil
}

// resolveUserID resolves "me" or a username string to a numeric GitLab user ID
// using exactly one API call:
//   - "me"  -> GET /user
//   - other -> GET /users?username=<name>
//
// Returns ExitNotFound (4) wrapped as ErrSilent if the username does not exist.
func resolveUserID(client *api.Client, name string) (int, error) {
	if name == "me" {
		me, err := client.Users.Me(apiCtx())
		if err != nil {
			return 0, handleAPIError(err, jsonMode)
		}
		return me.ID, nil
	}
	u, err := client.Users.GetByUsername(apiCtx(), name)
	if err != nil {
		return 0, handleAPIError(err, jsonMode)
	}
	if u == nil {
		return 0, failNotFound(fmt.Sprintf("user %q not found", name))
	}
	return u.ID, nil
}

// getFieldsFlag returns the parsed []string for a --fields flag, or nil if absent.
// Subcommands register the flag on their own flag set.
func getFieldsFlag(cmd *cobra.Command) []string {
	if cmd == nil {
		return nil
	}
	raw, _ := cmd.Flags().GetString("fields")
	if raw == "" {
		return nil
	}
	out := []string{}
	for _, part := range splitCSV(raw) {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitCSV(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, trim(cur))
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, trim(cur))
	return out
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// validateOutputPath rejects paths that contain ".." segments after cleaning.
// Absolute paths and clean relative paths are allowed; the user is presumed to
// know where they want output to land. The goal is to make AI-Agent-driven
// path injection (e.g. via untrusted issue body content) harder.
func validateOutputPath(p string) error {
	if p == "" {
		return errors.New("output path cannot be empty")
	}
	cleaned := filepath.ToSlash(filepath.Clean(p))
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return errors.New("output path must not contain '..' segments")
		}
	}
	return nil
}
