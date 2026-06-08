package output

import (
	"fmt"
	"strings"
)

const UntrustedKey = "_untrusted"

// ID renders numeric backend identifiers as strings for stable agent contracts.
func ID(v any) string {
	return fmt.Sprint(v)
}

// MarkUntrusted annotates fields that originate from external GitLab content.
func MarkUntrusted(m map[string]any, fields ...string) map[string]any {
	if len(fields) == 0 {
		return m
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || seen[field] {
			continue
		}
		if _, ok := m[field]; !ok {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	if len(out) > 0 {
		m[UntrustedKey] = out
	}
	return m
}

// FilterMap returns a copy of m containing only the keys requested in fieldNames.
// Lookup is CASE-INSENSITIVE (so --fields webUrl, weburl, WEBURL all work) but
// the result preserves the canonical key from m (so output JSON always uses the
// same key the schema documents).
func FilterMap(m map[string]any, fieldNames []string) map[string]any {
	if len(fieldNames) == 0 {
		return m
	}
	index := make(map[string]string, len(m))
	for k := range m {
		index[strings.ToLower(k)] = k
	}
	result := make(map[string]any, len(fieldNames))
	for _, name := range fieldNames {
		wanted := strings.TrimSpace(strings.ToLower(name))
		if origKey, ok := index[wanted]; ok {
			if origKey == UntrustedKey {
				continue
			}
			result[origKey] = m[origKey]
		}
	}
	if raw, ok := m[UntrustedKey]; ok {
		if filtered := filterUntrusted(raw, result); len(filtered) > 0 {
			result[UntrustedKey] = filtered
		}
	}
	return result
}

func filterUntrusted(raw any, selected map[string]any) []string {
	var fields []string
	switch v := raw.(type) {
	case []string:
		fields = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				fields = append(fields, s)
			}
		}
	default:
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, ok := selected[field]; ok {
			out = append(out, field)
		}
	}
	return out
}

// FlatUser is a token-efficient representation of a GitLab user.
type FlatUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	State    string `json:"state,omitempty"`
	WebURL   string `json:"webUrl,omitempty"`
}

// UserToMap converts a FlatUser to a map for field filtering.
func UserToMap(u FlatUser) map[string]any {
	m := map[string]any{
		"id":       ID(u.ID),
		"username": u.Username,
	}
	if u.Name != "" {
		m["name"] = u.Name
	}
	if u.Email != "" {
		m["email"] = u.Email
	}
	if u.State != "" {
		m["state"] = u.State
	}
	if u.WebURL != "" {
		m["webUrl"] = u.WebURL
	}
	return MarkUntrusted(m, "username", "name", "email")
}
