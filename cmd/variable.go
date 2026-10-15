package cmd

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The variable group: a project's CI/CD variables (list, get, create, update,
// delete, and bulk-import in variable_bulk.go). Variables configure the project's
// pipelines and often hold credentials, so every write is `configure`, dangerous by
// default (CLI-025), and no command ever shows a value: values go in through
// --value (a secret flag, Meta.Secrets) or --value-file, and come out nowhere.
//
// A key may exist once per environment scope. The ref is acme/web/-/variables/KEY
// for the scope *, and acme/web/-/variables/KEY:production for another scope; keys
// are letters, digits and _, so the colon is unambiguous, and a scope grant for the
// key covers each of its scopes.

var (
	variableFields        = []string{"ref", "key", "variable_type", "environment_scope", "protected", "masked", "hidden", "raw", "description"}
	variableDefaultFields = []string{"ref", "key", "variable_type", "environment_scope", "protected", "masked"}
	variableUntrusted     = []string{"key", "environment_scope", "description"}
	// variableScopePattern is what GitLab accepts in an environment scope: environment
	// names (letters, digits, spaces, - _ / $ { } .) and the * wildcard.
	variableScopePattern = regexp.MustCompile(`^[A-Za-z0-9 _./${}*-]{1,255}$`)
	variableTypes        = []string{"env_var", "file"}
)

func registerVariable(app *aicli.App) {
	group := addGroup(app, "variable", "List, read, create, update and delete a project's CI/CD variables (values are never shown)",
		"variables", "var")
	app.Schema("variable", aicli.SchemaDef{Shape: "object", Fields: variableFields, Untrusted: variableUntrusted})
	app.Schema("variable_page", aicli.SchemaDef{Shape: "page", Fields: variableFields, DefaultFields: variableDefaultFields,
		Untrusted: variableUntrusted})
	app.Schema("variable_deleted", aicli.SchemaDef{Shape: "object", Fields: []string{"ref", "deleted"}})
	addVariableList(app, group)
	addVariableGet(app, group)
	addVariableCreate(app, group)
	addVariableUpdate(app, group)
	addVariableDelete(app, group)
	addVariableBulkImport(app, group)
}

// variableName is a variable as a command line names it: KEY, or KEY:scope for a
// scope other than *.
func variableName(key, scope string) string {
	if scope == "" || scope == "*" {
		return key
	}
	return key + ":" + scope
}

// variableRef is a variable's ref (CLI-048) and the target key of its writes.
func variableRef(project, key, scope string) string {
	return project + "/-/variables/" + variableName(key, scope)
}

// variableOut is a variable as every variable command returns it: never its value.
func variableOut(project string, v api.Variable) map[string]any {
	return map[string]any{"ref": variableRef(project, v.Key, v.EnvironmentScope), "key": v.Key, "variable_type": v.VariableType,
		"environment_scope": v.EnvironmentScope, "protected": v.Protected, "masked": v.Masked, "hidden": v.Hidden, "raw": v.Raw,
		"description": optString(v.Description)}
}

// parseVariableArg reads a variable as an agent has it: KEY, KEY:scope, or the ref
// acme/web/-/variables/KEY[:scope]. project is "" when the input names none, and
// hasScope tells whether it names a scope.
func parseVariableArg(raw string) (project, key, scope string, hasScope bool, err error) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "/-/variables/"); i >= 0 {
		p, numeric, perr := parseProject(s[:i])
		if perr != nil || numeric {
			return "", "", "", false, errors.New("not a variable ref")
		}
		project, s = p, s[i+len("/-/variables/"):]
	}
	key, scope, hasScope = strings.Cut(s, ":")
	if !ciVariableKey.MatchString(key) || (hasScope && !variableScopeOK(scope)) {
		return "", "", "", false, errors.New("not a variable key")
	}
	return project, key, scope, hasScope, nil
}

