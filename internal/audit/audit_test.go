package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func newAuditTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	SetDirForTest(dir)
	t.Cleanup(func() { SetDirForTest("") })
	return dir
}

func TestLog_WritesJSONL(t *testing.T) {
	dir := newAuditTempDir(t)
	t.Setenv("GITLAB_NO_AUDIT", "")

	Log("gitlab-cli mr create", []string{"mr", "create", "--title", "x"}, 0, 12)

	files, err := Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 audit file, got %d (in %s)", len(files), dir)
	}

	f, err := os.Open(files[0])
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Scan()
	line := scanner.Bytes()

	var got map[string]any
	if err := json.Unmarshal(line, &got); err != nil {
		t.Fatalf("invalid JSONL line: %v (line=%s)", err, string(line))
	}
	for _, key := range []string{"ts", "cmd", "args", "exit", "ms"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing key %q in %v", key, got)
		}
	}
	if got["cmd"] != "gitlab-cli mr create" {
		t.Errorf("cmd = %v", got["cmd"])
	}
}

func TestLog_NoAuditEnv_SuppressesLog(t *testing.T) {
	_ = newAuditTempDir(t)
	t.Setenv("GITLAB_NO_AUDIT", "1")

	Log("anything", []string{"x"}, 0, 1)

	files, _ := Files()
	if len(files) != 0 {
		t.Errorf("expected no files when GITLAB_NO_AUDIT=1, got %d", len(files))
	}
}

func TestSanitizeArgs_TokenFlag(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "two-arg --token",
			in:   []string{"auth", "login", "--token", "secret"},
			want: []string{"auth", "login"},
		},
		{
			name: "combined --token=value",
			in:   []string{"auth", "login", "--token=secret"},
			want: []string{"auth", "login", "--token=***"},
		},
		{
			name: "short -t",
			in:   []string{"auth", "login", "-t", "secret"},
			want: []string{"auth", "login"},
		},
		{
			name: "case-insensitive --TOKEN=",
			in:   []string{"auth", "login", "--TOKEN=secret"},
			want: []string{"auth", "login", "--TOKEN=***"},
		},
		{
			name: "private-token",
			in:   []string{"--private-token", "abc", "extra"},
			want: []string{"extra"},
		},
		{
			name: "non-sensitive untouched",
			in:   []string{"--title", "Hello", "--description", "world"},
			want: []string{"--title", "Hello", "--description", "world"},
		},
		{
			name: "value flag (CI/CD secret) is redacted",
			in:   []string{"variable", "create", "--key", "MY_SECRET", "--value", "glpat-xxx"},
			want: []string{"variable", "create", "--key", "MY_SECRET"},
		},
		{
			name: "value=... combined form is redacted",
			in:   []string{"variable", "create", "--key", "K", "--value=glpat-xxx"},
			want: []string{"variable", "create", "--key", "K", "--value=***"},
		},
		{
			name: "variable flag two-arg KEY=VAL",
			in:   []string{"pipeline", "create", "--project", "1", "--ref", "main", "--variable", "SECRET=abc"},
			want: []string{"pipeline", "create", "--project", "1", "--ref", "main"},
		},
		{
			name: "variable=KEY=VAL combined form",
			in:   []string{"pipeline", "create", "--project", "1", "--variable=API_KEY=secret"},
			want: []string{"pipeline", "create", "--project", "1", "--variable=***"},
		},
		{
			name: "repeatable --variable flags",
			in:   []string{"pipeline", "create", "--variable", "A=1", "--variable", "B=2"},
			want: []string{"pipeline", "create"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeArgs(tc.in)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("sanitizeArgs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRetentionMonths(t *testing.T) {
	t.Setenv("GITLAB_AUDIT_RETENTION_MONTHS", "")
	if retentionMonths() != 3 {
		t.Errorf("default = %d, want 3", retentionMonths())
	}
	t.Setenv("GITLAB_AUDIT_RETENTION_MONTHS", "0")
	if retentionMonths() != 0 {
		t.Errorf("0 = %d, want 0", retentionMonths())
	}
	t.Setenv("GITLAB_AUDIT_RETENTION_MONTHS", "12")
	if retentionMonths() != 12 {
		t.Errorf("12 = %d, want 12", retentionMonths())
	}
	t.Setenv("GITLAB_AUDIT_RETENTION_MONTHS", "junk")
	if retentionMonths() != 3 {
		t.Errorf("invalid input fallback = %d, want 3", retentionMonths())
	}
}
