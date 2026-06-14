package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestParsePluralFlag_DedupPreservesOrder(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().StringSlice("ids", nil, "")
	if err := cmd.Flags().Set("ids", "3,1,2,1,3"); err != nil {
		t.Fatal(err)
	}
	got := parsePluralFlag(cmd, "ids")
	want := []string{"3", "1", "2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePluralFlag = %v, want %v", got, want)
	}
}

func TestParsePluralFlag_RepeatableAndComma(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().StringSlice("ids", nil, "")
	if err := cmd.Flags().Set("ids", "1,2"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("ids", "3"); err != nil {
		t.Fatal(err)
	}
	got := parsePluralFlag(cmd, "ids")
	want := []string{"1", "2", "3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mixed forms = %v, want %v", got, want)
	}
}

func TestParsePluralIntFlag_RejectsNonInteger(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().StringSlice("ids", nil, "")
	if err := cmd.Flags().Set("ids", "1,abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := parsePluralIntFlag(cmd, "ids"); err == nil {
		t.Fatal("expected error for non-integer id")
	}
}

func TestParseCommitActions_AllTypes(t *testing.T) {
	specs := []string{
		"create:path=a.txt;content=hi",
		"update:path=b.txt;content=yo",
		"delete:path=c.txt",
		"move:path=new.go;previous_path=old.go",
	}
	actions, targets, err := parseCommitActions(specs)
	if err != nil {
		t.Fatalf("parseCommitActions: %v", err)
	}
	if len(actions) != 4 {
		t.Fatalf("got %d actions, want 4", len(actions))
	}
	wantTargets := []string{"a.txt", "b.txt", "c.txt", "new.go"}
	if !reflect.DeepEqual(targets, wantTargets) {
		t.Errorf("targets = %v, want %v", targets, wantTargets)
	}
	if actions[0].Content != "hi" || actions[3].PreviousPath != "old.go" {
		t.Errorf("action fields not parsed: %+v", actions)
	}
}

func TestParseDotenvVariables_StripsQuotesAndComments(t *testing.T) {
	got := parseDotenvVariables("# c\nFOO=bar\nexport BAZ=\"qux\"\nQUOTED='v'\n\n")
	want := []importVar{{"FOO", "bar"}, {"BAZ", "qux"}, {"QUOTED", "v"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDotenvVariables = %v, want %v", got, want)
	}
}

func TestParseJSONVariables_PreservesOrder(t *testing.T) {
	got, err := parseJSONVariables(`{"Z":"1","A":"2","M":"3"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []importVar{{"Z", "1"}, {"A", "2"}, {"M", "3"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseJSONVariables = %v, want %v", got, want)
	}
}
