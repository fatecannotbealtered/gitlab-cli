//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/e2e"
)

var sharedFX *e2e.Fixture

func TestMain(m *testing.M) {
	e2e.LoadLocalEnv()
	if !e2e.Configured() {
		fmt.Fprintln(os.Stderr, "integration: GITLAB_CLI_HOST/TOKEN not set; tests will skip")
		os.Exit(m.Run())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	fx, err := e2e.Bootstrap(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration bootstrap failed: %v\n", err)
		os.Exit(1)
	}
	sharedFX = fx
	code := m.Run()
	teardownCtx, teardownCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer teardownCancel()
	e2e.Teardown(teardownCtx, sharedFX)
	os.Exit(code)
}
