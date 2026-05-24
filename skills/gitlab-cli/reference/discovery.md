# Discovery: project, user, search

## Context shortcut

Prefer `gitlab-cli context --json` for current project path before other commands.

## Projects

```bash
gitlab-cli project list --json --compact
gitlab-cli project list --owned --visibility private --json
gitlab-cli project get group/myproject --json --fields id,name,webUrl
gitlab-cli project members group/myproject --json
gitlab-cli project members 42 --query alice --limit 10 --json
```

Access levels: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner.

## Users

```bash
gitlab-cli user me --json --fields id,username
gitlab-cli user search --query alice --active --json
gitlab-cli user get alice --json
```

## Search

```bash
gitlab-cli search projects --query myapp --json
gitlab-cli search issues --query "login bug" --project G --json
gitlab-cli search mrs --query "feat auth" --project G --json
gitlab-cli search code --query "func main" --project G --json   # --project required
gitlab-cli search commits --query "fix crash" --project G --json
```

## Notes

- `search code` **requires** `--project`
- Global vs project-scoped: omit `--project` for global issue/mr/commit search
