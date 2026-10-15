package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The job group: get (with --wait), log, retry, play, cancel, artifacts, and wait
// (deprecated for get --wait). A job is named by its global ID, the number in its
// web URL (/-/jobs/<id>).

var (
	jobFields = []string{"ref", "id", "name", "stage", "status", "finished", "failure_reason", "allow_failure", "ref_name",
		"tag", "pipeline", "user", "created_at", "started_at", "finished_at", "duration_ms", "queued_duration_ms",
		"coverage", "artifacts", "artifacts_expire_at", "web_url"}
	jobDefaultFields = []string{"ref", "name", "stage", "status", "failure_reason", "allow_failure", "duration_ms", "web_url"}
	// Job and stage names come from .gitlab-ci.yml, the branch name from people.
	jobUntrusted = []string{"name", "stage", "ref_name"}
	jobLogFields = []string{"ref", "status", "finished", "log", "lines", "total_lines", "matched_lines", "total_bytes",
		"truncated", "truncated_reason"}
)

func registerJob(app *aicli.App) {
	group := addGroup(app, "job", "Read CI/CD jobs and their logs; retry, start or cancel them; download their artifacts", "jobs")
	app.Schema("job", aicli.SchemaDef{Shape: "object", Fields: jobFields, Untrusted: jobUntrusted})
	app.Schema("job_page", aicli.SchemaDef{Shape: "page", Fields: jobFields, DefaultFields: jobDefaultFields, Untrusted: jobUntrusted})
	app.Schema("job_log", aicli.SchemaDef{Shape: "object", Fields: jobLogFields, Untrusted: []string{"log"}})
	app.Schema("job_artifacts", aicli.SchemaDef{Shape: "object", Fields: []string{"ref", "path", "bytes", "sha256"}})
	addJobGet(app, group)
	addJobLog(app, group)
	addJobRetry(app, group)
	addJobPlay(app, group)
	addJobCancel(app, group)
	addJobArtifacts(app, group)
	addJobWait(app, group)
}

// jobOut is a job as every job command returns it. Artifacts are a bounded summary
// (CLI-043): their kinds and sizes; job artifacts downloads the archive.
func jobOut(project string, j api.Job) map[string]any {
	artifacts := make([]map[string]any, 0, len(j.Artifacts))
	for _, a := range j.Artifacts {
		artifacts = append(artifacts, map[string]any{"file_type": a.FileType, "size_bytes": a.Size})
	}
	out := map[string]any{
		"ref": jobRef(project, j.ID), "id": idString(j.ID), "name": j.Name, "stage": j.Stage, "status": j.Status,
		"finished": ciFinished(j.Status), "failure_reason": optString(j.FailureReason), "allow_failure": j.AllowFailure,
		"ref_name": j.Ref, "tag": j.Tag, "pipeline": nil, "user": ciUser(j.User), "created_at": utcTime(j.CreatedAt),
		"started_at": utcTime(j.StartedAt), "finished_at": utcTime(j.FinishedAt), "duration_ms": ciMillis(j.Duration),
		"queued_duration_ms": ciMillis(j.QueuedDuration), "coverage": nil, "artifacts": artifacts,
		"artifacts_expire_at": utcTime(j.ArtifactsExpireAt), "web_url": j.WebURL,
	}
	if j.Pipeline != nil && j.Pipeline.ID > 0 {
		out["pipeline"] = pipelineRef(project, j.Pipeline.ID)
	}
	if j.Coverage != nil {
		out["coverage"] = *j.Coverage
	}
	return out
}

// jobNotFound is the 404 of a job read.
func jobNotFound(project string) errOpt {
	return onNotFound("The project has no job with that ID, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli pipeline list --project %s`, then `gitlab-cli pipeline jobs <pipeline>`, to find the job.", project))
}

// jobTarget reads the job positional (or --job) and the project.
func jobTarget(cmd *cobra.Command, gl *lazyClient) (ciTarget, error) {
	return ciIDTarget(cmd, gl, "job", "jobs", "job", "gitlab-cli pipeline jobs <pipeline> --project <group/project>")
}

func addJobTargetFlags(c *cobra.Command) {
	c.Flags().String("job", "", "Job ID, ref (acme/web/-/jobs/456) or web URL")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the job is a ref or URL")
}

func addJobGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <job>", RunE: func(cmd *cobra.Command, _ []string) error {
		wait, err := ciWaitFlag(cmd)
		if err != nil {
			return err
		}
		return jobShow(app, cmd, wait, ciPollEvery)
	}}
	addJobTargetFlags(c)
	c.Flags().String("wait", "", "Wait until the job finishes, at most this long (up to 90s): 60s, 90s")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one job by ID, ref or URL; --wait waits up to 90s for it to finish",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "job", Positional: "job",
		Params:     map[string]aicli.ParamDoc{"wait": {Unit: "duration"}},
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{jobEvidence, liveEvidence("job")},
		Examples: []string{"gitlab-cli job get 456 --project acme/web",
			"gitlab-cli job get acme/web/-/jobs/456 --wait 60s"}})
}

// jobRead reads a job; with wait, it reads it again until it finishes or the wait
// ends, which a notice then says (CLI-049).
func jobRead(ctx context.Context, app *aicli.App, client *api.Client, t ciTarget, wait, poll time.Duration) (*api.Job, error) {
	read := func(ctx context.Context) (*api.Job, error) {
		j, err := client.Jobs.Get(ctx, t.project, t.id)
		if err != nil {
			return nil, readError(err, jobNotFound(t.project))
		}
		return j, nil
	}
	j, err := read(ctx)
	if err != nil || wait <= 0 || ciFinished(j.Status) {
		return j, err
	}
	finished, err := ciWait(ctx, wait, poll, func(ctx context.Context) (string, error) {
		nj, err := read(ctx)
		if err != nil {
			return "", err
		}
		j = nj
		return nj.Status, nil
	})
	if err != nil {
		return nil, err
	}
	if !finished {
		ciWaitEnded(app, "job", wait)
	}
	return j, nil
}

func jobShow(app *aicli.App, cmd *cobra.Command, wait, poll time.Duration) error {
	gl := gitlabFor(app)
	t, err := jobTarget(cmd, gl)
	if err != nil {
		return err
	}
	client, err := gl.get()
	if err != nil {
		return err
	}
	j, err := jobRead(cmd.Context(), app, client, t, wait, poll)
	if err != nil {
		return err
	}
	return app.OK(cmd, jobOut(t.project, *j))
}

// ---------------------------------------------------------------- job log

// jobLogBudget is the room, in JSON characters, for the log text of a JSON answer:
// the whole envelope stays within the 8,000-character success budget (CLI-042).
const jobLogBudget = 6000

// GitLab traces carry terminal control sequences: colours and erase-line (ESC [ ...),
// and collapsible-section markers ("section_start:1700000000:step_script\r\x1b[0K").
// JSON output strips them; --format raw passes the trace through untouched.
var (
	jobLogSection = regexp.MustCompile(`section_(?:start|end):[0-9]+:[A-Za-z0-9_.\-]+(?:\[[^\]\r\n]*\])?\r?`)
	jobLogANSI    = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)
)

// logLine is one line of a trace: its bytes (without the newline) and its text
// as JSON shows it.
type logLine struct {
	raw  []byte
	text string
}

// jobLogText is a trace line as JSON shows it: no section markers, no escape
// sequences or other control characters but tabs, and of a line a carriage return
// overwrote on a terminal only what was shown last.
func jobLogText(raw []byte) string {
	s := strings.TrimSuffix(string(raw), "\r")
	s = jobLogSection.ReplaceAllString(s, "")
	s = jobLogANSI.ReplaceAllString(s, "")
	if i := strings.LastIndex(s, "\r"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\t') || r == 0x7f {
			return -1
		}
		return r
	}, s)
	return strings.ToValidUTF8(s, "�")
}

// jobLogLines splits a trace into lines. With complete, a last line without a
// newline is left out and its bytes returned as rest (a running job may still be
// writing it).
func jobLogLines(trace []byte, complete bool) (lines []logLine, rest []byte) {
	if complete {
		i := bytes.LastIndexByte(trace, '\n')
		trace, rest = trace[:i+1], trace[i+1:]
	}
	if len(trace) == 0 {
		return nil, rest
	}
	parts := bytes.Split(trace, []byte("\n"))
	if len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	for _, p := range parts {
		lines = append(lines, logLine{raw: p, text: jobLogText(p)})
	}
	return lines, rest
}

