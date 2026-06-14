package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	// Tests run non-interactively; disable agent-safe restrictions unless explicitly testing them.
	_ = os.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
	_ = os.Setenv("GITLAB_CLI_RETRY_BASE_MS", "0")
}

var preserveTextFormatForReset bool

type rootPersistentFlagStateForTest struct {
	values             map[string]string
	changed            map[string]bool
	forceMode          bool
	confirmFlag        string
	dryRun             bool
	dangerousMode      bool
	formatMode         string
	jsonAlias          bool
	jsonMode           bool
	quietMode          bool
	compactJSON        bool
	activeCmd          *cobra.Command
	preserveTextFormat bool
}

func snapshotRootPersistentFlagsForTest() rootPersistentFlagStateForTest {
	state := rootPersistentFlagStateForTest{
		values:             map[string]string{},
		changed:            map[string]bool{},
		forceMode:          forceMode,
		confirmFlag:        confirmFlag,
		dryRun:             dryRun,
		dangerousMode:      dangerousMode,
		formatMode:         formatMode,
		jsonAlias:          jsonAlias,
		jsonMode:           jsonMode,
		quietMode:          quietMode,
		compactJSON:        compactJSON,
		activeCmd:          activeCmd,
		preserveTextFormat: preserveTextFormatForReset,
	}
	for _, name := range []string{"json", "format", "force", "dry-run", "dangerous", "confirm", "quiet", "compact"} {
		if flag := rootCmd.PersistentFlags().Lookup(name); flag != nil {
			state.values[name] = flag.Value.String()
			state.changed[name] = flag.Changed
		}
	}
	return state
}

func restoreRootPersistentFlagsForTest(t *testing.T, state rootPersistentFlagStateForTest) {
	t.Helper()
	for name, value := range state.values {
		if flag := rootCmd.PersistentFlags().Lookup(name); flag != nil {
			if err := flag.Value.Set(value); err != nil {
				t.Fatalf("restore flag %q: %v", name, err)
			}
			flag.Changed = state.changed[name]
		}
	}
	forceMode = state.forceMode
	confirmFlag = state.confirmFlag
	dryRun = state.dryRun
	dangerousMode = state.dangerousMode
	formatMode = state.formatMode
	jsonAlias = state.jsonAlias
	jsonMode = state.jsonMode
	quietMode = state.quietMode
	compactJSON = state.compactJSON
	activeCmd = state.activeCmd
	preserveTextFormatForReset = state.preserveTextFormat
}

// withAgentSafe enables default agent-safe restrictions for the duration of the test.
func withAgentSafe(t *testing.T) {
	t.Helper()
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "1")
	t.Setenv("GITLAB_CLI_ALLOW_FORCE", "")
	t.Setenv("GITLAB_CLI_ALLOW_SHOW_VALUES", "")
}

// resetRootPersistentFlags clears persistent CLI flags that leak between Execute() calls.
func resetRootPersistentFlags(t *testing.T) {
	t.Helper()
	preserveTextFormat := preserveTextFormatForReset
	for _, f := range []struct{ name, value string }{
		{"json", "false"},
		{"format", "json"},
		{"force", "false"},
		{"dry-run", "false"},
		{"dangerous", "false"},
		{"confirm", ""},
		{"quiet", "false"},
		{"compact", "false"},
	} {
		if err := rootCmd.PersistentFlags().Set(f.name, f.value); err != nil {
			t.Fatalf("reset flag %q: %v", f.name, err)
		}
		if flag := rootCmd.PersistentFlags().Lookup(f.name); flag != nil {
			flag.Changed = false
		}
	}
	forceMode = false
	confirmFlag = ""
	dryRun = false
	dangerousMode = false
	formatMode = formatJSON
	jsonAlias = false
	jsonMode = true
	quietMode = false
	compactJSON = false
	activeCmd = nil
	if preserveTextFormat {
		if err := rootCmd.PersistentFlags().Set("format", formatText); err != nil {
			t.Fatalf("preserve text format: %v", err)
		}
		if flag := rootCmd.PersistentFlags().Lookup("format"); flag != nil {
			flag.Changed = true
		}
		formatMode = formatText
		jsonMode = false
	}
}

