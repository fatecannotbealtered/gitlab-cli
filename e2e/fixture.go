//go:build integration

package e2e

// Fixture holds IDs created during E2E bootstrap (one shared project per test run).
type Fixture struct {
	Project       string // path_with_namespace, e.g. root/gitlab-cli-e2e-123
	ProjectID     int
	IssueIID      int
	IssueNoteID   int
	MRIID         int
	MRNoteID      int
	LabelName     string
	LabelID       int
	MilestoneID   int
	VariableKey   string
	BranchFeature string
	CommitSHA     string
	FilePath      string
	ReleaseTag    string
	PipelineID    int
	JobID         int
	HasPipeline   bool
	HasJob        bool
	GitCloneDir   string
}