// jobLogSel is what job log returns: the --tail, --grep and --max-bytes selection,
// raw or JSON, and --follow with its --wait.
type jobLogSel struct {
	tail, maxBytes int
	grep           *regexp.Regexp
	raw, follow    bool
	wait           time.Duration
}

// shown reports whether a line is shown at all: in JSON, a line that held only
// markers and escape sequences is left out; --grep matches the text JSON shows.
func (s jobLogSel) shown(l logLine) bool {
	if !s.raw && len(l.raw) > 0 && l.text == "" {
		return false
	}
	return s.grep == nil || s.grep.MatchString(l.text)
}

func readJobLogFlags(app *aicli.App, cmd *cobra.Command) (jobLogSel, error) {
	var s jobLogSel
	s.tail, _ = cmd.Flags().GetInt("tail")
	if s.tail < 0 {
		e := aicli.Usage("invalid_value", "--tail", "--tail must be 0 (every line) or more.")
		e.Details["expected"] = "0 or more"
		return s, e
	}
	s.maxBytes, _ = cmd.Flags().GetInt("max-bytes")
	if s.maxBytes < 0 {
		e := aicli.Usage("invalid_value", "--max-bytes", "--max-bytes must be 0 (no limit) or more.")
		e.Details["expected"] = "0 or more"
		return s, e
	}
	if pattern, _ := cmd.Flags().GetString("grep"); pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return s, aicli.Validation("--grep", "an RE2 regular expression, such as error|fail", "--grep is not a valid regular expression.")
		}
		s.grep = re
	}
	s.raw = aicli.OutputFormat(cmd) == "raw"
	s.follow, _ = cmd.Flags().GetBool("follow")
	wait, err := ciWaitFlag(cmd)
	if err != nil {
		return s, err
	}
	s.wait = wait
	if cmd.Flags().Changed("timeout") && s.follow {
		if cmd.Flags().Changed("wait") {
			e := aicli.Usage("conflicting_flags", "--timeout", "Give the time to follow once, with --wait.")
			e.Details["next_action"] = "Drop --timeout (deprecated) and keep --wait."
			return s, e
		}
		s.wait, _ = ciDeprecatedWait(app, cmd)
	}
	if s.follow {
		if s.wait == 0 {
			s.wait = 60 * time.Second
		}
		for _, f := range []string{"fields", "max-bytes"} {
			if cmd.Flags().Changed(f) {
				e := aicli.Usage("conflicting_flags", "--"+f, fmt.Sprintf("--%s does not apply to --follow, which streams one line per item.", f))
				e.Details["next_action"] = fmt.Sprintf("Drop --%s, or drop --follow.", f)
				return s, e
			}
		}
	}
	return s, nil
}

func addJobLog(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "log <job>", RunE: func(cmd *cobra.Command, _ []string) error {
		sel, err := readJobLogFlags(app, cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		t, err := jobTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		if sel.follow {
			return jobLogFollow(app, cmd, client, t, sel)
		}
		// --wait without --follow waits for the job to finish, then shows its log.
		j, err := jobRead(cmd.Context(), app, client, t, sel.wait, ciPollEvery)
		if err != nil {
			return err
		}
		trace, err := client.Jobs.Log(cmd.Context(), t.project, t.id)
		if err != nil {
			return readError(err, jobNotFound(t.project))
		}
		return jobLogReport(app, cmd, t, j, trace, sel)
	}}
	addJobTargetFlags(c)
	c.Flags().Int("tail", 100, "Lines to return from the end of the log; 0 returns every line")
	c.Flags().String("grep", "", "Only lines matching this regular expression (RE2), before --tail")
	c.Flags().Int("max-bytes", 0, "Return at most this many bytes, from the end; 0 sets no byte limit")
	c.Flags().Bool("follow", false, "Stream the log as NDJSON, one line per item, until the job finishes or --wait ends")
	c.Flags().String("wait", "", "Wait until the job finishes, at most this long (up to 90s); with --follow the default is 60s")
	c.Flags().Int("timeout", 0, "Seconds to follow")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{
		Summary: "Show the end of a job's log, 100 lines by default; --grep filters, --format raw passes the bytes through, --follow streams NDJSON",
		Aliases: []string{"logs", "trace"}, Effects: []aicli.Effect{aicli.Observe}, Output: "job_log", Positional: "job",
		RawOutput: true,
		Params: map[string]aicli.ParamDoc{"tail": {Unit: "lines"}, "max-bytes": {Unit: "bytes"}, "wait": {Unit: "duration"},
			"timeout": {Unit: "seconds"}},
		DeprecatedFlags: map[string]string{"timeout": "use --wait"},
		ErrorCodes:      gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{jobEvidence, liveEvidence("job")},
		Examples: []string{"gitlab-cli job log 456 --project acme/web",
			"gitlab-cli job log acme/web/-/jobs/456 --grep 'error|fail' --tail 40",
			"gitlab-cli job log 456 --project acme/web --format raw --tail 0",
			"gitlab-cli job log 456 --project acme/web --follow --wait 60s"}})
}

