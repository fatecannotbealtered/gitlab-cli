package output

import "github.com/fatecannotbealtered/gitlab-cli/internal/api"

// FlatVariable is a token-efficient representation of a GitLab CI/CD variable.
// The Value field is intentionally EXCLUDED to prevent accidental secret leakage
// in table output. Use FlatVariableWithValue only when --json is explicitly requested.
type FlatVariable struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Protected   bool   `json:"protected"`
	Masked      bool   `json:"masked"`
	Raw         bool   `json:"raw"`
	EnvScope    string `json:"envScope"`
	Description string `json:"description,omitempty"`
}

// FlatVariableWithValue includes the secret value — only for --json output.
type FlatVariableWithValue struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Type        string `json:"type"`
	Protected   bool   `json:"protected"`
	Masked      bool   `json:"masked"`
	Raw         bool   `json:"raw"`
	EnvScope    string `json:"envScope"`
	Description string `json:"description,omitempty"`
}

// ToFlatVariable converts an api.Variable to FlatVariable (no value).
func ToFlatVariable(v *api.Variable) FlatVariable {
	return FlatVariable{
		Key:         v.Key,
		Type:        v.VariableType,
		Protected:   v.Protected,
		Masked:      v.Masked,
		Raw:         v.Raw,
		EnvScope:    v.EnvironmentScope,
		Description: v.Description,
	}
}

// ToFlatVariableWithValue converts an api.Variable to FlatVariableWithValue (includes value).
func ToFlatVariableWithValue(v *api.Variable) FlatVariableWithValue {
	return FlatVariableWithValue{
		Key:         v.Key,
		Value:       v.Value,
		Type:        v.VariableType,
		Protected:   v.Protected,
		Masked:      v.Masked,
		Raw:         v.Raw,
		EnvScope:    v.EnvironmentScope,
		Description: v.Description,
	}
}

// VariableToMap converts a FlatVariable to a map for field filtering.
func VariableToMap(f FlatVariable) map[string]any {
	return map[string]any{
		"key":         f.Key,
		"type":        f.Type,
		"protected":   f.Protected,
		"masked":      f.Masked,
		"raw":         f.Raw,
		"envScope":    f.EnvScope,
		"description": f.Description,
	}
}

// VariableWithValueToMap converts a FlatVariableWithValue to a map for field filtering.
func VariableWithValueToMap(f FlatVariableWithValue) map[string]any {
	return map[string]any{
		"key":         f.Key,
		"value":       f.Value,
		"type":        f.Type,
		"protected":   f.Protected,
		"masked":      f.Masked,
		"raw":         f.Raw,
		"envScope":    f.EnvScope,
		"description": f.Description,
	}
}
