package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// UserAPI wraps user-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/users/
type UserAPI struct{ client *Client }

// Me retrieves the currently authenticated user (GET /user).
func (a *UserAPI) Me(ctx context.Context) (*User, error) {
	data, err := a.client.Get(ctx, a.client.APIPath("/user"))
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, fmt.Errorf("parsing user: %w", err)
	}
	return &u, nil
}

// Search searches for users (GET /users?search=...), up to limit across pages.
func (a *UserAPI) Search(ctx context.Context, query string, limit int) ([]User, error) {
	path := a.client.APIPath("/users") + "?search=" + url.QueryEscape(query)
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("parsing users: %w", err)
	}
	return users, nil
}

// SearchPage returns one page of the users whose name, username or public email
// matches query. GitLab keeps its search order for it: exact matches of the public
// email, username or name first, then by name. active keeps active users only.
//
// GET /api/v4/users?search=
func (a *UserAPI) SearchPage(ctx context.Context, query string, active bool, page, perPage int) ([]User, Pagination, error) {
	q := url.Values{}
	q.Set("search", strings.TrimSpace(query))
	if active {
		q.Set("active", "true")
	}
	data, pag, err := GetPage(ctx, a.client, a.client.APIPath("/users")+"?"+q.Encode(), page, perPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing users: %w", err)
	}
	return users, pag, nil
}

// GetByUsername fetches a user by username via /users?username=<name>.
// Returns nil, nil if no user found.
func (a *UserAPI) GetByUsername(ctx context.Context, username string) (*User, error) {
	path := a.client.APIPath("/users") + "?username=" + url.QueryEscape(username)
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("parsing users: %w", err)
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}
