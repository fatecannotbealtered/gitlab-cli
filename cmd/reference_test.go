package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestReference_JSON_HasAgentMetadata(t *testing.T) {
	resetRootPersistentFlags(t)
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"reference", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("reference: %v", err)
		}
	})
	var tree refTree
	unwrapJSONDataInto(t, out, &tree)
	if tree.Version == "" {
		t.Error("expected version in reference tree")
	}
	mrDelete := findRefCommandByPath(tree.Commands, "mr comment delete")
	if mrDelete == nil {
		t.Fatal("mr comment delete not found in reference tree")
	}
	if !mrDelete.Write {
		t.Error("mr comment delete should be write=true")
	}
	if !mrDelete.RequiresConfirmation {
		t.Error("mr comment delete should require confirmation")
	}
	if len(mrDelete.PositionalArgs) != 1 || mrDelete.PositionalArgs[0] != "iid" {
		t.Errorf("positionalArgs = %v, want [iid]", mrDelete.PositionalArgs)
	}
	mrDiff := findRefCommandByPath(tree.Commands, "mr diff")
	if mrDiff == nil {
		t.Fatal("mr diff not found")
	}
	if mrDiff.OutputType != "json" {
		t.Errorf("mr diff outputType = %q, want json", mrDiff.OutputType)
	}
	if !containsAllStrings(mrDiff.Formats, []string{"json", "text", "raw"}) {
		t.Errorf("mr diff formats = %v, want json/text/raw", mrDiff.Formats)
	}
}

func containsAllStrings(got, want []string) bool {
	seen := map[string]bool{}
	for _, item := range got {
		seen[item] = true
	}
	for _, item := range want {
		if !seen[item] {
			return false
		}
	}
	return true
}

func findRefCommandByPath(nodes []refCommand, path string) *refCommand {
	for i := range nodes {
		n := &nodes[i]
		if n.Path == path {
			return n
		}
		if found := findRefCommandByPath(n.Commands, path); found != nil {
			return found
		}
	}
	return nil
}

func testLeafCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use: use,
		Run: func(*cobra.Command, []string) {},
	}
}

func TestCollectPersistentRefFlags_SkipsHidden(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("visible", "v", "shown")
	_ = root.PersistentFlags().MarkHidden("visible")
	root.PersistentFlags().String("shown", "s", "shown")

	flags := collectPersistentRefFlags(root)
	if len(flags) != 1 || flags[0].Name != "shown" {
		t.Fatalf("collectPersistentRefFlags = %+v, want only shown", flags)
	}
}

func TestCommandToRef_HiddenReturnsEmpty(t *testing.T) {
	hidden := &cobra.Command{Use: "hidden", Hidden: true}
	if got := commandToRef(hidden, "", ""); got.Name != "" {
		t.Fatalf("hidden command should return empty ref, got %+v", got)
	}
}

func TestCommandToRef_WithAnnotationsAndChildren(t *testing.T) {
	parent := &cobra.Command{Use: "parent", Short: "parent cmd", Run: func(*cobra.Command, []string) {}}
	parent.Annotations = map[string]string{"write": "true", "riskLevel": "medium"}

	leaf := testLeafCmd("get <id>")
	leaf.Short = "get item"
	leaf.Annotations = map[string]string{"outputType": "text"}
	leaf.Flags().String("fields", "", "fields")

	hiddenChild := &cobra.Command{Use: "hidden", Hidden: true, Run: func(*cobra.Command, []string) {}}
	parent.AddCommand(leaf, hiddenChild)

	got := commandToRef(parent, "app ", "app/")
	if !got.Write || got.RiskLevel != "medium" {
		t.Fatalf("parent annotations missing: %+v", got)
	}
	if len(got.Commands) != 1 {
		t.Fatalf("expected one visible child, got %+v", got.Commands)
	}
	if got.Commands[0].OutputType != "text" {
		t.Fatalf("child OutputType = %q, want text", got.Commands[0].OutputType)
	}
	if len(got.Commands[0].PositionalArgs) != 1 || got.Commands[0].PositionalArgs[0] != "id" {
		t.Fatalf("PositionalArgs = %v", got.Commands[0].PositionalArgs)
	}
}

