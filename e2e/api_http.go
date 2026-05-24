//go:build integration

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// httpAPI is a minimal GitLab REST client for bootstrap (project create/delete).
type httpAPI struct {
	base   string
	token  string
	client *http.Client
}

func newHTTPAPI() *httpAPI {
	host := strings.TrimRight(strings.TrimSpace(os.Getenv("GITLAB_CLI_HOST")), "/")
	return &httpAPI{
		base:   host + "/api/v4",
		token:  strings.TrimSpace(os.Getenv("GITLAB_CLI_TOKEN")),
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (h *httpAPI) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.base+path, rdr)
	if err != nil {
		return 0, err
	}
	req.Header.Set("PRIVATE-TOKEN", h.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("gitlab %s %s: %s", method, path, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

type createdProject struct {
	ID                int    `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
}

// createProject creates a new project under the authenticated user's namespace.
func (h *httpAPI) createProject(ctx context.Context, name, path string) (*createdProject, error) {
	body := map[string]any{
		"name":                                  name,
		"path":                                  path,
		"initialize_with_readme":                true,
		"visibility":                            "private",
		"merge_method":                          "merge",
		"only_allow_merge_if_pipeline_succeeds": false,
	}
	var out createdProject
	_, err := h.do(ctx, http.MethodPost, "/projects", body, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (h *httpAPI) deleteProject(ctx context.Context, projectID int) error {
	_, err := h.do(ctx, http.MethodDelete, fmt.Sprintf("/projects/%d", projectID), nil, nil)
	return err
}

type repositoryTag struct {
	Name string `json:"name"`
}

func (h *httpAPI) createTag(ctx context.Context, projectPath, tag, ref string) error {
	path := "/projects/" + encodeProjectPath(projectPath) + "/repository/tags"
	body := map[string]string{"tag_name": tag, "ref": ref}
	_, err := h.do(ctx, http.MethodPost, path, body, nil)
	return err
}

func encodeProjectPath(p string) string {
	return strings.ReplaceAll(p, "/", "%2F")
}
