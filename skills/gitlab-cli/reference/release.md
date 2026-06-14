# Releases

```bash
gitlab-cli release list --project G --compact
gitlab-cli release get --project G --tag v1.0.0

gitlab-cli release create --project G --tag v1.0.0 --name "v1.0.0" \
  --ref main --description "..."
gitlab-cli release create --project G --tag v1.0.0 --name "v1.0.0" --idempotency-key rel-v100  # idempotent create
gitlab-cli release update --project G --tag v1.0.0 --name "v1.0.0"

# release delete is write-dangerous: --dangerous in BOTH steps.
gitlab-cli release delete --project G --tag v1.0.0 --dangerous --dry-run
gitlab-cli release delete --project G --tag v1.0.0 --dangerous --confirm <confirm_token>
```

## Release data payload (excerpt)

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
- `release delete` → `permissionTier: write-dangerous`; needs `--dangerous` in both `--dry-run` and `--confirm`
- `release create` accepts `--idempotency-key` (Idempotency-Key header + bound into the token)
