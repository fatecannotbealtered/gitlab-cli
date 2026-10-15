# 为 gitlab-cli 贡献代码

*[English](CONTRIBUTING.md) | 中文*

感谢你改进 **gitlab-cli**，一个为 AI Agent 设计的 GitLab CLI。本文介绍怎样构建、测试和提交改动。

> 这是一个为 AI 工具实验而分享的业余项目；维护者不提供商业支持，也不保证可用于生产环境。

## 构建的做法写在规范里

本仓库是一个 **AI 原生 CLI 工具**：主要调用方是 AI Agent。实现或修改功能之前，先读：

- **[AGENTS_zh.md](AGENTS_zh.md)**：入口，以及每次改动都要遵守的规则。
- **[`.agent/AGENT_zh.md`](.agent/AGENT_zh.md)**：操作手册，包括工作流、硬性约束和自检。
- **[`.agent/CLI-SPEC_zh.md`](.agent/CLI-SPEC_zh.md)**：机器接口：输出、错误、命令模型、命令发现、读取、写入检查（write gate）和自更新（`CLI-xxx`）。
- **[`.agent/SEC-SPEC_zh.md`](.agent/SEC-SPEC_zh.md)**：风险等级、不可信内容、scope 文件、密钥、状态存储和供应链（`SEC-xxx`）。
- **[`.agent/SKILL-SPEC_zh.md`](.agent/SKILL-SPEC_zh.md)**：随包附带的 Agent Skill（`SKL-xxx`）。
- **[`.agent/VERIFY-SPEC_zh.md`](.agent/VERIFY-SPEC_zh.md)**：符合性测试套件、测试、成熟度与证据，以及 Agent 评测（`VER-xxx`）。

