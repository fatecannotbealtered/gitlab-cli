package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
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
	t.Setenv("GITLAB_AUDIT_RETENTION_MONTHS", "-1")
	if retentionMonths() != 3 {
		t.Errorf("negative input fallback = %d, want 3", retentionMonths())
	}
}

func TestDir_DefaultUsesConfigDir(t *testing.T) {
	SetDirForTest("")
	t.Cleanup(func() { SetDirForTest("") })

	home := t.TempDir()
	t.Setenv("HOME", home)

	want := filepath.Join(config.Dir(), "audit")
	if got := Dir(); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestDir_TestOverride(t *testing.T) {
	dir := newAuditTempDir(t)
	if got := Dir(); got != dir {
		t.Errorf("Dir() = %q, want test override %q", got, dir)
	}
}

func TestFiles_NotExistReturnsNil(t *testing.T) {
	SetDirForTest(filepath.Join(t.TempDir(), "missing-audit-dir"))
	t.Cleanup(func() { SetDirForTest("") })

	files, err := Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if files != nil {
		t.Errorf("Files() = %v, want nil for missing dir", files)
	}
}

func denyDirRead(t *testing.T, dir string) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		for _, args := range [][]string{
			{"icacls", dir, "/inheritance:r"},
			{"icacls", dir, "/deny", os.Getenv("USERNAME") + ":(OI)(CI)R"},
		} {
			cmd := exec.Command(args[0], args[1:]...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Skipf("%v: %v\n%s", args, err, out)
			}
		}
		t.Cleanup(func() {
			_ = exec.Command("icacls", dir, "/grant", os.Getenv("USERNAME")+":(OI)(CI)F").Run()
		})
		// Privileged processes (e.g. GitHub Actions runners run elevated)
		// bypass deny ACLs via SeBackupPrivilege, so the deny may not bite.
		// The scenario is then untestable on this machine: skip, don't fail.
		if _, err := os.ReadDir(dir); err == nil {
			t.Skip("process bypasses deny ACL (privileged); cannot simulate unreadable dir")
		}
	default:
		if err := os.Chmod(dir, 0000); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	}
}

func TestFiles_ReadDirError(t *testing.T) {
	base := t.TempDir()
	locked := filepath.Join(base, "locked")
	if err := os.Mkdir(locked, 0700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	denyDirRead(t, locked)

	SetDirForTest(locked)
	t.Cleanup(func() { SetDirForTest("") })

	if _, err := Files(); err == nil {
		t.Fatal("expected Files error when audit dir is unreadable")
	}
}

func TestFiles_SortedJSONL(t *testing.T) {
	dir := newAuditTempDir(t)
	for _, name := range []string{"audit-z.jsonl", "audit-a.jsonl", "notes.txt"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatalf("WriteFile(%q): %v", name, err)
		}
	}

	files, err := Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 jsonl files, got %d: %v", len(files), files)
	}
	if filepath.Base(files[0]) != "audit-a.jsonl" || filepath.Base(files[1]) != "audit-z.jsonl" {
		t.Errorf("Files() order = %v, want audit-a then audit-z", files)
	}
}

func TestCleanup_RemovesOldFiles(t *testing.T) {
	dir := newAuditTempDir(t)
	t.Setenv("GITLAB_AUDIT_RETENTION_MONTHS", "3")

	oldPath := filepath.Join(dir, "audit-2020-01.jsonl")
	if err := os.WriteFile(oldPath, []byte("old\n"), 0600); err != nil {
		t.Fatalf("WriteFile old: %v", err)
	}
	keepPath := filepath.Join(dir, "audit-2099-12.jsonl")
	if err := os.WriteFile(keepPath, []byte("keep\n"), 0600); err != nil {
		t.Fatalf("WriteFile keep: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile other: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "audit-bad.txt"), []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile audit-bad: %v", err)
	}

	cleanup(dir)

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected old audit file removed, stat err = %v", err)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Errorf("expected recent audit file kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "other.txt")); err != nil {
		t.Errorf("expected unrelated file kept: %v", err)
	}
}

func TestCleanup_RetentionZeroSkipsDeletion(t *testing.T) {
	dir := newAuditTempDir(t)
	t.Setenv("GITLAB_AUDIT_RETENTION_MONTHS", "0")

	oldPath := filepath.Join(dir, "audit-2020-01.jsonl")
	if err := os.WriteFile(oldPath, []byte("old\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cleanup(dir)

	if _, err := os.Stat(oldPath); err != nil {
		t.Errorf("retention=0 should not delete old files: %v", err)
	}
}

func TestCleanup_ReadDirError(t *testing.T) {
	base := t.TempDir()
	notDir := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(notDir, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cleanup(notDir) // should return without panic
}

func TestLog_MkdirAllError(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocked")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	SetDirForTest(blocker)
	t.Cleanup(func() { SetDirForTest("") })
	t.Setenv("GITLAB_NO_AUDIT", "")

	Log("blocked", []string{"x"}, 1, 1) // MkdirAll fails; must not panic
}

func TestLog_OpenFileError(t *testing.T) {
	dir := newAuditTempDir(t)
	t.Setenv("GITLAB_NO_AUDIT", "")

	monthFile := "audit-" + time.Now().Format("2006-01") + ".jsonl"
	if err := os.Mkdir(filepath.Join(dir, monthFile), 0700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	Log("open-fail", []string{"y"}, 2, 3)

	files, err := Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected no jsonl files when OpenFile fails, got %v", files)
	}
}

func TestLog_MarshalError(t *testing.T) {
	_ = newAuditTempDir(t)
	t.Setenv("GITLAB_NO_AUDIT", "")

	orig := marshalEntry
	marshalEntry = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	t.Cleanup(func() { marshalEntry = orig })

	Log("marshal-fail", []string{"x"}, 1, 2)

	files, err := Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected no audit files on marshal error, got %d", len(files))
	}
}

func TestSanitizeArgs_LongValueRedacted(t *testing.T) {
	long := strings.Repeat("x", 257)
	got := sanitizeArgs([]string{"plain", long})
	want := []string{"plain", "[REDACTED len=257]"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("sanitizeArgs long value = %v, want %v", got, want)
	}
}