// variableScopeOK checks an environment scope; "." and ".." are no scope (and would
// not make a canonical target key).
func variableScopeOK(scope string) bool {
	if !variableScopePattern.MatchString(scope) {
		return false
	}
	for _, seg := range strings.Split(scope, "/") {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// variableSel is the variable a command names; scope is "" when the agent named
// none (GitLab then picks the only variable with the key).
type variableSel struct {
	project, key, scope string
}

func (s variableSel) name() string { return variableName(s.key, s.scope) }

// variableTarget reads the variable positional (or --key), --env-scope (or 1.x's
// --filter env_scope=<scope>) and the project, and binds the key, and a scope given
// in the ref, as one request (CLI-054). It needs no GitLab.
func variableTarget(cmd *cobra.Command, gl *lazyClient) (variableSel, error) {
	raw, _ := cmd.Flags().GetString("key")
	if strings.TrimSpace(raw) == "" {
		e := aicli.Usage("missing_parameter", "--key", fmt.Sprintf("`%s` needs the variable: its key, KEY:scope or its ref.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Pass the variable as the argument, for example `%s DEPLOY_ENV --project acme/web`.", cmd.CommandPath())
		return variableSel{}, e
	}
	refProject, key, refScope, hasScope, err := parseVariableArg(raw)
	if err != nil {
		e := aicli.Validation("--key", "a key of letters, digits and _ (DEPLOY_ENV), KEY:scope, or acme/web/-/variables/KEY",
			"That is not a CI/CD variable key or ref.")
		e.Details["next_action"] = "Run `gitlab-cli variable list --project <group/project>` to find the variable."
		return variableSel{}, e
	}
	flagScope, flagGiven, err := variableScopeFlag(cmd)
	if err != nil {
		return variableSel{}, err
	}
	if hasScope && flagGiven && refScope != flagScope {
		e := aicli.Usage("conflicting_flags", "--env-scope", "--env-scope names another environment scope than the argument.")
		e.Details["next_action"] = "Give the scope once: as KEY:scope in the argument, or with --env-scope."
		return variableSel{}, e
	}
	project, err := projectFlag(cmd, refProject, gl)
	if err != nil {
		return variableSel{}, err
	}
	if err := aicli.SetResolved(cmd, "key", key); err != nil {
		return variableSel{}, err
	}
	scope := flagScope
	if hasScope {
		scope = refScope
		if err := aicli.SetResolved(cmd, "env-scope", refScope); err != nil {
			return variableSel{}, err
		}
	}
	return variableSel{project: project, key: key, scope: scope}, nil
}

// variableScopeFlag reads --env-scope, or the deprecated --filter of 1.x
// (env_scope=production, or production).
func variableScopeFlag(cmd *cobra.Command) (string, bool, error) {
	var scope string
	given := cmd.Flags().Changed("env-scope")
	if given {
		scope, _ = cmd.Flags().GetString("env-scope")
	}
	if f := cmd.Flags().Lookup("filter"); f != nil && f.Changed {
		if given {
			e := aicli.Usage("conflicting_flags", "--filter", "Give the environment scope once, with --env-scope.")
			e.Details["next_action"] = "Drop --filter (deprecated) and keep --env-scope."
			return "", false, e
		}
		scope, given = strings.TrimPrefix(strings.TrimSpace(f.Value.String()), "env_scope="), true
	}
	scope = strings.TrimSpace(scope)
	if given && !variableScopeOK(scope) {
		return "", false, aicli.Validation("--env-scope", "an environment name or pattern such as production, review/* or *",
			"--env-scope is not an environment scope.")
	}
	return scope, given, nil
}

// variableNotFound is the 404 of a variable read.
func variableNotFound(project string) errOpt {
	return onNotFound("The project has no CI/CD variable with that key and scope, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli variable list --project %s` to find the variable.", project))
}

// variableFind reads the variable a command names. Without a scope GitLab answers
// with the only variable of that key, or 409 when several scopes have it: those are
// listed as candidates (E_AMBIGUOUS, CLI-048).
func variableFind(ctx context.Context, c *api.Client, sel variableSel) (*api.Variable, error) {
	v, err := c.Variables.Get(ctx, sel.project, sel.key, sel.scope)
	if err == nil {
		return v, nil
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 409 && sel.scope == "" {
		if e := variableAmbiguous(ctx, c, sel); e != nil {
			return nil, e
		}
	}
	return nil, readError(err, variableNotFound(sel.project), requiresRole("maintainer"))
}

// variableAmbiguous lists the scopes a key has (at most ten pages of variables).
// It returns nil when it cannot name two, and the caller reports GitLab's answer.
func variableAmbiguous(ctx context.Context, c *api.Client, sel variableSel) error {
	var cands []aicli.Candidate
	for page := 1; page <= 10; page++ {
		vars, pag, err := c.Variables.ListPage(ctx, sel.project, page, api.MaxPerPage)
		if err != nil {
			return readError(err, projectNotFound(), requiresRole("maintainer"))
		}
		for _, v := range vars {
			if v.Key == sel.key {
				cands = append(cands, aicli.Candidate{Ref: variableRef(sel.project, v.Key, v.EnvironmentScope), Label: v.EnvironmentScope})
			}
		}
		if pag.NextPage == 0 {
			break
		}
	}
	if len(cands) < 2 {
		return nil
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].Ref < cands[j].Ref })
	return aicli.Ambiguous("The key exists in several environment scopes; name one.", cands)
}

// resolve completes a write's variable when the agent named no scope: the
// target key must be canonical (SEC-012), so the one variable GitLab would act on is
// read first. The gate runs it (WriteSpec.ResolveTarget, Q1): at confirm only after
// the token checks, so a repeated confirm is token_used, never this read's
// E_NOT_FOUND (CI-L1). The scope stays out of the binding: the agent did not give
// it, and a variable that changed scope in between is a different target
// (token_mismatch on the target).
func (s *variableSel) resolve(gl *lazyClient) func(ctx context.Context) (aicli.Target, error) {
	return func(ctx context.Context) (aicli.Target, error) {
		if s.scope == "" {
			c, err := gl.get()
			if err != nil {
				return aicli.Target{}, err
			}
			v, err := variableFind(ctx, c, *s)
			if err != nil {
				return aicli.Target{}, err
			}
			s.scope = v.EnvironmentScope
		}
		return aicli.Target{Key: variableRef(s.project, s.key, s.scope), Kind: "variable"}, nil
	}
}

func addVariableTargetFlags(c *cobra.Command) {
	c.Flags().String("key", "", "Variable key (DEPLOY_ENV), KEY:scope (DATABASE_URL:production) or ref (acme/web/-/variables/DEPLOY_ENV)")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the variable is a ref")
	c.Flags().String("env-scope", "", "Environment scope (production, review/*, *); needed when the key exists in several scopes")
}

func addVariableList(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, _ []string) error {
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
		vars, pag, err := client.Variables.ListPage(cmd.Context(), project, req.Page, req.Limit)
		if err != nil {
			return readError(err, projectNotFound(), requiresRole("maintainer"))
		}
		items := make([]map[string]any, 0, len(vars))
		for _, v := range vars {
			items = append(items, variableOut(project, v))
		}
		scope := map[string]any{"project": project,
			"covers": "the project's own CI/CD variables; group and instance variables are not included, and values are never shown"}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, ""))
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List a project's CI/CD variables, without their values",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "variable_page",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{variableEvidence, liveEvidence("variable")},
		Examples: []string{"gitlab-cli variable list --project acme/web"}})
}

func addVariableGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <key>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := variableTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		v, err := variableFind(cmd.Context(), client, sel)
		if err != nil {
			return err
		}
		return app.OK(cmd, variableOut(sel.project, *v))
	}}
	addVariableTargetFlags(c)
	c.Flags().String("filter", "", "Environment scope: env_scope=production")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one CI/CD variable's settings, never its value",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "variable", Positional: "key",
		DeprecatedFlags: map[string]string{"filter": "use --env-scope"},
		ErrorCodes:      gitlabReadCodes,
		Maturity:        aicli.Verified, Evidence: []string{variableEvidence, liveEvidence("variable")},
		Examples: []string{"gitlab-cli variable get DEPLOY_ENV --project acme/web",
			"gitlab-cli variable get acme/web/-/variables/DATABASE_URL:production"}})
}

// variableWant is the requested state of a create or update; nil means not given.
type variableWant struct {
	value                  *string
	vtype, description     *string
	protected, masked, raw *bool
	create                 bool
	key, scope             string // create
}

// readVariableValue reads --value or --value-file (a file input, "-" for stdin): the
// value is a secret (Meta.Secrets), bound as a keyed digest and never shown.
func readVariableValue(app *aicli.App, cmd *cobra.Command) (*string, error) {
	if cmd.Flags().Changed("value-file") {
		b, err := app.ReadFileInput(cmd, "value-file")
		if err != nil {
			return nil, err
		}
		v := string(b)
		return &v, nil
	}
	if cmd.Flags().Changed("value") {
		v, _ := cmd.Flags().GetString("value")
		return &v, nil
	}
	return nil, nil
}

