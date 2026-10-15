# Compatibility

`gitlab-cli` 2.x talks to GitLab through its REST API v4. Its output follows ai-native-cli-spec 2.0 (`schema_version` `"2.0"`); the exact spec tag is pinned in `.agent/SPEC_VERSION`. Scripts written for 1.x need the changes in [UPGRADE-2.0.md](UPGRADE-2.0.md).

## GitLab

| Target | Status | Notes |
|---|---|---|
| GitLab EE 18.8 (self-managed) | Validated live | 2026-10-14, GitLab EE 18.8.10 in Docker without a license, with a shell runner: 81 of the 106 tool commands passed and are `verified`. The record, with every call and a read-back of every write: [evidence/2026-10-14-gitlab-ee-18.8.md](evidence/2026-10-14-gitlab-ee-18.8.md). |
| Other self-managed versions, CE or EE | Expected to work | Same REST API v4; not validated live for 2.0. |
| GitLab.com, GitLab Dedicated | Expected to work | Same REST API v4; not validated live for 2.0. |

The 25 tool commands that did not run live are tested against a fake GitLab only and stay `experimental`: the `issue bulk` and `mr bulk` commands except `issue bulk label`, `issue label`, `job cancel`, `job play`, `mr close`, `mr reopen`, `mr unapprove`, `mr current`, `mr comment delete`, `mr discussion unresolve`, `pipeline cancel`, `pipeline current`, `release delete`, `repo file delete`, `variable bulk-import`, and the deprecated `pipeline wait` and `job wait`. `gitlab-cli reference --index <group>` shows each command's maturity.

The token is a personal access token with the `api` scope, or `read_api` for reads only. What a command may do also depends on the account's role, protected branches, approval rules and the instance's settings; a refusal from GitLab is `E_FORBIDDEN`, with `details.required` naming the role when the command knows it.

## Features that depend on GitLab

| Feature | Needs | Notes |
|---|---|---|
| `repo commit list --author` | GitLab 15.10 or later | Older GitLab ignores the filter and returns commits by everyone. |
| Relevance order in `search` | GitLab advanced search (Elasticsearch or OpenSearch) | Basic search returns GitLab's own order, so the `search` commands say `sort: "upstream"` and explain it in `scope.ranking`. |
| `search commits` across all projects | GitLab advanced search | Within one project (`--project`), basic search works. |
| `auth status` token scopes and expiry | a personal access token | For other token types the fields are empty. |

GitLab 18.8's basic code search can return the same hit twice (a file-name match and a content match); `search code` passes both through.

## Platforms and channels

| Channel | Platforms | Notes |
|---|---|---|
| npm, `@fateforge/gitlab-cli` | Linux x64 and arm64, macOS x64 and arm64, Windows x64; Windows on arm64 runs the x64 binary under emulation | Node.js 16 or later runs the launcher. npm installs the binary from a platform package; nothing is downloaded at install time. `update` runs `npm install -g` for you. |
| GitHub Release archives | Linux and macOS on amd64 and arm64, Windows on amd64 | Each archive has the binary and `skills/`; `checksums.txt` is signed with Sigstore, and `update` verifies the signature and the checksum before it replaces the binary. |
| Go | building from a checkout | `make build` with the Go version in `go.mod`. `go install` is not a 2.0 channel: the module path has no `/v2` suffix, so `go install …@latest` keeps resolving to the newest 1.x tag. |

- **Credentials**: `auth login` needs an OS keyring: Windows Credential Manager, macOS Keychain, or a Secret Service on Linux (such as GNOME Keyring). On a headless Linux host or in a container, set `GITLAB_CLI_TOKEN` instead.
- **State directory**: on Linux and macOS it is created 0700 with 0600 files; on Windows it relies on the user profile's ACL. `gitlab-cli context` reports which in `state.protection`.
- **git**: `mr current`, `pipeline current`, `mr create --auto` and the `git_remote` that `context` reports read the local checkout with the `git` command, so they need `git` on `PATH`.
- **Tests**: CI runs the test suite, conformance suite included, on Linux, macOS and Windows; the live validation ran on Windows.
