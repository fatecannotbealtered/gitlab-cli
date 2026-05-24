//go:build integration

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

const ciYAML = `stages: [test]
e2e-job:
  stage: test
  script:
    - echo ok > artifact.txt
  artifacts:
    paths: [artifact.txt]
    expire_in: 1 day
`

// Bootstrap creates an isolated GitLab project and seed data for integration tests.
func Bootstrap(ctx context.Context) (*Fixture, error) {
	host := strings.TrimSpace(os.Getenv("GITLAB_CLI_HOST"))
	token := strings.TrimSpace(os.Getenv("GITLAB_CLI_TOKEN"))
	if host == "" || token == "" {
		return nil, fmt.Errorf("GITLAB_CLI_HOST and GITLAB_CLI_TOKEN required")
	}

	http := newHTTPAPI()
	suffix := strconv.FormatInt(time.Now().Unix(), 10)
	name := "gitlab-cli-e2e-" + suffix
	proj, err := http.createProject(ctx, name, "gitlab-cli-e2e-"+suffix)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	cfg := &config.Config{Host: host, Token: token}
	client := api.NewClient(cfg)
	path := proj.PathWithNamespace
	_ = enableSharedRunners(ctx)
	fx := &Fixture{
		Project:       path,
		ProjectID:     proj.ID,
		LabelName:     "e2e-label",
		VariableKey:   "E2E_VAR",
		BranchFeature: "feat/e2e",
		FilePath:      "README.md",
		ReleaseTag:    "v0.0.1-e2e",
	}

	iss, err := client.Issues.Create(ctx, path, api.IssueCreateOpts{Title: "e2e issue", Description: "integration seed"})
	if err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("create issue: %w", err)
	}
	fx.IssueIID = iss.IID

	note, err := client.Issues.AddNote(ctx, path, fx.IssueIID, "e2e comment")
	if err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("add issue note: %w", err)
	}
	fx.IssueNoteID = note.ID

	lbl, err := client.Labels.Create(ctx, path, api.LabelCreateOpts{Name: fx.LabelName, Color: "#112233", Description: "e2e"})
	if err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("create label: %w", err)
	}
	fx.LabelID = lbl.ID

	ms, err := client.Milestones.Create(ctx, path, api.MilestoneCreateOpts{Title: "e2e-milestone"})
	if err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("create milestone: %w", err)
	}
	fx.MilestoneID = ms.ID

	if _, err := client.Variables.Create(ctx, path, &api.VariableCreateOpts{Key: fx.VariableKey, Value: "e2e-value"}); err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("create variable: %w", err)
	}

	if _, err := client.Repos.CreateBranch(ctx, path, fx.BranchFeature, "main"); err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("create branch: %w", err)
	}

	if err := client.Repos.CreateFile(ctx, path, "e2e-mr.txt", api.FileWriteBody{
		Branch: fx.BranchFeature, Content: "mr seed\n", CommitMessage: "e2e: mr file",
	}); err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("create mr file: %w", err)
	}

	mr, err := client.MergeRequests.Create(ctx, path, &api.MergeRequestCreateRequest{
		Title: "e2e mr", SourceBranch: fx.BranchFeature, TargetBranch: "main",
	})
	if err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("create mr: %w", err)
	}
	fx.MRIID = mr.IID

	mrNote, err := client.MergeRequests.AddNote(ctx, path, fx.MRIID, "e2e mr comment")
	if err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("add mr note: %w", err)
	}
	fx.MRNoteID = mrNote.ID

	commits, err := client.Repos.ListCommits(ctx, path, &api.CommitListOpts{RefName: "main", Limit: 1})
	if err == nil && len(commits) > 0 {
		fx.CommitSHA = commits[0].ID
	}

	if fx.CommitSHA != "" {
		_ = http.createTag(ctx, path, fx.ReleaseTag, fx.CommitSHA)
		_, _ = client.Releases.Create(ctx, path, api.ReleaseCreateBody{TagName: fx.ReleaseTag, Name: "e2e release"})
	}

	fx.GitCloneDir, _ = cloneProjectRepo(host, token, path)

	// Add CI last so the pipeline under test is not racing earlier seed commits.
	if err := client.Repos.CreateFile(ctx, path, ".gitlab-ci.yml", api.FileWriteBody{
		Branch: "main", Content: ciYAML, CommitMessage: "e2e: add ci",
	}); err != nil {
		_ = http.deleteProject(ctx, proj.ID)
		return nil, fmt.Errorf("create .gitlab-ci.yml: %w", err)
	}
	_, _ = client.Pipelines.Create(ctx, path, api.PipelineCreateBody{Ref: "main"})

	waitCtx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	fx.HasPipeline, fx.PipelineID, fx.HasJob, fx.JobID = waitForPipelineJob(waitCtx, client, path)

	return fx, nil
}