// variableMaskable checks what GitLab requires of a masked value: at least 8
// characters, one line, no spaces. The message never quotes the value.
func variableMaskable(value string) error {
	if len([]rune(value)) >= 8 && !strings.ContainsFunc(value, unicode.IsSpace) {
		return nil
	}
	return aicli.Validation("--value", "at least 8 characters on one line, without spaces",
		"A masked variable's value must be at least 8 characters, on one line and without spaces.")
}

// variableChanges lists the requested fields with their current values when cur is
// known; the value only as a note. Field names are the output's (CLI-059).
func variableChanges(cur *api.Variable, w variableWant) []aicli.FieldChange {
	var before api.Variable
	if cur != nil {
		before = *cur
	}
	var out []aicli.FieldChange
	add := func(field string, b, a any) {
		ch := aicli.FieldChange{Field: field, After: a}
		if cur != nil {
			ch.Before = b
		}
		out = append(out, ch)
	}
	if w.create {
		add("key", nil, w.key)
		add("environment_scope", nil, w.scope)
	}
	if w.vtype != nil {
		add("variable_type", before.VariableType, *w.vtype)
	}
	if w.protected != nil {
		add("protected", before.Protected, *w.protected)
	}
	if w.masked != nil {
		add("masked", before.Masked, *w.masked)
	}
	if w.raw != nil {
		add("raw", before.Raw, *w.raw)
	}
	if w.description != nil {
		add("description", before.Description, *w.description)
	}
	if w.value != nil {
		// *** stands for the value, as in preview.inputs; the read-back shows *** too
		// when GitLab's answer matches the request (variableObserved).
		note := "set; the value is secret and never shown"
		if cur != nil {
			note = "replaced; the value is secret and never shown"
		}
		out = append(out, aicli.FieldChange{Field: "value", After: "***", Note: note})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// variableObserved is the read-back of the requested fields from GitLab's answer.
// The value is compared in memory and shown as *** (a mismatch names it), so the
// receipt says it was checked without quoting it; a hidden variable's value is not
// returned and so not checked.
func variableObserved(v api.Variable, w variableWant) (map[string]any, []string) {
	obs := map[string]any{}
	if w.create {
		obs["key"], obs["environment_scope"] = v.Key, v.EnvironmentScope
	}
	if w.vtype != nil {
		obs["variable_type"] = v.VariableType
	}
	if w.protected != nil {
		obs["protected"] = v.Protected
	}
	if w.masked != nil {
		obs["masked"] = v.Masked
	}
	if w.raw != nil {
		obs["raw"] = v.Raw
	}
	if w.description != nil {
		obs["description"] = v.Description
	}
	var mismatched []string
	if w.value != nil && v.Value != nil {
		obs["value"] = "***"
		if *v.Value != *w.value {
			mismatched = append(mismatched, "value")
		}
	}
	return obs, mismatched
}

func addVariableCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create", RunE: func(cmd *cobra.Command, _ []string) error {
		key, _ := cmd.Flags().GetString("key")
		key = strings.TrimSpace(key)
		if !ciVariableKey.MatchString(key) {
			return aicli.Validation("--key", "letters, digits and _, up to 255 characters", "--key must be letters, digits and _ only.")
		}
		value, err := readVariableValue(app, cmd)
		if err != nil {
			return err
		}
		scope, _ := cmd.Flags().GetString("env-scope")
		if scope = strings.TrimSpace(scope); !variableScopeOK(scope) {
			return aicli.Validation("--env-scope", "an environment name or pattern such as production, review/* or *",
				"--env-scope is not an environment scope.")
		}
		vtype, _ := cmd.Flags().GetString("type")
		protected, _ := cmd.Flags().GetBool("protected")
		masked, _ := cmd.Flags().GetBool("masked")
		raw, _ := cmd.Flags().GetBool("raw")
		want := variableWant{create: true, key: key, scope: scope, value: value, vtype: &vtype, protected: &protected,
			masked: &masked, raw: &raw}
		if cmd.Flags().Changed("description") {
			d, _ := cmd.Flags().GetString("description")
			want.description = &d
		}
		if masked {
			if err := variableMaskable(*value); err != nil {
				return err
			}
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		ref := variableRef(project, key, scope)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: ref, Kind: "variable"},
			ReadBack: "variable get " + shellArg(ref),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				pv := aicli.Preview{Action: fmt.Sprintf("Create CI/CD variable %s in %s", variableName(key, scope), project),
					CanApply: true, Changes: variableChanges(nil, want)}
				var apiErr *api.APIError
				switch _, err := client.Variables.Get(ctx, project, key, scope); {
				case err == nil:
					pv.CanApply = false
					pv.BlockedBy = []aicli.Block{{Kind: "already_exists",
						Message: "The project already has this key in this environment scope; change it with `variable update`."}}
				case errors.As(err, &apiErr) && apiErr.StatusCode == 404:
					// No such variable, or no such project: GitLab answers 404 to both.
					if err := projectExists(ctx, client, project); err != nil {
						return aicli.Preview{}, err
					}
				default:
					return aicli.Preview{}, readError(err, projectNotFound(), requiresRole("maintainer"))
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				opts := &api.VariableCreateOpts{Key: key, Value: *value, VariableType: vtype, Protected: protected, Masked: masked,
					Raw: raw, EnvironmentScope: scope}
				if want.description != nil {
					opts.Description = *want.description
				}
				v, err := client.Variables.Create(ctx, project, opts)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("maintainer"), projectNotFound())
				}
				obs, mismatched := variableObserved(*v, want)
				obs["ref"] = variableRef(project, v.Key, v.EnvironmentScope)
				return aicli.Result{Observed: obs, Mismatched: mismatched, Data: variableOut(project, *v), Untrusted: variableUntrusted}, nil
			},
		})
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("key", "", "Variable key: letters, digits and _")
	_ = c.MarkFlagRequired("key")
	addVariableValueFlags(c)
	c.MarkFlagsOneRequired("value", "value-file")
	c.Flags().String("type", "env_var", "env_var, or file (the value is written to a file whose path the variable holds)")
	c.Flags().Bool("protected", false, "Only pipelines of protected branches and tags get it")
	c.Flags().Bool("masked", false, "Hide the value in job logs (at least 8 characters, one line, no spaces)")
	c.Flags().Bool("raw", false, "Do not expand $references in the value")
	c.Flags().String("env-scope", "*", "Environment scope: * (every environment), production, review/*")
	c.Flags().String("description", "", "Description")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same create with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create a project CI/CD variable (configure: dangerous; the value is never shown)",
		Aliases: []string{"add", "new", "set"}, Effects: []aicli.Effect{aicli.Configure}, Output: "variable",
		Secrets: []string{"value"}, FileInputs: map[string]string{"value-file": "value"},
		Params:     map[string]aicli.ParamDoc{"type": {Enum: variableTypes}},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{variableEvidence, liveEvidence("variable")},
		Examples: []string{"gitlab-cli variable create --project acme/web --key DEPLOY_TOKEN --value-file token.txt --masked --protected --dangerous --dry-run",
			"gitlab-cli variable create --project acme/web --key DEPLOY_TOKEN --value-file token.txt --masked --protected --dangerous --confirm <confirm_token>"}})
}

