# Upgrading from gitlab-cli 1.x to 2.0

gitlab-cli 2.0 follows ai-native-cli-spec 2.0. It changes how you sign in, refuses writes until you grant a scope, renames some commands and flags, and changes the JSON output. Do these steps in order:

1. [Install 2.0 and its Skill](#install-20-and-its-skill)
2. [Sign in again](#sign-in-again)
3. [Grant a scope for writes](#grant-a-scope-for-writes)
4. [Change commands and flags](#change-commands-and-flags)
5. [Update scripts that read the output](#update-scripts-that-read-the-output)
6. [Clean up after 1.x](#clean-up-after-1x)

[CHANGELOG.md](../CHANGELOG.md) lists every change; agents read it with `gitlab-cli changelog --since 1.2.17`.

## Install 2.0 and its Skill

```bash
npm install -g @fateforge/gitlab-cli
npx skills@1.7.2 add "$(npm root -g)/@fateforge/gitlab-cli" -y -g
gitlab-cli --version
```

Install the Skill again as well: the 1.x Skill teaches commands, flags and output that 2.0 changed. `gitlab-cli doctor` warns in its `version` check when the installed Skill is older than the tool.

## Sign in again

2.0 does not read what 1.x used for credentials and settings:

- `~/.gitlab-cli/config.json` and `~/.gitlab-cli/profiles.json`. 1.x encrypted the token in these files with a key it kept in the OS keyring, so the token cannot be copied out by hand. Use the token you saved when you created it, or create a new personal access token in GitLab (User settings > Access tokens, scope `api`, or `read_api` for read-only use).
- `GITLAB_HOST` and `GITLAB_TOKEN`, the variables glab uses. Set `GITLAB_CLI_HOST` and `GITLAB_CLI_TOKEN` instead, so a token meant for another tool is never sent to this one's host.
- `auth profile list|use|remove`. Profiles now live in `settings.json` (below).
- `GITLAB_CLI_USER_AGENT`, `GITLAB_CLI_MAX_RETRIES`, `GITLAB_CLI_RETRY_BASE_MS`, `GITLAB_CLI_AGENT_SAFE`, `GITLAB_CLI_ALLOW_FORCE`, `GITLAB_CLI_ALLOW_SHOW_VALUES`, `GITLAB_CLI_NO_UPDATE_CHECK` and `GITLAB_CLI_INSTALL_METHOD`. Requests now carry `gitlab-cli/<version> aicli/<spec version>`, reads are retried at most twice, writes are never retried, and only `update` and `doctor` look for a new release.

Pick one way to sign in.

**Environment variables.** Set `GITLAB_CLI_TOKEN`, and `GITLAB_CLI_HOST` for a host other than gitlab.com, in the shell or secret manager your agent runs with. Nothing is stored and no scope is needed. This is the way for agent sessions.

**OS keyring.** `auth login` checks the token with GitLab, stores it in the OS keyring (entry `gitlab-cli` / `profile:<name>`) and writes the host to the profile in `settings.json`. It changes the tool's own credentials, so it is a gated write with the `configure` effect, which is dangerous: you need a scope that grants it, and two steps.

1. Grant the login for a short time in `~/.gitlab-cli/scope.json`. Only you write this file; the tool never does, and agents must not:

   ```json
   {"scope_id": "scp_login", "tool": "gitlab-cli", "granted_by": "<your name>",
    "granted_at": "2026-10-20T08:00:00Z", "expires_at": "2026-10-20T09:00:00Z",
    "effects": ["configure"], "dangerous": true, "targets": [{"key": "profiles"}]}
   ```

2. Preview, then confirm. Each step reads the token from stdin: paste it, then press Ctrl-D (Ctrl-Z and Enter on Windows).

   ```bash
   gitlab-cli auth login --host https://gitlab.example.com --token-file - --dangerous --dry-run
   gitlab-cli auth login --host https://gitlab.example.com --token-file - --dangerous --confirm <confirm_token>
   ```

   For several GitLab hosts, sign in once per profile: `--profile work`, `--profile home` and so on; a new profile needs `--host`. The first login makes its profile `default_profile`; later logins leave the default as it is. With two or more profiles, switch by editing `default_profile`, or pass `--profile` or set `GITLAB_CLI_PROFILE` for one command or one session.

3. Check it with `gitlab-cli auth status`, then replace `scope.json` with the scope you grant your agents (next step).

Where 2.0 keeps things:

| What | Where |
|---|---|
| Host and timeout, per profile | `~/.gitlab-cli/settings.json` (or `GITLAB_CLI_SETTINGS_FILE`); a secret in it makes it invalid |
| Token | `GITLAB_CLI_TOKEN`, else the OS keyring entry of the active profile, used only for the host it was stored for |
| Active profile | `--profile`, then `GITLAB_CLI_PROFILE`, then `default_profile`, then the only profile |
| Private CA | `GITLAB_CLI_CA_FILE` (a PEM file added to the system roots); TLS verification cannot be turned off |
| Diagnostics | `GITLAB_CLI_DEBUG=1` prints request lines on stderr, never the token |

```json
{"default_profile": "work",
 "profiles": {"work": {"host": "https://gitlab.example.com"},
              "home": {"host": "https://gitlab.com", "timeout": "10s"}}}
```

`gitlab-cli context` shows the profiles, the active one and where the token comes from (`credentials.source`).

## Grant a scope for writes

Reads work as soon as you are signed in. Every write is refused with `E_FORBIDDEN` until you write a scope file: `~/.gitlab-cli/scope.json`, or the file `GITLAB_CLI_SCOPE_FILE` names. The refusal carries `details.scope` with what is missing and a `grant_example`, which your agent relays to you. This grant allows ordinary changes under `acme/web` for a week, at most 100 operations:

```json
{"scope_id": "scp_2026-10-20_web", "tool": "gitlab-cli", "granted_by": "<your name>",
 "granted_at": "2026-10-20T08:00:00Z", "expires_at": "2026-10-27T08:00:00Z",
 "effects": ["change"], "dangerous": false, "targets": [{"key": "acme/web"}],
 "limits": {"max_operations": 100}}
```

- A target key starts with the project path, so `acme/web` covers `acme/web!5`, `acme/web#12/notes/99` and `acme/web/-/branches/main`, and the group key `acme` covers every project under `acme/`. List one entry per project or group in `targets`.
- Merges and deletes (`mr merge`, `repo branch delete`, `label delete`, a commit that deletes a file and others) also need `"dangerous": true`.
- CI/CD variable writes need `"configure"` and `"dangerous": true`.
- `job artifacts` needs `"artifact"` and the destination folder, an absolute path, in `"outputs"`; it never replaces an existing file.

`gitlab-cli doctor` says in its `authorization` check whether the file is valid and what it grants. To withdraw it, set `"revoked": true` or delete the file. The scope guards against injected instructions, an agent drifting off-task and wrong targets. It is not proof of approval: any program running as your OS user, the agent included, can edit it. Write it yourself and keep it narrow and short-lived. [SECURITY.md](../SECURITY.md) has the details.

How a write runs now:

1. Run the command with `--dry-run`. The answer is a preview (`data.preview`) and a `confirm_token`. A call with neither `--dry-run` nor `--confirm` returns the same and changes nothing.
2. Run the same command again without `--dry-run`, adding `--confirm <confirm_token>`. Dangerous commands need `--dangerous` in both steps.

A token works once, expires after 15 minutes and is bound to every input, the account, the host, the profile and the scope file; anything different at confirm is `E_CONFLICT`. 1.x tokens are refused: run the dry-run again. When a reply is lost (`E_OUTCOME_UNKNOWN`), run `gitlab-cli operation show --id <operation_id>` before anything else, and never send the write again.

## Change commands and flags

Flags marked deprecated still work in 2.x, with a `deprecated` notice in `meta.notices`; they go away in 3.0. Removed forms fail with `E_USAGE` (exit 2) and change nothing. The examples leave out `--project`: keep it where 1.x needed it, or pass a ref such as `acme/web#12` instead of the number.

| 1.x | 2.0 |
|---|---|
| `issue assign 12 alice` | `issue assign 12 --assignee alice` |
| `issue comment add 12 --body ...` | `issue comment create 12 --body ...` (`add` still works as an alias) |
| `pipeline wait 123`, `job wait 456` | `pipeline get 123 --wait 90s`, `job get 456 --wait 90s` (deprecated; see the exit codes below) |
| `job log 456` (the whole log) | `job log 456` returns the last 100 lines; `job log 456 --format raw --tail 0` prints the whole log |
| `job log 456 --follow --timeout 60` | `job log 456 --follow --wait 60s` |
| `repo file get --path a.md` (with the content) | `repo file get a.md` returns metadata; `repo file content a.md` returns lines, `--format raw` the bytes |
| `repo file get --path a.md --output a.md` | `repo file get a.md --format raw > a.md` |
| `mr diff 5` (the unified diff) | `mr diff 5` lists files with line counts; `--fields +diff` adds patches, `--format raw` prints the diff |
| `mr get 5` (with the description) | `mr get 5 --fields +description` |
| `mr list --all` | page with `--limit` (up to 100) and `--offset <next_offset>` |
| `--search <text>` | `--query <text>` (deprecated alias kept) |
| `pipeline list --status failed`, `pipeline jobs 123 --scope failed` | `--state failed` (deprecated aliases kept) |
| `repo commit list --ref-name dev` | `--ref dev` (deprecated alias kept) |
| `variable get --key K --filter env_scope=production` | `variable get K --env-scope production`, or `variable get K:production` |
| `variable get --show-values` | no replacement: read the value in GitLab |
| `label update --label-id 5 ...` | `label update <name> ...` (`--label-id` deprecated) |
| `milestone get --milestone-id 42` | `milestone get <iid or title>` (`--milestone-id` deprecated) |
| `issue create --milestone-id 42` | `issue create --milestone <iid or title>` (`--milestone-id` deprecated) |
| `project create --namespace-id 7` | `project create --namespace <group path>` (`--namespace-id` deprecated) |
| `mr discussion resolve --unresolve` | `mr discussion unresolve` (`--unresolve` deprecated) |
| `auth login --token <token>` | `auth login --token-file -`, with a scope and two steps ([above](#sign-in-again)) |
| `auth profile list` | `gitlab-cli context` (`profile.profiles`) |
| `auth profile use work` | `--profile work`, `GITLAB_CLI_PROFILE=work`, or `default_profile` in settings.json |
| `auth profile remove work` | `auth logout --profile work`, then remove the profile from settings.json |
| `context --no-strict` | `context`: it works offline and exits 0 when no one is signed in |
| `--force`, `update --reinstall` | gone; a write runs with `--dry-run`, then `--confirm` |

`--dangerous` is now also needed for `auth login`, `auth logout`, `label delete`, `issue comment delete`, `mr comment delete` and a `repo commit create` that deletes a file.

## Update scripts that read the output

Each call prints one compact JSON document on stdout (`--pretty` indents it). A list from `mr list --project acme/web --fields ref,title,web_url` looks like this:

```json
{"ok":true,"schema_version":"2.0","data":{"count":1,"has_more":false,"items":[{"ref":"acme/web!5","title":"Add retry","web_url":"https://gitlab.example.com/acme/web/-/merge_requests/5"}],"scope":{"limit":20,"project":"acme/web","state":"opened"},"sort":"updated_at_desc","total":1},"meta":{"duration_ms":120,"untrusted":["data.items[].title"]}}
```

| 1.x | 2.0 |
|---|---|
| `schema_version` `"1.0"` | `"2.0"` |
| camelCase keys: `webUrl`, `createdAt`, `defaultBranch` | snake_case: `web_url`, `created_at`, `default_branch` |
| `hasMore`, `limit`, `page`, `all` on lists | `has_more`, `next_offset`, `total` (when GitLab sends it), `scope`, `sort` |
| every field by default | the command's default fields; `--fields +a` adds one, `--fields all` returns all |
| `pathWithNamespace` (projects) | `ref` (`acme/web`) |
| `ref` of a pipeline or job (its branch or tag) | `ref_name`; `ref` is now the pipeline's or job's own ref, `acme/web/-/pipelines/123` |
| a job's `duration` in seconds | `duration_ms` |
| usernames: `carol` | user refs: `@carol` |
| `_untrusted` inside each object | `meta.untrusted`, the paths of text other people wrote |

Every resource carries a `ref` that commands accept back: `acme/web!5`, `acme/web#12`, `acme/web!5/notes/12`, `acme/web/-/jobs/456`, `@carol`. Times are UTC with `Z`; a member's expiry is a date, `expires_on`.

Errors are `{"code", "message", "retryable", "details"}`. `details.dispatch_state` says whether a write reached GitLab, and `details.next_action` says what to do next. New codes include `E_AMBIGUOUS` (a name matched several resources; `details.candidates`), `E_OUTCOME_UNKNOWN` (look the write up with `operation show`), `E_PARTIAL` (part of a batch was applied) and `E_VERIFY_FAILED` (the read-back disagrees).

A confirmed write returns `operation_id`, a `receipt` with what the tool read back (`observed`) and the `result`; a bulk command returns one receipt per item and a summary.

Exit codes keep their numbers: 2 usage or validation, 3 not found, 4 credentials, permission, configuration or scope, 5 confirmation required, 6 conflict or outcome unknown, 7 retry later, 8 timeout. New are 9 (a person must act first) and 130 (interrupted). `pipeline wait` and `job wait` no longer exit 6 when the pipeline or job failed, or 8 when the wait ran out: the status is in the output, and a `wait_ended` notice says it was still running.

## Clean up after 1.x

Once 2.0 works, delete what 1.x left in `~/.gitlab-cli`: `config.json`, `profiles.json`, `confirm.secret`, `confirm-consumed.json` and the `audit/` folder. Also delete the 1.x keyring entry `gitlab-cli` / `credential-master-key` (Windows Credential Manager, macOS Keychain or the Secret Service). 2.0 keeps its own files in the same folder: `settings.json`, `scope.json`, `confirm.key`, `operations/`, `locks/`, `scope-usage.json` and `update-check.json`.
