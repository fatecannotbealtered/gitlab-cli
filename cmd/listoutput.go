package cmd

import (
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

const listAllMax = 10000

// listLimitFromFlags resolves --limit and --all for list commands.
func listLimitFromFlags(cmd *cobra.Command) (limit int, fetchAll bool, err error) {
	fetchAll, _ = cmd.Flags().GetBool("all")
	if fetchAll {
		return listAllMax, true, nil
	}
	limit, err = requireLimit(cmd)
	return limit, false, err
}

func registerListFlags(cmd *cobra.Command) {
	if cmd.Flags().Lookup("limit") == nil {
		cmd.Flags().Int("limit", 20, "Max results per page (1-100); ignored when --all is set")
	}
	if cmd.Flags().Lookup("all") == nil {
		cmd.Flags().Bool("all", false, "Fetch all pages (up to 10000 items)")
	}
}

func printListJSON(cmd *cobra.Command, rows []map[string]any, pag api.Pagination, requestedLimit int, fetchAll bool) {
	fields := getFieldsFlag(cmd)
	items := make([]map[string]any, len(rows))
	for i, row := range rows {
		items[i] = output.FilterMap(row, fields)
	}
	count := len(items)
	hasMore := pag.NextPage > 0 && !fetchAll
	if !fetchAll && requestedLimit > 0 && count >= requestedLimit && pag.NextPage > 0 {
		hasMore = true
	}
	meta := output.ListMeta{
		Count:   count,
		Limit:   requestedLimit,
		Page:    pag.Page,
		Total:   pag.Total,
		HasMore: hasMore,
		All:     fetchAll,
	}
	output.PrintJSON(output.NewListEnvelope(items, meta))
}
