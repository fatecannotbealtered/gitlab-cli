# Migration findings: where MIGRATION-2.0.md and aicli fell short

The gitlab-cli pilot is the evidence that the migration guide and the shared library
work on a real tool. Every place the guide (`MIGRATION-2.0.md`, `template/INSTANTIATE.md`)
was wrong, unclear or incomplete, and every library bug, limitation or awkward API, is
recorded here with what happened and what the pilot did instead. Vendored files were
never edited (REPO-063): each workaround lives in tool code.

Severity: **high**: a tool that follows the guide fails conformance or ships a defect;
**medium**: every tool pays for a workaround; **low**: clarity or polish.

Group agents: add your findings under your group's heading at the end, in the same
form, and leave the other sections alone.

## Foundation: skeleton, auth, label, milestone

### Library (internal/aicli)

**L1 (high) R01 refuses the same verb alias on two groups.** `validate.go`
`validateTree` collects every command's bare aliases (`add`, `view`, `ls`) and
reports any word used twice, so `label create` and `milestone create` both aliased
`add` fail R01 ("alias 'add' collides with a command path or another alias"),
although `label add` and `milestone add` are distinct paths. Guide step 2 says
"Meta.Aliases accepts" `view`/`show`; with more than one noun group that is
impossible. Pilot: no leaf `Meta.Aliases`; aicli's default verb synonyms answer
`view`, `ls`, `add`, `rm` with `did_you_mean` (CLI-022 allows "accepted, or answered").
Fix: qualify aliases with the parent path before the collision check.

*Resolved by library fixes (P1).* Aliases are checked per parent; label and milestone
have their leaf aliases back (`ls`, `view`/`show`, `add`/`new`, `edit`, `rm`/`remove`),
R01 passes and `TestAgentGuesses` runs them.

**L2 (high) An integer ID flag makes the dry-run envelope invalid.** `binding.go`
`displayFlag` shows an int flag as a JSON number in `preview.inputs`, while
`ValidateEnvelope` requires keys named `id`, `iid` or `*_id` to be strings (CLI-007).
`label update --label-id 5 --dry-run` failed R04-style validation. Guide step 3
recommends "declare --iid and name it in Positional"; as an Int flag it breaks every
MR and issue write. Pilot: `--label-id` and `--milestone-id` are string flags parsed by
the tool; the notes tells the groups to do the same. Fix: render `*_id`/`iid` inputs
as strings, or say so in the guide.

*Resolved by library fixes (P2).* Checked with an `Int` positional `--iid` and an `Int`
`--note-id`: `preview.inputs` shows `"5"` and `"7"`, other ints stay numbers, and the
envelope is valid. gitlab-cli keeps its string flags; the notes says either works.

**L3 (medium) The changelog built-in breaks the success budget.** Without `--since`
it returns every entry; gitlab-cli's 1.x history (about 31,000 characters) made R05's
own `changelog` call fail R04 (over 8,000, CLI-042). Pilot: `App.Changelog` gets only
the newest sections that fit 5,000 characters (`recentChangelog` in `cmd/app.go`).
Fix: cap the entries (truncated with a continuation) or default to recent versions.

*Resolved by library fixes (P3).* `recentChangelog` is gone; `changelog` now returns
the 5 newest entries with `truncated: true` and `omitted: 16` (6.5 KB envelope).

**L4 (medium) A development build fails conformance.** INSTANTIATE step 5 sets
`var version = "dev"`, but R01 (`reference.version`), R05 (`changelog.current_version`)
and update's error details require a semantic version, and `validateChangelog` refuses
entries newer than the running version, so even `0.0.0-dev` fails with a real
CHANGELOG. Every `go test` run (no ldflags) failed. Pilot: an unknown version becomes
the next patch of the newest changelog release plus `-dev` (`1.2.18-dev`,
`buildVersion` in `cmd/app.go`). Fix: say this in INSTANTIATE and MIGRATION, or let
the checks accept a development marker.

*Resolved by library fixes (P14).* `buildVersion` became `appVersion` (ldflags, else
the `go install` module version, else `dev`); a test build reports `1.2.18-dev`. A
development build's `update` is now `E_UNSUPPORTED` at `discover`, so the
`--target-version` check in `TestUpdateIsTheBuiltIn` sets a release version first.

**L5 (medium) An idempotent replay is unreachable when Preview blocks duplicates.**
The gate runs Preview before it looks up `--idempotency-key`; a create whose Preview
says `already_exists` (GitLab refuses duplicate label names and milestone titles) gets
no token on the repeat, so CLI-062's replay never happens. Pilot: with a key, Preview
notes the existing resource instead of blocking. Fix: check the key before Preview, or
issue the token when a replayable record exists.

*Resolved by library fixes (P4).* The key special case is gone from both create
Previews, which block duplicates again; a repeat with the same key is replayed at the
dry-run (`TestLabelCreateRecordsTheNewRef`).

**L6 (medium) Scope boundaries leave out GitLab's own sigils.** `subtreeMatch` treats
only `/ ! # : @` as boundaries, so a grant for `acme/web` cannot cover `acme/web~bug`
(label) or `acme/web%3` (milestone), nor `&` (epic), `$` (snippet) or `*` (iteration).
The guide names refs for MRs and issues only. Pilot: path-style refs that mirror web
paths, `acme/web/-/labels/bug` and `acme/web/-/milestones/3`; GitLab's `~`/`%` forms
are still accepted as input. Fix: name the convention in the guide, or let a tool
declare extra boundaries.

*Resolved in the guide, not the library.* MIGRATION-2.0 now names the path-style key
convention for kinds without a boundary sigil. The pilot's keys stay as they are.

**L7 (medium) No API to find, read or write the settings file.** CLI-101 has `auth
login` write the host to the active profile, but aicli exports neither the settings
path (`<PREFIX>_SETTINGS_FILE` or `<state_dir>/settings.json`), nor the active profile
name, nor a writer. Pilot: `cmd/settingsfile.go` re-derives the state directory, the
path, the profile order and a strict-enough parser, and writes atomically; it must be
kept equal to `settings.go` by hand. Fix: export `App.SettingsPath`, `App.Profile` and
a `SetProfileValue` that validates like `parseSettings`.

*Resolved by library fixes (P5, D3).* `cmd/settingsfile.go` is deleted; auth uses
`App.Profile`, `App.SettingsPath` and `App.SetProfileSettings`. Login now writes the
keyring first and the settings second, and puts the keyring entry back if settings.json
cannot be written (tested). What it can no longer do is make the first profile the
default: see N2.

