package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// Refs (CLI-048). Every resource in output carries one ref, accepted back as input
// together with the resource's web URL; the ref of a write's target is its
// Target.Key, the canonical key scope files match (SEC-012). A scope grant for
// "acme/web" covers a key only when a "/", "!", "#", ":" or "@" follows the
// project path, so GitLab's "~" (label) and "%" (milestone) sigils cannot start
// a key. The forms:
//
//	project        acme/web
//	merge request  acme/web!5                  (mrRef)
//	issue          acme/web#12                 (issueRef)
//	label          acme/web/-/labels/bug       (labelRef; the name as GitLab shows it)
//	milestone      acme/web/-/milestones/3     (milestoneRef; the iid, as in its web URL)
//
// Other collections follow the label and milestone form, which mirrors GitLab's
// web paths: acme/web/-/pipelines/123, acme/web/-/jobs/456. A resource that belongs
// to a merge request or an issue adds its collection and ID to the parent's ref:
// acme/web!5/notes/12, acme/web!5/discussions/<id>, acme/web#12/notes/99 (GitLab's
// URLs and #note_12 anchors are accepted as input).
//
// A project is also accepted by its name ("payments"): projectFlag resolves it, or
// answers E_AMBIGUOUS with the candidates (CLI-048).

var (
	projectSegment = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
	digits         = regexp.MustCompile(`^[0-9]+$`)
)

func mrRef(project string, iid int) string    { return project + "!" + strconv.Itoa(iid) }
func issueRef(project string, iid int) string { return project + "#" + strconv.Itoa(iid) }
func labelRef(project, name string) string    { return project + "/-/labels/" + name }
func milestoneRef(project string, iid int) string {
	return project + "/-/milestones/" + strconv.Itoa(iid)
}

// parseProject canonicalises a project as an agent has it: a path ("acme/web",
// "acme/web/"), a web URL ("https://gitlab.example.com/acme/web/-/issues/3", the
// host is dropped), or a numeric ID ("42", numeric is true). The path is checked
// segment by segment, so a key built from it has no "." or ".." segment.
func parseProject(raw string) (path string, numeric bool, err error) {
	s := strings.TrimSpace(raw)
	if digits.MatchString(s) {
		return s, true, nil
	}
	if strings.Contains(s, "://") {
		u, perr := url.Parse(s)
		if perr != nil || u.Host == "" {
			return "", false, errors.New("not a project URL")
		}
		s = u.Path
	}
	if i := strings.Index(s, "/-/"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(strings.Trim(s, "/"), ".git")
	segs := strings.Split(s, "/")
	if len(segs) < 2 {
		return "", false, errors.New("a project path has a namespace and a name")
	}
	for _, seg := range segs {
		if !projectSegment.MatchString(seg) || strings.HasSuffix(seg, ".") {
			return "", false, errors.New("not a GitLab project path")
		}
	}
	return s, false, nil
}

// projectInvalid is E_USAGE for a --project value that is no project.
func projectInvalid(raw string) *aicli.Error {
	e := aicli.Usage("invalid_value", "--project", "--project needs a project path such as acme/web, its web URL, its numeric ID or its name.")
	e.Details["expected"] = "group/project, https://<host>/group/project, a numeric project ID, or a project name"
	return e
}

// projectFlag reads --project, or the project a ref named (refProject), and
// canonicalises it: a path or web URL locally; a numeric ID (one read) or a name
// (projectByName: one match, else E_AMBIGUOUS with the candidates or E_NOT_FOUND)
// through GitLab, because refs and scope keys use paths (CLI-048). The canonical
// path is bound with aicli.SetResolved, so "acme/web", its URL, its name and a ref
// that names it are one request (CLI-054). gl is the run's client; it is used only
// for an ID or a name.
func projectFlag(cmd *cobra.Command, refProject string, gl *lazyClient) (string, error) {
	raw, _ := cmd.Flags().GetString("project")
	given := cmd.Flags().Changed("project") && strings.TrimSpace(raw) != ""
	if !given && refProject == "" {
		e := aicli.Usage("missing_parameter", "--project", fmt.Sprintf("`%s` needs --project.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Add --project <group/project>, for example `%s --project acme/web`.", cmd.CommandPath())
		return "", e
	}
	path := refProject
	if given {
		p, err := projectFromFlag(cmd.Context(), raw, gl)
		if err != nil {
			return "", err
		}
		if refProject != "" && !strings.EqualFold(refProject, p) {
			e := aicli.Usage("conflicting_flags", "--project", "--project names another project than the ref given as the argument.")
			e.Details["next_action"] = "Give the project once: drop --project, or pass the bare name or number as the argument."
			return "", e
		}
		path = p
	}
	if err := aicli.SetResolved(cmd, "project", path); err != nil {
		return "", err
	}
	return path, nil
}

// projectFromFlag canonicalises a --project value: a path or URL as it is, a
// numeric ID or a name through GitLab.
func projectFromFlag(ctx context.Context, raw string, gl *lazyClient) (string, error) {
	if name := strings.TrimSpace(raw); projectIsName(name) {
		c, err := gl.get()
		if err != nil {
			return "", err
		}
		return projectByName(ctx, c, name)
	}
	p, numeric, err := parseProject(raw)
	if err != nil {
		return "", projectInvalid(raw)
	}
	if !numeric {
		return p, nil
	}
	c, err := gl.get()
	if err != nil {
		return "", err
	}
	return resolveProjectPath(ctx, c, p)
}

// resolveProjectPath turns a numeric project ID into its path_with_namespace.
func resolveProjectPath(ctx context.Context, c *api.Client, id string) (string, error) {
	data, err := c.Get(ctx, c.APIPath("/projects/"+api.EncodeProjectPath(id)))
	if err != nil {
		return "", readError(err, onNotFound("No project has that ID, or this account cannot see it.",
			"Pass the project path instead, for example --project acme/web."))
	}
	var p struct {
		PathWithNamespace string `json:"path_with_namespace"`
	}
	if err := jsonUnmarshal(data, &p); err != nil || p.PathWithNamespace == "" {
		return "", fmt.Errorf("parsing project %s: unexpected response", id)
	}
	return p.PathWithNamespace, nil
}

// projectExists reads the project (GET /projects/:path): nil when it exists,
// E_NOT_FOUND when it does not (GitLab answers 404 for a missing project and for a
// missing resource in it alike).
func projectExists(ctx context.Context, c *api.Client, project string) error {
	if _, err := c.Get(ctx, c.APIPath("/projects/"+api.EncodeProjectPath(project))); err != nil {
		return readError(err, projectNotFound())
	}
	return nil
}

// shellArg quotes a value for a copy-paste command line (a WriteSpec.ReadBack or a
// next_action) when it holds anything but plain characters.
func shellArg(s string) string {
	plain := func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._/:@%+=,-!#~", r)
	}
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !plain(r) }) {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`").Replace(s) + `"`
}

