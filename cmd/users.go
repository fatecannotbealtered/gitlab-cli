package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// Users are named the way GitLab writes them: "@carol" (userRef). Commands that
// take a user (assignees, reviewers, authors) accept "carol", "@carol" and "me",
// and resolve the name inside the write's Preview, so the preview shows the user
// it will send.

// userRef is a user's ref (CLI-048).
func userRef(username string) string { return "@" + username }

// resolveUser finds one user by username ("carol" or "@carol"), or the token's
// own user for "me". A name GitLab does not know is E_NOT_FOUND naming parameter.
func resolveUser(ctx context.Context, c *api.Client, name, parameter string) (*api.User, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "@")
	if name == "" {
		return nil, aicli.Validation(parameter, "a GitLab username, @username or me", "The user is empty.")
	}
	if name == "me" {
		u, err := c.Users.Me(ctx)
		if err != nil {
			return nil, readError(err)
		}
		return u, nil
	}
	u, err := c.Users.GetByUsername(ctx, name)
	if err != nil {
		return nil, readError(err)
	}
	if u == nil {
		e := aicli.NotFound(fmt.Sprintf("GitLab has no user with the username given to %s.", parameter),
			"Check the username (the name after @ in GitLab), or ask the user.")
		e.Details["parameter"] = parameter
		return nil, e
	}
	return u, nil
}