// addVariableValueFlags declares --value and --value-file: the value as a secret
// flag, or read from a file or stdin so it stays out of the command line.
func addVariableValueFlags(c *cobra.Command) {
	c.Flags().String("value", "", "The value (secret: never shown or stored); --value-file keeps it off the command line")
	c.Flags().String("value-file", "", "Read the value from this file, its exact content; - reads stdin")
}

// readVariableUpdate reads the changes of variable update; at least one is given.
func readVariableUpdate(app *aicli.App, cmd *cobra.Command) (variableWant, error) {
	var w variableWant
	value, err := readVariableValue(app, cmd)
	if err != nil {
		return w, err
	}
	w.value = value
	if cmd.Flags().Changed("type") {
		t, _ := cmd.Flags().GetString("type")
		w.vtype = &t
	}
	if cmd.Flags().Changed("description") {
		d, _ := cmd.Flags().GetString("description")
		w.description = &d
	}
	for name, dst := range map[string]**bool{"protected": &w.protected, "masked": &w.masked, "raw": &w.raw} {
		if cmd.Flags().Changed(name) {
			s, _ := cmd.Flags().GetString(name)
			b := s == "true"
			*dst = &b
		}
	}
	return w, nil
}

func addVariableUpdate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "update <key>", RunE: func(cmd *cobra.Command, _ []string) error {
		want, err := readVariableUpdate(app, cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, err := variableTarget(cmd, gl)
		if err != nil {
			return err
		}
		return app.Write(cmd, aicli.WriteSpec{
			ResolveTarget: sel.resolve(gl),
			ReadBack:      "variable get " + shellArg(variableRef(sel.project, sel.key, sel.scope)),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				cur, err := client.Variables.Get(ctx, sel.project, sel.key, sel.scope)
				if err != nil {
					return aicli.Preview{}, readError(err, variableNotFound(sel.project), requiresRole("maintainer"))
				}
				// A value that will be masked must be maskable: the new one, or the current
				// one when only --masked true is given (checked in memory, never shown).
				if (want.masked != nil && *want.masked) || (want.masked == nil && cur.Masked && want.value != nil) {
					v := want.value
					if v == nil {
						v = cur.Value
					}
					if v != nil {
						if err := variableMaskable(*v); err != nil {
							return aicli.Preview{}, err
						}
					}
				}
				return aicli.Preview{Action: fmt.Sprintf("Update CI/CD variable %s in %s", sel.name(), sel.project), CanApply: true,
					Changes: variableChanges(cur, want), Untrusted: []string{"changes[].before"}}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				v, err := client.Variables.Update(ctx, sel.project, sel.key, sel.scope, &api.VariableUpdateOpts{Value: want.value,
					VariableType: want.vtype, Protected: want.protected, Masked: want.masked, Raw: want.raw, Description: want.description})
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("maintainer"), variableNotFound(sel.project))
				}
				obs, mismatched := variableObserved(*v, want)
				return aicli.Result{Observed: obs, Mismatched: mismatched, Data: variableOut(sel.project, *v), Untrusted: variableUntrusted}, nil
			},
		})
	}}
	addVariableTargetFlags(c)
	addVariableValueFlags(c)
	c.Flags().String("type", "", "New type: env_var or file")
	c.Flags().String("protected", "", "true or false: only pipelines of protected branches and tags get it")
	c.Flags().String("masked", "", "true or false: hide the value in job logs")
	c.Flags().String("raw", "", "true or false: do not expand $references in the value")
	c.Flags().String("description", "", "New description (an empty value clears it)")
	// An update with nothing to change is a usage error before the gate (CLI-029).
	c.MarkFlagsOneRequired("value", "value-file", "type", "protected", "masked", "raw", "description")
	parent.AddCommand(c)
	bools := []string{"true", "false"}
	app.Register(c, aicli.Meta{Summary: "Change a CI/CD variable's value or settings (configure: dangerous; the value is never shown)",
		Aliases: []string{"edit"}, Effects: []aicli.Effect{aicli.Configure}, Output: "variable", Positional: "key",
		Secrets: []string{"value"}, FileInputs: map[string]string{"value-file": "value"},
		Params: map[string]aicli.ParamDoc{"type": {Enum: variableTypes}, "protected": {Enum: bools}, "masked": {Enum: bools},
			"raw": {Enum: bools}},
		ErrorCodes: gitlabWriteCodes,
		Maturity:   aicli.Verified, Evidence: []string{variableEvidence, liveEvidence("variable")},
		Examples: []string{"gitlab-cli variable update DEPLOY_ENV --project acme/web --value staging --dangerous --dry-run",
			"gitlab-cli variable update DEPLOY_ENV --project acme/web --value staging --dangerous --confirm <confirm_token>"}})
}

