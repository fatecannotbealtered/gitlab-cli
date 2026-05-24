package output

import "strings"

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
			result[origKey] = m[origKey]
		}
	}
	return result
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
		"id":       u.ID,
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
	return m
}
