# Releases

```bash
gitlab-cli release list --project G --compact
gitlab-cli release get --project G --tag v1.0.0

gitlab-cli release create --project G --tag v1.0.0 --name "v1.0.0" \
  --ref main --description "..."
gitlab-cli release update --project G --tag v1.0.0 --name "v1.0.0"
gitlab-cli release delete --project G --tag v1.0.0 --confirm v1.0.0
```

## Flat Release JSON (excerpt)

```json
{
  "tagName": "v1.0.0",
  "name": "Release 1.0.0",
  "description": "...",
  "author": "alice",
  "assetCount": 2
}
```

## Notes

- Tag must exist or pass `--ref` on create
- `--milestone` accepts comma-separated milestone titles