**L8 (medium) An undefined `--profile` fails every setting, so login cannot create a
profile.** `App.Setting` returns `E_CONFIG` for every setting while `--profile` names a
profile settings.json lacks, even for values set in the environment. `auth login
--profile lab --host ...` (CLI-101's way to add a profile) then failed twice: reading
its own timeout, and in the gate's call to `App.Identity`. Pilot:
`undefinedRequestedProfile` lets Identity report no account and the client take the
timeout from the environment in that case. Fix: let `Setting` fall back to flag,
environment and default for an undefined profile, or document the pattern.

*Resolved by library fixes (P6).* `auth login` declares `Meta.NewProfile`;
`undefinedRequestedProfile` is gone. `auth logout` declares it too (N4).

**L9 (medium) G04 needs an upstream write for configure-only local commands.** G04
requires at least one request to the fake upstream for any `change`, `deliver` or
`configure` command, but `auth login` and `auth logout` change the tool's own keyring
entry and settings file (CLI-025 classes them `configure`) and send nothing upstream.
Pilot: the harness's `Writes` counts the test keyring's writes too. Fix: accept a
receipt with `change: applied` for such commands, as G04 accepts `persistence: saved`
for artifact commands.

*Resolved by library fixes (P7).* The harness still counts the test keyring's writes,
which keeps "a refusal writes nothing" strict for login and logout; G04 no longer
needs it.

**L10 (medium) G03's added `--profile` breaks the guide's client order.** G03 adds one
unused optional flag; `--profile` is not in `isGlobalFlag`, so it is added whenever it
sorts first (`auth logout`'s sample). Building the client before `App.Write`, as guide
step 4's example does (`client, err := newClient(app)`), reads settings, fails with
`E_CONFIG` (exit 4) for the unknown profile, and G03 reports an error (it tolerates
only exit 2 or 3). Verified by moving `newClient` before the gate in `auth logout`.
Pilot: clients are built lazily inside Preview and Apply (`gitlabFor`). Fix: have G03
skip `--profile`, or change the guide's example.

*Resolved by library fixes (P8)* and in the guide (step 4 builds the client inside
Preview and Apply). Unknown profiles now stop every tool command up front (D4), so
where the client is built no longer changes how they fail. The lazy clients stay.

**L11 (low) Identity cannot describe or bind an environment token.** context reports
`credentials.configured` as `Identity.Account != ""`, and Identity must not call the
network (CLI-035), so a `GITLAB_CLI_TOKEN` user needs an invented account
(`env:GITLAB_CLI_TOKEN`), and swapping the token between dry-run and confirm passes
the account and host checks. Pilot: keyring entries keep the username `auth login`
verified; the env case uses the label. Fix: an Identity field for a credential bound
as an HMAC under the confirm key.

*Partly resolved by library fixes (B1).* The token from `GITLAB_CLI_TOKEN` is bound as
`setting.token` (a keyed digest) and the profile as `profile`, so a swapped token or
profile at confirm is `token_mismatch` (`TestConfirmIsBoundToTheCredentials`). context
still needs the invented account `env:GITLAB_CLI_TOKEN` for `credentials.configured`.

**L12 (low) Debugf cannot redact a secret it did not see.** It masks secret flags and
secret settings from flag, environment or default, but not a token read from the
keyring. Pilot: the client logs method, path, status and time only. Fix: an
`App.RedactValue(secret)`.

*Resolved by library fixes (D2).* `readCredentialFor` registers every keyring token
with `app.AddSecret`.

**L13 (low) The CLI-110 transport is not exported.** aicli builds proxy, CA file and
User-Agent handling for its own requests (`update_release.go`) but keeps it private,
so each tool re-implements it. Pilot: `gitlabTransport` in `cmd/client.go`. Fix:
export `App.HTTPTransport()`.

*Resolved by library fixes (P10), with a caveat.* `gitlabTransport` is gone; `clientFor`
uses `app.HTTPTransport()` and `app.UserAgent()`, but replaces the transport's proxy,
which reads the process environment instead of `App.Env` (N1).

**L14 (low) The corrected command line after an unknown verb is wrong when flags carry
values.** `unknownCommand` classifies arguments against the group, so `--project`'s
value counts as a positional: `label view bug --project acme/web` gets "does not take
those positional arguments; run --help" instead of `gitlab-cli label get bug --project
acme/web`. `did_you_mean` is right. Pilot: tested as is.

*Resolved by library fixes (P11).* `label gte bug --project acme/web` now suggests
`gitlab-cli label get bug --project acme/web` (`TestAgentGuesses`).

**L15 (low) `--fields` is validated but ignored on writes.** It is checked against the
output schema, but the gate's `data.result` is never projected (`label update ...
--confirm <t> --fields ref` returns all nine fields). Fix: project `result`, or refuse
`--fields` on writes.

*Resolved by library fixes (P12).* `label update ... --confirm <t> --fields ref,color`
returns those two fields in `data.result`; the receipt is untouched.

**L16 (low) `context --fields` refuses keys context returns.** The `aicli_context`
schema lists only the required keys, so `--fields profile` (added by aicli itself when
settings are declared) and any `ContextExtra` key are `E_USAGE`. ContextExtra also
cannot declare untrusted paths. Pilot: `git_remote` carries only a validated host and
path.

*Resolved by library fixes (P13).* `context --fields profile,git_remote` works.
ContextExtra still cannot declare untrusted paths.

**L17 (low) G03 rarely exercises the binding of typed values.** `mutateValue` appends
`-mutated`, which turns a colour or a date into an invalid value that the tool rejects
before the gate, so the suite only logs "binding not exercised" for `--color`,
`--due-date` and the like. Fix: mutate within the type when the value has a recognisable
shape (dates, hex colours).

*Resolved by library fixes (P9).* The suite no longer logs "binding not exercised" for
`--color` or `--due-date`; what remains are dropped required flags and invalid added
ones, which the tool must refuse.

### Guide (MIGRATION-2.0.md, INSTANTIATE.md)

**G1 (medium) Step 5 names code gitlab-cli does not have.** `cmd/agent_ux.go`,
`bindInvocation`, `controlFlags` and `GITLAB_CLI_LEGACY_CONFIRM_BINDING` exist in no
branch of gitlab-cli 1.2.17. The confirm code was `confirm.go`, `confirm_secret.go`,
`confirm_consumed.go`; `prepareWrite`, `dryRunOutput`, `prepareBatchWrite` lived in
`root.go` and `batch.go`; the agent-safe switches in `agent.go`.

*Resolved in the guide.* Step 5 now lists gitlab-cli's own confirm files and helpers.

**G2 (low) Step 1 says keep `contract-ext.json`.** gitlab-cli's only code,
`E_CANCELLED`, came from interactive confirmation prompts, which 2.0 does not have, so
it can no longer occur (CLI-026). Pilot: deleted the file (REPO-SPEC makes it optional).
The guide should say to drop codes 2.0 cannot raise.

*Resolved in the guide.* Step 1 says to drop codes 2.0 makes impossible, and the file
when none is left.

**G3 (low) Step 2's App example does not survive a staged migration.** `Shortcuts` to
`repo branch` and `project members` fail the tree check while those groups are not
registered; `VerbSynonyms` for `view`/`show` repeats aicli's defaults; the `Meta.Aliases`
advice runs into L1. Pilot: shortcuts are added by the group that registers the target.

*Resolved in the guide and the library.* The guide's "Staged migration" note adds each
shortcut, alias and synonym with its group; leaf aliases work again (P1).

**G4 (medium) Step 4 and CLI-062: forwarding the idempotency key can re-send a
write.** Go's `net/http` Transport treats a request with an `Idempotency-Key` header as
replayable and re-sends it after a failure on a reused keep-alive connection, which is
exactly the hidden write retry CLI-110 forbids, unless the upstream deduplicates.
GitLab's REST API does not honour the header; 1.x sent it anyway. Pilot: the header is
never sent; the gate's own replay covers repeats. The guide should warn about it.

*Resolved in the guide.* Step 4 warns against the header for an API that ignores it.

**G5 (medium) Step 9 assumes a release layout and module path gitlab-cli lacks.** "Keep
the release layout" does not fit: 1.2.17's `.goreleaser.yml` shipped the binary only
(`files: none*`); pilot switched to the template's `skills/**/*`. And a v2.0.0 Go module
whose path has no `/v2` cannot be `go install`ed at `@v2.x.y`, so
`UpdateConfig.GoPackage` cannot work and `go install` users are stranded. Pilot: no
`GoPackage`; the module path decision is left to the release phase.

*Resolved in the guide.* Step 9 describes the release layout and the `/v2` choice
(add the suffix, or leave `GoPackage` empty). The decision is still the release phase's.

**G6 (low) Step 10 leaves out CLI-110's numbers.** It says "retry reads only" but not
at most twice, only after connection failures and 429/502/503/504, `Retry-After` up to
10 s, nor the User-Agent format. 1.x retried 500 too, three times, waited up to 60 s,
and had `GITLAB_CLI_USER_AGENT`/`GITLAB_CLI_MAX_RETRIES` overrides. Pilot: implemented
CLI-110 as written; a request timeout above 30 s is refused (`GITLAB_CLI_TIMEOUT` may
lower it).

*Resolved in the guide.* Step 10 gives CLI-110's numbers and points at
`app.HTTPTransport()` and `app.UserAgent()`.

**G7 (medium) Step 10 "move the 1.x values" is not possible for the token.** 1.x kept
the token AES-encrypted in `config.json`/`profiles.json` with a master key in the OS
keyring, so users cannot move it by hand; they need the original token or a new one.
1.x also read glab's `GITLAB_HOST`/`GITLAB_TOKEN`, which `App.Settings` cannot express
(one variable per setting), and had `auth profile list|use|remove`. Pilot: dropped
both (a glab token is not sent to this tool's host by accident) and wrote
`docs/UPGRADE-2.0.md`; no importer.

*Partly resolved in the guide.* Step 10 now says 1.x users sign in again. 1.x's
`auth profile use` has no 2.0 equivalent, and with no API to set `default_profile`
(N2) choosing the default profile is a hand edit of settings.json.

**G8 (low) `auth login` under the gate is heavy for a person.** As `configure` it is
dangerous and scope-gated, so a person must write a scope file granting `configure`
and `dangerous` before the first sign-in, then run a dry-run and a confirm (with
`--token-file -`, pasting the token twice). The guide mentions `GITLAB_CLI_TOKEN` as
the way around it; the README has to explain both. Pilot: documented in
`docs/UPGRADE-2.0.md`.

*Acknowledged in the guide.* Step 10 says the release notes must explain the first
login's scope; the two-step login itself is unchanged.

**G9 (low) Step 6 and staged migrations.** (a) "point GITLAB_CLI_COVERAGE_FILE at a
temporary file" only works through `os.Setenv` when the App falls back to the process
environment; with a hermetic `App.Env` (AGENT.md's advice) the tests pass it in the
injected environment. (b) `go mod tidy` reads files of every build tag except `ignore`,
so excluding unmigrated files with a tag still needs the packages they import. Pilot:
empty placeholder packages `internal/output` and `internal/config` until the groups and
e2e are migrated.

*Resolved in the guide.* Step 6 passes the coverage file through `App.Env` too, and
the staged-migration note says `go mod tidy` reads tagged files.

### Round 3: the fixed library (resynced 2026-10-14)

**N1 (medium) `App.HTTPTransport` takes the proxy from the process environment.**
`transport.go` sets `tr.Proxy = http.ProxyFromEnvironment`, which reads `os.Getenv`
(once per process, then cached), while the CA file comes through `App.Env`. With a
hermetic `App.Env` (AGENT.md's advice, and how tests and the conformance harness
inject variables), the `HTTPS_PROXY` and `NO_PROXY` given to the App are ignored and
the process's apply; two Apps in one process cannot differ. `update` uses the same
transport. Pilot: `clientFor` replaces `tr.Proxy` with `proxyFromAppEnv`
(`golang.org/x/net/http/httpproxy` over `App.Env`). Fix: build the proxy function from
`a.env`.

**N2 (medium) Nothing sets `default_profile`, so a second login leaves no profile
active.** `SetProfileSettings` keeps `default_profile` (P5) and no other API writes it.
gitlab-cli's first login used to make its profile the default. Now, after `auth login`
(profile `default`) and `auth login --profile work`, settings.json has two profiles
and no default, so every later command without `--profile` has no active profile:
`App.Setting` falls back to the defaults (`https://gitlab.com`), and the tool can tell
this apart from "no settings file" only by checking `App.SettingsPath()` on disk.
(`App.Profile` also returns no profile for an invalid file, so a tool that keys
keyring entries by it must call `App.Setting` first to surface the `E_CONFIG`.)
Pilot: a settings file with no active profile is `E_USAGE` on `--profile`, whose
next_action asks the user to set `default_profile`; `docs/UPGRADE-2.0.md` says so.
Fix: let `SetProfileSettings` set `default_profile` when the file has none (or add
`SetDefaultProfile`), and expose the profile names and whether the file exists, as
`context.profile` reports them.

**N3 (low) D4's `E_CONFIG` is missing from commands' `error_codes`.** Any tool command
can now fail up front with `E_CONFIG` (unknown profile), but `errorCodesOf` adds it
to none, so a command that declares its own list without it (`auth logout` declared
`E_IO`, `E_NOT_FOUND`) shows a reference entry without the code. Pilot: logout
declares `E_CONFIG`. Fix: add `E_CONFIG` to every tool command when `App.Settings` is
declared.

**N4 (low) `Meta.NewProfile` is also what a cleanup command needs.** `auth logout
--profile gone`, for a profile deleted from settings.json whose keyring entry is
left, is stopped by D4 unless it declares `Meta.NewProfile`, whose name and docs
speak only of creating a profile. Pilot: logout declares it. Fix: document this use,
or name the field for what it does (`AnyProfile`).

## MR group

24 commands (17 writes) migrated: `mr list|get|current|diff|create|update|merge|close|
reopen|approve|unapprove`, `mr comment list|add|delete`, `mr discussion list|get|reply|
create|resolve|unresolve`, `mr bulk merge|approve|close|update`.

### Library (internal/aicli)

**MR-L1 (medium) `reference --command` cannot fit a write with many flags in 4,000
characters.** The entry lists the error codes twice (the command's `error_codes` and
the `error_codes` table, about 1,000 characters for a write's 17 codes) and the whole
output schema (about 700 for the 22-field `merge_request` with its default fields),
which leaves about 2,300 for the parameters. `mr create` (15 flags, one-line help)
was 4,552 characters and failed R01; `mr list` was 3,995. Pilot: shorter flag help, and
the writes' results got their own schema, `merge_request_result`, without default
fields (MR-L4); `mr create` is now 3,876. Fix: list the codes once, or leave the
schema's `default_fields` out of a write's entry, or give a write's entry more room.

**MR-L2 (medium) Nothing helps a tool keep its own data within the success budget
(CLI-042).** aicli cuts its own batch results and operation lists to 8,000 characters,
never the tool's data. An MR list item with the eval's nine default fields is 330 to
400 characters, so the default 20 items reach 6,500 to 8,000, and long titles break the
MUST. A page cannot end early with a `next_offset`, because GitLab pages by number and
`readPage` takes offsets in steps of `--limit`; and an `mr update --description`
preview carries the old and the new text, of any length, while shortening `after`
would break its comparison with `observed`. Pilot: descriptions are out of `mr get`'s
defaults and out of write results; lists and previews are left as they are. Fix: a
`Page` option that ends a page within the budget with a `continuation_reason`, and
guidance for long text in previews (a digest and a length, compared alike).

**MR-L3 (low) No API says whether the response will carry a field.** `pipeline_status`
costs a GitLab read per merge request, so `mr list` reads it only when the output will
show it: by default, with `+pipeline_status`, `pipeline_status` or `all`. The shared
`wantsField` answers explicit asks only; the default fields and the `+` form are
aicli's rules. Pilot: `mrWantsField` repeats `parseFields` and `project`. Fix:
`App.WantsField(cmd, name)`.

**MR-L4 (low) A confirmed write's `data.result` ignores the schema's default fields.**
`App.OK` and `App.List` cut to `DefaultFields` when `--fields` is absent;
`projectResult` returns the whole result unless `--fields` is given. The guide's step 3
`merge_request` schema leaves the description out of its defaults and is the Output of
`mr update`, whose confirm would return the description anyway. Pilot: write results
are built without the description and the diff refs, under `merge_request_result`.
Fix: apply the defaults to results too, or say that a write's Output is its full result.

**MR-L5 (low) `reference --command` shows the schema's `default_fields`, not
`Meta.DefaultFields`.** `referenceCommand` takes the schema from `schemaEntries()`,
while help's `Fields:` line and the projection use `Meta.DefaultFields` when set, so a
command that overrides them is described wrongly. Pilot: not used; the MR defaults
live in the schemas. Fix: put the command's effective defaults in its entry.

**MR-L6 (low) A batch preview's action is its first item's.** `planBatch` takes
`plans[0]`'s `Action` and appends " (N targets)", so an item action that names its
target ("Close merge request acme/web!5") reads "Close merge request acme/web!5 (2
targets)". Pilot: plan actions name no target ("Close the merge request");
`preview.targets` names them. Fix: say so on `BatchItem`, or take `BatchSpec.Action`.

**MR-L7 (low) `meta.untrusted` names fields the projection removed.** It lists every
untrusted field of the schema: each default `mr list` carries eight paths (about 250
characters), four for fields not in the response. Fix: list the paths that remain.

**MR-L8 (low) A renamed flag needs glue in every tool.** `DeprecatedFlags` only marks a
flag; `FlagAliases` accepts an old name for the whole App, without a notice. 1.x's `mr
list --search` (now `--query`, CLI-028) needs its own flag, a copy into `--query`, a
refusal when both are given. Pilot: `mrDeprecatedSearch`, a `PreRunE`. Fix:
`Meta.RenamedFlags` (old to new), bound as the new name, with the notice.

**MR-L9 (low) `SetResolved`'s doc says "only call it for flags the user gave".** A value
that came inside another input needs it for a flag the user did not give: the project
in `acme/web!5` (`projectFlag` already does this), the note ID in
`acme/web!5#note_12`, the thread ID in `#discussion_<id>`, the path in
`acme/web!5:src/app.go`; otherwise `acme/web!5#note_12` and `5 --project acme/web
--note-id 12` bind differently. Pilot: called for those flags; it works. Fix: document
the case.

### Guide (MIGRATION-2.0.md, notes.md)

**MR-G1 (medium) R02 wants a root shortcut for every nested group, but `comment` and
`bulk` are shared.** R02 failed for `mr comment`, `mr discussion` and `mr bulk` ("add
App.Shortcuts["comment"]"). Discussions are the merge requests' only, but `comment` is
nested under `issue` too and `bulk` under `issue` and `variable`: a root `comment`
serves one of them, and the groups' register functions would overwrite each other's
`app.Shortcuts["bulk"]` at integration. CLI-022 speaks of nouns a user names at the
root (`branch`); these are sub-resources. Pilot: `discussion` for `mr discussion`, and
the group's own keys `mr-comment` and `mr-bulk`, which satisfy R02 but which nobody
types. Fix: let R02 skip a group nested in a root noun whose name another root noun
also nests, or define root `comment` and `bulk` that take any ref (the sigil tells
`!5` from `#12`). Integration request: decide it once with the Issue and CI groups.

**MR-G2 (medium) No ref form for sub-resources of a merge request.** the notes and
step 4 give refs and keys for top-level kinds; comments, threads and changed files
have none. Pilot: GitLab's URL anchors after the MR's ref, `acme/web!5#note_12` and
`acme/web!5#discussion_<id>` (`#` is a scope boundary, so a grant for `acme/web!5`
covers them), and git's `<rev>:<path>` form, `acme/web!5:src/app.go`. Commands take
these refs, the web URL with its `#note_` anchor, or the MR with `--note-id`,
`--discussion-id` or `--path`. Integration request: issue comments need the same rule.

**MR-G3 (medium) Text can carry effects: GitLab quick actions.** GitLab runs a line such
as `/merge`, `/close` or `/assign @x` in a comment, a reply or a description as a
command, so `mr comment add` (`change`, not dangerous) can merge a merge request, and a
line copied from untrusted content can act (SEC-002). CLI-025 and the guide treat a
command's effects as fixed. Pilot: the previews of comments, replies, inline threads
and descriptions are blocked (`quick_action`) when a line outside a code block starts
with a slash command. Fix: name content-borne effects in CLI-025 and step 3; the Issue
group needs the same check.

**MR-G4 (medium) Step 3's "declare `--iid` and name it in Positional" does not work for
refs.** aicli sets the positional into its flag before the command runs, so an `Int`
`--iid` refuses `acme/web!5` and the URL (CLI-048). And a string `--mr` bound to the
canonical ref (`acme/web!5`) fails G03: changing `--project` changes both `mr` and
`project`, the gate names the first in sorted order (`--mr`), and G03 wanted
`--project`. Pilot: a string flag `mr` bound to the iid alone (as label binds the name
and milestone the iid); the project binds on its own. Fix: say both in step 3.

**MR-G5 (low) Step 4's example contradicts its text.** It builds the client before
`App.Write` (`client, err := newClient(app)`), which the paragraph after it (and L10's
resolution) rules out, and sets `Mismatched` and `Verification` by hand, which the
gate derives from `Observed` against the preview's changes. Pilot: as label and
milestone (`gitlabFor`; `Observed` only).

**MR-G6 (low) the notes' behaviour matrix assumes one receipt.** `runMatrix`'s
success, auth, gate and apply-failure rows read `data.receipt`; a batch has a receipt
per item and a summary, so all four fail for `mr bulk`. Pilot: `mrBatchRows`
(`cmd/mr_bulk_test.go`) replaces them through `matrixCase.Rows`, checking each item's
receipt, the summary and a dangerous batch's stop. Integration request: move it into
`matrix_test.go` for the Issue and CI bulk commands.

**MR-G7 (low) The eval's `pipeline_status` default costs a read per merge request.**
GitLab's MR list has no pipeline status (a single GET's `head_pipeline` has), so the
default `mr list` is 1 + N requests and `--pipeline-status` must scan; the guide cites
the eval's savings (13.6 to 6.1 calls), not this cost. Pilot: pipelines are read four
at a time and only when shown (MR-L3); `--pipeline-status` checks the 100 most recently
updated matching merge requests, says so in `scope`, and is `truncated` when more
match.

## Issue group

17 commands (14 writes): `issue list|get|create|update|close|reopen|assign|label`,
`issue comment list|create|delete`, `issue bulk close|reopen|update|label|assign|comment`.

### Library (internal/aicli)

**ISSUE-L1 (medium) R02 asks a root shortcut for every nested group, but a shortcut
word maps to one path.** `checkTree` reports `issue bulk` and `issue comment` as "not
reachable from the root; add App.Shortcuts["bulk"]" (and `["comment"]`). Those words
also name `mr bulk` and `mr comment`, so two groups that follow the hint assign the
same map key in their register functions; the last group registered wins, nothing
reports it, and each worktree passes on its own. `bulk` is also no noun a user would
name at the root (CLI-022 asks shortcuts for nouns). Pilot: keys that name the parent,
`issue-comment` and `issue-bulk`, which pass R02 but which no agent will type. Fix: let
a shortcut word map to several paths and answer E_AMBIGUOUS with candidates (CLI-048),
let a group opt out of R02 when its name is no root noun, and have the tree check
refuse a Shortcuts key assigned twice.

**ISSUE-L2 (low) A FieldChange cannot plan "becomes null".** `changeObjects` drops nil
`Before`/`After`, so `issue update --milestone ""` (CLI-029's empty value clears) has no
`after`, `autoVerify` does not compare the field, and a clear that is the only change
gets `verification: not_run`. Pilot: the change carries `note: "removed"`; Apply checks
the read-back itself (`Mismatched`, and `Verification: passed` when the clear is the
only change). Fix: an explicit null marker (`aicli.Null`) that previews as `after:
null` and compares with a null read-back.

**ISSUE-L3 (low) WriteBatch turns a failure every item shares into N blocks.**
`previewItems` makes any item Preview error with E_NOT_FOUND, E_VALIDATION,
E_FORBIDDEN, E_CONFLICT or E_AMBIGUOUS a block of that item. That is right for a
missing issue (#99), but an unknown `--assignee` on `issue bulk assign`, or a project
that does not exist, comes back as an `ok: true` dry-run with one identical
`not_found` block per issue, without the `parameter` and `next_action` a single
E_NOT_FOUND carries. Pilot: accepted and tested (`TestIssueBulkIsBlockedWhenAnItemCannotApply`).
Fix: a `BatchSpec.Preview` run once before the items whose error fails the batch.

**ISSUE-L4 (low) An object cannot say a field was cut.** CLI-042 keeps a default
success response within 8,000 characters, but `issue get` returns the description,
and bug reports with logs are longer than that. `Page` has `truncated`; an object has
no convention. Pilot: `issue get` returns the description whole; write results leave
it out (`issueResult`), and the receipt's `observed` carries it when the write changed
it. Fix: a field-level convention (`<field>_truncated` plus the command that fetches
the rest) or say in CLI-042 that a resource's own text is exempt.

### Guide (MIGRATION-2.0.md) and spec

**ISSUE-G1 (medium) Step 3 does not say that a command takes one positional.** R01
refuses a second ("a command binds at most one positional argument (CLI-023)"), and
1.x had `issue assign <iid> <username|me>`, documented in its Skill reference as
`issue assign 12 alice`. A positional cannot be kept as deprecated the way a flag can.
Pilot: `issue assign <issue> --assignee <user>`; the 1.x form gets `unexpected_argument`
whose `next_action` is the corrected line (`TestIssueAssignNamesUsersAsGiven`). The
release notes must list it. Fix: say in step 3 that commands with two positionals move
the second to a flag, and in step 10 how to announce it.

**ISSUE-G2 (medium) Binding the ref, as CLI-052's example shows, fails G03.** CLI-054
says to bind a resolved form with `SetResolved`, and CLI-052's example has `"inputs":
{"issue": "acme/web#12"}`. With the project also bound as `project`, G03's "changing
--project" changes both inputs; the gate names the first differing input in sorted
order, `--issue`, and G03 expects `--project` (`issue update`, `close`, every bulk
command on `--ids`). Pilot: the issue binds as its iid (`issue: "12"`, `ids:
["2","6"]`) beside `project`, as milestone binds its iid; the target key stays the ref.
Fix: tell step 4 to bind each input's own part, and change the CLI-052 example (or let
G03 accept any input derived from the mutated flag).

**ISSUE-G3 (medium) CLI-044 has no migration step.** CLI-044 says finding the top item
MUST happen in the tool and "a command never ships a list to the model for it to
count"; the guide never asks a migrating group to add the computations its 1.x
commands lacked. The eval's T11 (top author over all 46 issues, open and closed) is
such a question. Pilot: `issue list` returns `total` (T01's count needs no paging), and
`--state all --limit 100 --fields author` gives T11's whole answer set in one compact
page, but the agent still counts authors. No `issue count --by author` was added: a new
command is a product decision, not a migration step. Fix: say whether migrations add
such commands, and give a pattern for a grouped count (its shape and its completeness
keys).

**ISSUE-G4 (low) Bulk commands: dangerous or not?** SEC-001 says commands that cannot
be undone "(delete, merge, drop, purge, bulk changes) MUST be declared dangerous";
CLI-025 says mass changes MAY be; the template's `note bulk-tag` is not dangerous; the
review skill's static review flags an undeclared bulk command as Major; the eval's T06
scope grants `dangerous: false` for the bulk label task. Pilot: following CLI-025 and
the notes step 3 (irreversible), the bulk commands are not dangerous (every item can
be changed back), `issue comment delete` is. Fix: one wording in SEC-001, CLI-025 and
the review skill, for example "irreversible bulk changes".

**ISSUE-G5 (low) Is a comment `change` or `deliver`?** CLI-025's `deliver` covers
"notify", and a GitLab comment notifies the issue's participants; a `deliver` command
must list `Preview.Destinations`, which a comment has none of. Pilot: `change` (the
comment is stored on the target issue; the e-mail is GitLab's side effect). `mr
comment` needs the same answer. Fix: one sentence in CLI-025 or step 3.

**ISSUE-G6 (low) No ref form for sub-resources.** Step 4 and the notes name keys
for project-level kinds only (`acme/web/-/labels/bug`); a comment belongs to an issue.
Pilot: `acme/web#12/notes/99`, under the issue's key so a grant for `acme/web#12` covers
its comments; the web URL `…/-/issues/12#note_99` is accepted too. Fix: name the
convention for children of an MR or issue (`acme/web!5/notes/77`).

### Integration requests

- `matrix_test.go`: its success, permission, gate and apply-failure rows read a single
  write's receipt; batch commands need per-item rows. `issueBatchRows` in
  `cmd/issue_bulk_test.go` is generic and could move there for `mr bulk` and the
  variable bulk command.
- Root words `comment` and `bulk` (ISSUE-L1): decide them once for the issue and MR
  groups; `issue-comment` and `issue-bulk` can go when the library allows it.
- MR comments and discussions: use the ref shape of ISSUE-G6.
- Release notes and Skill: `issue assign <iid> <user>` is now `--assignee`;
  `issue comment add` is `create` (`add`, `new` stay aliases); `--search` and
  `--milestone-id` are deprecated for `--query` and `--milestone`; results use refs
  (`acme/web#12`, `@carol`) and snake_case keys.
- `e2e/bootstrap.go` calls `Issues.Create` and `Issues.AddNote`; their signatures are
  unchanged.

## Repo group

`repo tree`, `repo file` (get, content, create, update, delete), `repo branch`
(list, get, create, delete), `repo commit` (list, get, diff, create) and `release`
(list, get, create, update, delete): 19 commands, 9 writes.

### Library (internal/aicli)

**REPO-L1 (medium) A flag alias cannot be limited to the commands it fits.** The
fleet's guess list (CLI-022) says `--branch` means `--name` on branch commands, but
`App.FlagAliases` is global: `rewriteFlag` turns the alias into its target on every
command that has the target and lacks the alias. `FlagAliases["branch"] = "name"`
would make `label create --branch x` set the label's name, and `release create v1
--branch main` would name the release "main", silently. Pilot: `repoFlagAliases`
sets a pflag normalize function on the branch commands only (`--branch` → `--name`;
also `--message` ↔ `--commit-message` on the file and commit writes); the alias binds
as its flag, and reference does not list it. Fix: `Meta.FlagAliases`, or aliases
scoped to a group path.

**REPO-L2 (medium) WriteBatch has no way to call a native bulk endpoint once.**
CLI-063 says a native bulk endpoint is "called in chunks behind the same contract
... each chunk's outcome mapped onto its items", but `WriteBatch` only calls each
item's `Apply` in turn. `repo commit create` (GitLab's atomic actions[] commit, one
request for every file) makes the first item's Apply send the commit and every later
Apply report that one outcome (`commitRun.apply`); each item still gets its own
lease and timeout, and items 2..N read as dispatched although only one request left.
Fix: a batch-level apply, e.g. `BatchSpec.ApplyChunk func(ctx, ops []Op) ([]Result,
error)` with a chunk size, whose outcome the gate maps onto the items.

**REPO-L3 (medium) A file named inside a flag's value cannot be bound.**
`Meta.FileInputs` maps whole flags, so `--action 'update:path=a;content_file=b'`
would bind the path `b`, not its bytes (CLI-054), and the file could change between
the dry-run and the confirm. Pilot: the actions are parsed before the gate, each
content_file is read once, and `SetResolved` replaces every `--action` value with a
canonical form holding the content's SHA-256 (`repoAction.canonical`): inline content
and the same bytes from a file bind alike, and a changed file is `token_mismatch`
naming `--action` (tested). Fix: `aicli.BindContent(cmd, name, bytes)`, or document
the SetResolved pattern for structured values.

**REPO-L4 (low) Danger cannot follow the input.** `Meta.Dangerous` is per command:
`repo commit create` with only delete actions removes files without `--dangerous`,
while `repo file delete` needs it. Pilot: commit create is not dangerous, as in 1.x.
Fix: let `Preview` raise danger for one run (checked like `Meta.Dangerous`).

**REPO-L5 (low) The tree check and R01 disagree on examples.** The tree check
(R02) accepts any example that starts with the tool's name, so `gitlab-cli branch
delete old-experiment ...` (the root shortcut, the eval task's own wording) passes
registration, but R01's reference validator requires the canonical path and fails
the conformance run. Pilot: examples use `repo branch delete`. Fix: one rule in both
checks; either way the reference never shows the shortcut.

**REPO-L6 (low) No parser for CLI-028's times and durations.** CLI-028 defines
`--since`/`--until` as ISO 8601 or relative (`7d`), echoed as absolute UTC in
`scope`, and durations as `<n>ms|s|m|h|d`, but aicli parses neither. Pilot:
`repoTimeFlag` (ISO date, date-time, or ago). Fix: `aicli.ParseTime` and
`aicli.ParseDuration`, so every tool reads `7d` alike.

**REPO-L7 (low) A RawOutput command must read --format itself.** `App.OK` writes
`[]byte` or `string` data raw and JSON-encodes anything else, so a command whose JSON
and raw forms differ (an object vs the file's bytes) has to know the format before it
calls OK; aicli exposes no accessor. Pilot: `rawFormat(cmd)` reads the flag. Fix:
`App.Format(cmd)`, or `App.OKRaw(cmd, obj, raw)`.

**REPO-L8 (low) App.Debugf is not safe for concurrent use.** It writes to
`App.Stderr` without a lock. `repo commit list` over several projects (CLI-041's
fan-out) reads them from 8 goroutines through the GitLab client, which logs each
request with `app.Debugf`; with `GITLAB_CLI_DEBUG` set and a test's `bytes.Buffer` as
stderr that is a data race (no test turns debug on during a fan-out; `-race` could
not run here, no cgo). Fix: a mutex around the debug write (and `Notice`).

### Guide (MIGRATION-2.0.md)

**REPO-G1 (medium) Refs that hold people's text are untrusted, and the guide does
not say so.** Branch names, tags and file paths are text people wrote (CLI-046), and
`acme/web/-/branches/<name>` or `acme/web/-/blob/<ref>/<path>` carries them, so a ref
is external content too; the worked example's label schema lists `name` but not
`ref`. Pilot: every repo and release schema whose ref holds a name lists `ref` as
untrusted (commit refs, a SHA only, do not). Fix: say so in step 12 next to `ref`,
and add `ref` to label's untrusted fields.

**REPO-G2 (medium) Step 4's key rule does not cover compound or nested targets.**
It names path-style keys for kinds without a sigil, but not a resource named by two
parts that can both hold "/" (a file: branch and path; GitLab's own
`/-/blob/<ref>/<path>` cannot be split without knowing the refs), what an atomic
multi-target write is keyed by, or how keys should nest so a person's grant reads the
way they think. Pilot: a file write's key is the file's ref on its branch
(`acme/web/-/blob/<branch>/<path>`), so `acme/web/-/blob/develop/docs` grants that
branch's docs/ (tested, including `docsite.md` refused); a ref or URL whose branch
may hold "/" is split by looking the branches up before the gate; commit create has
one target per touched file, a move two. A grant for `acme/web/-/branches/<b>` covers
creating and deleting that branch, not commits to it. Fix: a paragraph on compound
keys, and on making key prefixes follow what people grant.

**REPO-G3 (low) Shortcuts: the example shows two, R02 wants one per nested group.**
Step 2's App lists `branch` and `member`; the tree check requires a shortcut for every
nested group (`repo file`, `repo commit` too), and CLI-022's fleet list also names
`tree`, a leaf. A migrator learns this from the R02 failure. Pilot: `branch`, `file`,
`commit`, `tree` and the plurals, added in `registerRepo`. Fix: say "one per nested
group" in step 2 and the staged-migration note.

**REPO-G4 (low) No pattern for a heavy body bigger than the success budget.** CLI-043
moves file contents, diffs and messages to their own commands, but one file, one
diff or one set of release notes can still exceed 8,000 characters (CLI-042). Pilot:
the JSON forms carry a bounded window (`repo file content` pages lines with
`--offset`/`--limit`; `repo commit diff` pages files by size; `commit get` and
`release get` cut the message or notes at 4,000 characters with `*_truncated` and a
notice), and `--format raw` (Meta.RawOutput) carries the whole body. Fix: name this
pattern in step 12.

**REPO-G5 (low) A 1.x read's --output has no migration path but "artifact".** Step 3
says a command that writes a file declares `artifact` or moves the write to its own
command; it does not offer `--format raw` with shell redirection for a read like
`repo file get --output`, nor how the old flag should answer. Pilot: the flag is
hidden and returns `E_USAGE` whose next_action is the `--format raw` command; there
is no artifact command. Fix: list this option.

**REPO-G6 (low) A default field that costs extra.** CLI-040 and step 3 do not say
whether a default field may be absent when its data was not asked for. Commit line
counts need GitLab's `with_stats`; the shared matrix requires every default field on
every item. Pilot: `additions` and `deletions` are default fields of `commit_page`,
present with `--with-stats` or when `--fields` names them; the matrix row runs with
`--with-stats`. Fix: allow it explicitly (or say to request the data whenever the
field is a default).

### Integration requests

- **Project-listing routes in the fake.** The commit fan-out registers `GET
  /api/v4/groups/{group}/projects` in `cmd/repo_test.go`. It does not register `GET
  /api/v4/projects` (`--all-projects`), which the Project group will likely own: two
  registrations of one ServeMux pattern panic. `--all-projects` is tested in
  `internal/api` and for its usage error only. Keep one route of each at merge.
- **Project enumeration.** The fan-out uses `RepoAPI.GroupProjectPaths` and
  `MemberProjectPaths` (internal/api/repo.go), not `ProjectAPI.ListGroupProjects`
  and `List` (project.go), which 1.x wrote for this fan-out. Consolidate once
  project.go settles.
- **Repository fakes for other groups.** Branches, files, commits, tags and releases
  of the fake live in `gl.state["repos"]` (`cmd/repo_test.go`, `release_test.go`) with
  their routes; a group that needs a branch or a commit (MR create, pipeline refs)
  should reuse them rather than register the same patterns.
- **Matrix rows for batch writes.** `runMatrix`'s success, auth, gate and
  apply-failure rows read one receipt, so a `WriteBatch` command replaces them (commit
  create does, in `repo_commit_test.go`); its pagination and success rows need a
  non-null `ref` on every list item.

## CI group

Pipelines, jobs and CI/CD variables: 21 commands, 11 writes (`job artifacts` is the
fleet's first `artifact` command, `variable bulk-import` gitlab-cli's first batch,
`job log --follow` its first stream).

### Library (internal/aicli)

**CI-L1 (high) A read before the gate hides the gate's answers at confirm.** G04
requires a repeated confirm to be `token_used`, but whatever the tool does before
`App.Write` runs first. `variable delete OLD_TOKEN` resolves the variable's
environment scope before the gate (the target key must be canonical, which
the notes allows); after the delete, the repeated confirm got `E_NOT_FOUND` from that
read, and `job artifacts`'s own "file exists" check answered its repeated confirm (G04
failed on both). `label delete --label-id` has the same latent fault: after the delete
the ID no longer resolves, so its repeat is `E_NOT_FOUND`; the conformance sample uses
the name and does not see it. There is no hook to resolve a target inside the gate,
after the token is verified. Pilot: with `--confirm`, a variable that is gone is not an
error in the resolution, and the key falls back to the input's form, which matches no
other token (Preview then reports the variable missing); the existence check of
`--output` runs only without `--confirm`. Fix: a `WriteSpec.Resolve func(ctx) (Target,
error)` that the gate calls after confirm steps 1 to 8, and a sentence in the guide that
checks before `App.Write` must not run at confirm.

**CI-L2 (medium) `App.WriteOutput` does not fit a download.** CLI-042 points to it as
"the `--output` helper", but (a) its `Target.Key` is the local path, so a scope whose
`targets` name the project (`acme/web`) cannot cover `job artifacts`, and an unknown
outcome blocks the path, not the job; (b) `Produce` returns `[]byte`, so a whole
artifacts archive (GitLab allows hundreds of MB) is held in memory; (c) its Preview
reads nothing upstream, so a job without artifacts gets a token and fails only at
confirm. Pilot: `App.Write` with the job's ref as the target and the file in
`Preview.Outputs`; Preview reads the job (blocked `no_artifacts`, `no_directory`);
Apply streams into a temporary file beside the target and links it into place, never
replacing a file (`ciCreateFrom`). Fix: `OutputSpec.Target`, a streaming
`Produce(ctx, io.Writer)`, and a hook for the upstream part of the preview.

**CI-L3 (medium) An existing output path of a create-only command is refused as an
overwrite.** The gate's scope check counts every existing path in `Preview.Outputs` as
an overwrite that `roots` must cover (C1), also for a command that declares only
`artifact`. A dry-run on a taken path, or a confirm after the file appeared, is
`E_FORBIDDEN` asking the person for `roots`, a grant that cannot help, while the
command's own `exists` block stays hidden in `details.preview`; `WriteOutput` behaves
the same. Declaring `persist` as well is no way out: every declared effect is checked,
so every grant would need `persist` even to create a file. Pilot: `artifact` only;
`job artifacts` refuses an existing `--output` before the gate (`E_VALIDATION`), except
at confirm (CI-L1), where a file that appeared is the gate's `E_FORBIDDEN` (missing
`roots`), or, with `roots` granted, `state_drift`; nothing is overwritten. Fix: for a
command without `persist`, report an existing output as a block, not as missing
`roots`; check `artifact` and `persist` per path, not per command.

**CI-L4 (low) `App.OK` serves `--format raw` from the JSON value.** A `RawOutput`
command answers with bytes in raw mode and an object in JSON mode, but `App.OK` takes
one value (raw only from `[]byte` or `string`, which JSON would show as a base64 or
plain string), and nothing tells the command which format is running. Pilot: `job log`
reads `--format` itself and passes the bytes or the object. Fix: `App.Format(cmd)`, or
`App.OKRaw(cmd, data, raw)`.

**CI-L5 (low) Stream items inherit the command's object schema.** `Stream.Item`
applies the command's `Output` schema to every item: each `job log --follow` line
(`{"line": ...}`) lists `data.log`, the object's untrusted field, in `meta.untrusted`,
and `--fields` would project the lines by the object's fields. With `--format raw` the
raw lines are followed by a JSON summary line. Pilot: untrusted paths passed per item,
`--fields` refused with `--follow`. Fix: a separate item schema
(`Meta.StreamItem`), and a documented raw-stream summary (or none).

**CI-L6 (low) Previews cannot say "one of".** A retry leaves a pipeline or job
`pending` or `running`, a cancel `canceling` or `canceled`, so a change's `after`
cannot be written and the gate's verification compares nothing (`not_run`). Pilot: the
change has `before` and a note; Apply sets `Outcome.Verification` from GitLab's answer
(`ciVerifyStatus`), so a retry GitLab ignored is `E_VERIFY_FAILED`. Fix: an `AnyOf`
in `FieldChange`, or document tool-set verification for state transitions.

**CI-L7 (low) `ValidateEnvelope` holds every response to 8,000 characters.** CLI-042
limits responses "with default flags", but the validator, and so every test helper
that validates envelopes, refuses any larger success, so `--limit 100` pages cannot be
tested. Pilot: `job log`'s JSON keeps within the budget whatever the flags (the rest
via `--format raw`); no test uses large pages. Fix: let the validator take the flags'
context, or make the spec's rule unconditional.

**CI-L8 (low) No parser for CLI-028's durations.** `--wait` (and relative `--since`)
take `<n>ms|s|m|h|d`; Go's `time.ParseDuration` has no `d`, and aicli exports none.
Pilot: `ciParseDuration`. Fix: export one.

### Guide (MIGRATION-2.0.md)

**CI-G1 (medium) Local file writes: the guide names neither the helper nor the
choices.** Step 3 says `job artifacts --output` declares `artifact` (`persist` if it
can overwrite), and step 6 shows its sample, but no step says how to write the file
(`App.WriteOutput` is not mentioned), that the target should stay the upstream
resource (CI-L2), that a new file needs `outputs` and an existing one `roots`, or that
declaring `persist` makes every grant need it (CI-L3). Step 6's harness grants `roots`
"because the checks repeat the sample, which overwrites its file", which holds only
for a command that overwrites: a create-only command needs `Harness.Reset` to remove
the file instead (gitlab-cli: `addConformanceReset`).

**CI-G2 (medium) 1.x wait commands have no mapping.** `pipeline wait` and `job wait`
waited without limit (`--timeout 0`) and reported a failed pipeline as exit 6 and a
timeout as exit 8. CLI-049 and CLI-028 want `--wait <duration>` on status reads,
bounded well under the harness's two minutes, and `ok` now follows the read, so a
failed pipeline is data, not an exit code. Pilot: `pipeline get` and `job get` take
`--wait` (at most 90s; a `wait_ended` notice when it ends first); `job log --wait`
waits, then shows the log; the 1.x commands are deprecated for `get --wait`, keep
`--timeout` (lowered to 90s with a notice) and lose their exit codes. Fix: a row in
step 4's or step 12's table.

**CI-G3 (medium) 1.x's secret-value switch has no 2.0 counterpart.** Step 2 maps
`cmd/agent.go`'s switches to "the App and the fields above", but `--show-values`
(variable values, refused unless `GITLAB_CLI_ALLOW_SHOW_VALUES=1`) is about secrets
read from the upstream, which SEC-021 (secret parameters) does not cover. Pilot: no
command outputs a variable value; `--show-values` is gone (an unknown flag); a write's
read-back compares the value in memory and shows `***`. The Skill still names the
variable (skills phase). Fix: say how 2.0 treats secrets the upstream returns.

**CI-G4 (low) "A masked variable's `--value`" cannot be declared.** Step 3 puts
"masked variable `--value`" in `Secrets`, but `Meta.Secrets` is per flag, not per
value of `--masked`. Pilot: `--value` (and `--value-file`) are secret on every
variable write; unmasked values are often credentials too. Fix: say "the `--value` of
variable writes".

### Integration requests (shared gitlab-cli test code)

**CI-I1 `matrixCase` cannot set the scope.** `newTestEnv` grants `change` and
`configure`, so every write row of an `artifact` (or `deliver`) command fails on the
scope. `job artifacts` overrides six rows (`jobArtifactsRows`, with `ciArtifactEnv`).
Request: a scope (or env hook) in `matrixCase`.

**CI-I2 The matrix's write rows read single receipts.** `success`, `gate` and `apply
failure` read `data.receipt`, which a batch (CLI-063) does not have, and `apply
failure` expects a single write's details. `variable bulk-import` overrides them
(`variableBulkRows`); `mr bulk` and `issue bulk` will need the same. Request:
batch-aware rows.

**CI-I3 `testEnv.run` refuses NDJSON.** It validates stdout as one envelope, so a
stream fails with "holds more than one JSON document". Request: validate with
`aicli.ValidateStream` when stdout holds several documents, as the conformance suite
does (`isStreamOutput`). Pilot: `ciRunStream` in `cmd/job_test.go`.

## Project group

Migrated: `project list|get|create|members`, `user me|get|search`, `search
projects|issues|mrs|code|commits` (12 commands, one write: `project create`).

### Library (internal/aicli)

**PROJ-L1 (medium) `Meta.DefaultFields` is missing from `reference`.** It changes the
projection and `--help`'s Fields line, but `reference --command` and the full
`reference` publish only the output schema's `default_fields` (the command entry has no
default fields). Help and reference then disagree (CLI-030 calls reference help's
machine twin), and whatever reads reference is told the wrong defaults: `search
projects` reused `project_page` with shorter `Meta.DefaultFields`, and the shared
behaviour matrix, which takes the defaults from reference, failed its success row on
fields the command does not return. Pilot: `search projects` has its own schema
(`search_project_page`); no command uses `Meta.DefaultFields`. Fix: publish
`default_fields` in the command entry when `Meta.DefaultFields` is set, or drop the field.

**PROJ-L2 (low) No way to ask whether `--fields` will return a field.** A field that
costs a request should be fetched only when shown. `wantsField` (cmd/list.go) covers
explicit names and `all`, which is enough for a field outside the defaults (label
counts); for a default field it also needs the defaults and the `+` add mode, which
aicli parses (`parseFields`) but keeps private. `project members`' `membership` is such
a field (one read of the direct members). Pilot: `projectWantsMembership` re-reads the
raw `--fields` value. Fix: export `App.WantsField(cmd, name)`, honouring the defaults and
the add mode.

**PROJ-L3 (low) `App.Schema` silently replaces a schema registered twice.** It assigns
into a map, so when two groups register the same name (`user`, `project_page`) the
later one wins and the tree check cannot see the clash. Nothing broke here; spotted while
naming this group's schemas, with five groups registering into one App at merge.
Pilot: every schema is named after this group's nouns (`project_*`, `user*`,
`search_*`). Fix: report a duplicate schema name in `checkTree`.

### Guide (MIGRATION-2.0.md) and spec

**PROJ-G1 (medium) CLI-029 assumes the upstream ranks searches by relevance.** GitLab
ranks by relevance only with advanced search (Premium, Elasticsearch or OpenSearch).
Basic search, all a Free or self-managed CE instance has (the eval's GitLab included),
lists issues and merge requests newest first and states no order for projects, and
nothing in an answer says which search served it. So `sort: "relevance"`, as CLI-029
and this task require, is false on most self-managed instances, and `created_at_desc`
would be false with advanced search. Pilot: the five `search` commands state
`sort: "relevance"` plus `scope.ranking`: "relevance where GitLab has advanced search;
basic search does not rank by relevance". `user search` really is relevance-ordered
(GitLab keeps exact username, name or email matches first), so it states relevance with
no caveat. Fix: let CLI-029 accept the upstream's own order when the tool cannot know it
(`sort: "upstream"`, with scope saying what that is), or require such a scope note, and
mention it in step 12.

**PROJ-G2 (low) A target that depends on the account needs a read before the gate.**
Step 4 and the notes allow reading GitLab before `App.Write` only to resolve an input
(a title, a numeric ID, a deprecated `--x-id`). `project create` without `--namespace`
creates in the account's own namespace, whose path is the username, and the target key
needs it; `Identity` cannot supply it (no network, and a `GITLAB_CLI_TOKEN` account is
`env:GITLAB_CLI_TOKEN`, L11). Pilot: one `GET /user` before the gate; the username is
checked like a typed path because it goes into the key and the read-back command
(SEC-002). Fix: name this case in step 4.

**PROJ-G3 (low) CLI-041's member example cannot be built from GitLab's REST API.** Its
item has `via: "group acme"`, but `/members/all` returns each user once at their highest
access with no source, and nothing else short of GraphQL names the group. Pilot:
`membership: direct|inherited`, from one read of the direct members among the page's
users (`user_ids[]`), made only when the field is shown; a direct member whose shown
access is higher than their direct grant is `inherited`. Fix: an example the REST API
supports, or say that `via` needs GraphQL.

**PROJ-G4 (low) GitLab dates under `*_at` keys fail R04.** aicli's envelope check holds
every `*_at` value to an ISO 8601 UTC time (CLI-007), and GitLab returns some dates under
such keys (a member's `expires_at: "2026-12-31"`, tokens' `expires_at`). Copying the 1.x
key made every member list with an expiry invalid. Pilot: `expires_on`, a `YYYY-MM-DD`
date. Fix: step 12 should say to rename `*_at` keys that hold dates (or give them as
times).

### Integration requests (shared files) and API changes

- **I1 Project names belong in `projectFlag` (cmd/refs.go).** CLI-048 has names as
  natural input for projects, but `projectFlag` takes paths, URLs and IDs only, so
  `mr list --project payments` is E_USAGE `invalid_value`. Pilot: `projectTarget` and
  `projectByName` (cmd/project.go) resolve a bare name for this group's commands
  (`project get|members`, `search ... --project`): GitLab's search, kept to projects
  whose name or path holds the name's words in order; one match, `E_AMBIGUOUS` with up
  to 10 candidates, or `E_NOT_FOUND`; at most 3 pages of 100 hits, after which one match
  is `E_USAGE`. Request: move them into refs.go, add `E_AMBIGUOUS` to `gitlabReadCodes`
  and `gitlabWriteCodes`, and drop `projectReadCodes`.
- **I2 `requiresRole` speaks of a role "on the project" (cmd/errors.go).** A create in a
  group needs a group role. Pilot: `projectCreateError` rewrites `next_action` after
  `classifyWriteError`. Request: let `requiresRole` name the resource.
- **I3 The fake's shared `GET /projects/{project}` returns only id, path and web URL**
  (cmd/fakegitlab_test.go), and a group can add projects to `gl.projects` but no fields,
  so `project get` is tested for its ref, ID and URL only (`project list` covers the
  rest). Request: let `fakeProject` carry extra fields the route returns.
- **I4 Merge hazards.** Fake routes share one ServeMux, so a second group registering a
  pattern this group registers panics at merge: `GET /api/v4/projects` (the Repo
  group's commit fan-out reads it), `POST /api/v4/projects`, `GET /api/v4/search`,
  `/projects/{project}/search`, `/projects/{project}/members[/all]`,
  `/namespaces/{id}`. The seed adds `fin/payments-service` and `legacy/payments` to the
  shared `gl.projects` (the eval's two payments projects), outside `acme/` on purpose.
- **API (internal/api).** Unchanged for other groups and shared code: the `User` type
  (types.go untouched), `Projects.List`, `ListGroupProjects`, `Get`, `Create`,
  `Users.Me`, `Users.Search`, `GetByUsername`. `Project` gained fields. Removed, used by
  no other file: `ProjectAPI.Members` (direct members only, the CLI-041 trap; use
  `EffectiveMembersPage`), `SearchProject` and the multi-page `Search.Projects`,
  `Issues`, `MergeRequests`, `Code`, `Commits` (now `*Page`). Added: `ListPage`,
  `EffectiveMembersPage`, `DirectMembersPage`, `GetNamespace`, `Users.SearchPage`.

## Integration of the five groups (2026-10-14)

All five branches merged without a textual conflict (the groups kept to their own
files and sections). The integration requests, as handled in the shared files; the
decisions are in notes.md, "Integration decisions":

- **Root shortcuts (MR-G1, ISSUE-L1).** No bare `comment`, `bulk` or `discussion`
  at the root: `mr-comment(s)`, `mr-discussion(s)`, `mr-bulk`, `issue-comment(s)`,
  `issue-bulk`; bare words only for nouns one group has. `addShortcut` (cmd/app.go)
  panics when a word is taken by another path. The library fixes asked for (a word
  for several paths answered with E_AMBIGUOUS, an opt-out of R02 for nested groups
  that are no root noun, a tree check for a key assigned twice) stay open.
- **Sub-resource refs (MR-G2, ISSUE-G6).** One form, `<parent ref>/<collection>/<id>`:
  the MR group's `acme/web!5#note_12` and `#discussion_<id>` became
  `acme/web!5/notes/12` and `acme/web!5/discussions/<id>`; the anchors and web URLs
  are still accepted.
- **Batch rows (MR-G6, Issue, CI-I2, Repo).** The shared matrix runs batch rows for
  every `BatchFlag` command, with `OneRequest` for repo commit create. `mrBatchRows`,
  `issueBatchRows`, `variableBulkRows` and the four repo commit rows are gone; what
  they checked beyond the shared rows moved to focused tests
  (`TestVariableBulkImportPlansAndKeepsValuesSecret`, `TestRepoCommitCreateIsOneCommit`).
- **Matrix scope (CI-I1).** `matrixCase.Setup` prepares each row's world; job
  artifacts uses it, and its custom upstream-failure and exit-code rows are gone.
- **NDJSON (CI-I3).** `testEnv.run` validates a stream with `aicli.ValidateStream`;
  `ciRunStream` is gone.
- **Project names (PROJ I1).** `projectFlag` resolves names for every command;
  `projectTarget` and `projectReadCodes` are gone, `E_AMBIGUOUS` is in
  `gitlabReadCodes` (`TestProjectNamesWorkEverywhere`).
- **Role wording (PROJ I2).** `requiresRoleOn(role, resource)`; `projectCreateError`
  uses it instead of rewriting next_action.
- **The fake (PROJ I3, I4, Repo).** `fakeProjectDetails` fills the shared
  `GET /projects/{project}` from the Project group's metadata; no pattern was
  registered twice. Open: the Repo group's fan-out still lists projects with
  `RepoAPI.GroupProjectPaths`/`MemberProjectPaths` beside `ProjectAPI.List` and
  `ListGroupProjects`, to consolidate in internal/api.

Found while integrating:

- **I-1 (low, library) A bare word several groups nest gets the first group's
  next_action.** `gitlab-cli comment list acme/web!5` answers `did_you_mean` with
  `issue comment` and `mr comment`, but next_action says "Run `gitlab-cli issue
  comment list acme/web!5`", wrong for an MR ref. Part of ISSUE-L1's fix: several
  candidates of equal standing should be E_AMBIGUOUS, or next_action should not
  pick one.
- **I-2 (medium) The suite outgrows go test's default timeout.** With every group,
  `go test ./cmd/` takes about 9 minutes on Windows (TestConformance alone 4.5,
  for 60 writes), so `-race` exceeds go test's 10-minute default. Pilot: `-timeout
  45m` in the Makefile and the workflows. Fix: profile `RunConformance` (per-check
  App and state store setup) before more tools adopt it.
- **I-3 (open, tool) Users render two ways.** Issue, CI, Repo and Project give
  user fields as refs (`"author": "@carol"`, as `userRef`); MR gives usernames
  (`"author": "carol"`, `assignees`, `reviewers`, `approved_by`). Aligning MR on
  refs touches its previews and read-backs (they must compare alike) and about 24
  test assertions; left for a decision.

## Round 4: spec v2.0.0-rc.2 (synced 2026-10-14)

Synced from the GitHub tag (`node scripts/sync-spec.js --version v2.0.0-rc.2`; 62
files, identical to the worktree), `check-spec.js` passes its remote check.

**Resolved in rc.2, workaround replaced:**

- N1 (Q2): aicli's transport reads the proxy from `App.Env`; `proxyFromAppEnv` is gone.
- N2 (Q3): the first `auth login` sets `default_profile`; `App.Profiles` replaces the
  settings-file `os.Stat`. N3 (Q25): `E_CONFIG` is in every command's error codes.
  N4 (Q4): `Meta.AllowUnknownProfile` on login and logout.
- CI-L1, PROJ-G2 (Q1): `WriteSpec.ResolveTarget` for `variable update|delete`
  without a scope, `label update|delete --label-id` and `project create` without
  `--namespace`; no read before `App.Write` names a target any more.
- CI-L2, CI-L3 (Q8, Q9): job artifacts streams through `App.OpenOutput`;
  `ciCreateFrom` and the pre-gate existence check are gone.
- REPO-L2 (Q12): repo commit create sends its actions through `BatchSpec.ApplyAll`.
  REPO-L4 (Q13): a commit that deletes a file is dangerous (`Preview.Dangerous`).
- REPO-L1 (Q11): `Meta.FlagAliases` replaces `repoFlagAliases`. MR-L8 (Q10):
  `Meta.RenamedFlags` replaces the glue for `--search` (mr, issue, project, repo
  branch), `--ref-name`, pipeline `--status` and `--scope`.
- CI-L4, REPO-L7 (Q14): `aicli.OutputFormat`. CI-L8, REPO-L6 (Q15):
  `aicli.ParseDuration` and `ParseTime` (`--wait 1m30s` is refused now, as CLI-028
  says). MR-L3, PROJ-L2 (Q17): `wantsField` wraps `aicli.WantsField`; `mrWantsField`
  and `projectWantsMembership` are gone. REPO-G6: commit line counts, default
  fields, are now read whenever they are shown.
- ISSUE-L1, MR-G1 (Q6): `App.AddShortcut` replaces the tool's `addShortcut`.
- PROJ-G1 (CLI-029): the five search commands say `sort: "upstream"`. I-3: MR shows
  users as `@carol`.

Fixed in the library, the group's workaround kept (harmless): MR-L1 (Q7), MR-L4
(Q16), MR-L5 and PROJ-L1 (Q5), MR-L6 (Q19), MR-L7 (Q20), PROJ-L3 (Q18), REPO-L5
(Q21), REPO-L8 (Q22), CI-L5 (Q23), CI-L7 (Q24), MR-L9 (Q26).

**New problems with rc.2:**

- **RC2-1 (medium) Q3 also picks a default for a file a person left without one.**
  `SetProfileSettings` sets `default_profile` whenever the file has none, not only for
  the first profile: with `work` and `home` and no default (a person's choice),
  `auth login --profile lab` makes `lab` the profile of every later command, silently
  (`TestAuthLoginProfiles`). Fix: set it only when the file had no profile.
- **RC2-2 (low) `ReadBack` cannot name a target that resolves inside the gate.** It is
  a string fixed before `ResolveTarget` runs, so `project create` in the account's
  namespace reads back with `project list --owned --query <path>`, and a variable
  whose scope is looked up names the input's form. Fix: a `ReadBack` that takes the
  resolved target, or a `{target}` placeholder.
- **RC2-3 (low) The library calls rc.2 "2.0.0".** `.agent/SPEC_VERSION` pins
  `v2.0.0-rc.2`, but the manifest's `spec_version`, `reference` and the User-Agent
  (`aicli/2.0.0`) say 2.0.0, so a release candidate cannot be told from the release.
