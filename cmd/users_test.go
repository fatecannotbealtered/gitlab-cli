package cmd

import (
	"context"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// TestResolveUser covers the shared user lookup the MR, issue and project groups
// use for --assignee, --reviewer and the like.
func TestResolveUser(t *testing.T) {
	e := newTestEnv(t)
	c, err := clientFor(e.app(), e.gl.URL, testToken)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for in, want := range map[string]string{"erin": "erin", "@dave": "dave", "me": "carol", " @Erin ": "erin"} { //nolint:gocritic // the spaces are the input under test
		u, err := resolveUser(ctx, c, in, "--assignee")
		if err != nil || u.Username != want || userRef(u.Username) != "@"+want {
			t.Errorf("resolveUser(%q) = %v, %v", in, u, err)
		}
	}
	_, err = resolveUser(ctx, c, "nobody", "--assignee")
	if e, ok := err.(*aicli.Error); !ok || e.Code != "E_NOT_FOUND" || e.Details["parameter"] != "--assignee" {
		t.Errorf("an unknown user: %v", err)
	}
	if _, err = resolveUser(ctx, c, "@", "--reviewer"); err == nil {
		t.Error("an empty user must fail")
	}
}
