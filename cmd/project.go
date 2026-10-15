package cmd

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The project group: the project, user and search nouns (docs/migration-2.0/notes.md). A project
// is named by its path (acme/web, also its ref), web URL, numeric ID or name; a name
// that matches several projects is E_AMBIGUOUS with the candidates (CLI-048).
// project members lists effective members, direct and inherited from groups, and its
// scope says so (CLI-041). Users are @carol refs (user.go); search takes --query and
// states GitLab's ranking (search.go).

const projectEvidence = "simulated:cmd/project_test.go"

var (
	projectFields = []string{"ref", "id", "name", "namespace", "description", "visibility", "default_branch", "archived",
		"web_url", "http_url_to_repo", "ssh_url_to_repo", "star_count", "forks_count", "created_at", "last_activity_at"}
	projectDefaultFields = []string{"ref", "name", "visibility", "default_branch", "archived", "last_activity_at", "web_url"}
	projectUntrusted     = []string{"name", "description", "default_branch"}

	// expires_on is GitLab's expires_at, a date: aicli holds every *_at key to a UTC time.
	projectMemberFields = []string{"ref", "id", "username", "name", "state", "access", "access_level", "membership",
		"expires_on", "web_url"}
	projectMemberDefaultFields = []string{"ref", "name", "state", "access", "membership", "expires_on"}
	projectMemberUntrusted     = []string{"name"}

	projectAccessNames = map[int]string{5: "minimal_access", 10: "guest", 15: "planner", 20: "reporter", 30: "developer",
		40: "maintainer", 50: "owner"}

	// projectPathPattern is a project path GitLab accepts (14.9 and later): letters,
	// digits, '_', '-' and '.', starting and ending with a letter or digit, no two
	// special characters in a row.
	projectPathPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9]|[._-][A-Za-z0-9])*$`)
)

// projectMembership is the scope of project members: what an agent must not assume
// (CLI-041; gitlab-cli 1.2.17 listed direct members only, and an agent reported a
// member missing who had access through the group).
const (
	projectMembership    = "effective: direct and inherited from groups"
	projectMembersCovers = "each user once, at their highest access; members of groups the project is shared with " +
		"can be missing unless this account is a member of the project"
)

// registerProject registers the project group: project, user and search.
func registerProject(app *aicli.App) {
	group := addGroup(app, "project", "List, read and create projects, and list who has access to them", "projects")
	app.Schema("project", aicli.SchemaDef{Shape: "object", Fields: projectFields, Untrusted: projectUntrusted})
	app.Schema("project_page", aicli.SchemaDef{Shape: "page", Fields: projectFields, DefaultFields: projectDefaultFields,
		Untrusted: projectUntrusted})
	app.Schema("project_member_page", aicli.SchemaDef{Shape: "page", Fields: projectMemberFields,
		DefaultFields: projectMemberDefaultFields, Untrusted: projectMemberUntrusted})
	addProjectList(app, group)
	addProjectGet(app, group)
	addProjectCreate(app, group)
	addProjectMembers(app, group)
	// member and members are the fleet's measured root guesses for project members
	// (CLI-022), with or without a list verb after them.
	for _, word := range []string{"member", "members", "member list", "members list"} {
		app.AddShortcut(word, "project", "members")
	}
	registerUser(app)
	registerSearch(app)
}

// projectOut is a project as every command returns it; fields GitLab did not send
// are null. search projects drops visibility and archived, which GitLab's search
// API never sends.
func projectOut(p api.Project) map[string]any {
	namespace := ""
	if i := strings.LastIndex(p.PathWithNamespace, "/"); i > 0 {
		namespace = p.PathWithNamespace[:i]
	}
	out := map[string]any{
		"ref": p.PathWithNamespace, "id": idString(p.ID), "name": p.Name, "namespace": optString(namespace),
		"description": p.Description, "visibility": optString(p.Visibility), "default_branch": optString(p.DefaultBranch),
		"archived": nil, "web_url": p.WebURL, "http_url_to_repo": optString(p.HTTPURLToRepo),
		"ssh_url_to_repo": optString(p.SSHURLToRepo), "star_count": p.StarCount, "forks_count": p.ForksCount,
		"created_at": utcTime(p.CreatedAt), "last_activity_at": utcTime(p.LastActivityAt),
	}
	if p.Archived != nil {
		out["archived"] = *p.Archived
	}
	return out
}

// projectMemberOut is a member as project members returns it; membership is
// "direct", "inherited" or "" (not computed: --fields left it out).
func projectMemberOut(m api.ProjectMember, membership string) map[string]any {
	var access any
	if name, ok := projectAccessNames[m.AccessLevel]; ok {
		access = name
	}
	return map[string]any{
		"ref": userRef(m.Username), "id": idString(m.ID), "username": m.Username, "name": m.Name, "state": optString(m.State),
		"access": access, "access_level": m.AccessLevel, "membership": optString(membership),
		"expires_on": optString(m.ExpiresAt), "web_url": optString(m.WebURL),
	}
}

// ---------------------------------------------------------------- list, get, members

func addProjectList(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		query, err := projectQuery(cmd)
		if err != nil {
			return err
		}
		owned, _ := cmd.Flags().GetBool("owned")
		membership, _ := cmd.Flags().GetBool("membership")
		visibility, _ := cmd.Flags().GetString("visibility")
		client, err := gitlabFor(app).get()
		if err != nil {
			return err
		}
		// Asked for explicitly, so the stated sort holds whatever GitLab's default is.
		ps, pag, err := client.Projects.ListPage(cmd.Context(), api.ProjectListOpts{Search: query, Owned: owned,
			Membership: membership, Visibility: visibility, OrderBy: "created_at", Sort: "desc", Page: req.Page, PerPage: req.Limit})
		if err != nil {
			return readError(err)
		}
		items := make([]map[string]any, 0, len(ps))
		for _, p := range ps {
			items = append(items, projectOut(p))
		}
		scope := map[string]any{"covers": "every project this account can see, public projects of others and archived ones included"}
		switch {
		case owned:
			scope["covers"], scope["owned"] = "projects this account owns (its own namespace, or the owner role), archived ones included", true
		case membership:
			scope["covers"] = "projects this account is a member of, directly or through a group, archived ones included"
		}
		if membership {
			scope["membership"] = true
		}
		if query != "" {
			scope["query"] = query
		}
		if visibility != "" {
			scope["visibility"] = visibility
		}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "created_at_desc"))
	}}
	c.Flags().String("query", "", "Only projects whose path, name or description contains this text")
	c.Flags().Bool("owned", false, "Only projects this account owns: its own namespace's, and those where it has the owner role")
	c.Flags().Bool("membership", false, "Only projects this account is a member of")
	c.Flags().String("visibility", "", "Only projects with this visibility: public, internal or private")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List the projects this account can see, newest first",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "project_page",
		Params:       map[string]aicli.ParamDoc{"visibility": {Enum: []string{"public", "internal", "private"}}},
		RenamedFlags: map[string]string{"search": "query"},
		ErrorCodes:   gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{projectEvidence, liveEvidence("project")},
		Examples: []string{"gitlab-cli project list --membership", "gitlab-cli project list --query payments --fields +description"}})
}

// projectQuery reads --query (1.x's --search too: Meta.RenamedFlags, CLI-081).
func projectQuery(cmd *cobra.Command) (string, error) {
	q, _ := cmd.Flags().GetString("query")
	return strings.TrimSpace(q), nil
}

func addProjectGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <project>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		p, err := client.Projects.Get(cmd.Context(), project)
		if err != nil {
			return readError(err, projectNotFound())
		}
		return app.OK(cmd, projectOut(*p))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL, numeric ID or name")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one project by path, URL, ID or name",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "project", Positional: "project",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{projectEvidence, liveEvidence("project")},
		Examples: []string{"gitlab-cli project get acme/web", "gitlab-cli project get payments"}})
}

func addProjectMembers(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "members <project>", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		query, _ := cmd.Flags().GetString("query")
		query = strings.TrimSpace(query)
		ms, pag, err := client.Projects.EffectiveMembersPage(cmd.Context(), project,
			api.MemberListOpts{Query: query, Page: req.Page, PerPage: req.Limit})
		if err != nil {
			return readError(err, projectNotFound())
		}
		via := map[int]string{}
		if wantsField(cmd, "membership") && len(ms) > 0 { // one more read, only when shown
			if via, err = projectMembershipOf(cmd.Context(), client, project, ms); err != nil {
				return err
			}
		}
		items := make([]map[string]any, 0, len(ms))
		for _, m := range ms {
			items = append(items, projectMemberOut(m, via[m.ID]))
		}
		scope := map[string]any{"project": project, "membership": projectMembership, "covers": projectMembersCovers}
		if query != "" {
			scope["query"] = query
		}
		// GitLab documents no order for this list, so the page states none.
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, ""))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL, numeric ID or name")
	c.Flags().String("query", "", "Only members whose name, username or email contains this text")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{
		Summary: "List everyone with access to a project: its direct members and those inherited from its groups",
		Aliases: []string{"member"}, Effects: []aicli.Effect{aicli.Observe}, Output: "project_member_page", Positional: "project",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{projectEvidence, liveEvidence("project")},
		Examples: []string{"gitlab-cli project members acme/web", "gitlab-cli project members acme/web --query carol"}})
}

// projectMembershipOf tells, for each listed member, where the access shown comes
// from: "direct" when the project's own membership grants it, "inherited" when a
// group does (a parent group, or a group the project is shared with). GitLab's
// effective list does not say, so one read of the direct members among them
// decides; a direct member with less access than shown has it through a group.
func projectMembershipOf(ctx context.Context, c *api.Client, project string, ms []api.ProjectMember) (map[int]string, error) {
	ids := make([]int, 0, len(ms))
	for _, m := range ms {
		ids = append(ids, m.ID)
	}
	direct, _, err := c.Projects.DirectMembersPage(ctx, project, api.MemberListOpts{UserIDs: ids, PerPage: api.MaxPerPage})
	if err != nil {
		return nil, readError(err, projectNotFound())
	}
	level := map[int]int{}
	for _, d := range direct {
		level[d.ID] = d.AccessLevel
	}
	out := make(map[int]string, len(ms))
	for _, m := range ms {
		if l, ok := level[m.ID]; ok && l >= m.AccessLevel {
			out[m.ID] = "direct"
		} else {
			out[m.ID] = "inherited"
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- create

// projectCreateWant is the requested project: name and path (given, or made from
// the name as GitLab would, and always sent, so the target key is the ref GitLab
// creates), description and visibility ("" when not given).
type projectCreateWant struct {
	name, path, description, visibility string
}

func readProjectCreateFlags(cmd *cobra.Command) (projectCreateWant, error) {
	var w projectCreateWant
	w.name, _ = cmd.Flags().GetString("name")
	w.name = strings.TrimSpace(w.name)
	if w.name == "" {
		return w, aicli.Validation("--name", "a project name", "The project name is empty.")
	}
	if cmd.Flags().Changed("path") {
		w.path, _ = cmd.Flags().GetString("path")
		w.path = strings.TrimSpace(w.path)
	} else if w.path = projectSlug(w.name); w.path == "" {
		return w, aicli.Validation("--path", "a path of letters, digits, '_', '-' or '.'",
			"The name has no letters or digits a path can be made from; pass --path.")
	}
	if !projectPathOK(w.path) {
		return w, aicli.Validation("--path", "letters, digits, '_', '-' and '.', starting and ending with a letter or digit",
			"--path is not a project path GitLab accepts.")
	}
	w.description, _ = cmd.Flags().GetString("description")
	w.visibility, _ = cmd.Flags().GetString("visibility")
	return w, nil
}

// projectSlug makes a project path from a name the way GitLab does when no path is
// given: lowercase, with a dash between words ("Payments API v2" gives
// payments-api-v2). Letters outside ASCII are dropped.
func projectSlug(name string) string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	return strings.Join(words, "-")
}

// projectPathOK checks a project path: GitLab's characters and shape, at most 255
// characters, not ending in .git or .atom.
func projectPathOK(p string) bool {
	lower := strings.ToLower(p)
	return len(p) <= 255 && projectPathPattern.MatchString(p) && !strings.HasSuffix(lower, ".git") && !strings.HasSuffix(lower, ".atom")
}

// projectNamespace is where a create puts the project: its full path, and whether it
// was named (Preview then reads its ID) or is the account's own.
type projectNamespace struct {
	path  string
	id    int // --namespace-id, until resolve reads its path
	named bool
}

// projectCreateNamespace reads where the project goes from the flags: --namespace
// (a group or user path, or its web URL), the deprecated --namespace-id, or neither,
// the account's own namespace. The last two need a read to name the path the target
// key uses; resolve makes it inside the gate (WriteSpec.ResolveTarget, Q1).
func projectCreateNamespace(cmd *cobra.Command) (projectNamespace, error) {
	raw, _ := cmd.Flags().GetString("namespace")
	given := cmd.Flags().Changed("namespace") && strings.TrimSpace(raw) != ""
	if cmd.Flags().Changed("namespace-id") {
		if given {
			e := aicli.Usage("conflicting_flags", "--namespace-id", "Give the namespace once: with --namespace, not also with --namespace-id.")
			e.Details["next_action"] = "Drop --namespace-id and keep --namespace."
			return projectNamespace{}, e
		}
		id, _ := cmd.Flags().GetInt("namespace-id")
		if id <= 0 {
			return projectNamespace{}, aicli.Validation("--namespace-id", "a positive namespace ID", "--namespace-id must be a positive number.")
		}
		return projectNamespace{id: id, named: true}, nil
	}
	if given {
		path, ok := projectNamespacePath(raw)
		if !ok {
			return projectNamespace{}, aicli.Validation("--namespace", "a group or user path such as acme or acme/platform, or its web URL",
				"--namespace is not a group or user path.")
		}
		if err := aicli.SetResolved(cmd, "namespace", path); err != nil {
			return projectNamespace{}, err
		}
		return projectNamespace{path: path, named: true}, nil
	}
	return projectNamespace{}, nil
}

// resolve reads the path of a namespace given by --namespace-id, or of the
// account's own namespace (its username). A path GitLab returned is checked like one
// typed, as it goes into the target key (SEC-002).
func (ns *projectNamespace) resolve(ctx context.Context, gl *lazyClient) error {
	if ns.path != "" {
		return nil
	}
	c, err := gl.get()
	if err != nil {
		return err
	}
	if ns.id > 0 {
		n, err := c.Projects.GetNamespace(ctx, strconv.Itoa(ns.id))
		if err != nil {
			return readError(err, onNotFound("No namespace has that ID, or this account cannot see it.",
				"Pass the group's path with --namespace instead, for example --namespace acme."))
		}
		path, ok := projectNamespacePath(n.FullPath)
		if !ok {
			return aicli.Validation("--namespace-id", "the ID of a group or user namespace",
				"GitLab returned a namespace path this tool cannot use; pass --namespace instead.")
		}
		ns.path = path
		return nil
	}
	me, err := c.Users.Me(ctx)
	if err != nil {
		return readError(err)
	}
	path, ok := projectNamespacePath(me.Username)
	if !ok {
		return aicli.Validation("--namespace", "a group or user path such as acme",
			"This account's username cannot be used as a namespace path here; pass --namespace.")
	}
	ns.path = path
	return nil
}

// projectNamespacePath canonicalises a namespace as an agent has it: a path
// ("acme", "acme/platform/"), or a web URL (https://gitlab.example.com/acme, also
// /groups/acme/-/...). Each segment is checked, so a key built from it has no "."
// or ".." segment.
func projectNamespacePath(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return "", false
		}
		s = strings.TrimPrefix(strings.Trim(u.Path, "/"), "groups/")
	}
	if i := strings.Index(s, "/-/"); i >= 0 {
		s = s[:i]
	}
	s = strings.Trim(s, "/")
	if s == "" {
		return "", false
	}
	for _, seg := range strings.Split(s, "/") {
		if !projectSegment.MatchString(seg) || strings.HasSuffix(seg, ".") {
			return "", false
		}
	}
	return s, true
}

func addProjectCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create", RunE: func(cmd *cobra.Command, _ []string) error {
		want, err := readProjectCreateFlags(cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		ns, err := projectCreateNamespace(cmd)
		if err != nil {
			return err
		}
		var ref string      // the new project's path, once the namespace is known
		var namespaceID int // read by Preview, which the gate runs again right before Apply
		readBack := "project list --owned --query " + shellArg(want.path)
		if ns.path != "" {
			readBack = "project get " + ns.path + "/" + want.path
		}
		return app.Write(cmd, aicli.WriteSpec{
			ResolveTarget: func(ctx context.Context) (aicli.Target, error) {
				if err := ns.resolve(ctx, gl); err != nil {
					return aicli.Target{}, err
				}
				ref = ns.path + "/" + want.path
				return aicli.Target{Key: ref, Kind: "project"}, nil
			},
			ReadBack: readBack,
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				pv := aicli.Preview{Action: "Create project " + ref, CanApply: true, Changes: projectCreateChanges(ref, want)}
				// GitLab refuses a second project at the same path: block the plan
				// instead of sending a doomed request. A repeat of an applied create with
				// the same --idempotency-key never gets here: the gate replays it (P4).
				var apiErr *api.APIError
				switch _, err := client.Projects.Get(ctx, ref); {
				case err == nil:
					pv.CanApply = false
					pv.BlockedBy = []aicli.Block{{Kind: "already_exists",
						Message: "A project already exists at that path; choose another --path or --namespace."}}
					return pv, nil
				case errors.As(err, &apiErr) && apiErr.StatusCode == 404:
				default:
					return aicli.Preview{}, readError(err)
				}
				if ns.named {
					n, err := client.Projects.GetNamespace(ctx, ns.path)
					if err != nil {
						return aicli.Preview{}, readError(err, onNotFound("No group or user namespace has that path, or this account cannot see it.",
							"Check --namespace: it is the group's path in its URL, like acme or acme/platform."))
					}
					namespaceID = n.ID
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				p, err := client.Projects.Create(ctx, &api.ProjectCreateRequest{Name: want.name, Path: want.path,
					Description: want.description, Visibility: want.visibility, NamespaceID: namespaceID})
				if err != nil {
					return aicli.Result{}, projectCreateError(err)
				}
				return aicli.Result{Observed: projectCreateObserved(*p, want), Data: projectOut(*p), Untrusted: projectUntrusted}, nil
			},
		})
	}}
	c.Flags().String("name", "", "Project name")
	c.Flags().String("path", "", "Project path, the last part of its URL; made from the name when left out")
	c.Flags().String("namespace", "", "Group or user path to create it in (acme, acme/platform); this account's own when left out")
	c.Flags().Int("namespace-id", 0, "Namespace ID")
	c.Flags().String("description", "", "Description")
	c.Flags().String("visibility", "", "private, internal or public; GitLab's default when left out")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same create with the same key returns the first result")
	_ = c.MarkFlagRequired("name")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create a project in a group or in this account's own namespace",
		Aliases: []string{"add", "new"}, Effects: []aicli.Effect{aicli.Change}, Output: "project",
		Params:          map[string]aicli.ParamDoc{"visibility": {Enum: []string{"private", "internal", "public"}}},
		DeprecatedFlags: map[string]string{"namespace-id": "pass the group's path with --namespace"},
		ErrorCodes:      gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{projectEvidence, liveEvidence("project")},
		Examples: []string{`gitlab-cli project create --name "Payments API" --namespace acme --visibility private --dry-run`,
			`gitlab-cli project create --name "Payments API" --namespace acme --visibility private --confirm <confirm_token>`}})
}

// projectCreateChanges is what a create sets, by output field, so the gate can
// compare it with the read-back: the new project's ref (its path) and name, and the
// description and visibility when given.
func projectCreateChanges(ref string, w projectCreateWant) []aicli.FieldChange {
	out := []aicli.FieldChange{{Field: "name", After: w.name}, {Field: "ref", After: ref}}
	if w.description != "" {
		out = append(out, aicli.FieldChange{Field: "description", After: w.description})
	}
	if w.visibility != "" {
		out = append(out, aicli.FieldChange{Field: "visibility", After: w.visibility})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// projectCreateObserved is the read-back of projectCreateChanges from GitLab's
// answer. The ref is observed too, so `operation show` finds the project after a
// lost reply (CLI-059).
func projectCreateObserved(p api.Project, w projectCreateWant) map[string]any {
	obs := map[string]any{"ref": p.PathWithNamespace, "name": p.Name}
	if w.description != "" {
		obs["description"] = p.Description
	}
	if w.visibility != "" {
		obs["visibility"] = p.Visibility
	}
	return obs
}

// projectCreateError classifies a failed create (classifyWriteError). A 403 names
// the role GitLab asks for by default to create projects in a group (CLI-014).
func projectCreateError(err error) error {
	return classifyWriteError(err, requiresRoleOn("maintainer", "the group (or the role the group allows to create projects)"))
}