// jobLogReport answers job log: --grep, then --tail, then --max-bytes; in JSON, the
// lines that fit the response budget, from the end, where a CI log says what failed.
// Whatever is left out is counted and said in truncated_reason.
func jobLogReport(app *aicli.App, cmd *cobra.Command, t ciTarget, j *api.Job, trace []byte, sel jobLogSel) error {
	all, _ := jobLogLines(trace, false)
	var matched []logLine
	for _, l := range all {
		if sel.shown(l) {
			matched = append(matched, l)
		}
	}
	out, reason := matched, ""
	if sel.tail > 0 && len(out) > sel.tail {
		out = out[len(out)-sel.tail:]
		reason = fmt.Sprintf("Only the last %d of %d lines are shown (--tail %d); raise --tail, or find a line with --grep.",
			sel.tail, len(matched), sel.tail)
	}
	cut := false
	if sel.maxBytes > 0 {
		if out, cut = jobLogLastBytes(out, sel.maxBytes, sel.raw); cut {
			reason = fmt.Sprintf("Only the last %d bytes are shown (--max-bytes %d).", sel.maxBytes, sel.maxBytes)
		}
	}
	if sel.raw {
		var b bytes.Buffer
		for _, l := range out {
			b.Write(l.raw)
			b.WriteByte('\n')
		}
		return app.OK(cmd, b.Bytes())
	}
	var budgetCut bool
	if out, budgetCut = jobLogFit(out, jobLogBudget); budgetCut {
		cut = true
		reason = fmt.Sprintf("Only the last %d lines fit the response size limit; use --format raw for more, or find a line with --grep.", len(out))
	}
	texts := make([]string, len(out))
	for i, l := range out {
		texts[i] = l.text
	}
	data := map[string]any{"ref": jobRef(t.project, t.id), "status": j.Status, "finished": ciFinished(j.Status),
		"log": strings.Join(texts, "\n"), "lines": len(out), "total_lines": len(all), "total_bytes": len(trace),
		"truncated": cut || len(out) < len(matched)}
	if sel.grep != nil {
		data["matched_lines"] = len(matched)
	}
	if data["truncated"] == true {
		data["truncated_reason"] = reason
	}
	return app.OK(cmd, data)
}

// jobLogLastBytes keeps the last max bytes of lines (raw bytes, or the JSON text),
// cutting the first kept line at a character boundary.
func jobLogLastBytes(lines []logLine, max int, raw bool) ([]logLine, bool) {
	size := func(l logLine) int {
		if raw {
			return len(l.raw) + 1
		}
		return len(l.text) + 1
	}
	total := 0
	for i := len(lines) - 1; i >= 0; i-- {
		n := size(lines[i])
		if total+n <= max {
			total += n
			continue
		}
		room := max - total - 1
		if room <= 0 {
			return lines[i+1:], true
		}
		l := lines[i]
		if raw {
			b := l.raw[len(l.raw)-room:]
			for len(b) > 0 && !utf8.RuneStart(b[0]) {
				b = b[1:]
			}
			l.raw = b
		} else {
			s := l.text[len(l.text)-room:]
			for len(s) > 0 && !utf8.RuneStart(s[0]) {
				s = s[1:]
			}
			l.text = s
		}
		return append([]logLine{l}, lines[i+1:]...), true
	}
	return lines, false
}

