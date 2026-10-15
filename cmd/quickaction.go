package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// quickActionLine is a line GitLab runs as a quick action (/merge, /close, /move
// group/other, /assign @x) when it posts a comment or saves the description of a
// merge request or an issue, outside code blocks.
var quickActionLine = regexp.MustCompile(`^/[a-z][a-z_]*(\s|$)`)

// hasQuickAction reports a line GitLab would run as a quick action. Such a line
// would make a comment or a description merge, close, move or reassign its merge
// request or issue, effects the command does not declare (CLI-025), and is a way
// for text an agent copies from elsewhere to act (SEC-002). The merge request and
// issue writes that send a comment, a reply or a description block on it.
func hasQuickAction(text string) bool {
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if !fenced && quickActionLine.MatchString(line) {
			return true
		}
	}
	return false
}

// quickActionBlock is the preview's block for a field (comment, reply,
// description) that holds a quick action.
func quickActionBlock(field string) aicli.Block {
	return aicli.Block{Kind: "quick_action", Message: fmt.Sprintf(
		"A line of the %s starts with a slash command such as /merge or /close, which GitLab would run as a quick action; "+
			"put it in a code block, or use the gitlab-cli command for that change.", field)}
}
