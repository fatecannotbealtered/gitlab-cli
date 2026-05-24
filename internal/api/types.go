package api

// ===== User =====
//
// Returned by /api/v4/user (current user) and /api/v4/users (search).
// The full GitLab User schema has many fields — we only model what the CLI
// surfaces (extra fields will be tolerated by json.Unmarshal).

type User struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	AvatarURL string `json:"avatar_url"`
	Bot       bool   `json:"bot"`
}
