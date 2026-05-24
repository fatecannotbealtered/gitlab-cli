package output

// ListMeta describes pagination for list command JSON output.
type ListMeta struct {
	Count   int  `json:"count"`
	Limit   int  `json:"limit"`
	Page    int  `json:"page,omitempty"`
	Total   int  `json:"total,omitempty"`
	HasMore bool `json:"hasMore"`
	All     bool `json:"all,omitempty"`
}

// ListEnvelope is the standard JSON shape for list commands.
type ListEnvelope struct {
	Items []map[string]any `json:"items"`
	ListMeta
}

// NewListEnvelope builds a list response with projected items.
func NewListEnvelope(items []map[string]any, meta ListMeta) ListEnvelope {
	if items == nil {
		items = []map[string]any{}
	}
	return ListEnvelope{Items: items, ListMeta: meta}
}