这些规范、`contract/` 和 aicli 库（`internal/aicli/`）是按 `.agent/SPEC_VERSION` 里的标签，从 [ai-native-cli-spec](https://github.com/fatecannotbealtered/ai-native-cli-spec) 复制过来的副本。它们优先于你的默认习惯，在本仓库里只读：副本和固定版本的规范不一致时，`node scripts/check-spec.js` 会让 CI 失败。需要改它们，就到规范仓库提 issue 或 pull request；新规范发布后，更新 `.agent/SPEC_VERSION`，再运行 `node scripts/sync-spec.js`（REPO-063）。不用这个库、自己实现输出、错误或写入检查的代码，不会被合并（CLI-001、CLI-050）。

## 开发环境

你需要 Go（版本见 `go.mod`），以及 Node.js 16 或更新版本（用于 npm 包装和规范、版本脚本）。

```bash
# Clone
git clone https://github.com/fatecannotbealtered/gitlab-cli.git
cd gitlab-cli

go mod download
make build                      # or: go build -o bin/gitlab-cli ./cmd/gitlab-cli
make test                       # go vet, then go test -race ./... (conformance suite included)
./bin/gitlab-cli --help

# Vendored files and version locations
node scripts/check-spec.js && node scripts/check-version.js
```

如果下载依赖很慢，可以用区域代理（例如 `GOPROXY=https://goproxy.cn,direct`）。race 测试需要 `CGO_ENABLED=1` 和一个 C 编译器。

## 命令

| 目标 | 命令 |
|------|------|
| 构建 | `make build` |
| 测试（含符合性测试套件） | `make test` → `go test -race -timeout 45m ./...` |
| 只跑符合性测试套件 | `make conformance` |
| 静态检查 | `make lint` → `golangci-lint run ./...` |
| 格式化 | `make fmt` → `gofmt -w .` |
| 漏洞审计 | `make vuln` → `govulncheck ./...` |
| 副本与固定版本一致 | `node scripts/check-spec.js`（`make spec-check`） |
| 改了固定版本后重新复制 | `node scripts/sync-spec.js`（`make spec-sync`） |
| 各处版本号一致 | `node scripts/check-version.js` |
| 命令地图保持最新 | `UPDATE_SKILL_MAP=1 go test ./cmd -run CommandMap` 会重写它们 |

`make` 目标由变量驱动；在 Windows 上请直接用底层的工具命令。`cmd` 的测试大约要 9 分钟，带 `-race` 更久，所以 `make test` 放宽了超时。推送之前，在本地跑一遍 **lint + test**。

## 分支与提交规范

- 从默认分支拉出新分支：`git checkout -b feat/your-feature`。
- 一个分支只做一件逻辑改动；请人 review 之前，先 rebase 默认分支。
- 提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```text
<type>: <description>

<optional body>
```

| 类型 | 用于 |
|------|------|
| `feat` | 新功能 |
| `fix` | 修复缺陷 |
| `refactor` | 不改变行为的代码重构 |
| `docs` | 文档改动 |
| `test` | 新增或更新测试 |
| `chore` | 构建、CI、依赖或工具链改动 |
| `perf` | 性能优化 |
| `ci` | CI/CD 流水线改动 |

例子：`feat(mr): add --reviewer to mr list`、`fix(job): keep log follow from repeating lines`、`docs: sync README_zh`。适用时，按 ID 引用规范的规则（`fix: bind the body file by content (CLI-054)`）。

## CI 检查

`.github/workflows/ci.yml` 在每次推送和每个 pull request 上运行：格式检查和 lint；在 Linux、macOS 和 Windows 上运行测试（含符合性测试套件）；依赖审计（`govulncheck` 和 `npm audit`）；`check-version.js` 和 `check-spec.js`。任何一步失败都会阻止合并。`release.yml` 在签名或发布任何东西之前，对打了标签的提交再跑一遍同样的检查（REPO-070、REPO-071、VER-008）。

## 测试与证据

“测试过”是什么意思，由 [`.agent/VERIFY-SPEC_zh.md`](.agent/VERIFY-SPEC_zh.md) 定义：

- **符合性测试套件（VER-001）**。`cmd/conformance_test.go` 运行 `aicli.RunConformance`，用的是会统计写入次数的模拟 GitLab（`cmd/fakegitlab_test.go`），每个写命令一个 `Sample`，再加一个 scope。它对每个写命令检查写入检查（G00 至 G10），并对整个工具检查 `reference`、命令树、用法错误、JSON envelope、`context`、`doctor` 和 `operation`（R01 至 R05）。缺少样例的写命令会让套件失败。绝不跳过、过滤或放松任何一项检查。
- **真实输出（VER-002）**。命令级测试都通过同一个辅助函数运行 CLI（`cmd/testenv_test.go` 里的 `newTestEnv` 和 `e.run`）：它执行 App，用库自带的校验器校验 JSON envelope（NDJSON 流则逐行校验），并对照 contract.json 检查退出码。
- **按实际运行统计覆盖（VER-003）**。注册表里的每个命令（包括内置命令）都至少在一个命令级测试里运行过。`cmd/main_test.go` 里的 `TestMain` 把 `GITLAB_CLI_COVERAGE_FILE` 指向一个临时文件，库把每个执行过的命令记在里面；完整跑完之后，`aicli.CheckCoverage` 列出每个从未运行过的命令（R06）。
- **行为矩阵（VER-004）**。`runMatrix`（`cmd/matrix_test.go`）为 `experimental` 及以上的每个命令运行适用的各行：成功、无效输入、认证与权限、上游失败、空结果、分页、退出码、读取没有副作用，写入还有写入检查和 apply 失败；bulk 命令按每个条目检查。不适用的行是一个被跳过的子测试，跳过原因以 `not applicable:` 开头。每个改变可观察行为的缺陷修复，都要加一个回归测试。
- **成熟度（VER-005）**。每个命令在 `Meta` 里声明 `Maturity` 和 `Evidence`，级别不能超过证据：`draft`（已声明）、`experimental`（对模拟 GitLab 跑过行为矩阵，`simulated:` 证据）、`verified`（另有一次对真实 GitLab 的运行记录，`live:` 证据在 `docs/evidence/` 里，见 [docs/E2E.md](docs/E2E.md)）、`stable`（另外通过了 Agent 评测，并且没有未关闭的 P1 缺陷，`eval:` 证据）。改动命令的请求、解析或输出，会让它的 `live` 和 `eval` 证据作废：在同一次改动里重新运行，或者降低成熟度。
- **Agent 评测（VER-006）**。发布改动了命令、输出或 Skill 的版本之前，用 `skills/gitlab-cli/test-prompts.json` 运行 Agent 评测，带 Skill 和不带 Skill 各跑一次。任何命令要标为或保持 `stable`，都必须先做这项评测。
- **不碰用户状态（REPO-073）**。测试把状态目录指向临时目录（`App.StateDir` 或 `GITLAB_CLI_STATE_DIR`），并把系统凭据库换成内存里的替身，绝不碰用户的确认密钥、操作记录、scope 文件或保存的 token。

## 新增或修改命令

1. **客户端**：把上游调用加到 `internal/api/` 里对应领域的文件。复用共享的客户端和辅助函数（列表的一页用 `GetPage`），用参数构造 URL（绝不拼接用户输入），并遵守客户端的规则：每个请求都有超时，TLS 一律校验，读取最多重试两次，写入绝不重试（CLI-110）。
2. **命令**：每个命令组在自己的文件里用一个函数注册自己（`cmd/mr.go` 里的 `registerMR`，等等）；最短的完整例子是 `cmd/label.go`。每个叶子命令都用 `app.Register(cmd, aicli.Meta{...})` 注册：摘要、效果、`Dangerous`、成熟度和证据、输出 schema（`app.Schema`）、错误码（`gitlabReadCodes` 或 `gitlabWriteCodes`）和示例（写命令的示例写出 `--dry-run` 和 `--confirm` 这一对，危险命令写出 `--dangerous`）；适用时再加上 `Positional`、`FileInputs`、`Secrets`、`BatchFlag`、`RenamedFlags` 或 `DeprecatedFlags`（CLI-020 至 CLI-027）。flag 名称取自共享词汇（`--limit`、`--offset`、`--query`、`--state`、`--since`），动词保持标准含义（CLI-028、CLI-029）。`--help` 和 `reference` 由注册表生成，所以新命令不需要额外接线就会出现在两者里。
3. **名称和 ref**：项目用 `projectFlag` 读取（路径、网页 URL、数字 ID 或名称；对上几个项目时返回 `E_AMBIGUOUS`），用户用 `resolveUser` 读取（`carol`、`@carol`、`me`）。输出里的每个资源都带一个 GitLab 形式的 `ref`（`acme/web!5`、`acme/web#12`、`acme/web/-/pipelines/123`；子资源接在父资源后面：`acme/web!5/notes/12`），命令接受这个 ref 和网页 URL，写入的目标键就是它的 ref（CLI-048、SEC-012）。嵌套命令组的根快捷词用 `app.AddShortcut`；几个组都有的名词要带上组名（`mr-comment`、`issue-bulk`）。
4. **读取**通过 `app.OK` 或 `app.List` 返回（一页 GitLab 结果用 `gitlabPage`），并带上 `scope`，写明列表实际覆盖了什么（CLI-041）。**写入**通过 `app.Write`（或 `app.WriteBatch`）返回：`Preview` 只读，`Apply` 只发送一次并读回改动的内容。GitLab 客户端在这两者里面创建（`gitlabFor(app)`），这样被 scope 拒绝的写入什么都不会发出。只有情况属实时，才把 `Apply` 的错误包装成 `NotDispatched` 或 `Rejected`（CLI-050、CLI-059）。
5. **测试**：上面行为矩阵的各行，每个新写命令的符合性 `Sample`，以及放在该组测试旁边的模拟 GitLab 路由。
6. **Skill**：重新生成命令地图。它在 `skills/gitlab-cli/SKILL.md` 里，也在 `README.md` 和 `README_zh.md` 的能力一节里，夹在 `aicli:command-map` 标记之间。它由注册表渲染，由命令地图测试检查；`UPDATE_SKILL_MAP=1 go test ./cmd -run CommandMap` 会重写每一块。绝不手工编辑。然后更新 `SKILL.md` 里的任务流程和讲领域知识的那几行，并添加或调整 `test-prompts.json` 的条目（`id`、`scenario`、`prompt`、`expected`，以及可选的 `grader_notes`）。`SKILL.md` 保持在 150 行以内，不重复 flag 和 schema，这些由 `gitlab-cli reference --command <path>` 提供（SKL-002、SKL-003、SKL-011）。`metadata.version` 和 `metadata.min_version` 通过 `npm version` 跟随 `package.json`；绝不手工编辑。
7. **文档**：在 `CHANGELOG.md` 的 `## [Unreleased]` 下加一行。Agent 更新后通过 `gitlab-cli changelog` 读它，所以要写明改了哪些命令、flag 和字段。将要移除的命令或 flag，要设置 `Meta.Deprecated`、`Meta.RenamedFlags` 或 `Meta.DeprecatedFlags`，并在 `Deprecated` 下加一行；只能在 MAJOR 版本里移除（CLI-081）。每个版本的那一节要写得精简：不带 `--since` 的 `gitlab-cli changelog` 至少完整返回最新的一个版本，而符合性测试套件要求这个回应不超过 8,000 个字符（CLI-042）。

## Pull Request 指南

1. 尽量 **一个 PR 只做一件逻辑改动**。
2. **测试**：行为有改动，就要加测试或改测试。
3. **文档**：命令、flag 或输出有变化时，更新面向用户的文档和 Skill。
4. **提交**：遵循 Conventional Commits；任何地方都不能出现密钥或真实 token。

### PR 检查清单

- [ ] 只有一件逻辑改动，diff 集中
- [ ] 测试已添加或更新，并且通过（`make test`），含符合性测试套件
- [ ] 每个新写命令都有符合性 `Sample`，每个新命令都在测试里运行（VER-001、VER-003）
- [ ] 每个改动过的命令，成熟度仍和证据相符（VER-005）
- [ ] lint 和格式检查通过
- [ ] `node scripts/check-spec.js` 通过：没有手工编辑任何副本
- [ ] `node scripts/check-version.js` 通过
- [ ] 命令地图已重新生成（Skill 和 README 的能力一节）；任务流程和 `test-prompts.json` 已调整
- [ ] 改动涉及命令、输出或 Skill，并且有命令是 `stable` 时，已运行 Agent 评测（VER-006）
- [ ] `CHANGELOG.md`（`## [Unreleased]`）已更新
- [ ] **双语文档已同步**：每处 `*.md` 改动都同步到对应的 `*_zh.md`（反过来也一样）
- [ ] 代码、测试、测试数据、证据记录和提交历史里，都没有密钥、token 或真实凭据
- [ ] 提交信息遵循 Conventional Commits

## 安全

不要为未公开的安全漏洞开公开的 issue。见 [SECURITY_zh.md](SECURITY_zh.md)。
