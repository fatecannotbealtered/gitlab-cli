# gitlab-cli

[![CI](https://github.com/fatecannotbealtered/gitlab-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/fatecannotbealtered/gitlab-cli/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | 中文

面向 AI Agent 的 GitLab 命令行工具。覆盖 Merge Request、Issue、Pipeline、Job、Repository、Release、Label、Milestone、CI Variable、Search 等领域，提供严格的机器可读契约，专为 AI Agent 与脚本自动化设计。

Go 编写，单文件静态二进制。兼容 **GitLab.com / 自托管 GitLab / GitLab Dedicated**。

## 为什么再造一个 GitLab CLI？

GitLab 官方有非常优秀的 CLI [glab](https://gitlab.com/gitlab-org/cli)——但 glab 是给"人"用的。`gitlab-cli` **专为 AI Agent 而生**，沿用 [`jira-cli`](https://github.com/fatecannotbealtered/jira-cli) 的设计：

- **强 JSON 契约**——所有命令默认输出扁平、token 高效的 JSON。
- **统一输出格式**——全局 `--format json|text|raw`；`text` 给人读，`raw` 给未包装的 bytes/log/diff；`--json` 保留为 `--format json` 的兼容别名。
- **`--fields key,name,state` 字段投影**——部分命令支持，进一步压缩 JSON 输出。
- **`--dry-run`**——所有写命令支持执行前预览。
- **`--confirm <token>`**——非交互写操作确认（推荐）。
- **`--force`**——跳过确认（Agent-safe 模式下需 `GITLAB_CLI_ALLOW_FORCE=1`）。
- **机器可读错误信封**——含 `errorCode`、`statusCode`、可执行 `hint`。
- **语义化退出码**（`0`/`2`/`3`/`4`/`5`/`6`/`7`/`8`/`9`/`10`）。
- **JSONL 审计日志**——所有写命令记录到 `~/.gitlab-cli/audit/`。
- **`SKILL.md`**——通过 [`npx skills`](https://github.com/vercel-labs/skills) 安装，供兼容的 AI 编程助手识别能力清单。

交互场景请用 glab，脚本与 Agent 编排请用 gitlab-cli。两者用同一个 PAT，互不打架。

## 免责声明

本项目用于个人学习、研究和 AI 工具实验，**不提供 SLA 与商业支持**，**不保证生产可用**。软件按"原样"提供。

## 安装

### 推荐方式

将下面整段复制发给您的 Agent（或自行执行）：

```bash
# 请帮我安装 gitlab-cli，并记住以后操作 GitLab 都使用 gitlab-cli（默认输出 JSON）。
# 安装 CLI
npm install -g @fatecannotbealtered-/gitlab-cli

# 安装 CLI Skill（必需）— 复制到你支持的 skills 目录下
npx skills add fatecannotbealtered/gitlab-cli -y -g

# 登录并验证
gitlab-cli auth login
gitlab-cli doctor
```

### 备选：go install

```bash
go install github.com/fatecannotbealtered/gitlab-cli/cmd/gitlab-cli@latest
```

### 备选：直接下载二进制

到 [GitHub Releases](https://github.com/fatecannotbealtered/gitlab-cli/releases) 下载，加入 PATH。

## 鉴权

`gitlab-cli` 使用 GitLab Personal Access Token（PAT，需要至少 `api` scope）。

### 交互式登录

```bash
gitlab-cli auth login
gitlab-cli doctor       # 验证连通性
gitlab-cli auth status  # 查看当前鉴权来源
gitlab-cli auth logout  # 删除凭据
```

### 非交互式登录（CI / AI Agent）

```bash
gitlab-cli auth login --host https://gitlab.example.com --token <PAT>
```

### 环境变量（推荐）

| 变量 | 说明 |
|---|---|
| `GITLAB_CLI_HOST` | Host URL —— **最高优先级**，与 glab 隔离 |
| `GITLAB_CLI_TOKEN` | PAT —— 最高优先级 |
| `GITLAB_HOST` | Host URL —— 与 glab 共享 |
| `GITLAB_TOKEN` | PAT —— 与 glab 共享 |
| `NO_COLOR` | 禁用彩色输出 |
| `GITLAB_CLI_USER_AGENT` | 自定义 HTTP User-Agent |
| `GITLAB_NO_AUDIT` | 设为 `1` 禁用审计 |
| `GITLAB_AUDIT_RETENTION_MONTHS` | 审计文件保留月数（默认 `3`，`0` 永久保留）|

优先级：`GITLAB_CLI_*` > `GITLAB_*` > active profile > `~/.gitlab-cli/config.json`。

### 生成 PAT

1. GitLab → **User Settings** → **Access Tokens**。
2. 创建至少有 `api` scope 的 token。

## 命令

> 运行 `gitlab-cli reference` 可打印全部命令/子命令/Flag 的结构化 JSON。

### 鉴权与自检

```bash
gitlab-cli auth login [--host URL] [--token PAT]
gitlab-cli auth logout
gitlab-cli auth status
gitlab-cli doctor
gitlab-cli update [--check] [--target-version vX.Y.Z] [--reinstall]
```

### Merge Request / Issue / Pipeline / Job / Repo / Release / Label / Milestone / Variable / Search / User / Project

> 完整 flag 与 Agent 元数据请运行 `gitlab-cli reference`；如需人类可读 Markdown，用 `gitlab-cli reference --format text`。

### 用户与项目

```bash
gitlab-cli user me
gitlab-cli user search --query <q> [--active] [--limit N]
gitlab-cli user get <username>

gitlab-cli project list [--owned] [--membership] [--search <q>] [--visibility <v>]
gitlab-cli project get <id-or-path>
gitlab-cli project members <id-or-path> [--query <q>]
```

### 搜索

```bash
gitlab-cli search projects --query <q>
gitlab-cli search issues   --query <q> [--project <id>]
gitlab-cli search mrs      --query <q> [--project <id>]
gitlab-cli search code     --query <q>  --project <id>    # 代码搜索必须 --project
gitlab-cli search commits  --query <q> [--project <id>]
```

### Merge Request

```bash
gitlab-cli mr list      --project <id> [--state opened|closed|merged|all]
gitlab-cli mr get       --project <id> <iid> [--fields ...]
gitlab-cli mr current                            # 自动用 git 上下文
gitlab-cli mr create    --project <id> --title <t> --source-branch <s> --target-branch main
gitlab-cli mr create    --auto [--target-branch main] [--title <t>] [--draft]
gitlab-cli mr update    --project <id> <iid> [--title ...] [--add-labels ...]
gitlab-cli mr merge     --project <id> <iid> [--squash] [--should-remove-source-branch] [--confirm <token>]
gitlab-cli mr close     --project <id> <iid> [--confirm <token>]
gitlab-cli mr reopen    --project <id> <iid> [--confirm <token>]
gitlab-cli mr approve   --project <id> <iid>
gitlab-cli mr unapprove --project <id> <iid>
gitlab-cli mr diff      --project <id> <iid>      # 默认 JSON；用 --format raw 输出原始 unified diff
gitlab-cli mr comment add    --project <id> <iid> --body <text>
gitlab-cli mr comment list   --project <id> <iid>
gitlab-cli mr comment delete --project <id> <iid> --note-id <id> [--confirm <token>]
```

### Issue

```bash
gitlab-cli issue list      --project <id> [--state ...] [--assignee <u>] [--label l1,l2]
gitlab-cli issue get       <iid> --project <id>
gitlab-cli issue create    --project <id> --title <t> [--description <d>] [--label l1,l2]
gitlab-cli issue update    <iid> --project <id> [--add-labels ...] [--remove-labels ...]
gitlab-cli issue close     <iid> --project <id> [--confirm <token>]
gitlab-cli issue reopen    <iid> --project <id> [--confirm <token>]
gitlab-cli issue assign    <iid> <username|me> --project <id>
gitlab-cli issue label     <iid> --project <id> --add l1,l2 --remove l3,l4
gitlab-cli issue comment add    <iid> --project <id> --body <t>
gitlab-cli issue comment list   <iid> --project <id>
gitlab-cli issue comment delete <iid> --project <id> --note-id <id> [--confirm <token>]
```

### Label & Milestone

```bash
gitlab-cli label list   --project <id>
gitlab-cli label create --project <id> --name <n> --color <#hex|named> [--priority N]
gitlab-cli label update --project <id> --label-id N [--name ...] [--color ...]
gitlab-cli label delete --project <id> --label-id N [--confirm <token>]

gitlab-cli milestone list   --project <id> [--state active|closed|all]
gitlab-cli milestone get    --project <id> --milestone-id N
gitlab-cli milestone create --project <id> --title <t> [--due-date YYYY-MM-DD]
gitlab-cli milestone update --project <id> --milestone-id N [--title ...] [--state-event close|activate]
gitlab-cli milestone close  --project <id> --milestone-id N [--confirm <token>]
```

### 工作流上下文

```bash
gitlab-cli context              # 一键获取当前 git + GitLab 上下文（AI Agent 编排起点）
gitlab-cli context
```

### Pipeline / Job

```bash
gitlab-cli pipeline list    --project <id> [--ref <b>] [--status ...]
gitlab-cli pipeline get     --project <id> <pipeline_id>
gitlab-cli pipeline current
gitlab-cli pipeline create  --project <id> --ref <b> [--variable KEY=VAL]... [--confirm <token>]
gitlab-cli pipeline retry   --project <id> <pipeline_id> [--confirm <token>]
gitlab-cli pipeline cancel  --project <id> <pipeline_id> [--confirm <token>]
gitlab-cli pipeline jobs    --project <id> <pipeline_id> [--scope ...]
gitlab-cli pipeline wait    --project <id> <pipeline_id> [--timeout 300] [--interval 10]

gitlab-cli job get       --project <id> <job_id>
gitlab-cli job log       --project <id> <job_id>           # 默认 JSON；用 --format raw 输出原始 trace
gitlab-cli job log       --project <id> <job_id> --follow  # 默认 JSON buffer；用 --format text/raw 流式输出
gitlab-cli job retry     --project <id> <job_id> [--confirm <token>]
gitlab-cli job cancel    --project <id> <job_id> [--confirm <token>]
gitlab-cli job artifacts --project <id> <job_id> --output artifacts.zip
gitlab-cli job wait      --project <id> <job_id> [--timeout 300] [--interval 5]
```

### Repository / Release

```bash
gitlab-cli repo file get    --project <id> --path <p> [--ref <b>] [--output <path>]
gitlab-cli repo file create --project <id> --path <p> --branch <b> --content ... --commit-message <m>
gitlab-cli repo file update --project <id> --path <p> --branch <b> --content ... --commit-message <m>
gitlab-cli repo file delete --project <id> --path <p> --branch <b> --commit-message <m> [--confirm <token>]

gitlab-cli repo branch list   --project <id> [--search <q>]
gitlab-cli repo branch create --project <id> --name <n> --ref <source>
gitlab-cli repo branch delete --project <id> --name <n> [--confirm <token>]

gitlab-cli repo commit list --project <id> [--ref-name <b>] [--since ...] [--until ...] [--path <p>]
gitlab-cli repo commit get  --project <id> <sha>
gitlab-cli repo tree --project <id> [--path <p>] [--ref <b>] [--recursive]

gitlab-cli release list   --project <id>
gitlab-cli release get    --project <id> --tag <tag>
gitlab-cli release create --project <id> --tag <tag> --name <n> [--description <d>] [--ref <b>] [--milestone m1,m2]
gitlab-cli release update --project <id> --tag <tag> [--name ...] [--description ...]
gitlab-cli release delete --project <id> --tag <tag> [--confirm <token>]
```

### CI/CD 变量

```bash
gitlab-cli variable list   --project <id>                                     # 默认脱敏 value
gitlab-cli variable get    --project <id> --key <k> [--filter env_scope=<scope>]
gitlab-cli variable create --project <id> --key <k> --value <v> [--type env_var|file] [--protected] [--masked] [--env-scope <s>]
gitlab-cli variable update --project <id> --key <k> [--value <v>] [--protected] [--masked]
gitlab-cli variable delete --project <id> --key <k> [--filter env_scope=<scope>] [--confirm <token>]
```

`variable` 子命令可加 `--show-values` 在 JSON 输出中包含密钥明文（默认脱敏）。

> 完整 flag 清单运行 `gitlab-cli reference` 自动列出（默认生成结构化 JSON，给 AI Agent 解析用）。

## JSON 输出

默认输出 JSON。需要人类可读输出时用 `--format text`；需要未包装 bytes/log/diff 时用 `--format raw`（仅部分命令支持）。`--json` 保留为 `--format json` 的兼容别名。

```bash
# 默认扁平 JSON
gitlab-cli auth status
gitlab-cli doctor

# 仅选择需要的字段（部分命令支持 --fields，如 mr get、issue list）
gitlab-cli mr get --project <id> <iid> --fields iid,title,state

# 抑制 text 辅助输出
gitlab-cli doctor --quiet

# 紧凑 JSON（更少 token）
gitlab-cli doctor --compact
```

错误信封含错误码与可执行提示：

```json
{
  "error": "GitLab API error 404: 404 Project Not Found",
  "statusCode": 404,
  "errorCode": "NOT_FOUND",
  "hint": "Verify the resource (project path, IID, ID) exists and you have permission to view it"
}
```

## 全局 Flag

| Flag | 含义 |
|---|---|
| `--format json|text|raw` | 输出格式。默认 `json`；`text` 为人类可读；`raw` 为未包装 bytes/log/diff（仅部分命令支持） |
| `--json` | `--format json` 的兼容别名；不要和 `--format text/raw` 同时使用 |
| `--compact` | 紧凑 JSON（无缩进，仅影响 `--format json`） |
| `--quiet` | 抑制 text 辅助输出 |
| `--fields a,b,c` | 从扁平 JSON 投影字段（**仅部分命令**支持，且只支持 JSON） |
| `--dry-run` | 预览写命令而不执行 |
| `--confirm <token>` | 非交互确认（写操作推荐；token 见 `--dry-run` 或错误提示） |
| `--force` | 跳过确认（Agent-safe 模式下需 `GITLAB_CLI_ALLOW_FORCE=1`） |

## 退出码

| 退出码 | 含义 |
|---|---|
| 0 | 成功 |
| 2 | 参数错误 |
| 3 | 鉴权错误（401） |
| 4 | 资源不存在 |
| 5 | 权限不足 |
| 6 | 限流 |
| 7 | 网络/服务端错误 |
| 8 | 超时（`pipeline wait` / `job wait`） |
| 9 | CI/CD 失败（`pipeline wait` / `job wait` 以 failed/canceled/skipped 结束） |
| 10 | 用户取消 / 缺少确认（使用 `--confirm <token>` 非交互确认） |

## Agent-safe 模式（默认开启）

- `--force` 需设置 `GITLAB_CLI_ALLOW_FORCE=1`（用户明确授权后）
- `--show-values` 需设置 `GITLAB_CLI_ALLOW_SHOW_VALUES=1`
- 写操作优先使用 `--confirm <token>` 而非 `--force`
- 设置 `GITLAB_CLI_AGENT_SAFE=0` 可关闭全部限制

自更新也遵循同样流程：

```bash
gitlab-cli update --check
gitlab-cli update --dry-run
gitlab-cli update --confirm <targetVersion>
```

## 多 Profile

```bash
gitlab-cli auth login --host URL --profile work
gitlab-cli auth profile list
gitlab-cli auth profile use work
```

## 安全

- 凭据保存在 `~/.gitlab-cli/config.json`（文件 `0600`，目录 `0700`）。
- 所有写命令以 JSONL 形式记录在 `~/.gitlab-cli/audit/`。
- 含 token 的 flag（`--token`、`-t`、`--private-token`、`--oauth-token`、`--job-token`）以及密钥值（`--value`、`--variable`）在审计日志中自动脱敏。
- 默认强制 HTTPS，仅本地开发场景允许 `http://`。

漏洞反馈请见 [SECURITY.md](SECURITY.md)。

本地端到端测试见 [docs/E2E.md](docs/E2E.md)（以 Windows + PowerShell 为主；Linux/macOS 见 Non-Windows 小节）。

## 贡献

欢迎贡献，详见 [CONTRIBUTING.md](CONTRIBUTING.md)。Release 记录见 [CHANGELOG.md](CHANGELOG.md)。

## 许可

MIT © fatecannotbealtered
