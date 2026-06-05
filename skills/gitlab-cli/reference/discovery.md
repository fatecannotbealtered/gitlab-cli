# Discovery: project, user, search

## Context shortcut

Prefer `gitlab-cli context` for current project path before other commands.

## Projects

```bash
gitlab-cli project list --compact
gitlab-cli project list --owned --visibility private
gitlab-cli project get group/myproject --fields id,name,webUrl
gitlab-cli project members group/myproject
gitlab-cli project members 42 --query alice --limit 10
```

Access levels: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner.

## Users

```bash
gitlab-cli user me --fields id,username
gitlab-cli user search --query alice --active
gitlab-cli user get alice
```

## Search

```bash
gitlab-cli search projects --query myapp
gitlab-cli search issues --query "login bug" --project G
gitlab-cli search mrs --query "feat auth" --project G
gitlab-cli search code --query "func main" --project G   # --project required
gitlab-cli search commits --query "fix crash" --project G
```

## Notes

- `search code` **requires** `--project`
- Global vs project-scoped: omit `--project` for global issue/mr/commit search