func TestCommandToRef_LeafDefaultOutputType(t *testing.T) {
	leaf := testLeafCmd("leaf")
	got := commandToRef(leaf, "prefix ", "path/")
	if got.OutputType != "json" {
		t.Fatalf("OutputType = %q, want json", got.OutputType)
	}
}

func TestCmdHasFieldsFlag(t *testing.T) {
	if cmdHasFieldsFlag(nil) {
		t.Fatal("nil command should not have fields flag")
	}
	withFields := &cobra.Command{Use: "x"}
	withFields.Flags().String("fields", "", "")
	if !cmdHasFieldsFlag(withFields) {
		t.Fatal("expected fields flag to be detected")
	}
}

func TestCollectRefFlags_SkipsHiddenAndDuplicates(t *testing.T) {
	parent := &cobra.Command{Use: "parent", Run: func(*cobra.Command, []string) {}}
	parent.PersistentFlags().String("json", "false", "json output")

	child := testLeafCmd("child")
	child.Flags().String("project", "", "project")
	child.Flags().String("secret", "", "hidden")
	_ = child.Flags().MarkHidden("secret")
	child.Flags().String("json", "false", "local json")
	child.Flags().String("fields", "", "fields")
	parent.AddCommand(child)

	flags := collectRefFlags(child)
	names := make(map[string]bool)
	for _, f := range flags {
		names[f.Name] = true
	}
	for _, want := range []string{"project", "fields", "json"} {
		if !names[want] {
			t.Fatalf("missing flag %q in %+v", want, flags)
		}
	}
	if names["secret"] {
		t.Fatalf("hidden flag should be omitted: %+v", flags)
	}
}

func TestCollectRefFlags_NoParent(t *testing.T) {
	cmd := testLeafCmd("solo")
	cmd.Flags().String("project", "", "project")
	flags := collectRefFlags(cmd)
	if len(flags) != 1 || flags[0].Name != "project" {
		t.Fatalf("collectRefFlags without parent = %+v", flags)
	}
}

func TestCollectRefFlags_SkipsPersistentDuplicateOfLocal(t *testing.T) {
	parent := &cobra.Command{Use: "parent", Run: func(*cobra.Command, []string) {}}
	child := testLeafCmd("child")
	child.Flags().String("mode", "a", "local mode")
	child.PersistentFlags().String("mode", "b", "persistent mode")
	parent.AddCommand(child)

	flags := collectRefFlags(child)
	modeCount := 0
	for _, f := range flags {
		if f.Name == "mode" {
			modeCount++
		}
	}
	if modeCount != 1 {
		t.Fatalf("mode flag should appear once, got %+v", flags)
	}
}

func TestCollectRefFlags_InheritsPersistentOnly(t *testing.T) {
	parent := &cobra.Command{Use: "parent", Run: func(*cobra.Command, []string) {}}
	child := testLeafCmd("child")
	child.Flags().String("project", "", "project")
	child.PersistentFlags().String("profile", "", "config profile")
	child.PersistentFlags().String("hidden-persist", "x", "hidden")
	_ = child.PersistentFlags().MarkHidden("hidden-persist")
	parent.AddCommand(child)

	flags := collectRefFlags(child)
	names := map[string]bool{}
	for _, f := range flags {
		names[f.Name] = true
	}
	if !names["project"] || !names["profile"] {
		t.Fatalf("expected project and persistent profile flags, got %+v", flags)
	}
	if names["hidden-persist"] {
		t.Fatalf("hidden persistent flag should be omitted: %+v", flags)
	}
}

func TestCollectRefFlags_NoPersistentWhenParentMissing(t *testing.T) {
	cmd := testLeafCmd("solo")
	cmd.Flags().String("alpha", "", "alpha")
	cmd.PersistentFlags().String("beta", "b", "beta")

	flags := collectRefFlags(cmd)
	if len(flags) != 2 {
		t.Fatalf("collectRefFlags(solo) = %+v", flags)
	}
}

func TestRefFlagFromPflag_EmptyDefault(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("empty", "", "empty default")
	got := refFlagFromPflag(cmd.Flags().Lookup("empty"))
	if got.Default != "-" {
		t.Fatalf("Default = %q, want -", got.Default)
	}
}

