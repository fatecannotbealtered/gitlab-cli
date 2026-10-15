package cmd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The CI group: pipelines (pipeline.go), jobs (job.go) and CI/CD variables
// (variable.go, variable_bulk.go). This file holds what they share: the register
// function, pipeline and job refs, statuses, waiting and KEY=VALUE variables.

const (
	pipelineEvidence = "simulated:cmd/pipeline_test.go"
	jobEvidence      = "simulated:cmd/job_test.go"
	variableEvidence = "simulated:cmd/variable_test.go"
)

// registerCI registers the pipeline, job and variable groups, in that order (the
// order of the Skill's command map).
func registerCI(app *aicli.App) {
	registerPipeline(app)
	registerJob(app)
	registerVariable(app)
}

// Refs mirror GitLab's web paths (docs/migration-2.0/notes.md): acme/web/-/pipelines/123 and
// acme/web/-/jobs/456 carry the global ID, as the URLs do.
func pipelineRef(project string, id int) string { return project + "/-/pipelines/" + strconv.Itoa(id) }
func jobRef(project string, id int) string      { return project + "/-/jobs/" + strconv.Itoa(id) }

// The statuses GitLab reports for pipelines and jobs, as the commands use them.
var (
	// pipelineStates are the values of GitLab's pipeline status filter.
	pipelineStates = []string{"created", "waiting_for_resource", "preparing", "pending", "running", "success", "failed",
		"canceled", "skipped", "manual", "scheduled"}
	// jobStates are the values of GitLab's job scope filter.
	jobStates = []string{"created", "pending", "running", "failed", "success", "canceled", "skipped",
		"waiting_for_resource", "manual"}
	// ciFinishedStates do not change by themselves (manual waits for a person).
	ciFinishedStates = []string{"success", "failed", "canceled", "skipped", "manual"}
	// ciActiveStates are queued or running: what GitLab can cancel, and what a
	// retried or started pipeline or job reports.
	ciActiveStates = []string{"created", "waiting_for_resource", "preparing", "pending", "running", "scheduled"}
	// ciCanceledStates are what a canceled pipeline or job reports (GitLab 17 and
	// later report canceling while the jobs stop).
	ciCanceledStates = []string{"canceling", "canceled"}
)

// ciFinished reports a status that will not change by itself.
func ciFinished(status string) bool { return slices.Contains(ciFinishedStates, status) }

// ciMillis turns GitLab's seconds into milliseconds, the unit in the output's keys
// (CLI-007); nil stays nil.
func ciMillis(seconds *float64) any {
	if seconds == nil {
		return nil
	}
	return int64(math.Round(*seconds * 1000))
}

// ciUser is a user's ref (@carol), or nil.
func ciUser(u *api.User) any {
	if u == nil || u.Username == "" {
		return nil
	}
	return userRef(u.Username)
}

// ciVerifyStatus is the verification of a retry, play or cancel: GitLab answers
// with the pipeline or job after the change, and its status must be one of want.
// The preview cannot name the exact status (pending or running), so the tool
// compares instead of the gate.
func ciVerifyStatus(status string, want []string) (string, []string) {
	if slices.Contains(want, status) {
		return "passed", nil
	}
	return "failed", []string{"status"}
}

// ciParseID reads a pipeline or job as an agent has it: its ID ("123", "#123"), its
// ref ("acme/web/-/pipelines/123") or its web URL
// ("https://gitlab.example.com/acme/web/-/pipelines/123/failures"). collection is
// "pipelines" or "jobs"; project is "" when the input names none.
func ciParseID(raw, collection string) (project string, id int, err error) {
	s := strings.TrimSpace(raw)
	part := "/-/" + collection + "/"
	if strings.Contains(s, "://") {
		u, perr := url.Parse(s)
		if perr != nil || !strings.Contains(u.Path, part) {
			return "", 0, errors.New("not a " + collection + " URL")
		}
		s = u.Path
	}
	if i := strings.Index(s, part); i >= 0 {
		p, numeric, perr := parseProject(s[:i])
		n := strings.SplitN(s[i+len(part):], "/", 2)[0]
		if perr != nil || numeric || !digits.MatchString(n) {
			return "", 0, errors.New("not a ref")
		}
		id, _ = strconv.Atoi(n)
		if id <= 0 {
			return "", 0, errors.New("not a ref")
		}
		return p, id, nil
	}
	s = strings.TrimPrefix(s, "#")
	if !digits.MatchString(s) {
		return "", 0, errors.New("not an ID")
	}
	if id, _ = strconv.Atoi(s); id <= 0 {
		return "", 0, errors.New("not an ID")
	}
	return "", id, nil
}

// ciTarget is the pipeline or job a command acts on.
type ciTarget struct {
	project string
	id      int
}

// ciIDTarget reads the pipeline or job of a command, the positional argument (or
// --<flag>), and the project. The ID is bound with aicli.SetResolved, so the ID,
// the ref and the URL are one request (CLI-054). find is the command that lists
// them.
func ciIDTarget(cmd *cobra.Command, gl *lazyClient, flag, collection, noun, find string) (ciTarget, error) {
	raw, _ := cmd.Flags().GetString(flag)
	if strings.TrimSpace(raw) == "" {
		e := aicli.Usage("missing_parameter", "--"+flag, fmt.Sprintf("`%s` needs the %s: its ID, ref or web URL.", cmd.CommandPath(), noun))
		e.Details["next_action"] = fmt.Sprintf("Pass the %s as the argument, for example `%s 123 --project acme/web`.", noun, cmd.CommandPath())
		return ciTarget{}, e
	}
	refProject, id, err := ciParseID(raw, collection)
	if err != nil {
		e := aicli.Validation("--"+flag, fmt.Sprintf("a %s ID (123), its ref (acme/web/-/%s/123) or its web URL", noun, collection),
			fmt.Sprintf("That is not a %s ID, ref or web URL.", noun))
		e.Details["next_action"] = fmt.Sprintf("Run `%s` to find the %s.", find, noun)
		return ciTarget{}, e
	}
	project, err := projectFlag(cmd, refProject, gl)
	if err != nil {
		return ciTarget{}, err
	}
	if err := aicli.SetResolved(cmd, flag, strconv.Itoa(id)); err != nil {
		return ciTarget{}, err
	}
	return ciTarget{project: project, id: id}, nil
}

