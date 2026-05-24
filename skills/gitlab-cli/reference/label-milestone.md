# Labels & Milestones

## Labels

```bash
gitlab-cli label list --project G --json --compact
gitlab-cli label create --project G --name bug --color "#FF0000" --json
gitlab-cli label update --project G --label-id 1 --name critical --json
gitlab-cli label delete --project G --label-id 1 --confirm 1 --json
```

Apply to issues/MRs via `issue label` or `mr update --add-labels`.

## Milestones

```bash
gitlab-cli milestone list --project G --state active --json
gitlab-cli milestone get --project G --milestone-id 5 --json
gitlab-cli milestone create --project G --title "v1.0" --due-date 2025-12-31 --json
gitlab-cli milestone update --project G --milestone-id 5 --title "v1.1" --json
gitlab-cli milestone close --project G --milestone-id 5 --confirm 5 --json
```

Assign to issues: `issue create --milestone-id N` or `issue update --milestone-id N`.

## Notes

- Label `--color`: `#RRGGBB` or named colors (red, blue, …)
- Milestone dates: `YYYY-MM-DD`
- Use `--milestone-id` (global id), not IID, for milestone commands
