# Open Source Checklist

Use this before publishing a release or pushing a newly prepared public branch.

## Repository

- [ ] `README.md` and `README_zh.md` are in sync.
- [ ] `CHANGELOG.md` has an `Unreleased` section and the release section follows Keep a Changelog.
- [ ] `LICENSE`, `NOTICE.md`, `CONTRIBUTING.md`, `SECURITY.md`, and `CODE_OF_CONDUCT.md` are present.
- [ ] `AGENTS.md` points agents to `.agent/AGENT.md`.
- [ ] `docs/COMPATIBILITY.md` documents supported GitLab targets.

## Security

- [ ] No real tokens, cookies, private URLs, or customer data are committed.
- [ ] Saved credentials are encrypted at rest and credential files are written with `0600`.
- [ ] GitLab-controlled text fields are tagged with `_untrusted`.
- [ ] Write commands require `--dry-run` then `--confirm <confirm_token>`.
- [ ] npm publishes the main wrapper package and OS/CPU platform packages with provenance.
- [ ] Standalone binary install/update paths hard-fail when checksum verification is unavailable or mismatched.
- [ ] `npm audit` and Go tests pass for release dependencies.

## Release

- [ ] `package.json` version, git tag, and `CHANGELOG.md` release heading match.
- [ ] Functional Contract Coverage is 100%: public README, Skill, `reference`, `--help`, `context`, `doctor`, `changelog`, and `update` behavior has command-level tests.
- [ ] `reference.release_readiness.level` is accurate: `stable` has FCC 100%, mock upstream/contract tests, and recorded live smoke/E2E evidence; missing live evidence is `beta`; missing command-level coverage is `unpublishable`.
- [ ] `doctor` includes a `release_readiness` check whose status matches the declared release level.
- [ ] `gitlab-cli reference --compact` includes version, risk tier, security metadata, and command risk annotations.
- [ ] `gitlab-cli changelog --since <previous-version>` reports the release delta from `CHANGELOG.md`.
- [ ] GoReleaser artifacts include `checksums.txt`.
- [ ] The npm package includes CLI scripts, Skill files, README, license, notice, security, and compatibility docs.
