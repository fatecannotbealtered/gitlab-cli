// Command maturitymap prints where each gitlab-cli command's RunE is defined: one
// line per command with its path, the source file and the line, separated by
// tabs. The live suite's set_maturity.py reads it to find a command's Meta, the
// first Maturity line after its RunE (docs/E2E.md). Run it from the repository
// root:
//
//	go run ./e2e/maturitymap
package main

import (
	"fmt"
	"reflect"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/cmd"
)

func main() {
	app := cmd.NewApp(func(string) string { return "" })
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		if c.RunE == nil || c.HasSubCommands() {
			return
		}
		fn := runtime.FuncForPC(reflect.ValueOf(c.RunE).Pointer())
		if fn == nil {
			return
		}
		file, line := fn.FileLine(fn.Entry())
		fmt.Printf("%s\t%s\t%d\n", c.CommandPath(), file, line)
	}
	walk(app.Root)
}
