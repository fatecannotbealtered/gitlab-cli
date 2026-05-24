---
name: gitlab-cli
description: GitLab CLI for AI Agents. Use --json. Read reference/*.md for the module you need — do not load the whole skill upfront.
metadata: {"openclaw":{"emoji":"🦊","requires":{"bins":["gitlab-cli"]}}}
---

# gitlab-cli

Terminal control of GitLab (MR, Issue, CI, repo, release, variables). **GitLab.com / self-managed / Dedicated**.

> CLI: `npm install -g @fatecannotbealtered-/gitlab-cli`
>
> Skill: `npx skills add fatecannotbealtered/gitlab-cli -y -g`

## How to use this skill (progressive disclosure)

1. **Always start here** — run bootstrap commands below.
2. **Open only the reference doc that matches the user's task** (see index).
3. **For exact flags in the installed version** — run `gitlab-cli reference --json --compact`.

Do **not** read every file under `reference/` unless the task spans multiple domains.

## Bootstrap (every session)

```bash
# Prefer env vars over --token on the command line
# export GITLAB_CLI_HOST=https://gitlab.example.com
# export GITLAB_CLI_TOKEN=<PAT>

gitlab-cli context --json --compact      # who/where/project; exit 3 if not authed (--no-strict to override)
gitlab-cli doctor --json --compact       # auth + latency
```

First-time setup: ask user for GitLab URL + PAT (`api` scope), then `gitlab-cli auth login --host <URL> --profile default` (token via env recommended).

## Agent defaults

| Rule | Detail |
|------|--------|
| Output | Always `--json --compact`; add `--quiet` when piping |
| Writes | `--dry-run --json` first, then `--confirm <token>` (see error message for token) |
| Force | Avoid `--force`; needs `GITLAB_CLI_ALLOW_FORCE=1` in agent-safe mode |
| Secrets | Never `--show-values` unless user asks + `GITLAB_CLI_ALLOW_SHOW_VALUES=1` |
| Discovery | `gitlab-cli reference --json` for `write`, `requiresConfirmation`, `riskLevel` |

Full contracts (exit codes, error JSON, list envelope, audit): **[reference/contracts.md](reference/contracts.md)**

## Reference index

| User intent | Read this |
|-------------|-----------|
| 登录 / 多实例 / 自检 | [reference/bootstrap.md](reference/bootstrap.md) |
| 合并代码 / Review / MR 评论 | [reference/mr.md](reference/mr.md) |
| Issue / Bug / 任务 / 评论 | [reference/issue.md](reference/issue.md) |
| CI 流水线 / Job 日志 / 等构建 | [reference/ci.md](reference/ci.md) |
| 分支 / 文件 / 提交 / 目录 | [reference/repo.md](reference/repo.md) |
| Release 发布 | [reference/release.md](reference/release.md) |
| Label / Milestone | [reference/label-milestone.md](reference/label-milestone.md) |
| CI/CD 变量 / 密钥 | [reference/variable.md](reference/variable.md) |
| 搜项目 / 搜代码 / 成员 / 用户 | [reference/discovery.md](reference/discovery.md) |
| 全局 flag / 退出码 / JSON 错误 | [reference/contracts.md](reference/contracts.md) |

## Quick task → command

| Task | Command |
|------|---------|
| List open MRs | `gitlab-cli mr list --project G --json --compact` |
| Merge MR | `gitlab-cli mr merge --project G 42 --confirm 42 --json` |
| Comment on MR | `gitlab-cli mr comment add --project G 42 --body "..."` |
| Wait for CI | `gitlab-cli pipeline wait --project G ID --timeout 600 --json` |
| Job log | `gitlab-cli job log --project G JOB_ID` |

## vs glab

- **glab** — human terminal UX
- **gitlab-cli** — agents: flat JSON, semantic exit codes, `--dry-run`, audit log

Both can share `GITLAB_TOKEN`; prefer `GITLAB_CLI_*` to isolate.
