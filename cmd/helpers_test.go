package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/spf13/cobra"
)

func TestRequireLimit_OK(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("limit", 20, "")
	_ = cmd.Flags().Set("limit", "50")

	limit, err := requireLimit(cmd)
	if err != nil {
		t.Fatalf("requireLimit: %v", err)
	}
	if limit != 50 {
		t.Fatalf("limit = %d, want 50", limit)
	}
}

func TestRequireLimit_Invalid(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("limit", 20, "")
	_ = cmd.Flags().Set("limit", "0")

	_, err := requireLimit(cmd)
	if err == nil {
		t.Fatal("expected error for limit 0")
	}
}

func TestSleepContext_ZeroDuration(t *testing.T) {
	if err := sleepContext(context.Background(), 0); err != nil {
		t.Fatalf("sleepContext(0): %v", err)
	}
	if err := sleepContext(context.Background(), -1*time.Second); err != nil {
		t.Fatalf("sleepContext(-1): %v", err)
	}
}

func TestSleepContext_Completes(t *testing.T) {
	if err := sleepContext(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("sleepContext: %v", err)
	}
}

func TestSleepContext_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sleepContext(ctx, time.Hour)
	if err != context.Canceled {
		t.Fatalf("sleepContext cancel = %v, want context.Canceled", err)
	}
}

func TestListLimitFromFlags_All(t *testing.T) {
	cmd := &cobra.Command{}
	registerListFlags(cmd)
	_ = cmd.Flags().Set("all", "true")

	limit, fetchAll, err := listLimitFromFlags(cmd)
	if err != nil {
		t.Fatalf("listLimitFromFlags: %v", err)
	}
	if !fetchAll {
		t.Fatal("fetchAll = false, want true")
	}
	if limit != listAllMax {
		t.Fatalf("limit = %d, want %d", limit, listAllMax)
	}
}

func TestListLimitFromFlags_LimitError(t *testing.T) {
	cmd := &cobra.Command{}
	registerListFlags(cmd)
	_ = cmd.Flags().Set("limit", "0")

	_, _, err := listLimitFromFlags(cmd)
	if err == nil {
		t.Fatal("expected error for invalid limit")
	}
}

func TestPrintListJSON_HasMoreAndFields(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("fields", "", "")
	_ = cmd.Flags().Set("fields", "title")

	rows := []map[string]any{
		{"title": "one", "state": "opened"},
	}
	pag := api.Pagination{Page: 1, Total: 10, NextPage: 2}

	out := captureStdout(t, func() {
		printListJSON(cmd, rows, pag, 1, false)
	})
	for _, want := range []string{`"hasMore": true`, `"title": "one"`, `"count": 1`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
	if strings.Contains(out, `"state"`) {
		t.Errorf("fields filter should drop state, got:\n%s", out)
	}
}

func TestPrintListJSON_FetchAllNoHasMore(t *testing.T) {
	cmd := &cobra.Command{}
	rows := []map[string]any{{"title": "one"}}
	pag := api.Pagination{Page: 1, Total: 1, NextPage: 2}

	out := captureStdout(t, func() {
		printListJSON(cmd, rows, pag, 1, true)
	})
	if strings.Contains(out, `"hasMore": true`) {
		t.Errorf("fetchAll should suppress hasMore, got:\n%s", out)
	}
	if !strings.Contains(out, `"all": true`) {
		t.Errorf("expected all=true in meta, got:\n%s", out)
	}
}
