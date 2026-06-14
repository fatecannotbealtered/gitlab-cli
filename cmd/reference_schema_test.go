package cmd

import "testing"

// TestReference_EveryLeafCommandHasRealSchemaAndExample guards against
// output_schema regressing to a stub: every leaf command must reference a schema
// label that exists in the Schemas map with a non-empty field list, and must
// carry at least one runnable example. This keeps `reference` a usable source of
// truth for agents.
func TestReference_EveryLeafCommandHasRealSchemaAndExample(t *testing.T) {
	tree := buildReferenceTree(rootCmd)
	if len(tree.Commands) == 0 {
		t.Fatal("reference enumerated zero commands")
	}
	if len(tree.Schemas) == 0 {
		t.Fatal("reference enumerated zero schemas")
	}

	var leaves int
	var walk func(nodes []refCommand)
	walk = func(nodes []refCommand) {
		for _, c := range nodes {
			if len(c.Commands) > 0 {
				walk(c.Commands)
				continue
			}
			leaves++
			if c.OutputSchema == "" {
				t.Errorf("%s: empty output_schema", c.Path)
				continue
			}
			schema, ok := tree.Schemas[c.OutputSchema]
			if !ok {
				t.Errorf("%s: output_schema %q not defined in schemas map", c.Path, c.OutputSchema)
				continue
			}
			if len(schema.Fields) == 0 {
				t.Errorf("%s: schema %q has no fields (stub)", c.Path, c.OutputSchema)
			}
			if len(c.Examples) == 0 {
				t.Errorf("%s: no examples", c.Path)
			}
		}
	}
	walk(tree.Commands)

	if leaves == 0 {
		t.Fatal("reference enumerated zero leaf commands")
	}
}

// TestReferenceSchemas_NoOrphans ensures every defined schema label is actually
// referenced by at least one command, so the catalogue cannot drift into dead
// entries that lie about the contract.
func TestReferenceSchemas_NoOrphans(t *testing.T) {
	used := map[string]bool{}
	for _, label := range commandSchemaLabels {
		used[label] = true
	}
	for label := range referenceSchemas() {
		if !used[label] {
			t.Errorf("schema %q is defined but no command references it", label)
		}
	}
}
