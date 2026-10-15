package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The user noun of the project group: the signed-in account, one user, a search. A
// user's ref is @username (userRef, CLI-048); user get also takes the bare username,
// me, the profile URL and the display name, which can match several users
// (E_AMBIGUOUS).

const userEvidence = "simulated:cmd/user_test.go"

var (
	userFields        = []string{"ref", "id", "username", "name", "state", "bot", "email", "web_url"}
	userDefaultFields = []string{"ref", "name", "state", "web_url"}
	userUntrusted     = []string{"name"}
)

func registerUser(app *aicli.App) {
	group := addGroup(app, "user", "Read users: the signed-in account, one user, or a search by name", "users")
	app.Schema("user", aicli.SchemaDef{Shape: "object", Fields: userFields, Untrusted: userUntrusted})
	app.Schema("user_page", aicli.SchemaDef{Shape: "page", Fields: userFields, DefaultFields: userDefaultFields,
		Untrusted: userUntrusted})
	addUserMe(app, group)
	addUserGet(app, group)
	addUserSearch(app, group)
}

// userOut is a user as every user command returns it. email is there only when
// GitLab shows it (the account's own, or to an administrator).
func userOut(u api.User) map[string]any {
	return map[string]any{"ref": userRef(u.Username), "id": idString(u.ID), "username": u.Username, "name": u.Name,
		"state": optString(u.State), "bot": u.Bot, "email": optString(u.Email), "web_url": optString(u.WebURL)}
}

func addUserMe(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "me", RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := gitlabFor(app).get()
		if err != nil {
			return err
		}
		u, err := client.Users.Me(cmd.Context())
		if err != nil {
			return readError(err)
		}
		return app.OK(cmd, userOut(*u))
	}}
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show the account the token signs in as (user get me does the same)",
		Effects: []aicli.Effect{aicli.Observe}, Output: "user", ErrorCodes: gitlabReadCodes,
		Maturity: aicli.Verified, Evidence: []string{userEvidence, liveEvidence("user")},
		Examples: []string{"gitlab-cli user me", "gitlab-cli user me --fields ref,email"}})
}

func addUserGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <user>", RunE: func(cmd *cobra.Command, _ []string) error {
		raw, _ := cmd.Flags().GetString("user")
		if strings.TrimSpace(raw) == "" {
			e := aicli.Usage("missing_parameter", "--user", "`gitlab-cli user get` needs the user: @username, username, me or a name.")
			e.Details["next_action"] = "Pass the user as the argument, for example `gitlab-cli user get @carol`."
			return e
		}
		client, err := gitlabFor(app).get()
		if err != nil {
			return err
		}
		u, err := userTarget(cmd.Context(), client, raw)
		if err != nil {
			return err
		}
		return app.OK(cmd, userOut(*u))
	}}
	c.Flags().String("user", "", "@carol, carol, me, a profile URL or a display name")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one user by @username, username, profile URL, display name or me",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "user", Positional: "user",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{userEvidence, liveEvidence("user")},
		Examples: []string{"gitlab-cli user get @carol", `gitlab-cli user get "Carol Danvers"`}})
}

// userTarget finds the user an agent named (resolveUser): @carol, carol, me or a
// profile URL. A plain word that is no username is then taken as a display name
// (userByName); @carol and URLs name a username only.
func userTarget(ctx context.Context, c *api.Client, raw string) (*api.User, error) {
	name, exact, err := userArg(c.Host(), raw)
	if err != nil {
		return nil, err
	}
	u, err := resolveUser(ctx, c, name, "--user")
	var ae *aicli.Error
	if err == nil || exact || name == "me" || !errors.As(err, &ae) || ae.Code != "E_NOT_FOUND" {
		return u, err
	}
	return userByName(ctx, c, name)
}

