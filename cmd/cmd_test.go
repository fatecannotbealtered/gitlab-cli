package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
)

func TestExitCodeForStatus(t *testing.T) {
	cases := map[int]int{
		401: ExitAuth,
		403: ExitForbidden,
		404: ExitNotFound,
		429: ExitRateLimit,
		500: ExitNetwork,
		503: ExitNetwork,
		400: ExitBadArgs,
		422: ExitBadArgs,
	}
	for status, want := range cases {
		if got := exitCodeForStatus(status); got != want {
			t.Errorf("exitCodeForStatus(%d) = %d, want %d", status, got, want)
		}
	}
}

func TestSetExitCode_Monotonic(t *testing.T) {
	lastExit = 0
	setExitCode(ExitAuth)
	if LastExitCode() != ExitAuth {
		t.Fatalf("expected %d, got %d", ExitAuth, LastExitCode())
	}
	setExitCode(ExitOK) // Should not decrease
	if LastExitCode() != ExitAuth {
		t.Fatalf("exit code should not decrease: expected %d, got %d", ExitAuth, LastExitCode())
	}
	setExitCode(ExitNetwork)
	if LastExitCode() != ExitNetwork {
		t.Fatalf("expected %d, got %d", ExitNetwork, LastExitCode())
	}
	lastExit = 0
}

func TestRootHelp_ContainsGlobalFlags(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"gitlab-cli", "--json", "--quiet", "--dry-run", "--force"} {
		if !strings.Contains(out, want) {
			t.Errorf("help should contain %q, got:\n%s", want, out)
		}
	}
}

func TestRootVersion_ContainsBinaryName(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"--version"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	if !strings.Contains(buf.String(), "gitlab-cli") {
		t.Errorf("version output missing 'gitlab-cli', got:\n%s", buf.String())
	}
}

func TestReference_ContainsAuthAndDoctor(t *testing.T) {
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = false

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"reference"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{
		"# gitlab-cli Command Reference",
		"## gitlab-cli auth",
		"## gitlab-cli doctor",
		"## gitlab-cli reference",
		"--json",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("reference output missing %q", want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	cases := map[string][]string{
		"":            {""},
		"a":           {"a"},
		"a,b":         {"a", "b"},
		"a, b , c":    {"a", "b", "c"},
		"\ta\t,\tb\t": {"a", "b"},
	}
	for in, want := range cases {
		got := splitCSV(in)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("splitCSV(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestQuietMode_SuppressesOutput(t *testing.T) {
	orig := output.Quiet
	defer func() { output.Quiet = orig }()

	output.Quiet = true
	out := captureStdout(t, func() {
		output.Success("should be suppressed")
		output.Info("should be suppressed")
		output.Bold("should be suppressed")
	})
	if out != "" {
		t.Errorf("quiet should suppress, got: %q", out)
	}
}

func TestDryRunOutput_PlainText(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = false
	out := captureStdout(t, func() {
		if !dryRunOutput("create mr", map[string]any{"title": "x"}) {
			t.Error("dryRunOutput should return true")
		}
	})
	if !strings.Contains(out, "[dry-run] create mr") {
		t.Errorf("expected dry-run prefix, got: %s", out)
	}
}

func TestDryRunOutput_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true
	out := captureStdout(t, func() {
		dryRunOutput("delete mr", map[string]any{"iid": 5})
	})
	for _, want := range []string{`"action": "delete mr"`, `"dryRun": true`, `"iid": 5`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in JSON dry-run, got:\n%s", want, out)
		}
	}
}

func TestErrorCodeFromStatus_AllMapped(t *testing.T) {
	cases := map[int]output.ErrorCode{
		401: output.ErrAuth,
		403: output.ErrForbidden,
		404: output.ErrNotFound,
		409: output.ErrConflict,
		429: output.ErrRateLimit,
		500: output.ErrServer,
		503: output.ErrServer,
		400: output.ErrValidation,
		422: output.ErrValidation,
	}
	for status, want := range cases {
		got := output.ErrorCodeFromStatus(status)
		if got != want {
			t.Errorf("ErrorCodeFromStatus(%d) = %v, want %v", status, got, want)
		}
	}
}

func TestValidateOutputPath_RejectsDotDot(t *testing.T) {
	if err := validateOutputPath("../etc/passwd"); err == nil {
		t.Error("expected error for path with .., got nil")
	}
}

func TestValidateOutputPath_AcceptsNormal(t *testing.T) {
	if err := validateOutputPath("output.zip"); err != nil {
		t.Errorf("expected nil for normal path, got: %v", err)
	}
}

func TestValidateLimit(t *testing.T) {
	cases := []struct {
		in      int
		want    int
		wantErr bool
	}{
		{0, 0, true},
		{1, 1, false},
		{50, 50, false},
		{100, 100, false},
		{200, 100, false},
	}
	for _, tc := range cases {
		got, err := validateLimit(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("validateLimit(%d) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("validateLimit(%d) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("validateLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestReference_JSON(t *testing.T) {
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"reference", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"version"`, `"commands"`, `"auth"`} {
		if !strings.Contains(out, want) {
			t.Errorf("reference --json missing %q, got:\n%s", want, out)
		}
	}
}

func TestHandleAPIError_JSON(t *testing.T) {
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	// Capture stderr where PrintErrorJSONWithCode writes.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	apiErr := &api.APIError{StatusCode: 404}
	_ = handleAPIError(apiErr, true)

	_ = w.Close()
	os.Stderr = origStderr
	<-done
	_ = r.Close()

	out := buf.String()
	if !strings.Contains(out, "404") && !strings.Contains(out, "error") {
		t.Errorf("expected JSON error envelope with 404, got:\n%s", out)
	}
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit after API error, got %d", lastExit)
	}
}

func TestResolveContent_Inline(t *testing.T) {
	content, encoding, err := resolveContent("hello", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "hello" {
		t.Errorf("content = %q, want %q", content, "hello")
	}
	if encoding != "text" {
		t.Errorf("encoding = %q, want %q", encoding, "text")
	}
}

func TestGitCommitSubject(t *testing.T) {
	// gitCommitSubject runs git log -1 --pretty=%s
	// In a non-git directory it returns ""
	result := gitCommitSubject()
	// Just verify it doesn't panic and returns a string
	_ = result
}