func setTextFormatForTest(t *testing.T) {
	t.Helper()
	formatFlag := rootCmd.PersistentFlags().Lookup("format")
	jsonFlag := rootCmd.PersistentFlags().Lookup("json")

	if err := rootCmd.PersistentFlags().Set("format", formatText); err != nil {
		t.Fatalf("set text format: %v", err)
	}
	if formatFlag != nil {
		formatFlag.Changed = true
	}
	if err := rootCmd.PersistentFlags().Set("json", "false"); err != nil {
		t.Fatalf("clear json alias: %v", err)
	}
	if jsonFlag != nil {
		jsonFlag.Changed = false
	}
	formatMode = formatText
	jsonAlias = false
	jsonMode = false
	preserveTextFormatForReset = true
	clearFieldsFlagsForTest(t, rootCmd)

	t.Cleanup(func() {
		formatMode = formatJSON
		jsonAlias = false
		jsonMode = true
		preserveTextFormatForReset = false
		if formatFlag != nil {
			_ = formatFlag.Value.Set(formatJSON)
			formatFlag.Changed = false
		}
		if jsonFlag != nil {
			_ = jsonFlag.Value.Set("false")
			jsonFlag.Changed = false
		}
	})
}

func clearFieldsFlagsForTest(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	if cmd == nil {
		return
	}
	if flag := cmd.Flags().Lookup("fields"); flag != nil {
		if err := cmd.Flags().Set("fields", ""); err != nil {
			t.Fatalf("clear fields flag: %v", err)
		}
		flag.Changed = false
	}
	for _, child := range cmd.Commands() {
		clearFieldsFlagsForTest(t, child)
	}
}

func confirmArgsForTest(t *testing.T, action string, payload any) []string {
	t.Helper()
	prevActive := activeCmd
	activeCmd = nil
	defer func() { activeCmd = prevActive }()
	token, _ := newConfirmToken(action, payload)
	return []string{"--confirm", token}
}

