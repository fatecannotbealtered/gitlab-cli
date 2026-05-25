package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func captureStdoutStderr(fn func()) (stdout, stderr string) {
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	fn()
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr
	var outBuf, errBuf bytes.Buffer
	_, _ = outBuf.ReadFrom(rOut)
	_, _ = errBuf.ReadFrom(rErr)
	return outBuf.String(), errBuf.String()
}

func withNoColor(t *testing.T, fn func()) {
	t.Helper()
	prev := os.Getenv("NO_COLOR")
	t.Setenv("NO_COLOR", "1")
	fn()
	if prev == "" {
		os.Unsetenv("NO_COLOR")
	} else {
		os.Setenv("NO_COLOR", prev)
	}
}

func withColorEnabled(t *testing.T, fn func()) {
	t.Helper()
	prevNC := os.Getenv("NO_COLOR")
	os.Unsetenv("NO_COLOR")

	console, err := openCharDevice()
	if err != nil {
		t.Skip("character device unavailable:", err)
	}
	defer console.Close()

	oldOut := os.Stdout
	os.Stdout = console
	defer func() { os.Stdout = oldOut }()

	if !isTerminal(os.Stdout) {
		t.Skip("stdout is not a terminal; cannot exercise color path")
	}

	fn()

	if prevNC == "" {
		os.Unsetenv("NO_COLOR")
	} else {
		os.Setenv("NO_COLOR", prevNC)
	}
}

func openCharDevice() (*os.File, error) {
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		return f, nil
	}
	return os.OpenFile("/dev/tty", os.O_WRONLY, 0)
}

func TestIsTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if isTerminal(f) {
		t.Error("closed file should not be terminal")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r) {
		t.Error("pipe read end should not be terminal")
	}

	if console, err := openCharDevice(); err == nil {
		defer console.Close()
		if !isTerminal(console) {
			t.Log("CONOUT$/dev/tty is not ModeCharDevice in this environment")
		}
	}
}

func TestNoColor(t *testing.T) {
	withNoColor(t, func() {
		if !noColor() {
			t.Error("NO_COLOR=1 should disable color")
		}
	})

	os.Unsetenv("NO_COLOR")
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()
	if !noColor() {
		t.Error("non-TTY stdout should disable color")
	}
}

func TestColorize(t *testing.T) {
	withNoColor(t, func() {
		if got := colorize("\033[31m", "x"); got != "x" {
			t.Errorf("noColor colorize = %q, want x", got)
		}
	})
	withColorEnabled(t, func() {
		got := colorize("\033[31m", "x")
		if got == "x" || !strings.Contains(got, "\033[31m") {
			t.Errorf("colorize with TTY = %q, want ANSI-wrapped", got)
		}
	})
}

func TestPrintFunctions_NoColor(t *testing.T) {
	withNoColor(t, func() {
		oldQuiet := Quiet
		Quiet = false
		defer func() { Quiet = oldQuiet }()

		stdout, stderr := captureStdoutStderr(func() {
			Success("ok")
			Error("bad")
			Warn("careful")
			Info("note")
			Bold("head")
			Gray("dim")
		})
		for _, line := range []string{stdout, stderr} {
			if strings.Contains(line, "\033[") {
				t.Errorf("NO_COLOR output should not contain ANSI: %q", line)
			}
		}
		if !strings.Contains(stdout, "ok") {
			t.Errorf("Success stdout = %q", stdout)
		}
		if !strings.Contains(stderr, "bad") {
			t.Errorf("Error stderr = %q", stderr)
		}
	})
}

func TestPrintFunctions_Quiet(t *testing.T) {
	oldQuiet := Quiet
	Quiet = true
	defer func() { Quiet = oldQuiet }()

	stdout, _ := captureStdoutStderr(func() {
		Success("hidden")
		Info("hidden")
		Bold("hidden")
		Gray("hidden")
	})
	if stdout != "" {
		t.Errorf("Quiet should suppress stdout, got %q", stdout)
	}
}

func TestFormatFunctions(t *testing.T) {
	withNoColor(t, func() {
		if FormatCyan("x") != "x" {
			t.Error("FormatCyan should pass through without color")
		}
	})
	withColorEnabled(t, func() {
		for _, fn := range []func(string) string{
			FormatCyan, FormatCyanBold, FormatGray, FormatGreen,
			FormatRed, FormatYellow, FormatBlue,
		} {
			got := fn("x")
			if got == "x" {
				t.Errorf("%T returned plain text", fn)
			}
		}
	})
}

