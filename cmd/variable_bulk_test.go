package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return p
}

// variableImportServer mocks create (POST) + update (PUT). Keys in existing
// already exist, so create returns 400 and the importer falls back to update.
func variableImportServer(t *testing.T, existing map[string]bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			// create: key is in the JSON body; infer "exists" by counting — simpler
			// to key off a header is not possible, so read query/body minimally.
			// We treat any key marked existing as a 400 conflict.
			body := readAllForTest(r)
			for k := range existing {
				if strings.Contains(body, `"`+k+`"`) {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"message":{"key":["has already been taken"]}}`))
					return
				}
			}
			_, _ = w.Write([]byte(`{"key":"X","value":"v","environment_scope":"*"}`))
		case http.MethodPut:
			_, _ = w.Write([]byte(`{"key":"X","value":"v","environment_scope":"*"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	return srv
}

func readAllForTest(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	buf := make([]byte, 4096)
	n, _ := r.Body.Read(buf)
	return string(buf[:n])
}

func TestVariableBulkImport_DryRun_Dotenv(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	resetSliceFlagsForTest(t, []string{"variable", "bulk-import"})
	file := writeTempFile(t, "vars.env", "FOO=bar\n# comment\nexport BAZ=\"qux\"\n")
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "x")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"variable", "bulk-import", "--project", "g/r", "--file", file, "--dangerous", "--dry-run", "--json", "--compact"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"total":2`, `"targets":["FOO","BAZ"]`, `"action":"variable bulk-import"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run, got:\n%s", want, out)
		}
	}
}

func TestVariableBulkImport_DryRun_JSON(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	resetSliceFlagsForTest(t, []string{"variable", "bulk-import"})
	file := writeTempFile(t, "vars.json", `{"A":"1","B":"2"}`)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "x")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"variable", "bulk-import", "--project", "g/r", "--file", file, "--dangerous", "--dry-run", "--json", "--compact"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"targets":["A","B"]`) {
		t.Errorf("expected JSON key order A,B, got:\n%s", out)
	}
}

func TestVariableBulkImport_Confirm_CreateAndUpdate(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv := variableImportServer(t, map[string]bool{"EXISTING": true})
	defer srv.Close()
	file := writeTempFile(t, "vars.env", "NEWKEY=a\nEXISTING=b\n")
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"variable", "bulk-import", "--project", "g/r", "--file", file, "--dangerous", "--json", "--compact"}))
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"succeeded":2`, `"failed":0`, `"action":"created"`, `"action":"updated"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in import output, got:\n%s", want, out)
		}
	}
}

func TestVariableBulkImport_PartialFailure(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	// Server returns 403 on POST for forbidden keys (not an "exists" conflict),
	// so the importer surfaces a per-item failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := readAllForTest(r)
		if strings.Contains(body, `"DENIED"`) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
			return
		}
		_, _ = w.Write([]byte(`{"key":"OK","value":"v","environment_scope":"*"}`))
	}))
	defer srv.Close()
	file := writeTempFile(t, "vars.env", "OKKEY=a\nDENIED=b\n")
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"variable", "bulk-import", "--project", "g/r", "--file", file, "--dangerous", "--json", "--compact"}))
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"ok":true`, `"succeeded":1`, `"failed":1`, `"E_FORBIDDEN"`, `"target":"DENIED"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in partial-failure output, got:\n%s", want, out)
		}
	}
}

// bulk-import writes/overwrites secret CI variables, so it is write-dangerous:
// even with a valid file and --dry-run it must be rejected without --dangerous,
// mirroring TestMRBulkMerge_MissingDangerous_Rejected.
func TestVariableBulkImport_MissingDangerous_Rejected(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	resetSliceFlagsForTest(t, []string{"variable", "bulk-import"})
	file := writeTempFile(t, "vars.env", "FOO=bar\nBAZ=qux\n")
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "x")

	out := captureStdout(t, func() {
		// Dangerous batch without --dangerous is rejected even with --dry-run.
		rootCmd.SetArgs([]string{"variable", "bulk-import", "--project", "g/r", "--file", file, "--dry-run", "--json", "--compact"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "E_CONFIRMATION_REQUIRED") || !strings.Contains(out, "--dangerous") {
		t.Errorf("expected dangerous-gate rejection, got:\n%s", out)
	}
}

func TestVariableBulkImport_MissingFile(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	resetSliceFlagsForTest(t, []string{"variable", "bulk-import"})
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "x")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"variable", "bulk-import", "--project", "g/r", "--file", "/nonexistent/path.env", "--dry-run", "--json", "--compact"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "E_VALIDATION") {
		t.Errorf("expected E_VALIDATION for missing file, got:\n%s", out)
	}
}
