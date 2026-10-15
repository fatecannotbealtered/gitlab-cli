package cmd

import (
	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// Lists (CLI-028, CLI-041). A list command takes --limit (the page size, 1-100,
// default 20; a larger value is E_USAGE, never lowered) and --offset (copied from
// the previous page's next_offset). It fetches exactly one GitLab page, so has_more
// and next_offset come from GitLab's X-Next-Page, and total from X-Total when
// GitLab sends it. scope echoes what the list covers: the project, the filters
// applied, and coverage narrower than the name suggests.

const (
	defaultLimit = 20
	maxLimit     = api.MaxPerPage
)

// addPageFlags declares --limit and --offset on a list command.
func addPageFlags(c *cobra.Command) {
	c.Flags().Int("limit", defaultLimit, "Items per page, 1-100")
	c.Flags().Int("offset", 0, "Items to skip: next_offset from the previous page")
}

// pageRequest is a validated --limit and --offset, and the GitLab page they select.
type pageRequest struct {
	Limit, Offset, Page int
}

// readPage validates --limit and --offset. GitLab pages by page number, so the
// offset must be a multiple of the limit: next_offset always is.
func readPage(cmd *cobra.Command) (pageRequest, error) {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	if limit < 1 || limit > maxLimit {
		e := aicli.Usage("invalid_value", "--limit", "--limit must be between 1 and 100.")
		e.Details["expected"] = "1-100"
		return pageRequest{}, e
	}
	if offset < 0 || offset%limit != 0 {
		e := aicli.Usage("invalid_value", "--offset", "--offset must be next_offset from the previous page with the same --limit.")
		e.Details["expected"] = "0 or a multiple of --limit (next_offset of the previous page)"
		return pageRequest{}, e
	}
	return pageRequest{Limit: limit, Offset: offset, Page: offset/limit + 1}, nil
}

// gitlabPage is the aicli page for one GitLab page of items (count items). scope
// gets the applied limit and, past the first page, the offset.
func gitlabPage(items any, count int, req pageRequest, pag api.Pagination, scope map[string]any, sort string) aicli.Page {
	if scope == nil {
		scope = map[string]any{}
	}
	scope["limit"] = req.Limit
	if req.Offset > 0 {
		scope["offset"] = req.Offset
	}
	p := aicli.Page{Items: items, Count: count, Scope: scope, Sort: sort}
	if pag.NextPage > 0 {
		next := req.Offset + req.Limit
		p.HasMore, p.NextOffset = true, &next
	}
	if pag.HasTotal && pag.Total >= count {
		total := pag.Total
		p.Total = &total
	}
	return p
}

// wantsField reports whether the response will show one of names, by aicli's rules
// for --fields and the default fields (aicli.WantsField, Q17): a command fetches a
// field that costs GitLab extra work (label counts, commit line counts, a pipeline
// per merge request) only then.
func wantsField(cmd *cobra.Command, names ...string) bool {
	for _, n := range names {
		if aicli.WantsField(cmd, n) {
			return true
		}
	}
	return false
}
