# gitlab-cli Live-Smoke Evidence

Aggregated pass/fail evidence for safe live-smoke runs. No real data, tokens, or
secrets are recorded here — only command, method, and result.

`release_readiness.live_smoke_status` remains **verified** (see also
`docs/E2E-ACCEPTANCE-REPORT.md`).

---

## 2026-06-15 — batch commands (CLI-SPEC §15) live-smoke

**Environment:** Windows · Docker `gitlab-cli-e2e` · GitLab EE 18.8.10 ·
http://localhost:8929 · root PAT (api scope), minted out-of-band via
`gitlab-rails runner`, revoked after the run. Disposable project
`root/smoke-batch-0615` created for the run and destroyed afterward; no other
data touched.

**Method:** every batch command runs the §15 gate path
(`--dry-run` → inspect `confirm_token` → `--confirm <token>`). Live writes were
restricted to a small reversible batch (2–3 objects) and reverted immediately.
Dangerous/irreversible batches were dry-run only during the night-time run; the
`mr bulk merge` live run below was completed during a daytime, attended session
on the same isolated Docker instance (disposable project, minted PAT revoked).

### Dry-run / envelope-shape (all new batch commands)

| Command | Result | Method |
|---|---|---|
| `repo commit create` | PASS | dry-run preview: `total` + ordered `targets` + per-target `changes` + one `confirm_token` |
| `issue bulk close` | PASS | dry-run envelope shape |
| `issue bulk reopen` | PASS | dry-run envelope shape |
| `issue bulk update` | PASS | dry-run envelope shape |
| `issue bulk label` | PASS | dry-run envelope shape (add/remove in preview scope) |
| `issue bulk assign` | PASS | dry-run envelope shape (assignee in preview scope) |
| `issue bulk comment` | PASS | dry-run envelope shape |
| `mr bulk approve` | PASS | dry-run envelope shape |
| `mr bulk close` | PASS | dry-run envelope shape |
| `mr bulk update` | PASS | dry-run envelope shape |
| `mr bulk merge` | PASS | dangerous gate: no `--dangerous` → `E_CONFIRMATION_REQUIRED` (exit 5); with `--dangerous` → preview emitted |
| `variable bulk-import` | PASS | dangerous gate: no `--dangerous` → `E_CONFIRMATION_REQUIRED` (exit 5); with `--dangerous` → preview emitted |

### Live reversible writes (small batch, reverted)

| Command | Result | Method |
|---|---|---|
| `repo commit create` (class A, atomic) | PASS — live | 2-action atomic commit; `items[]` all ok, `summary {2,2,0}`, real commit returned |
| `issue bulk close` (3 iids) | PASS — live | 3/3 succeeded; reverted via `issue bulk reopen` (3/3) |
| `issue bulk label` add/remove | PASS — live | add then remove (revert) |
| `issue bulk comment` | PASS — live | 1 note created |
| `issue bulk assign me` | PASS — live | 2/2 assigned |
| `variable bulk-import` (create) | PASS — live | 2 keys `created`; `--dangerous` gate exercised |
| `variable bulk-import` (re-run) | PASS — live | same 2 keys `updated` — create-or-update idempotency proven |
| `mr bulk update` add/remove label | PASS — live | add then remove (revert) |
| `mr bulk approve` | PASS — live | 1/1 approved |
| `mr bulk close` | PASS — live | 1/1 closed; reverted via `mr reopen` |
| `mr bulk merge` (2 mergeable MRs) | PASS — live | `--dangerous` two-step gate; 2/2 merged, `items[]` per-target ok, `summary {2,2,0}`, real merge commits returned |

### Dangerous / irreversible — `mr bulk merge` now real-machine executed

`mr bulk merge` was upgraded from dry-run-only to **live PASS** on a disposable
project (3 mergeable MRs + 1 deliberately-conflicting MR; project destroyed and
the minted PAT revoked afterward).

| Scenario | Result | Method |
|---|---|---|
| Missing `--dangerous` gate | PASS — live | dry-run without `--dangerous` → `E_CONFIRMATION_REQUIRED` (exit 5) |
| `--dangerous --dry-run` preview | PASS — live | preview with `total` + ordered `targets` + per-target `changes` + one `confirm_token` |
| Live batch merge (2 mergeable) | PASS — live | `--dangerous --confirm <token>` → 2/2 merged, `items[]` per-target ok, `summary {2,2,0}`; both MRs `state=merged` with real merge commits |
| Single-use confirm token replay | PASS — live | replaying the consumed merge token → `E_CONFLICT` (exit 6) |
| Partial-failure aggregation | PASS — live | `ids 4,3` (4 = conflicting MR) with `--continue-on-error=true` → item 4 `E_VALIDATION`, item 3 merged; `summary {2,1,1}`, top-level `ok:true` (failed item did not block the rest) |

### Shared batch contract points (live)

| Contract point (CLI-SPEC §15) | Result | Method |
|---|---|---|
| Single-use confirm token | PASS — live | replaying a consumed token → `E_CONFLICT` (re-verified on `mr bulk merge`) |
| Partial-failure aggregation | PASS — live | `ids 1,999` → item 1 ok, item 999 `E_NOT_FOUND retryable:false`; `summary {2,1,1}`; top-level `ok:true` (re-verified on `mr bulk merge` with a real conflict) |
| `--continue-on-error=false` stop + skipped tail | PASS — live | `ids 999,1,2` stopped at first failure; `skipped:["1","2"]` reported for resume |
| `--dangerous` two-step gate | PASS — live | enforced on `mr bulk merge` and `variable bulk-import` in both dry-run and confirm steps |

**Cleanup:** smoke project destroyed; minted PAT revoked. No secrets written to
any file or log.

**release_readiness:** unchanged — `live_smoke_status: verified` (this run
reinforces existing evidence; no version or readiness level changed).
