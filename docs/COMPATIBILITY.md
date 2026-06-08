# Compatibility Matrix

`gitlab-cli` targets the GitLab REST API v4.

| Target | Status | Notes |
|---|---|---|
| GitLab.com | Supported | Primary compatibility target. |
| GitLab Self-Managed | Supported | Requires REST API v4 and a PAT with scopes matching the requested operation. |
| GitLab Dedicated | Supported | Same API contract as GitLab.com/Self-Managed for covered endpoints. |
| GitLab Data Center | Supported where REST API v4 endpoints match | Validate instance-specific policies, SSO, and rate limits with `gitlab-cli doctor`. |

## Verified Areas

| Area | API Surface |
|---|---|
| Auth/context | `/user`, project lookup, local git remote detection |
| Merge requests | list/get/create/update/merge/close/reopen/approve/unapprove/diff/notes |
| Issues | list/get/create/update/close/reopen/assign/label/notes |
| CI/CD | pipelines, jobs, logs, artifacts, variables |
| Repository | raw files, file writes, branches, commits, tree |
| Project metadata | projects, members, labels, milestones, releases |
| Search | project, issue, merge request, blob/code, commit search |

## Notes For Agents

Run `gitlab-cli context --compact` and `gitlab-cli doctor --compact` before acting on a new instance. Instance-level permissions, protected branch rules, approval rules, and variable visibility can change command outcomes even when the API endpoint exists.