// parseIIDRef reads a merge request or issue as an agent has it: "5", sigil+"5"
// ("!5", "#5"), a ref ("acme/web!5"), or a web URL with urlPart
// ("/-/merge_requests/", "/-/issues/"). project is "" when the input names none.
func parseIIDRef(raw, sigil, urlPart string) (project string, iid int, err error) {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "://") || strings.Contains(s, urlPart) {
		if strings.Contains(s, "://") {
			u, perr := url.Parse(s)
			if perr != nil {
				return "", 0, errors.New("not a URL")
			}
			s = u.Path
		}
		i := strings.Index(s, urlPart)
		if i < 0 {
			return "", 0, fmt.Errorf("the URL has no %s part", urlPart)
		}
		p, _, perr := parseProject(s[:i])
		n, nerr := strconv.Atoi(strings.Trim(strings.SplitN(s[i+len(urlPart):], "/", 2)[0], " "))
		if perr != nil || nerr != nil || n <= 0 {
			return "", 0, errors.New("not a ref")
		}
		return p, n, nil
	}
	if i := strings.LastIndex(s, sigil); i >= 0 {
		head, tail := s[:i], s[i+len(sigil):]
		n, nerr := strconv.Atoi(tail)
		if nerr != nil || n <= 0 {
			return "", 0, errors.New("not a ref")
		}
		if head == "" {
			return "", n, nil
		}
		p, _, perr := parseProject(head)
		if perr != nil {
			return "", 0, perr
		}
		return p, n, nil
	}
	n, nerr := strconv.Atoi(s)
	if nerr != nil || n <= 0 {
		return "", 0, errors.New("not a number or ref")
	}
	return "", n, nil
}

// ---------------------------------------------------------------- naming a project

