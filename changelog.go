package gitlabcli

import _ "embed"

// ChangelogMarkdown is embedded from the repository's single changelog source.
//
//go:embed CHANGELOG.md
var ChangelogMarkdown string
