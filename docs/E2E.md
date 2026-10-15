# Live end-to-end suite

`e2e/` runs gitlab-cli against a real GitLab and records what happened: every call's argv, exit code and stdout, the
contract check of that stdout, and for each confirmed write a read-back made outside the tool. The record goes to
`docs/evidence/`, and commands that passed there may be `verified` (CLI-027, VER-005). The suite covers the built-ins,
every command group with its reads and at least one write, the write gate (bare call, reused and mutated tokens,
state drift, no scope, a spent budget, the idempotent replay, `--profile` switched at confirm, `--dangerous`) and a
lost reply through a fault proxy.

The current record is [`docs/evidence/2026-10-14-gitlab-ee-18.8.md`](evidence/2026-10-14-gitlab-ee-18.8.md): 166
calls on GitLab EE 18.8, all as expected, 81 commands verified.

## What you need

- Docker with Compose v2, about 8 GB of memory and 10 GB of disk for GitLab. The first start takes 5 to 15 minutes.
- Go (the version in `go.mod`; the suite builds the binary from this checkout) and Python 3.10 or later (standard
  library only).
- An OS keyring the tool can write: `auth login` and `auth logout` are part of the run. The Windows Credential
  Manager works; the suite has run on Windows. On Linux the tool needs a Secret Service on the session bus.
