package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// mr bulk: one write across many merge requests of one project, through
// App.WriteBatch (CLI-063): one preview listing every merge request, one token, a
// receipt per item and a summary. GitLab has no bulk endpoint for these, so each
// item is the single command's plan (mr_write.go), previewed, blocked and verified
// the same way. bulk merge is dangerous, so it stops at the first failure unless
// --continue-on-error (the library's default for dangerous batches).

const mrBulkEvidence = "simulated:cmd/mr_bulk_test.go"

func registerMRBulk(app *aicli.App, parent *cobra.Command) {
	group := &cobra.Command{Use: "bulk"}
	parent.AddCommand(group)
	app.Group(group, "Merge, approve, close or update many merge requests of a project in one gated call")
	addMRBulkMerge(app, group)
	addMRBulkApprove(app, group)
	addMRBulkClose(app, group)
	addMRBulkUpdate(app, group)
}

// mrBulkTargets reads --project and --ids (iids, !iids or refs of that project),
// drops repeats with the order kept, and binds the iids (CLI-054, CLI-063).
func mrBulkTargets(cmd *cobra.Command, gl *lazyClient) ([]mrSel, error) {
	project, err := projectFlag(cmd, "", gl)
	if err != nil {
		return nil, err
	}
	raws, _ := cmd.Flags().GetStringSlice("ids")
	var sels []mrSel
	var iids []string
	seen := map[int]bool{}
	for _, raw := range raws {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		p, iid, _, err := parseMRArg(raw)
		if err != nil {
			return nil, aicli.Validation("--ids", "merge request iids (5,6) or refs of the --project (acme/web!5)", "An --ids entry is not a merge request iid or ref.")
		}
		if p != "" && !strings.EqualFold(p, project) {
			e := aicli.Usage("conflicting_flags", "--ids", "An --ids entry names another project than --project; a batch acts on one project.")
			e.Details["next_action"] = "Run one batch per project."
			return nil, e
		}
		if !seen[iid] {
			seen[iid] = true
			sels = append(sels, mrSel{project: project, iid: iid})
			iids = append(iids, strconv.Itoa(iid))
		}
	}
	if len(iids) > 0 {
		if err := aicli.SetResolved(cmd, "ids", iids...); err != nil {
			return nil, err
		}
	}
	return sels, nil
}

// mrBulk runs one plan per merge request through the batch gate.
func mrBulk(app *aicli.App, cmd *cobra.Command, plan func(gl *lazyClient, sel mrSel) mrPlan) error {
	gl := gitlabFor(app)
	sels, err := mrBulkTargets(cmd, gl)
	if err != nil {
		return err
	}
	items := make([]aicli.BatchItem, 0, len(sels))
	for _, sel := range sels {
		items = append(items, plan(gl, sel).item())
	}
	return app.WriteBatch(cmd, aicli.BatchSpec{Items: items})
}

func addMRBulkFlags(c *cobra.Command) {
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().StringSlice("ids", nil, "Merge request iids or refs, comma-separated or repeated")
	_ = c.MarkFlagRequired("ids")
}

func mrBulkMeta(summary, output string, dangerous bool, example string) aicli.Meta {
	dangerFlag := ""
	if dangerous {
		dangerFlag = " --dangerous"
	}
	return aicli.Meta{Summary: summary, Effects: []aicli.Effect{aicli.Change}, Dangerous: dangerous, Output: output, BatchFlag: "ids",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{mrBulkEvidence},
		Examples: []string{fmt.Sprintf("gitlab-cli mr bulk %s%s --dry-run", example, dangerFlag),
			fmt.Sprintf("gitlab-cli mr bulk %s%s --confirm <confirm_token>", example, dangerFlag)}}
}

func addMRBulkMerge(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "merge", RunE: func(cmd *cobra.Command, _ []string) error {
		w := readMRMergeFlags(cmd)
		return mrBulk(app, cmd, func(gl *lazyClient, sel mrSel) mrPlan { return mrMergePlan(gl, sel, w) })
	}}
	addMRBulkFlags(c)
	addMRMergeFlags(c)
	parent.AddCommand(c)
	app.Register(c, mrBulkMeta("Merge many merge requests, each at the head commit its preview shows (cannot be undone; stops at the first failure)",
		"merge_request_result", true, "merge --project acme/web --ids 5,6"))
}

func addMRBulkApprove(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "approve", RunE: func(cmd *cobra.Command, _ []string) error {
		return mrBulk(app, cmd, func(gl *lazyClient, sel mrSel) mrPlan { return mrApprovePlan(gl, sel, true) })
	}}
	addMRBulkFlags(c)
	parent.AddCommand(c)
	app.Register(c, mrBulkMeta("Approve many merge requests as this account", "merge_request_approval", false,
		"approve --project acme/web --ids 5,8"))
}

func addMRBulkClose(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "close", RunE: func(cmd *cobra.Command, _ []string) error {
		return mrBulk(app, cmd, func(gl *lazyClient, sel mrSel) mrPlan { return mrStatePlan(gl, sel, "close") })
	}}
	addMRBulkFlags(c)
	parent.AddCommand(c)
	app.Register(c, mrBulkMeta("Close many merge requests without merging them", "merge_request_result", false,
		"close --project acme/web --ids 5,6"))
}

func addMRBulkUpdate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "update", RunE: func(cmd *cobra.Command, _ []string) error {
		w, err := readMRUpdateFlags(app, cmd)
		if err != nil {
			return err
		}
		return mrBulk(app, cmd, func(gl *lazyClient, sel mrSel) mrPlan { return mrUpdatePlan(gl, sel, w) })
	}}
	addMRBulkFlags(c)
	addMRUpdateFields(c, false)
	c.MarkFlagsOneRequired("title", "description", "description-file", "add-labels", "remove-labels", "target-branch")
	parent.AddCommand(c)
	meta := mrBulkMeta("Change the title, description, labels or target branch of many merge requests", "merge_request_result", false,
		"update --project acme/web --ids 5,6 --add-labels needs-review")
	meta.FileInputs = map[string]string{"description-file": "description"}
	app.Register(c, meta)
}