func addVariableDelete(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "delete <key>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := variableTarget(cmd, gl)
		if err != nil {
			return err
		}
		return app.Write(cmd, aicli.WriteSpec{
			ResolveTarget: sel.resolve(gl),
			ReadBack:      "variable get " + shellArg(variableRef(sel.project, sel.key, sel.scope)),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				if _, err := client.Variables.Get(ctx, sel.project, sel.key, sel.scope); err != nil {
					return aicli.Preview{}, readError(err, variableNotFound(sel.project), requiresRole("maintainer"))
				}
				return aicli.Preview{Action: fmt.Sprintf("Delete CI/CD variable %s from %s permanently", sel.name(), sel.project),
					CanApply: true, Changes: []aicli.FieldChange{{Field: "variable", Before: sel.name(),
						Note: "deleted with its value; pipelines no longer get it; cannot be undone"}}}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				if err := client.Variables.Delete(ctx, sel.project, sel.key, sel.scope); err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("maintainer"), variableNotFound(sel.project))
				}
				// Read back: the variable must be gone.
				var apiErr *api.APIError
				switch _, err := client.Variables.Get(ctx, sel.project, sel.key, sel.scope); {
				case errors.As(err, &apiErr) && apiErr.StatusCode == 404:
					return aicli.Result{Outcome: aicli.Outcome{Verification: "passed"}, Observed: map[string]any{"exists": false},
						Data: map[string]any{"ref": variableRef(sel.project, sel.key, sel.scope), "deleted": true}}, nil
				case err == nil:
					return aicli.Result{Outcome: aicli.Outcome{Verification: "failed"}, Observed: map[string]any{"exists": true},
						Mismatched: []string{"exists"}, Data: map[string]any{"ref": variableRef(sel.project, sel.key, sel.scope), "deleted": false}}, nil
				}
				return aicli.Result{Data: map[string]any{"ref": variableRef(sel.project, sel.key, sel.scope), "deleted": true}}, nil
			},
		})
	}}
	addVariableTargetFlags(c)
	c.Flags().String("filter", "", "Environment scope: env_scope=production")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Delete a CI/CD variable and its value permanently (configure: dangerous)",
		Aliases: []string{"rm", "remove"}, Effects: []aicli.Effect{aicli.Configure}, Output: "variable_deleted", Positional: "key",
		DeprecatedFlags: map[string]string{"filter": "use --env-scope"},
		ErrorCodes:      gitlabWriteCodes,
		Maturity:        aicli.Verified, Evidence: []string{variableEvidence, liveEvidence("variable")},
		Examples: []string{"gitlab-cli variable delete OLD_TOKEN --project acme/web --dangerous --dry-run",
			"gitlab-cli variable delete OLD_TOKEN --project acme/web --dangerous --confirm <confirm_token>"}})
}