func TestStatusBadge_AllBranches(t *testing.T) {
	states := map[string]bool{
		"opened": true, "running": true, "pending": true, "in_progress": true,
		"merged": true, "success": true, "closed": true,
		"failed": true, "canceled": true,
		"skipped": true, "manual": true, "scheduled": true,
		"locked": true, "created": true,
		"custom": true,
	}
	for state := range states {
		_ = StatusBadge(state)
		_ = StatusBadge(strings.ToUpper(state))
	}
	withColorEnabled(t, func() {
		got := StatusBadge("opened")
		if got == "opened" {
			t.Error("StatusBadge should colorize on TTY")
		}
	})
}

func TestPrintJSONErr_SuccessAndError(t *testing.T) {
	origCompact := Compact
	defer func() { Compact = origCompact }()
	Compact = false

	stdout, stderr := captureStdoutStderr(func() {
		if err := PrintJSONErr(map[string]any{"a": 1}); err != nil {
			t.Errorf("PrintJSONErr success: %v", err)
		}
	})
	if !strings.Contains(stdout, `"a"`) {
		t.Errorf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q", stderr)
	}

	_, stderr = captureStdoutStderr(func() {
		err := PrintJSONErr(make(chan int))
		if err == nil {
			t.Error("expected marshal error")
		}
	})
	if !strings.Contains(stderr, "json marshal error") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestPrintJSON_ExitOnError(t *testing.T) {
	if os.Getenv("TEST_PRINTJSON_EXIT") == "1" {
		PrintJSON(make(chan int))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPrintJSON_ExitOnError$", "-test.count=1")
	cmd.Env = append(os.Environ(), "TEST_PRINTJSON_EXIT=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", exitErr.ExitCode())
	}
}

func TestPrintErrorJSON(t *testing.T) {
	origCompact := Compact
	defer func() { Compact = origCompact }()
	Compact = false

	_, stderr := captureStdoutStderr(func() {
		PrintErrorJSON("not found", 404)
		PrintErrorJSON("local", 0)
	})
	if !strings.Contains(stderr, `"errorCode": "NOT_FOUND"`) {
		t.Errorf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, `"errorCode": "UNKNOWN_ERROR"`) {
		t.Errorf("status 0 stderr = %q", stderr)
	}
}

func TestPrintErrorJSONWithCode(t *testing.T) {
	_, stderr := captureStdoutStderr(func() {
		PrintErrorJSONWithCode("cancelled", 0, ErrCancelled)
	})
	if !strings.Contains(stderr, `"errorCode": "CANCELLED"`) {
		t.Errorf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "confirmation") {
		t.Errorf("expected cancelled hint in %q", stderr)
	}
}

func TestHintForErrorCode_DefaultAndCancelled(t *testing.T) {
	if got := HintForErrorCode(ErrCancelled); got == "" {
		t.Error("ErrCancelled should have hint")
	}
	if got := HintForErrorCode(ErrorCode("CUSTOM")); got != "" {
		t.Errorf("unknown code hint = %q, want empty", got)
	}
}

func TestEmitErrorPayload_Fallback(t *testing.T) {
	orig := emitJSONMarshal
	emitJSONMarshal = func(any) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { emitJSONMarshal = orig }()

	_, stderr := captureStdoutStderr(func() {
		emitErrorPayload("fail", 500, ErrServer)
	})
	if !strings.Contains(stderr, `"errorCode": "SERVER_ERROR"`) {
		t.Errorf("fallback stderr = %q", stderr)
	}
}

func TestPrintJSON_Compact(t *testing.T) {
	orig := Compact
	defer func() { Compact = orig }()

	Compact = false
	var indented bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	PrintJSON(map[string]any{"a": 1})
	_ = w.Close()
	os.Stdout = old
	_, _ = indented.ReadFrom(r)
	if !strings.Contains(indented.String(), "\n") {
		t.Error("expected indented JSON to contain newlines")
	}

	Compact = true
	var compact bytes.Buffer
	r2, w2, _ := os.Pipe()
	os.Stdout = w2
	PrintJSON(map[string]any{"a": 1})
	_ = w2.Close()
	os.Stdout = old
	_, _ = compact.ReadFrom(r2)
	out := strings.TrimSpace(compact.String())
	if strings.Contains(out, "\n") {
		t.Errorf("expected single-line compact JSON, got: %q", out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}
