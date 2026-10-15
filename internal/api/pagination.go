package api

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

const maxPerPage = 100

// MaxPerPage is the largest page GitLab serves (per_page).
const MaxPerPage = maxPerPage

// listPerPage returns per_page for list helpers (default 20, max 100).
func listPerPage(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > maxPerPage {
		return maxPerPage
	}
	return limit
}

// GetPage fetches one page of a GitLab list endpoint: page (from 1) and per_page
// (1-100) are set on path's query, which may carry the endpoint's filters. The
// Pagination tells whether another page follows (NextPage) and, when GitLab reports
// it, the total (HasTotal). 2.0 lists fetch one upstream page per CLI page, so
// has_more and next_offset come straight from GitLab (CLI-041).
func GetPage(ctx context.Context, c *Client, path string, page, perPage int) ([]byte, Pagination, error) {
	basePath, q, err := splitPathAndQuery(path)
	if err != nil {
		return nil, Pagination{}, err
	}
	if page < 1 {
		page = 1
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(listPerPage(perPage)))
	return c.GetWithPagination(ctx, basePath+"?"+q.Encode())
}

// PaginateGET fetches a GitLab list endpoint across pages using X-Next-Page until
// limit items are collected or there are no more pages. path may include query
// parameters; per_page and page are set or overridden by this helper.
// The returned Pagination reflects the last page fetched (use NextPage for hasMore).
func PaginateGET(ctx context.Context, c *Client, path string, limit int) ([]byte, Pagination, error) {
	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = 20
	}
	perPage := effectiveLimit
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	basePath, q, err := splitPathAndQuery(path)
	if err != nil {
		return nil, Pagination{}, err
	}
	q.Set("per_page", strconv.Itoa(perPage))

	if effectiveLimit <= maxPerPage {
		fullPath := basePath
		if enc := q.Encode(); enc != "" {
			fullPath += "?" + enc
		}
		data, pag, err := c.GetWithPagination(ctx, fullPath)
		return data, pag, err
	}

	var merged []json.RawMessage
	var lastPag Pagination
	page := 1
	for {
		q.Set("page", strconv.Itoa(page))
		fullPath := basePath + "?" + q.Encode()
		data, pag, err := c.GetWithPagination(ctx, fullPath)
		if err != nil {
			return nil, Pagination{}, err
		}
		lastPag = pag
		var chunk []json.RawMessage
		if err := json.Unmarshal(data, &chunk); err != nil {
			return nil, Pagination{}, err
		}
		merged = append(merged, chunk...)
		if len(merged) >= effectiveLimit {
			merged = merged[:effectiveLimit]
			break
		}
		if pag.NextPage == 0 {
			break
		}
		page = pag.NextPage
	}
	out, err := json.Marshal(merged)
	return out, lastPag, err
}

func splitPathAndQuery(path string) (string, url.Values, error) {
	idx := strings.Index(path, "?")
	if idx < 0 {
		return path, url.Values{}, nil
	}
	q, err := url.ParseQuery(path[idx+1:])
	if err != nil {
		return "", nil, err
	}
	return path[:idx], q, nil
}
