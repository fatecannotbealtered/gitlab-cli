# Contributing

Thanks for improving gitlab-cli. This document covers building, testing, and submitting changes.

This is a side project shared for AI tooling experimentation; maintainers do not provide commercial support or production guarantees — see the README disclaimer.

## Development setup

- Go **1.25+** (see `go.mod`)
- Optional: **Node.js 16+** if you work on the npm install scripts
- Optional: **golangci-lint** (CI runs it on Linux)

Clone and verify:

```bash
git clone https://github.com/fatecannotbealtered/gitlab-cli.git
cd gitlab-cli
go mod download
go test ./...
go build -o bin/gitlab-cli ./cmd/gitlab-cli
./bin/gitlab-cli --help
```

If `go mod download` is slow, try a regional proxy, for example:

```bash
# Example (China)
set GOPROXY=https://goproxy.cn,direct
```

## Commands

| Goal | Command |
|------|---------|
| Run tests (race) | `go test -race ./...` |
| Format | `gofmt -w .` |
| Vet | `go vet ./...` |
| Lint | `golangci-lint run ./...` (or `make lint` on Unix) |
| Build with version | `make build` (Unix) or `go build -ldflags "-s -w -X github.com/fatecannotbealtered/gitlab-cli/cmd.version=dev" -o bin/gitlab-cli.exe ./cmd/gitlab-cli` (Windows) |
| Local goreleaser snapshot | `make snapshot` |

CI mirrors `.github/workflows/ci.yml`: tidy modules, gofmt check (Linux), golangci-lint, `go vet`, build, `go test -race`, and a `--help` smoke test on linux/macos/windows on the Go version pinned in `go.mod`.

Local GitLab end-to-end tests (Docker Compose, 85 leaf commands): see [docs/E2E.md](docs/E2E.md) (**Windows / PowerShell** scripts; cross-platform notes inside). Run `go test -tags=integration ./e2e/...` after `scripts/e2e-up.ps1` on Windows.

When adding commands, ensure `markWrite` / `markConfirm` / `markOutputType` annotations are set where applicable — they feed `gitlab-cli reference --compact`.

## Adding a new domain

`gitlab-cli` is sliced by GitLab REST domain (e.g. `mr`, `issue`, `pipeline`). To add a new one:

1. **DTOs** → define them in `internal/api/<domain>.go` alongside the API methods (not in `types.go` — that file only contains the shared `User` type).
2. **API client** → create `internal/api/<domain>.go` with `type <Domain>API struct{ client *Client }` and the methods you need; reuse `c.client.Get/Post/Put/Delete` and `EncodeProjectPath`.
3. **Wire into `Client`** → add a pointer field on `Client` in `internal/api/client.go` and instantiate it inside `NewClient`. Keep the field list alphabetical.
4. **Command** → create `cmd/<domain>.go` with a parent `<domain>Cmd` and one `Cobra` subcommand per action. In `init()`:
   - `parentCmd.AddCommand(childCmd)` for every subcommand.
   - Register flags on each subcommand's `.Flags()`.
   - For write commands, call `markWrite(cmd)` so they are recorded in audit logs.
5. **Tests** →
   - `internal/api/<domain>_test.go` using `httptest.Server`.
   - Add command-level behaviour tests to `cmd/cmd_test.go` (or split if it grows).
6. **AI Skill** → add detail to `skills/gitlab-cli/reference/<domain>.md`; keep `skills/gitlab-cli/SKILL.md` as a short index (progressive disclosure). Link new domains from the SKILL index table.
7. **Docs** → README.md / README_zh.md command list; add entries under `## [Unreleased]` in CHANGELOG.md (create the section when preparing the next release).
8. **Optional** → add a `FlatXxx` + `XxxToMap` in `internal/output/flatten_<domain>.go` if the JSON output should be flattened, and a `printXxxJSON` helper in your `cmd/<domain>.go` render path.

`cmd/reference.go` walks the cobra tree automatically, so new commands appear in `gitlab-cli reference` without code changes.

## Pull requests

1. **One logical change per PR** when possible.
2. **Tests**: add or update tests for behaviour changes.
3. **Docs**: update `README.md` / `README_zh.md` if user-facing flags change; add a line to `CHANGELOG.md` under `## [Unreleased]` when cutting the next version.
4. **Commits**: clear messages following Conventional Commits (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`, `perf:`, `ci:`); no secrets or real tokens in code or docs.

## Security

Do not open public issues for undisclosed security vulnerabilities. See [SECURITY.md](SECURITY.md).
