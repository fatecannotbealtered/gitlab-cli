package output

import (
	"strings"
	"testing"
)

func TestFilterMap_NoFilter_ReturnsAll(t *testing.T) {
	m := map[string]any{"a": 1, "b": 2}
	out := FilterMap(m, nil)
	if len(out) != 2 {
		t.Errorf("expected 2 keys, got %d", len(out))
	}
}

func TestFilterMap_SelectsCaseInsensitive(t *testing.T) {
	m := map[string]any{"id": 1, "username": "alice", "email": "a@x"}
	out := FilterMap(m, []string{"ID", "Email"})
	if len(out) != 2 {
		t.Errorf("expected 2 keys, got %d (%v)", len(out), out)
	}
	// FilterMap preserves the original key from m (case-insensitive lookup only)
	if out["id"] != 1 {
		t.Errorf("id = %v, want 1", out["id"])
	}
	if out["email"] != "a@x" {
		t.Errorf("email = %v, want a@x", out["email"])
	}
}

func TestFilterMap_IgnoresUnknown(t *testing.T) {
	m := map[string]any{"id": 1}
	out := FilterMap(m, []string{"id", "not-a-field"})
	if len(out) != 1 {
		t.Errorf("expected 1 key, got %v", out)
	}
}

func TestUserToMap_OmitsEmpty(t *testing.T) {
	u := FlatUser{ID: 1, Username: "alice"}
	m := UserToMap(u)
	if _, ok := m["email"]; ok {
		t.Error("email should be omitted when empty")
	}
	if _, ok := m["name"]; ok {
		t.Error("name should be omitted when empty")
	}
}

func TestErrorCodeFromStatus(t *testing.T) {
	cases := map[int]ErrorCode{
		401: ErrAuth,
		403: ErrForbidden,
		404: ErrNotFound,
		409: ErrConflict,
		429: ErrRateLimit,
		500: ErrServer,
		503: ErrServer,
		400: ErrValidation,
		422: ErrValidation,
		0:   ErrUnknown,
	}
	for status, want := range cases {
		got := ErrorCodeFromStatus(status)
		// 0 is special: ErrorCodeFromStatus returns ErrUnknown via default branch
		if status == 0 && got != ErrUnknown {
			t.Errorf("ErrorCodeFromStatus(0) = %v, want %v", got, want)
		}
		if status != 0 && got != want {
			t.Errorf("ErrorCodeFromStatus(%d) = %v, want %v", status, got, want)
		}
	}
}

func TestHintForErrorCode_AllCodesHaveHint(t *testing.T) {
	codes := []ErrorCode{
		ErrConfig, ErrAuth, ErrForbidden, ErrNotFound, ErrConflict,
		ErrRateLimit, ErrServer, ErrValidation, ErrNetwork,
	}
	for _, c := range codes {
		if HintForErrorCode(c) == "" {
			t.Errorf("HintForErrorCode(%v) returned empty hint", c)
		}
	}
}

func TestStripAnsi(t *testing.T) {
	in := "\033[31mhello\033[0m"
	out := stripAnsi(in)
	if out != "hello" {
		t.Errorf("stripAnsi = %q, want hello", out)
	}
}

func TestRuneWidth_CJK(t *testing.T) {
	if runeWidth("中文") != 4 {
		t.Errorf("CJK width should be 4, got %d", runeWidth("中文"))
	}
	if runeWidth("abc") != 3 {
		t.Errorf("ASCII width should be 3, got %d", runeWidth("abc"))
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello world", 5); !strings.HasSuffix(got, "\u2026") {
		t.Errorf("truncate should end with ellipsis, got %q", got)
	}
	if got := truncate("ok", 5); got != "ok" {
		t.Errorf("short string should be unchanged, got %q", got)
	}
}

func TestStatusBadge_AllStates(t *testing.T) {
	// Sanity: badge function should not panic for arbitrary state strings.
	for _, s := range []string{"opened", "closed", "merged", "running", "failed", "skipped", "manual", "unknown_state"} {
		_ = StatusBadge(s)
	}
}
