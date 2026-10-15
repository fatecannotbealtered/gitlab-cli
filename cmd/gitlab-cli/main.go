// Command gitlab-cli is GitLab for AI agents, built on the aicli library
// (ai-native-cli-spec 2.0).
package main

import (
	"os"

	"github.com/fatecannotbealtered/gitlab-cli/cmd"
)

func main() { os.Exit(cmd.NewApp(os.Getenv).Execute(os.Args[1:])) }