- Optional: a checkout of [ai-native-cli-spec](https://github.com/fatecannotbealtered/ai-native-cli-spec) at the
  tag in `.agent/SPEC_VERSION`, for its Python validator and its fault proxy.

## 1. Start GitLab

`docker-compose.e2e.yml` runs GitLab EE 18.8.10 (no license, Free features) at `http://localhost:8929` and a
`gitlab-runner`. On Windows:

```powershell
.\scripts\e2e-up.ps1 -Wait
```

This starts both containers, waits for the sign-in page, creates a token for `root` (scope `api`) in
`scripts/e2e.local.env` (gitignored; the token is not printed) and registers an instance runner with the shell
executor, which the pipeline and job phase needs. `-DataRoot D:\gitlab-cli-e2e` keeps GitLab's data in folders
instead of Docker volumes. Stop GitLab with `.\scripts\e2e-down.ps1`, or `.\scripts\e2e-down.ps1 -Volumes` to delete
its data.

On other systems, run `docker compose -f docker-compose.e2e.yml up -d` and wait until
`docker inspect --format '{{.State.Health.Status}}' gitlab-cli-e2e` says `healthy`. Create a token for `root` with
scope `api` in the web UI, then register the runner as `scripts/e2e-runner-register.ps1` does: create an instance
runner with `POST /api/v4/user/runners`, run `gitlab-runner register --executor shell --url http://gitlab:8929` in
the `gitlab-cli-e2e-runner` container with the token it returned, and set `clone_url = "http://gitlab:8929"` in the
runner's `config.toml`.

If something goes wrong:

- The web sign-in fails: `.\scripts\e2e-reset-password.ps1`.
- Pipelines stay pending after GitLab's data was reset: `.\scripts\e2e-runner-register.ps1 -Force`.
- To check the runner by itself: `.\scripts\e2e-demo-ci.ps1` runs one pipeline in a new project `ci-demo-<time>`
  (outside any sandbox; delete it afterwards).
- Docker Desktop on another drive: `.\scripts\install-docker-d.ps1`.

## 2. Accounts and tokens

The suite uses three tokens, all with scope `api`. It reads each from the environment variable below and nowhere
else: a token is never printed, logged, written to a file or put on a command line.

| Variable | Account and use |
|---|---|
| `GITLAB_E2E_ADMIN_TOKEN` | An administrator, such as `root`. Only the harness uses it: setup, read-backs, the edit between dry-run and confirm in the gate phase, and teardown. The tool never gets it. |
| `GITLAB_E2E_OWNER_TOKEN` | The account named by `GITLAB_E2E_OWNER` (default `eval-bot`). The tool signs in as it; it owns the sandbox group. |
| `GITLAB_E2E_REVIEWER_TOKEN` | The account named by `GITLAB_E2E_REVIEWER` (default `bob`). Developer in the sandbox; approves the merge request. |

The owner and the reviewer are ordinary accounts. In PowerShell, this loads the `root` token that `e2e-wait.ps1`
wrote, creates the two accounts if they do not exist, and gives each a new token for 7 days, without printing any
token:

```powershell
Get-Content scripts\e2e.local.env | ForEach-Object {
  if ($_ -match '^GITLAB_CLI_TOKEN=(.+)$') { $env:GITLAB_E2E_ADMIN_TOKEN = $Matches[1].Trim() } }
$api = "http://localhost:8929/api/v4"
$h = @{ "PRIVATE-TOKEN" = $env:GITLAB_E2E_ADMIN_TOKEN }
function Get-E2EToken([string]$name) {
  $u = @(Invoke-RestMethod "$api/users?username=$name" -Headers $h)[0]
  if (-not $u) {
    $u = Invoke-RestMethod -Method Post "$api/users" -Headers $h -Body @{ username = $name; name = $name;
      email = "$name@example.com"; password = "$([guid]::NewGuid())-Aa1!"; skip_confirmation = "true" }
  }
  (Invoke-RestMethod -Method Post "$api/users/$($u.id)/personal_access_tokens" -Headers $h -Body @{
    name = "gitlab-cli-e2e"; "scopes[]" = "api"; expires_at = (Get-Date).AddDays(7).ToString("yyyy-MM-dd") }).token
}
$env:GITLAB_E2E_OWNER_TOKEN = Get-E2EToken "eval-bot"
$env:GITLAB_E2E_REVIEWER_TOKEN = Get-E2EToken "bob"
```

In the web UI the same is Admin Area, Users, the user, Impersonation Tokens.

## 3. Run the suite

```powershell
$env:GITLAB_E2E_RUN_DIR = "$HOME\gitlab-cli-e2e\run-1"   # a new folder outside the repository
$env:GITLAB_E2E_VALIDATOR = "<spec checkout>\template\python\aicli\validate.py"     # optional
$env:GITLAB_E2E_FAULTPROXY = "<spec checkout>\tools\agent-eval\harness\faultproxy.py"   # optional
python e2e/scenario.py run
```

`run` is setup, every phase in order, then teardown, which also runs after a failure. The phases take about ten
minutes, most of it waiting for pipelines. `python e2e/scenario.py --help` lists the phases and the variables;
`python e2e/scenario.py setup`, then phases by name, then `teardown` runs the same in steps (each phase builds on
the earlier ones). `python e2e/live.py --as owner -- <arguments>` runs one unrecorded call in the same isolation.

| Variable | Default | Meaning |
|---|---|---|
| `GITLAB_E2E_URL` | `http://localhost:8929` | GitLab's URL |
| `GITLAB_E2E_RUN_DIR` | (required) | The run directory, outside the repository: the binary, the isolated home, the records |
| `GITLAB_E2E_OWNER`, `GITLAB_E2E_REVIEWER` | `eval-bot`, `bob` | The two accounts (section 2) |
| `GITLAB_E2E_GROUP` | `gitlab-cli-e2e` | The sandbox group setup creates and teardown deletes |
| `GITLAB_E2E_PROFILE` | `e2e` | The tool profile `auth login` writes to the OS keyring and `auth logout` removes |
| `GITLAB_E2E_BIN` | built from this checkout | The gitlab-cli binary to test |
| `GITLAB_E2E_VALIDATOR` | ContractLite | The spec's `template/python/aicli/validate.py`. Without it, `e2e/live.py` checks the envelope, error and exit-code rules of `contract/contract.json` only; use the spec's validator for a run you record. |
| `GITLAB_E2E_FAULTPROXY` | not set | The spec's `tools/agent-eval/harness/faultproxy.py`; without it the lost-reply phase is skipped |
| `GITLAB_E2E_GRANTED_BY` | `the person who ran the live suite` | `granted_by` of the run's scope file |
| `GITLAB_E2E_EVIDENCE_NAME` | `<date>-gitlab-<ce\|ee>-<major.minor>` | The record's file name |

### What the run writes

- **GitLab: only the sandbox.** Setup checks that each token belongs to the account it is named for, then creates
  the group with the admin API: the owner as Owner, the reviewer as Developer, and a second project named `sandbox`
  in the subgroup `twin`, so a bare project name is ambiguous. The tool creates `<group>/sandbox` itself and works
  there. The run's scope file grants the tool that group and its own keyring profile, nothing else; anything else
  the run touches it only reads.
- **The tool runs isolated.** `USERPROFILE`, `HOME`, `APPDATA`, `LOCALAPPDATA` and `TEMP` point into the run
  directory, and `GITLAB_CLI_STATE_DIR` at a folder inside it. The only thing outside the run directory is the
  keyring entry `gitlab-cli` / `profile:<profile>`; the auth phase removes it, and so does teardown when a run
  stopped before that.
- **Teardown** deletes only the group this run's setup created: its projects for good, then the group. GitLab 18
  keeps a deleted top-level group until its deletion delay ends, renamed `<group>-deletion_scheduled-<id>`, so the
  name is free again at once. It removes the run's isolated home and scratch files and keeps `out/` (the records)
  and `bin/`.

## 4. Record the evidence

```powershell
python e2e/build_evidence.py      # docs/evidence/<name>.json, and <run>/out/verdict.json
python e2e/build_md.py            # after teardown: docs/evidence/<name>.md
python e2e/set_maturity.py --dry  # which commands it would raise to verified
```

`build_evidence.py` writes the JSON record from `<run>/out`: every call with its argv, exit code, stdout, duration,
contract check and read-back, the GitLab version, the scope, the secret scan and the fault proxy's log. It refuses
to write if any token or CI/CD variable value appears, and it writes paths inside the run directory as `<run>`.
`build_md.py` adds the finish time and the cleanup report to the JSON and writes the Markdown twin, one section per
command group. Put what the run found (tool defects, upstream quirks) in `<run>/notes.md`, as a numbered list,
before you run it: that becomes the record's "Defects and observations".

`liveEvidence(section)` in `cmd/evidence.go` cites the record that `liveRecord` names. A command is `verified` only
when it passed there. For a new record, point `liveRecord` at it, then run `python e2e/set_maturity.py`: it raises
each experimental command that passed to `aicli.Verified` with `liveEvidence("<section>")`, and lists the commands
it cannot edit (two commands sharing one RunE, a Meta a helper builds) for a manual edit. It edits nothing while
`liveRecord` names another record, and then lists the verified commands the new run did not pass, which would lose
their evidence. It finds each command's Meta through `go run ./e2e/maturitymap`. Then run `gofmt -w cmd` and
`go test ./cmd/`.

## CI

`.github/workflows/integration.yml` runs by manual dispatch only. It builds the binary and runs the suite's built-ins
phase (`--version`, `context`, `reference`, `changelog`) through the harness with the spec's validator, which needs
no GitLab. The live job is commented out: it needs a GitLab in Docker with a registered runner, the three tokens as
repository secrets, and an OS keyring the tool can write, and the suite has only run on Windows so far. The
workflow file says what enabling it takes.
