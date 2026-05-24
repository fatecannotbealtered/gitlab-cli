# Security Policy

## Supported Versions

Only the latest minor version receives security updates.

## Reporting a Vulnerability

Please **do not open public GitHub issues for undisclosed vulnerabilities**.

Email a description and reproduction steps to the maintainer:

- **Contact**: guosong6886@gmail.com

A response and triage decision will normally arrive within **5 business days**.

## What this CLI handles

- A user-supplied **GitLab Personal Access Token (PAT)** with `api` scope.
- The token is stored at `~/.gitlab-cli/config.json` (`0600`, directory `0700`) and/or read from environment variables.
- The token is **never logged** by this CLI: every audit-log entry redacts `--token`, `-t`, `--private-token`, `--oauth-token`, `--job-token`, `--password`, `--value`, and `--variable` flag values.
- All network traffic goes to the host configured by the user. HTTPS is required by default; `http://` is allowed only if the user explicitly opts in for local development.

## What we expect from contributors

- No secrets or real tokens in code, tests, fixtures, or commit history.
- Use parameterised request building (`url.PathEscape` / `url.QueryEscape`); never concatenate user-controlled strings into URLs.
- Treat data returned by the GitLab API as untrusted input — do not blindly execute or render it.
- When new flags handle credentials, add them to `internal/audit.sensitiveFlags`.
