package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdoutToFile(t *testing.T, fn func()) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	fn()
	_ = f.Close()
	os.Stdout = old
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTruncate_Boundaries(t *testing.T) {
	if got := truncate("anything", 0); got != "" {
		t.Errorf("maxWidth 0 = %q, want empty", got)
	}
	if got := truncate("anything", -1); got != "" {
		t.Errorf("maxWidth -1 = %q, want empty", got)
	}
	if got := truncate("ok", 5); got != "ok" {
		t.Errorf("short string = %q, want ok", got)
	}
	if got := truncate("\033[31mhello\033[0m", 3); !strings.HasSuffix(got, "\u2026") {
		t.Errorf("ANSI string truncate = %q, want ellipsis suffix", got)
	}
	// CJK: each rune width 2; maxWidth 3 leaves room for one CJK + ellipsis
	if got := truncate("中文测试", 3); got != "中\u2026" {
		t.Errorf("CJK truncate = %q, want 中…", got)
	}
}

func TestIsCJK_Ranges(t *testing.T) {
	cases := []rune{
		'中',
		'あ',
		'ア',
		'한',
		0x2E80, 0x2F00, 0x3000, 0x3200, 0x3300,
		0xF900, 0xFE30, 0xFF01, 0xFFE0, 0x20000,
	}
	for _, r := range cases {
		if !isCJK(r) {
			t.Errorf("isCJK(%U) = false, want true", r)
		}
	}
	if isCJK('A') {
		t.Error("ASCII 'A' should not be CJK")
	}
}

func TestTermWidth_NonTTY(t *testing.T) {
	orig := stdoutGetSize
	stdoutGetSize = func(int) (int, int, error) { return 0, 0, os.ErrInvalid }
	defer func() { stdoutGetSize = orig }()
	if got := termWidth(); got != 120 {
		t.Errorf("termWidth on error = %d, want default 120", got)
	}
}

func TestTermWidth_Success(t *testing.T) {
	orig := stdoutGetSize
	stdoutGetSize = func(int) (int, int, error) { return 100, 0, nil }
	defer func() { stdoutGetSize = orig }()
	if got := termWidth(); got != 100 {
		t.Errorf("termWidth = %d, want 100", got)
	}
}

func TestTable_QuietAndEmpty(t *testing.T) {
	oldQuiet := Quiet
	t.Cleanup(func() { Quiet = oldQuiet })

	Quiet = true
	out := captureStdoutToFile(t, func() {
		Table([]string{"A"}, [][]string{{"1"}})
	})
	if out != "" {
		t.Errorf("Quiet Table should produce no output, got %q", out)
	}

	out = captureStdoutToFile(t, func() {
		Table(nil, nil)
	})
	if out != "" {
		t.Errorf("empty headers Table should produce no output, got %q", out)
	}
}

func TestTable_WideLayout(t *testing.T) {
	oldQuiet := Quiet
	Quiet = false
	t.Cleanup(func() { Quiet = oldQuiet })

	headers := []string{"COL1", "COL2", "COL3", "COL4", "COL5", "COL6", "COL7", "COL8", "COL9", "COL10"}
	row := make([]string, len(headers))
	for i := range row {
		row[i] = strings.Repeat("x", 20)
	}
	rows := [][]string{row, row}

	out := captureStdoutToFile(t, func() {
		Table(headers, rows)
	})
	if !strings.Contains(out, "┌") || !strings.Contains(out, "└") {
		t.Errorf("Table output missing borders: %q", out)
	}
}

func TestTable_TruncatesWideCells(t *testing.T) {
	oldQuiet := Quiet
	Quiet = false
	t.Cleanup(func() { Quiet = oldQuiet })

	out := captureStdoutToFile(t, func() {
		Table([]string{"H"}, [][]string{{strings.Repeat("w", 200)}})
	})
	if !strings.Contains(out, "\u2026") {
		t.Error("wide cell should be truncated with ellipsis")
	}
}

func TestTable_ShortRowPadding(t *testing.T) {
	oldQuiet := Quiet
	Quiet = false
	t.Cleanup(func() { Quiet = oldQuiet })

	out := captureStdoutToFile(t, func() {
		Table([]string{"NAME", "VAL"}, [][]string{{"a"}})
	})
	if !strings.Contains(out, "│") {
		t.Errorf("expected table row, got %q", out)
	}
}

func TestTable_MinColumnWidthBreak(t *testing.T) {
	oldQuiet := Quiet
	Quiet = false
	t.Cleanup(func() { Quiet = oldQuiet })

	headers := make([]string, 60)
	rows := make([][]string, 1)
	rows[0] = make([]string, 60)
	for i := range headers {
		headers[i] = strings.Repeat("W", 5)
		rows[0][i] = strings.Repeat("v", 5)
	}

	out := captureStdoutToFile(t, func() {
		Table(headers, rows)
	})
	if out == "" {
		t.Fatal("expected table output")
	}
}

func TestCellPadding(t *testing.T) {
	if got := cellPadding(3, 5); got != 0 {
		t.Errorf("cellPadding(3,5) = %d, want 0", got)
	}
	if got := cellPadding(5, 3); got != 2 {
		t.Errorf("cellPadding(5,3) = %d, want 2", got)
	}
}

func TestTable_HeaderPaddingClamp(t *testing.T) {
	oldQuiet := Quiet
	Quiet = false
	t.Cleanup(func() { Quiet = oldQuiet })

	orig := stdoutGetSize
	stdoutGetSize = func(int) (int, int, error) { return 4, 0, nil }
	defer func() { stdoutGetSize = orig }()

	// ToUpper("ß") => "SS" (wider than col width 1 after shrink) exercises padding < 0 clamp.
	out := captureStdoutToFile(t, func() {
		Table([]string{"ß"}, [][]string{{strings.Repeat("x", 200)}})
	})
	if !strings.Contains(out, "│") {
		t.Fatalf("expected table output, got %q", out)
	}
}

func TestRuneWidth_NonCJKUnicode(t *testing.T) {
	if w := runeWidth("abc"); w != 3 {
		t.Errorf("runeWidth ascii = %d, want 3", w)
	}
}