// userArg reads a user as an agent has it: @carol (exact), carol, me, or a profile
// URL (exact): https://gitlab.example.com/carol or .../users/carol, the host and
// any relative URL root of the configured GitLab left out.
func userArg(host, raw string) (name string, exact bool, err error) {
	s := strings.TrimSpace(raw)
	if !strings.Contains(s, "://") {
		return strings.TrimPrefix(s, "@"), strings.HasPrefix(s, "@"), nil
	}
	path := ""
	if u, perr := url.Parse(s); perr == nil && u.Host != "" {
		path = u.Path
		if h, herr := url.Parse(host); herr == nil && strings.Trim(h.Path, "/") != "" {
			path = strings.TrimPrefix(path, strings.TrimRight(h.Path, "/"))
		}
	}
	path = strings.TrimPrefix(strings.Trim(path, "/"), "users/")
	if !projectSegment.MatchString(path) {
		return "", false, aicli.Validation("--user", "@username, username, me, a profile URL or a display name",
			"That URL is not a user's profile URL.")
	}
	return path, true, nil
}

// userByName finds a user by display name among GitLab's matches for it: users whose
// name or username has the name's words, in order and as whole words
// (projectNameMatches). One is that user; several are E_AMBIGUOUS (CLI-048); with
// more matches than one page holds, one proves nothing and E_USAGE asks for the
// @username.
func userByName(ctx context.Context, c *api.Client, name string) (*api.User, error) {
	users, pag, err := c.Users.SearchPage(ctx, name, false, 1, api.MaxPerPage)
	if err != nil {
		return nil, readError(err)
	}
	var matches []api.User
	for _, u := range users {
		if projectNameMatches(name, u.Name, u.Username) {
			matches = append(matches, u)
		}
	}
	more := pag.NextPage > 0
	search := fmt.Sprintf("`gitlab-cli user search --query %s`", shellArg(name))
	switch {
	case len(matches) == 1 && !more:
		return &matches[0], nil
	case len(matches) >= 2:
		cands := make([]aicli.Candidate, 0, len(matches))
		for _, u := range matches {
			cands = append(cands, aicli.Candidate{Ref: userRef(u.Username), Label: u.Name})
		}
		return nil, projectAmbiguous("More than one user has that name; choose one by their @username.", "--user", cands, more, search)
	case more:
		e := aicli.Usage("invalid_value", "--user", "Too many users match that name to pick one; pass the @username.")
		e.Details["expected"] = "@username"
		e.Details["next_action"] = fmt.Sprintf("Run %s to find the user, then pass their @username.", search)
		return nil, e
	}
	e := aicli.NotFound("GitLab has no user with that username or name, or this account cannot see them.",
		fmt.Sprintf("Run %s to find the user.", search))
	e.Details["parameter"] = "--user"
	return nil, e
}

func addUserSearch(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "search", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		query, _ := cmd.Flags().GetString("query")
		if query = strings.TrimSpace(query); query == "" {
			return aicli.Validation("--query", "part of a name or username, or a whole public email", "--query is empty.")
		}
		active, _ := cmd.Flags().GetBool("active")
		client, err := gitlabFor(app).get()
		if err != nil {
			return err
		}
		users, pag, err := client.Users.SearchPage(cmd.Context(), query, active, req.Page, req.Limit)
		if err != nil {
			return readError(err)
		}
		items := make([]map[string]any, 0, len(users))
		for _, u := range users {
			items = append(items, userOut(u))
		}
		scope := map[string]any{"query": query, "covers": "users this account can see whose name or username contains the text, or whose public email equals it"}
		if active {
			scope["state"] = "active"
		}
		// GitLab keeps its search order here: exact matches of the email, username or
		// name first, then by name (CLI-029).
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "relevance"))
	}}
	c.Flags().String("query", "", "Part of a name or username, or a whole public email")
	c.Flags().Bool("active", false, "Only active users: not blocked or deactivated")
	_ = c.MarkFlagRequired("query")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Search users by name, username or public email, exact matches first",
		Effects: []aicli.Effect{aicli.Observe}, Output: "user_page", ErrorCodes: gitlabReadCodes,
		Maturity: aicli.Verified, Evidence: []string{userEvidence, liveEvidence("user")},
		Examples: []string{"gitlab-cli user search --query carol", "gitlab-cli user search --query dave --active"}})
}
