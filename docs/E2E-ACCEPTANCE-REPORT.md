# gitlab-cli Full-Command E2E Acceptance Report

> **HTML version** (open in browser): [E2E-ACCEPTANCE-REPORT.html](./E2E-ACCEPTANCE-REPORT.html)  
> Regenerate: `.\scripts\generate-e2e-report-html.ps1`

**Acceptance date:** 2026-05-23  
**Environment:** Windows · Docker `gitlab-cli-e2e` + `gitlab-cli-e2e-runner` · http://localhost:8929  
**GitLab version:** CE 17.11.4 (container healthy)

> **Note:** This report was produced on **Windows** using PowerShell scripts. See [E2E.md](./E2E.md) for the Windows-focused setup guide.

---

## 1. Acceptance summary

| Check | Criterion | Result | Evidence |
|-------|-----------|--------|----------|
| Leaf command count | Enumerable via `reference --compact` | **85** | §2.1 |
| Unit test covers all 85 commands | `TestUnit_EveryLeafCommandHasTest` PASS | **Pass** | §2.2 |
| All unit tests green | `go test ./...` | **Pass** | §2.3 |
| Integration: one run per leaf command | `TestAllCommands_EveryLeaf` | **85 PASS + 0 SKIP = 85** | §2.4, §3 |
| Local GitLab connectivity | `doctor --compact` `data.authValid` | **Pass** | §2.5 |

**Overall:** **Accepted.**  
Includes full **pipeline / job** path; `gitlab-cli-e2e-runner` registered with `clone_url=http://gitlab:8929`; **all 85 commands exit 0**.

---

## 2. Execution evidence

### 2.1 Command tree size (85 leaf commands)

```text
> go run ./cmd/gitlab-cli reference --compact | python -c "..."
leaf_commands 85
```

### 2.2 Unit test: every command must have `rootCmd.SetArgs` test

```text
> go test ./cmd -run TestUnit_EveryLeafCommandHasTest -count=1
ok  	github.com/fatecannotbealtered/gitlab-cli/cmd	0.624s
```

### 2.3 Unit test: all packages pass

```text
> go test ./... -count=1
ok  	github.com/fatecannotbealtered/gitlab-cli/cmd	12.019s
ok  	github.com/fatecannotbealtered/gitlab-cli/internal/api	4.171s
ok  	github.com/fatecannotbealtered/gitlab-cli/internal/audit	0.137s
ok  	github.com/fatecannotbealtered/gitlab-cli/internal/config	0.170s
ok  	github.com/fatecannotbealtered/gitlab-cli/internal/gitctx	3.144s
ok  	github.com/fatecannotbealtered/gitlab-cli/internal/output	0.132s
```

### 2.4 Integration test: table-driven, real GitLab API

```text
> go test -tags=integration -v -count=1 -timeout=25m ./e2e/...
--- PASS: TestAllCommands_EveryLeaf (10.34s)
PASS
ok  	github.com/fatecannotbealtered/gitlab-cli/e2e	54.887s
```

Detail CSV (machine-readable, generated locally, not committed): `scripts/e2e-report.csv` (85 rows)

```text
> powershell -File scripts/generate-e2e-report.ps1
Wrote 85 rows to scripts/e2e-report.csv
  PASS: 85
```

Full verbose log (local, not committed): `scripts/e2e-report-verbose.log`

### 2.5 Local GitLab and CLI auth

```text
> docker ps --filter name=gitlab-cli-e2e
gitlab-cli-e2e   Up (healthy)   0.0.0.0:8929->8929/tcp
gitlab-cli-e2e-runner   Up

> go run ./cmd/gitlab-cli doctor --compact
{
  "ok": true,
  "schema_version": "1.0",
  "data": {
    "configExists": true,
    "authValid": true,
    "latencyMs": 84,
    "host": "http://localhost:8929",
    "username": "root",
    "name": "Administrator"
  },
  "meta": {
    "duration_ms": 84
  }
}
```

---

## 3. Per-command integration results (85/85 PASS)

| Result | Count | Notes |
|--------|-------|-------|
| PASS | 85 | Real API / dry-run exit 0 |
| SKIP | 0 | — |
| FAIL | 0 | — |

Includes **pipeline** / **job** subcommands (list, get, jobs, retry, cancel, wait, log, artifacts, etc.) — all PASS.

### 3.1 PASS samples (from verbose log)

```text
--- PASS: TestAllCommands_EveryLeaf/pipeline_list (0.10s)
--- PASS: TestAllCommands_EveryLeaf/pipeline_wait (0.11s)
--- PASS: TestAllCommands_EveryLeaf/job_log (0.11s)
--- PASS: TestAllCommands_EveryLeaf/job_wait (0.16s)
--- PASS: TestAllCommands_EveryLeaf/mr_diff (0.12s)
--- PASS: TestAllCommands_EveryLeaf/repo_file_get (0.20s)
```

Full list of 85: see `scripts/e2e-report-verbose.log`.

---

## 4. Fixes in this acceptance cycle (repeatable runs)

| Issue | Fix |
|-------|-----|
| On Windows, `auth login` integration test wrote real `%USERPROFILE%\.gitlab-cli`, breaking “no config” unit tests | `e2e` sets both `HOME` and `USERPROFILE`; `config.Dir()` prefers `HOME`; unit tests use `isolateConfigHome(t)` |
| Integration subprocess missing command prefix (`auth login`, etc.) | Fixed argument table in `e2e/command_case.go` |
| `reference` leaf paths contained `<iid>` placeholders | `e2e/NormalizeLeafPath` for comparison |
| Runner token invalid after GitLab volume recreate; `clone_url` not applied on Windows | `e2e-runner-register.ps1` verify-first + `-Force` re-register; reliable `clone_url=http://gitlab:8929` in PowerShell |

---

## 5. Reproduce (Windows)

```powershell
# From repository root
.\scripts\e2e-up.ps1 -Wait
.\scripts\e2e-runner-register.ps1
powershell -NoProfile -Command "go test ./... -count=1"
go test -tags=integration -v -count=1 -timeout=25m ./e2e/...
powershell -File .\scripts\generate-e2e-report.ps1
powershell -File .\scripts\generate-e2e-report-html.ps1
```

If GitLab data volume was recreated: `.\scripts\e2e-runner-register.ps1 -Force`

---

## 6. Notes

- Acceptance report HTML/MD are **snapshots** and may be committed; `scripts/e2e-report*.log/csv` are local artifacts (gitignored).
- GitHub push CI runs unit tests only; full E2E requires local Docker (see [E2E.md](./E2E.md)).