// jobLogFit keeps the last lines whose JSON text fits budget characters; a single
// line longer than the budget keeps its end.
func jobLogFit(lines []logLine, budget int) ([]logLine, bool) {
	used := 0
	for i := len(lines) - 1; i >= 0; i-- {
		n := jsonLen(lines[i].text) + 2 // the joining newline is \n in JSON
		if used+n <= budget {
			used += n
			continue
		}
		if i == len(lines)-1 {
			l := lines[i]
			l.text = jsonTail(l.text, budget)
			return []logLine{l}, true
		}
		return lines[i+1:], true
	}
	return lines, false
}

// jsonLen is the length of s as a JSON string, without the quotes.
func jsonLen(s string) int {
	b, _ := json.Marshal(s)
	return len(b) - 2
}

// jsonTail is the end of s that fits budget JSON characters.
func jsonTail(s string, budget int) string {
	used, start := 0, len(s)
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:start])
		n := jsonLen(string(r))
		if used+n > budget {
			break
		}
		used += n
		start -= size
	}
	return s[start:]
}

// jobLogFollow streams the log as NDJSON (CLI-006): the last --tail lines, then each
// new line, until the job finishes or --wait ends; the summary line says which.
func jobLogFollow(app *aicli.App, cmd *cobra.Command, client *api.Client, t ciTarget, sel jobLogSel) error {
	ctx := cmd.Context()
	j, err := client.Jobs.Get(ctx, t.project, t.id)
	if err != nil {
		return readError(err, jobNotFound(t.project))
	}
	trace, err := client.Jobs.Log(ctx, t.project, t.id)
	if err != nil {
		return readError(err, jobNotFound(t.project))
	}
	s, err := app.Stream(cmd)
	if err != nil {
		return err
	}
	count := 0
	emit := func(lines []logLine) error {
		for _, l := range lines {
			if !sel.shown(l) {
				continue
			}
			count++
			if sel.raw {
				if err := s.Item(append(append([]byte{}, l.raw...), '\n')); err != nil {
					return err
				}
				continue
			}
			if err := s.Item(map[string]any{"line": l.text}, "line"); err != nil {
				return err
			}
		}
		return nil
	}
	lines, rest := jobLogLines(trace, true)
	var start []logLine
	for _, l := range lines {
		if sel.shown(l) {
			start = append(start, l)
		}
	}
	if sel.tail > 0 && len(start) > sel.tail {
		start = start[len(start)-sel.tail:]
	}
	if err := emit(start); err != nil {
		return err
	}
	offset := len(trace) - len(rest)
	finished := ciFinished(j.Status)
	deadline := time.Now().Add(sel.wait)
	for !finished {
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		if err := ciSleep(ctx, min(ciPollEvery, left)); err != nil {
			return err
		}
		chunk, err := client.Jobs.TraceFrom(ctx, t.project, t.id, offset)
		if err != nil {
			return readError(err, jobNotFound(t.project))
		}
		lines, rest := jobLogLines(chunk, true)
		if err := emit(lines); err != nil {
			return err
		}
		offset += len(chunk) - len(rest)
		if j, err = client.Jobs.Get(ctx, t.project, t.id); err != nil {
			return readError(err, jobNotFound(t.project))
		}
		finished = ciFinished(j.Status)
	}
	if finished {
		// The end of the log, a last line without a newline included.
		chunk, err := client.Jobs.TraceFrom(ctx, t.project, t.id, offset)
		if err != nil {
			return readError(err, jobNotFound(t.project))
		}
		lines, _ := jobLogLines(chunk, false)
		if err := emit(lines); err != nil {
			return err
		}
	}
	summary := map[string]any{"ref": jobRef(t.project, t.id), "status": j.Status, "finished": finished, "count": count,
		"has_more": !finished}
	if !finished {
		summary["continuation_reason"] = fmt.Sprintf("The job was still running when the %s wait ended; run job log --follow again to keep following.", sel.wait)
	}
	s.Summary(summary)
	return nil
}

// ---------------------------------------------------------------- retry, play, cancel

// jobAction is a retry, a play or a cancel: what blocks it for a status, the
// statuses GitLab's answer shows when it worked, and the request.
type jobAction struct {
	verb, note string
	blocks     func(status string) []aicli.Block
	worked     []string
	extra      []aicli.FieldChange
	send       func(ctx context.Context, c *api.Client, project string, id int) (*api.Job, error)
	newJob     bool // GitLab answers with a new job (retry)
}

