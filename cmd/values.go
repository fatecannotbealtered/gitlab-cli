package cmd

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Output values (CLI-007): snake_case keys, IDs as strings, times in UTC with Z.

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

// idString renders a GitLab numeric ID as the contract's string ID.
func idString(id int) string { return strconv.Itoa(id) }

// utcTime renders a GitLab time as UTC with Z; nil for an empty or unreadable
// value (a time key holds a time or null).
func utcTime(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// optInt is nil for a missing number.
func optInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// optString is nil for an empty string (an unset date, for example).
func optString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func ptr[T any](v T) *T { return &v }
