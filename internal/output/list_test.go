package output

import (
	"testing"
)

func TestNewListEnvelope_NilItems(t *testing.T) {
	env := NewListEnvelope(nil, ListMeta{Count: 0, HasMore: false})
	if env.Items == nil {
		t.Fatal("Items should be non-nil empty slice")
	}
	if len(env.Items) != 0 {
		t.Fatalf("len(Items) = %d, want 0", len(env.Items))
	}
}

func TestNewListEnvelope_WithItems(t *testing.T) {
	items := []map[string]any{{"id": 1}}
	meta := ListMeta{Count: 1, Limit: 10, HasMore: false}
	env := NewListEnvelope(items, meta)
	if len(env.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(env.Items))
	}
	if env.Count != 1 || env.Limit != 10 {
		t.Fatalf("meta not copied: %+v", env.ListMeta)
	}
}