func withConfirmForTest(t *testing.T, args []string) []string {
	t.Helper()
	state := snapshotRootPersistentFlagsForTest()
	preserveTextFormatForReset = false
	resetRootPersistentFlags(t)
	defer restoreRootPersistentFlagsForTest(t, state)

	// Slice flags (StringArray/StringSlice, e.g. --action, --ids) append across
	// reused FlagSet parses, so the dry-run inside this helper would otherwise
	// double the resolved target set on the confirm run and invalidate the token.
	// Reset them on the resolved command before each parse so dry-run and confirm
	// see the same batch.
	resetSliceFlagsForTest(t, args)

	previewArgs := append([]string{}, args...)
	previewArgs = append(previewArgs, "--dry-run", "--json")
	out := captureStdout(t, func() {
		rootCmd.SetArgs(previewArgs)
		_ = rootCmd.Execute()
	})
	resetSliceFlagsForTest(t, args)

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			ConfirmToken string `json:"confirm_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("parse dry-run confirm token: %v\noutput:\n%s", err, out)
	}
	if !env.OK || env.Data.ConfirmToken == "" {
		t.Fatalf("dry-run did not return confirm token:\n%s", out)
	}
	confirmed := append([]string{}, args...)
	return append(confirmed, "--confirm", env.Data.ConfirmToken)
}

// resetCompactForTest restores the global compact-JSON state after a test that
// passes --compact, so it does not leak into later tests that assert on indented
// JSON. output.Compact is only re-synced on the next Execute()'s OnInitialize, so
// a test that errors out before another Execute would otherwise leave it set.
func resetCompactForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		compactJSON = false
		output.Compact = false
		if flag := rootCmd.PersistentFlags().Lookup("compact"); flag != nil {
			_ = flag.Value.Set("false")
			flag.Changed = false
		}
	})
}

// resetSliceFlagsForTest clears repeatable slice flags (StringArray/StringSlice)
// on the command the args resolve to, so a reused FlagSet does not accumulate
// values across multiple Execute() calls within one test.
func resetSliceFlagsForTest(t *testing.T, args []string) {
	t.Helper()
	cmd, _, err := rootCmd.Find(args)
	if err != nil || cmd == nil {
		return
	}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace([]string{})
			f.Changed = false
		}
	})
}

func argsTargetWriteCommandForTest(args []string) bool {
	cmd, _, err := rootCmd.Find(args)
	return err == nil && cmd != nil && isWriteCommand(cmd)
}

func resetCommandFlagForTest(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("reset flag %s: %v", name, err)
	}
	if flag := cmd.Flags().Lookup(name); flag != nil {
		flag.Changed = false
	}
}

func resetVariableFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	for _, kv := range []struct {
		cmd         *cobra.Command
		name, value string
	}{
		{variableListCmd, "project", ""},
		{variableListCmd, "limit", "20"},
		{variableListCmd, "fields", ""},
		{variableGetCmd, "project", ""},
		{variableGetCmd, "key", ""},
		{variableGetCmd, "filter", ""},
		{variableGetCmd, "fields", ""},
		{variableCreateCmd, "project", ""},
		{variableCreateCmd, "key", ""},
		{variableCreateCmd, "value", ""},
		{variableCreateCmd, "type", "env_var"},
		{variableCreateCmd, "protected", "false"},
		{variableCreateCmd, "masked", "false"},
		{variableCreateCmd, "raw", "false"},
		{variableCreateCmd, "env-scope", "*"},
		{variableCreateCmd, "description", ""},
		{variableUpdateCmd, "project", ""},
		{variableUpdateCmd, "key", ""},
		{variableUpdateCmd, "value", ""},
		{variableUpdateCmd, "type", ""},
		{variableUpdateCmd, "protected", ""},
		{variableUpdateCmd, "masked", ""},
		{variableUpdateCmd, "raw", ""},
		{variableUpdateCmd, "env-scope", ""},
		{variableUpdateCmd, "description", ""},
		{variableDeleteCmd, "project", ""},
		{variableDeleteCmd, "key", ""},
		{variableDeleteCmd, "filter", ""},
	} {
		resetCommandFlagForTest(t, kv.cmd, kv.name, kv.value)
	}
	if err := variableCmd.PersistentFlags().Set("show-values", "false"); err != nil {
		t.Fatalf("reset show-values: %v", err)
	}
	if flag := variableCmd.PersistentFlags().Lookup("show-values"); flag != nil {
		flag.Changed = false
	}
}

func resetAuthLoginFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	authLoginHostFlag = ""
	authLoginTokenFlag = ""
	authLoginProfileFlag = "default"
	for _, kv := range []struct{ name, value string }{
		{"host", ""},
		{"token", ""},
		{"profile", "default"},
	} {
		if err := authLoginCmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset auth login flag %q: %v", kv.name, err)
		}
	}
}

// withStdinInput replaces os.Stdin with a pipe preloaded with input (non-TTY for term.IsTerminal).
func withStdinInput(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})
}

// writeGitLabCLIFile writes a file under ~/.gitlab-cli/ inside home.
func writeGitLabCLIFile(t *testing.T, home, name, content string) {
	t.Helper()
	dir := filepath.Join(home, ".gitlab-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// blockProfilesRemove makes profiles.json a non-empty directory so os.Remove fails.
func blockProfilesRemove(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".gitlab-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	profDir := filepath.Join(dir, "profiles.json")
	if err := os.Mkdir(profDir, 0o700); err != nil {
		t.Fatalf("mkdir profiles.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "block"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write block file: %v", err)
	}
}

// blockConfigRemove makes config.json a non-empty directory so os.Remove fails.
func blockConfigRemove(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".gitlab-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfgDir := filepath.Join(dir, "config.json")
	if err := os.Mkdir(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir config.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "block"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write block file: %v", err)
	}
}

// withNonInteractiveStdin replaces os.Stdin with a pipe so requireConfirm treats the session as non-TTY.
func withNonInteractiveStdin(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})
}

// isolateConfigHome points config file resolution away from the developer machine (Windows uses USERPROFILE).
func isolateConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return dir
}

// captureStdout captures stdout during fn().
// On Windows pipes can deadlock if the buffer fills, so we drain concurrently.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	os.Stdout = orig
	<-done
	_ = r.Close()
	return buf.String()
}

// captureCombinedOutput captures stdout and stderr during fn().
func captureCombinedOutput(t *testing.T, fn func()) string {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout = outW
	os.Stderr = errW

	var outBuf, errBuf bytes.Buffer
	outDone := make(chan struct{})
	errDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&outBuf, outR)
		close(outDone)
	}()
	go func() {
		_, _ = io.Copy(&errBuf, errR)
		close(errDone)
	}()

	fn()

	_ = outW.Close()
	_ = errW.Close()
	os.Stdout = origOut
	os.Stderr = origErr
	<-outDone
	<-errDone
	_ = outR.Close()
	_ = errR.Close()
	return outBuf.String() + errBuf.String()
}

// captureStderr captures stderr during fn().
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	os.Stderr = orig
	<-done
	_ = r.Close()
	return buf.String()
}

func unwrapJSONDataMap(t *testing.T, out string) map[string]any {
	t.Helper()
	data := unwrapJSONData(t, out)
	m, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("data is %T, want object\noutput:\n%s", data, out)
	}
	return m
}

func unwrapJSONDataInto(t *testing.T, out string, target any) {
	t.Helper()
	data := unwrapJSONData(t, out)
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal data: %v\noutput:\n%s", err, out)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("unmarshal data: %v\noutput:\n%s", err, out)
	}
}

func unwrapJSONData(t *testing.T, out string) any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Fatalf("expected ok=true envelope, got:\n%s", out)
	}
	data, ok := env["data"]
	if !ok {
		t.Fatalf("missing data in envelope:\n%s", out)
	}
	return data
}