// ---------------------------------------------------------------- waiting

// ciPollEvery is how often a wait reads the status again; tests shorten it.
var ciPollEvery = 5 * time.Second

// ciMaxWait bounds --wait, so a command ends well within the two minutes an agent
// harness gives a call (CLI-049).
const ciMaxWait = 90 * time.Second

// ciWaitFlag reads --wait: a duration of at most 90 s; "" or 0 does not wait. A
// longer value is refused, never lowered (CLI-028).
func ciWaitFlag(cmd *cobra.Command) (time.Duration, error) {
	raw, _ := cmd.Flags().GetString("wait")
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	d, err := aicli.ParseDuration(raw) // CLI-028: <n>ms, <n>s, <n>m, <n>h or <n>d (Q15)
	if err != nil || d < 0 || d > ciMaxWait {
		e := aicli.Usage("invalid_value", "--wait", "--wait must be a duration of at most 90s, such as 60s.")
		e.Details["expected"] = "a duration up to 90s: <n>ms, <n>s or <n>m (60s, 500ms)"
		e.Details["next_action"] = "Wait at most 90s per call; run the command again to keep waiting."
		return 0, e
	}
	return d, nil
}

// ciSleep waits d, or returns E_INTERRUPTED when the run is interrupted.
func ciSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return aicli.NewError("E_INTERRUPTED", "The wait was interrupted; reads change nothing.", map[string]any{
			"dispatch_state": "not_started", "next_action": "Run the command again if it is still needed."})
	case <-t.C:
		return nil
	}
}

// ciWait reads the status again every poll until it is finished or wait has
// passed, and reports whether it finished.
func ciWait(ctx context.Context, wait, poll time.Duration, read func(context.Context) (string, error)) (bool, error) {
	deadline := time.Now().Add(wait)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return false, nil
		}
		if err := ciSleep(ctx, min(poll, left)); err != nil {
			return false, err
		}
		status, err := read(ctx)
		if err != nil {
			return false, err
		}
		if ciFinished(status) {
			return true, nil
		}
	}
}

// ciWaitEnded tells the agent that the pipeline or job was still going when the
// wait ended (CLI-047): the output is its current state, not its result.
func ciWaitEnded(app *aicli.App, noun string, wait time.Duration) {
	app.Notice(aicli.Notice{Type: "wait_ended", Severity: "info",
		Message: fmt.Sprintf("The %s had not finished when the %s wait ended; run the same command again to keep waiting.", noun, wait)})
}

// ciDeprecatedWait reads the --timeout and --interval seconds of the deprecated
// `pipeline wait` and `job wait`: no timeout waits the most --wait allows, and a
// longer one is lowered to it with a notice (the deprecated commands keep working).
func ciDeprecatedWait(app *aicli.App, cmd *cobra.Command) (wait, poll time.Duration) {
	timeout, _ := cmd.Flags().GetInt("timeout")
	wait = time.Duration(timeout) * time.Second
	if timeout <= 0 || wait > ciMaxWait {
		if timeout > 0 {
			app.Notice(aicli.Notice{Type: "wait_lowered", Severity: "warning",
				Message: "--timeout is above 90s, the longest a call may wait; waiting 90s. Run the command again to keep waiting."})
		}
		wait = ciMaxWait
	}
	poll = ciPollEvery
	if interval, _ := cmd.Flags().GetInt("interval"); cmd.Flags().Changed("interval") && interval > 0 {
		poll = time.Duration(interval) * time.Second
	}
	return wait, poll
}

// ---------------------------------------------------------------- KEY=VALUE variables

// ciVariableKey is what GitLab accepts as a CI/CD variable key.
var ciVariableKey = regexp.MustCompile(`^[A-Za-z0-9_]{1,255}$`)

// ciKeyValues reads a repeatable KEY=VALUE flag (pipeline and job variables). The
// values may be secrets: the flag is in Meta.Secrets, and no message names them.
func ciKeyValues(cmd *cobra.Command, flag string) (keys, values []string, err error) {
	raw, _ := cmd.Flags().GetStringArray(flag)
	seen := map[string]bool{}
	for i, kv := range raw {
		key, value, ok := strings.Cut(kv, "=")
		key = strings.TrimSpace(key)
		if !ok || !ciVariableKey.MatchString(key) {
			e := aicli.Validation("--"+flag, "KEY=VALUE, the key made of letters, digits and _",
				fmt.Sprintf("--%s number %d is not KEY=VALUE with a valid key.", flag, i+1))
			return nil, nil, e
		}
		if seen[key] {
			return nil, nil, aicli.Validation("--"+flag, "each key once", fmt.Sprintf("--%s sets the same key twice.", flag))
		}
		seen[key] = true
		keys, values = append(keys, key), append(values, value)
	}
	return keys, values, nil
}

// ciSortedKeys returns the keys in order, for previews (deterministic, CLI-057).
func ciSortedKeys(keys []string) []string {
	out := append([]string{}, keys...)
	slices.Sort(out)
	return out
}