// jobWrite runs a retry, play or cancel through the gate. The base version is the
// status, as for pipelines.
func jobWrite(app *aicli.App, cmd *cobra.Command, a jobAction) error {
	gl := gitlabFor(app)
	t, err := jobTarget(cmd, gl)
	if err != nil {
		return err
	}
	ref := jobRef(t.project, t.id)
	return app.Write(cmd, aicli.WriteSpec{
		Target:   aicli.Target{Key: ref, Kind: "job"},
		ReadBack: "job get " + ref,
		Preview: func(ctx context.Context) (aicli.Preview, error) {
			client, err := gl.get()
			if err != nil {
				return aicli.Preview{}, err
			}
			cur, err := client.Jobs.Get(ctx, t.project, t.id)
			if err != nil {
				return aicli.Preview{}, readError(err, jobNotFound(t.project))
			}
			changes := append([]aicli.FieldChange{{Field: "status", Before: cur.Status, Note: a.note}}, a.extra...)
			pv := aicli.Preview{Action: fmt.Sprintf("%s job %s", a.verb, ref), CanApply: true, Changes: changes,
				BaseVersion: cur.Status, BaseSource: "status"}
			if b := a.blocks(cur.Status); len(b) > 0 {
				pv.CanApply, pv.BlockedBy = false, b
			}
			return pv, nil
		},
		Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
			client, err := gl.get()
			if err != nil {
				return aicli.Result{}, aicli.NotDispatched(err)
			}
			j, err := a.send(ctx, client, t.project, t.id)
			if err != nil {
				return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), jobNotFound(t.project))
			}
			obs := map[string]any{"status": j.Status}
			if a.newJob {
				// The new job's ref is observed too, so `operation show` finds it after a
				// lost reply (CLI-059).
				obs["ref"] = jobRef(t.project, j.ID)
			}
			res := aicli.Result{Observed: obs, Data: jobOut(t.project, *j), Untrusted: jobUntrusted}
			res.Outcome.Verification, res.Mismatched = ciVerifyStatus(j.Status, a.worked)
			return res, nil
		},
	})
}

func addJobRetry(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "retry <job>", RunE: func(cmd *cobra.Command, _ []string) error {
		return jobWrite(app, cmd, jobAction{verb: "Retry", newJob: true, worked: ciActiveStates,
			note: "GitLab runs a new copy of the job; the result is the new job",
			blocks: func(status string) []aicli.Block {
				switch {
				case status == "manual" || status == "scheduled":
					return []aicli.Block{{Kind: "manual_job", Message: "A manual or scheduled job is started with `job play`, not retried."}}
				case !slices.Contains([]string{"failed", "canceled", "success"}, status):
					return []aicli.Block{{Kind: "not_finished", Message: "Only a failed, canceled or successful job can be retried."}}
				}
				return nil
			},
			send: func(ctx context.Context, c *api.Client, project string, id int) (*api.Job, error) {
				return c.Jobs.Retry(ctx, project, id)
			}})
	}}
	addJobTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Run a finished job again as a new job (a manual job is started with job play)",
		Effects: []aicli.Effect{aicli.Change}, Output: "job", Positional: "job", ErrorCodes: gitlabWriteCodes,
		Maturity: aicli.Verified, Evidence: []string{jobEvidence, liveEvidence("job")},
		Examples: []string{"gitlab-cli job retry 456 --project acme/web --dry-run",
			"gitlab-cli job retry 456 --project acme/web --confirm <confirm_token>"}})
}