// Teardown deletes the bootstrap project.
func Teardown(ctx context.Context, fx *Fixture) {
	if fx == nil || fx.ProjectID == 0 {
		return
	}
	_ = newHTTPAPI().deleteProject(ctx, fx.ProjectID)
	if fx.GitCloneDir != "" {
		_ = os.RemoveAll(fx.GitCloneDir)
	}
}

func enableSharedRunners(ctx context.Context) error {
	// Best-effort: GitLab CE often ships with shared runners disabled.
	cmd := exec.CommandContext(ctx, "docker", "exec", "gitlab-cli-e2e", "gitlab-rails", "runner",
		"ApplicationSetting.current.update!(shared_runners_enabled: true, allow_runner_registration_token: true)")
	return cmd.Run()
}

func waitForPipelineJob(ctx context.Context, client *api.Client, project string) (hasPipeline bool, pipelineID int, hasJob bool, jobID int) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return false, 0, false, 0
		case <-tick.C:
			pipes, err := client.Pipelines.List(ctx, project, &api.PipelineListOpts{Ref: "main", Limit: 5})
			if err != nil || len(pipes) == 0 {
				continue
			}
			p := pipes[0]
			pipelineID = p.ID
			hasPipeline = true
			if p.Status != "success" {
				continue
			}
			jobs, err := client.Pipelines.Jobs(ctx, project, p.ID, nil)
			if err != nil || len(jobs) == 0 {
				continue
			}
			for _, j := range jobs {
				if j.Status == "success" {
					return hasPipeline, pipelineID, true, j.ID
				}
			}
		}
	}
}

func cloneProjectRepo(host, token, projectPath string) (string, error) {
	dir, err := os.MkdirTemp("", "gitlab-cli-e2e-git-*")
	if err != nil {
		return "", err
	}
	u := strings.TrimRight(host, "/")
	var cloneURL string
	if strings.HasPrefix(u, "https://") {
		cloneURL = fmt.Sprintf("https://oauth2:%s@%s/%s.git", token, strings.TrimPrefix(u, "https://"), projectPath)
	} else {
		cloneURL = fmt.Sprintf("http://oauth2:%s@%s/%s.git", token, strings.TrimPrefix(u, "http://"), projectPath)
	}
	cmd := exec.Command("git", "clone", "--depth", "1", cloneURL, dir)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("git clone: %w\n%s", err, out)
	}
	// checkout feature branch for mr current
	_ = exec.Command("git", "-C", dir, "fetch", "origin", "feat/e2e:feat/e2e").Run()
	_ = exec.Command("git", "-C", dir, "checkout", "-B", "feat/e2e", "FETCH_HEAD").Run()
	return dir, nil
}

// ProjectFlag returns --project flag args for CLI commands.
func (f *Fixture) ProjectFlag() []string {
	return []string{"--project", f.Project}
}

func (f *Fixture) IssueIIDStr() string {
	return strconv.Itoa(f.IssueIID)
}

func (f *Fixture) MRIIDStr() string {
	return strconv.Itoa(f.MRIID)
}

func (f *Fixture) JobIDStr() string {
	return strconv.Itoa(f.JobID)
}

func (f *Fixture) PipelineIDStr() string {
	return strconv.Itoa(f.PipelineID)
}

func (f *Fixture) MilestoneIDStr() string {
	return strconv.Itoa(f.MilestoneID)
}

func (f *Fixture) NoteIDStr() string   { return strconv.Itoa(f.IssueNoteID) }
func (f *Fixture) MRNoteIDStr() string { return strconv.Itoa(f.MRNoteID) }
func (f *Fixture) LabelIDStr() string  { return strconv.Itoa(f.LabelID) }