func TestWalkCommands_LeafMetadata(t *testing.T) {
	root := &cobra.Command{Use: "root", Short: "root cmd", Run: func(*cobra.Command, []string) {}}
	leaf := testLeafCmd("delete <id>")
	leaf.Short = "delete thing"
	leaf.Annotations = map[string]string{
		"write":      "true",
		"confirm":    "true",
		"riskLevel":  "high",
		"outputType": "text",
	}
	leaf.Flags().String("fields", "", "fields")
	root.AddCommand(leaf)

	var lines []string
	walkCommands(root, &lines, "")
	out := strings.Join(lines, "\n")
	for _, want := range []string{
		"## root delete <id>",
		"write",
		"confirm",
		"risk=high",
		"output=text",
		"args=id",
		"supports --fields",
		"### Flags",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("walkCommands output missing %q in:\n%s", want, out)
		}
	}
}

func TestWalkCommands_SkipsHiddenChild(t *testing.T) {
	root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) {}}
	visible := testLeafCmd("visible")
	visible.Short = "shown"
	hidden := &cobra.Command{Use: "hidden", Hidden: true, Short: "secret", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(visible, hidden)

	var lines []string
	walkCommands(root, &lines, "")
	out := strings.Join(lines, "\n")
	if strings.Contains(out, "hidden") {
		t.Fatalf("hidden child should be skipped:\n%s", out)
	}
	if !strings.Contains(out, "visible") {
		t.Fatalf("visible child should appear:\n%s", out)
	}
}

func TestWalkCommands_LeafWithoutAnnotations(t *testing.T) {
	root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) {}}
	leaf := testLeafCmd("show")
	leaf.Short = "show something"
	root.AddCommand(leaf)

	var lines []string
	walkCommands(root, &lines, "")
	out := strings.Join(lines, "\n")
	if strings.Contains(out, "write") || strings.Contains(out, "output=json") {
		t.Fatalf("leaf without annotations should not emit meta block:\n%s", out)
	}
}

func TestWalkCommands_LeafDefaultOutputWhenAnnotated(t *testing.T) {
	root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) {}}
	leaf := testLeafCmd("run")
	leaf.Short = "run job"
	leaf.Annotations = map[string]string{"write": "true"}
	root.AddCommand(leaf)

	var lines []string
	walkCommands(root, &lines, "")
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "output=json") {
		t.Fatalf("expected default output=json when outputType omitted:\n%s", out)
	}
}

func TestBuildReferenceTree_SkipsHiddenTopLevel(t *testing.T) {
	root := &cobra.Command{Use: "root", Version: "test", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(testLeafCmd("visible"))
	root.AddCommand(&cobra.Command{Use: "hidden", Hidden: true, Run: func(*cobra.Command, []string) {}})

	tree := buildReferenceTree(root)
	if len(tree.Commands) != 1 || tree.Commands[0].Path != "visible" {
		t.Fatalf("buildReferenceTree = %+v", tree.Commands)
	}
}

func TestParsePositionalArgs(t *testing.T) {
	got := parsePositionalArgs("delete <id> [force]")
	if len(got) != 1 || got[0] != "id" {
		t.Fatalf("parsePositionalArgs = %v, want [id]", got)
	}
	if parsePositionalArgs("list") != nil {
		t.Fatal("list command should have no positional args")
	}
}

func TestWalkCommands_HiddenRoot(t *testing.T) {
	hidden := &cobra.Command{Use: "hidden", Hidden: true, Run: func(*cobra.Command, []string) {}}
	var lines []string
	walkCommands(hidden, &lines, "")
	if len(lines) != 0 {
		t.Fatalf("hidden root should produce no output, got %v", lines)
	}
}

func TestWalkCommands_LeafWithoutFlags(t *testing.T) {
	root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) {}}
	leaf := testLeafCmd("ping")
	leaf.Short = "ping service"
	root.AddCommand(leaf)

	var lines []string
	walkCommands(root, &lines, "")
	out := strings.Join(lines, "\n")
	if strings.Contains(out, "### Flags") {
		t.Fatalf("leaf without flags should not render flags table:\n%s", out)
	}
}

func TestWalkCommands_SkipsUnavailableChild(t *testing.T) {
	root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) {}}
	unavailable := &cobra.Command{Use: "noop"}
	root.AddCommand(unavailable)

	var lines []string
	walkCommands(root, &lines, "")
	out := strings.Join(lines, "\n")
	if strings.Contains(out, "noop") {
		t.Fatalf("unavailable child should be skipped:\n%s", out)
	}
}