func addJobPlay(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "play <job>", RunE: func(cmd *cobra.Command, _ []string) error {
		keys, values, err := ciKeyValues(cmd, "variable")
		if err != nil {
			return err
		}
		vars := make([]api.JobVariable, len(keys))
		for i := range keys {
			vars[i] = api.JobVariable{Key: keys[i], Value: values[i]}
		}
		a := jobAction{verb: "Start", note: "the job starts", worked: ciActiveStates,
			blocks: func(status string) []aicli.Block {
				switch {
				case status == "manual" || status == "scheduled":
					return nil
				case slices.Contains([]string{"failed", "canceled", "success"}, status):
					return []aicli.Block{{Kind: "not_manual", Message: "Only a manual or scheduled job can be started; a finished job is run again with `job retry`."}}
				}
				return []aicli.Block{{Kind: "not_manual", Message: "Only a manual or scheduled job can be started; this job is already queued or running."}}
			},
			send: func(ctx context.Context, c *api.Client, project string, id int) (*api.Job, error) {
				return c.Jobs.Play(ctx, project, id, vars)
			}}
		// Job variables may be secrets: the preview names the keys only (the flag is in
		// Meta.Secrets, so the inputs show *** and bind a keyed digest).
		if len(keys) > 0 {
			a.extra = []aicli.FieldChange{{Field: "variables", After: ciSortedKeys(keys), Note: "the values are not shown"}}
		}
		return jobWrite(app, cmd, a)
	}}
	addJobTargetFlags(c)
	c.Flags().StringArray("variable", nil, "Job variable KEY=VALUE, repeatable; values are secret: never shown or stored")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Start a manual or scheduled (delayed) job, optionally with job variables",
		Aliases: []string{"start"}, Effects: []aicli.Effect{aicli.Change}, Output: "job", Positional: "job",
		Secrets: []string{"variable"}, ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{jobEvidence},
		Examples: []string{"gitlab-cli job play 456 --project acme/web --dry-run",
			"gitlab-cli job play 456 --project acme/web --confirm <confirm_token>"}})
}

func addJobCancel(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "cancel <job>", RunE: func(cmd *cobra.Command, _ []string) error {
		return jobWrite(app, cmd, jobAction{verb: "Cancel", note: "the job stops", worked: ciCanceledStates,
			blocks: func(status string) []aicli.Block {
				if slices.Contains(ciActiveStates, status) {
					return nil
				}
				return []aicli.Block{{Kind: "not_cancelable", Message: "The job is not queued or running, so it cannot be canceled."}}
			},
			send: func(ctx context.Context, c *api.Client, project string, id int) (*api.Job, error) {
				return c.Jobs.Cancel(ctx, project, id)
			}})
	}}
	addJobTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Cancel a pending or running job (job retry runs it again)",
		Effects: []aicli.Effect{aicli.Change}, Output: "job", Positional: "job", ErrorCodes: gitlabWriteCodes,
		Maturity: aicli.Experimental, Evidence: []string{jobEvidence},
		Examples: []string{"gitlab-cli job cancel 456 --project acme/web --dry-run",
			"gitlab-cli job cancel 456 --project acme/web --confirm <confirm_token>"}})
}

// ---------------------------------------------------------------- artifacts

// jobArtifactsTimeout bounds a download (WriteSpec.Timeout): archives can be large.
const jobArtifactsTimeout = 10 * time.Minute

func addJobArtifacts(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "artifacts <job>", RunE: func(cmd *cobra.Command, _ []string) error {
		raw, _ := cmd.Flags().GetString("output")
		path, err := filepath.Abs(strings.TrimSpace(raw))
		if strings.TrimSpace(raw) == "" || err != nil {
			e := aicli.Usage("invalid_value", "--output", "--output must name the new file to write.")
			e.Details["expected"] = "a file path such as ./artifacts.zip"
			return e
		}
		// The command only creates files: the gate refuses an --output that exists, at
		// the dry-run and at the confirm, as E_VALIDATION (artifact without persist, Q9).
		// The absolute path is what the scope checks and what is written, so it is what
		// the token binds: ./a.zip from another directory is another request.
		if err := aicli.SetResolved(cmd, "output", path); err != nil {
			return err
		}
		gl := gitlabFor(app)
		t, err := jobTarget(cmd, gl)
		if err != nil {
			return err
		}
		ref := jobRef(t.project, t.id)
		return app.Write(cmd, aicli.WriteSpec{
			// The target is the job, so a scope narrowed to the project covers it; the file
			// is the preview's output, which the scope's outputs must cover (SEC-012).
			Target:   aicli.Target{Key: ref, Kind: "job_artifacts"},
			ReadBack: "job get " + ref,
			Timeout:  jobArtifactsTimeout,
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				j, err := client.Jobs.Get(ctx, t.project, t.id)
				if err != nil {
					return aicli.Preview{}, readError(err, jobNotFound(t.project))
				}
				changes := []aicli.FieldChange{{Field: "path", After: path, Note: "a new file; an existing file is never overwritten"}}
				if j.ArtifactsFile != nil {
					changes = append([]aicli.FieldChange{{Field: "bytes", After: j.ArtifactsFile.Size}}, changes...)
				}
				pv := aicli.Preview{Action: fmt.Sprintf("Save the artifacts archive of job %s as %s", ref, path), CanApply: true,
					Changes: changes, Outputs: []string{path}}
				if b := jobArtifactsBlocks(j, path); len(b) > 0 {
					pv.CanApply, pv.BlockedBy = false, b
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				return jobArtifactsSave(ctx, app, client, t, path)
			},
		})
	}}
	addJobTargetFlags(c)
	c.Flags().String("output", "", "The new file to write the artifacts archive to (an existing file is never overwritten)")
	_ = c.MarkFlagRequired("output")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Download a job's artifacts archive into a new local file (--output)",
		Aliases: []string{"download"}, Effects: []aicli.Effect{aicli.Artifact}, Output: "job_artifacts", Positional: "job",
		Params:     map[string]aicli.ParamDoc{"output": {Type: "path"}},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{jobEvidence, liveEvidence("job")},
		Examples: []string{"gitlab-cli job artifacts 456 --project acme/web --output ./artifacts.zip --dry-run",
			"gitlab-cli job artifacts 456 --project acme/web --output ./artifacts.zip --confirm <confirm_token>"}})
}