// projectIsName tells a project name from a path, URL or numeric ID: it has no "/"
// and is not a number. GitLab projects always have a namespace, so "web" is a name.
func projectIsName(s string) bool {
	return s != "" && !strings.Contains(s, "/") && !digits.MatchString(s)
}

// projectNamePages bounds a name lookup: at most this many pages of 100 search hits.
const projectNamePages = 3

// projectByName resolves a project name among the projects this account can see
// (CLI-048). GitLab's search also matches descriptions and parts of words, so the
// hits are kept only when the name's words appear, in order and as whole words, in
// the project's name or path (projectNameMatches): "payments" matches
// acme/payments-service and legacy/payments, not acme/repayments. One match is that
// project; several are E_AMBIGUOUS with the candidates; none is E_NOT_FOUND. When
// more hits remain than the lookup reads, one match proves nothing: E_USAGE asks
// for the path.
func projectByName(ctx context.Context, c *api.Client, name string) (string, error) {
	var matches []api.Project
	more := false
	for page := 1; page <= projectNamePages; page++ {
		ps, pag, err := c.Projects.ListPage(ctx, api.ProjectListOpts{Search: name, Simple: true, OrderBy: "path", Sort: "asc",
			Page: page, PerPage: api.MaxPerPage})
		if err != nil {
			return "", readError(err)
		}
		for _, p := range ps {
			if projectNameMatches(name, p.Name, p.Path) {
				matches = append(matches, p)
			}
		}
		if more = pag.NextPage > 0; !more {
			break
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].PathWithNamespace < matches[j].PathWithNamespace })
	list := fmt.Sprintf("`gitlab-cli project list --query %s`", shellArg(name))
	switch {
	case len(matches) == 1 && !more:
		return matches[0].PathWithNamespace, nil
	case len(matches) >= 2:
		cands := make([]aicli.Candidate, 0, len(matches))
		for _, p := range matches {
			label := p.NameWithNamespace
			if label == "" {
				label = p.PathWithNamespace
			}
			cands = append(cands, aicli.Candidate{Ref: p.PathWithNamespace, Label: label})
		}
		return "", projectAmbiguous("More than one project has that name; choose one by its ref.", "--project", cands, more, list)
	case more:
		e := aicli.Usage("invalid_value", "--project", "Too many projects match that name to pick one; pass the project's path.")
		e.Details["expected"] = "group/project, its web URL or numeric ID"
		e.Details["next_action"] = fmt.Sprintf("Run %s with --membership to narrow the matches, then pass the path.", list)
		return "", e
	}
	e := aicli.NotFound("No project this account can see has that name.",
		fmt.Sprintf("Run %s to find the project, then pass its path.", list))
	e.Details["parameter"] = "--project"
	return "", e
}

// projectCandidates is how many candidates an E_AMBIGUOUS lists; projectLabelChars
// caps each label (external text) so the error stays well inside its budget.
const (
	projectCandidates = 10
	projectLabelChars = 80
)

// projectAmbiguous is E_AMBIGUOUS for a name that matched the candidates (CLI-048).
// When it lists only some of them (more than projectCandidates matched, or the
// lookup stopped early), more_candidates says so and next_action names the command
// that lists every match.
func projectAmbiguous(message, parameter string, cands []aicli.Candidate, more bool, list string) *aicli.Error {
	if len(cands) > projectCandidates {
		cands, more = cands[:projectCandidates], true
	}
	for i := range cands {
		cands[i].Label = projectClip(cands[i].Label, projectLabelChars)
	}
	e := aicli.Ambiguous(message, cands)
	e.Details["parameter"] = parameter
	if more {
		e.Details["more_candidates"] = true
		e.Details["next_action"] = fmt.Sprintf("Ask the user which one they mean (%s lists every match), then run the command again with its ref.", list)
	}
	return e
}

// projectNameMatches reports whether the words of name appear, in order and as whole
// words, in one of names (case and punctuation aside): "payments" matches
// "payments-service" and "Legacy Payments", "payments service" matches "Payments
// Service", and "pay" matches neither.
func projectNameMatches(name string, names ...string) bool {
	want := projectWords(name)
	if len(want) == 0 {
		return false
	}
	for _, n := range names {
		have := projectWords(n)
		for i := 0; i+len(want) <= len(have); i++ {
			if slices.Equal(have[i:i+len(want)], want) {
				return true
			}
		}
	}
	return false
}

func projectWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// projectClip shortens external text to at most n characters, marking the cut.
func projectClip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
