# gitlab-cli Full-Command E2E Acceptance Report

**Acceptance date:** 2026-06-12
**Environment:** Windows · Docker `gitlab-cli-e2e` + `gitlab-cli-e2e-runner` · http://localhost:8929
**GitLab version:** EE 18.8.10 (container healthy; previous acceptance was CE 17.11.4 — this run re-validates across a major upstream version)
**Suite:** `go test -tags=integration ./e2e/...` (90 leaf commands enumerated live from `reference`)

---

## 1. Acceptance summary

| Check | Criterion | Result |
|-------|-----------|--------|
| Leaf command count | Enumerated live via `reference --json` (envelope `data.commands`) | **90** |
| Enumeration guard | `TestAllCommands_EveryLeaf` cases == reference leaves, fails on 0 | **Pass (90 == 90)** |
| Guard-of-the-guard | `TestLeafPathsParsesEnvelope` (>= 80 leaves or fail) | **Pass** |
| All leaf commands | One integration case per leaf against live GitLab | **89 PASS / 1 SKIP / 0 FAIL** |
| Skipped | `update` (queries GitHub Releases; covered by unit tests) | by design |
| Wall time | Bootstrap (project + push + pipeline) + 90 cases | **70 s** |

Write-type commands exercise the `--dry-run` contract path inside the suite
(confirm token issued, exit 0). The full mutating cycle is covered by the
recorded manual smoke below.

## 2. Recorded manual live smoke (same date, same instance)

### Credential-at-rest: keyring master key (commit 98354c8)

| Step | Result |
|---|---|
| `auth login` requires dry-run → confirm | PASS (`E_CONFIRMATION_REQUIRED`, exit 5 without token) |
| Login persists zero plaintext: token absent from all files under `~/.gitlab-cli` | PASS (KDF marker `keyring-master-key-v1`) |
| Cross-process recovery: new process, no env token → `auth status` configured, `project list` hits live API | PASS |
| `context` reports `credentials.storage: keyring`, `encrypted_at_rest: true` | PASS |
| `auth logout` (dry-run → confirm) clears profile and keyring entry | PASS (`configured: false` after) |

### Write confirmation chain (HMAC tokens, commit ec28f78)

| Step | Result |
|---|---|
| Write without confirm | PASS (`E_CONFIRMATION_REQUIRED`) |
| `issue create --dry-run` issues `ct_*` token | PASS |
| Confirm in a separate process (same HOME) creates the issue; `_untrusted` tags present | PASS |
| Tampered token rejected | PASS (`E_CONFLICT`) |

### Error taxonomy

| Path | Result |
|---|---|
| No credentials | `E_AUTH`, envelope on stdout |
| Nonexistent project | `E_NOT_FOUND`, retryable false |
| Invalid token | `E_AUTH` |

## 3. Defects found by this acceptance run (all fixed)

1. **e2e leaf enumeration parsed the wrong JSON level.** `LeafPaths()` read
   `commands` at the top level; after the envelope unification the tree lives
   under `data.commands`, so it enumerated **0 leaves**. The rewritten guard
   (cases != leaves → fail) caught it; the previous guard would have passed
   silently. Consequence: in the first run of this acceptance, `t.Fatalf`
   fired **before** any sub-test, meaning zero commands were actually executed
   while the suite consumed 13 minutes of bootstrap.
2. **Four new leaves had no integration case**: `auth profile list/use/remove`
   (keyring/profiles work) and `changelog` (FCC work). Added; the `auth login`
   case also moved to the dry-run contract path (bare invocation now correctly
   exits 5).
3. **Runner lost `clone_url`** after re-registration, so every pipeline failed
   with `Failed to connect to localhost:8929` inside the runner container and
   bootstrap waited out its 20-minute deadline. Restored
   `clone_url = "http://gitlab:8929"`; bootstrap dropped from a 20-minute
   timeout to under a minute.

## 4. Reproduce

```bash
# GitLab + runner up (see docs/E2E.md), then:
export GITLAB_CLI_HOST=http://localhost:8929
export GITLAB_CLI_TOKEN=<root PAT, api scope>
go test -tags=integration -count=1 -timeout=25m -v ./e2e/...
```

The runner's `config.toml` must contain `clone_url = "http://gitlab:8929"`
under `[[runners]]` or pipeline/job cases will fail to fetch sources.