// jobArtifactsBlocks says why the archive cannot be saved at path: the job has
// none, or the path is a directory or has none (an existing file is the gate's
// refusal, Q9).
func jobArtifactsBlocks(j *api.Job, path string) []aicli.Block {
	var out []aicli.Block
	if j.ArtifactsFile == nil {
		out = append(out, aicli.Block{Kind: "no_artifacts", Message: "The job has no artifacts archive: none was uploaded, or it expired."})
	}
	switch info, err := os.Lstat(path); {
	case err == nil && info.IsDir():
		out = append(out, aicli.Block{Kind: "is_directory", Message: "The output path is a directory; name a new file in it."})
	case err == nil:
	default:
		if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
			out = append(out, aicli.Block{Kind: "no_directory", Message: "The output file's directory does not exist."})
		}
	}
	return out
}

// jobArtifactsSave streams the archive into a new file through App.OpenOutput (Q8):
// a temporary file beside path, committed only when the download is complete, never
// over an existing file (a file that appeared since the preview is Q9's
// E_VALIDATION). A failed download or write leaves nothing.
func jobArtifactsSave(ctx context.Context, app *aicli.App, client *api.Client, t ciTarget, path string) (aicli.Result, error) {
	out, e := app.OpenOutput(path, false)
	if e != nil {
		return aicli.Result{}, aicli.NotDispatched(e)
	}
	if err := client.Jobs.ArtifactsTo(ctx, t.project, t.id, out); err != nil {
		out.Abort()
		// The download failed, so nothing was saved: the read's own error.
		return aicli.Result{}, aicli.NotDispatched(readError(err, jobNotFound(t.project), requiresRole("reporter")))
	}
	if e := out.Commit(); e != nil {
		return aicli.Result{}, aicli.NotDispatched(e)
	}
	obs := map[string]any{"path": out.Path(), "bytes": out.Size()}
	data := map[string]any{"ref": jobRef(t.project, t.id), "path": out.Path(), "bytes": out.Size(), "sha256": out.SHA256()}
	return aicli.Result{Observed: obs, Data: data}, nil
}

func addJobWait(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "wait <job>", RunE: func(cmd *cobra.Command, _ []string) error {
		wait, poll := ciDeprecatedWait(app, cmd)
		return jobShow(app, cmd, wait, poll)
	}}
	addJobTargetFlags(c)
	c.Flags().Int("timeout", 0, "Seconds to wait, at most 90 (0: 90)")
	c.Flags().Int("interval", 0, "Seconds between status reads")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Wait up to 90s for a job to finish and show it",
		Deprecated: "use `job get <job> --wait <duration>`",
		Effects:    []aicli.Effect{aicli.Observe}, Output: "job", Positional: "job",
		Params:     map[string]aicli.ParamDoc{"timeout": {Unit: "seconds"}, "interval": {Unit: "seconds"}},
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Experimental, Evidence: []string{jobEvidence},
		Examples: []string{"gitlab-cli job wait 456 --project acme/web --timeout 60"}})
}
