# Agent Entry

This repository is an AI-native CLI project. Start with [.agent/AGENT.md](.agent/AGENT.md) before changing CLI behavior, Skill instructions, security-sensitive code, or release packaging.

The local specs under `.agent/` are the source of truth:

- `.agent/CLI-SPEC.md` for the machine-readable CLI contract.
- `.agent/SEC-SPEC.md` for untrusted content, credential storage, blast radius, and supply-chain rules.
- `.agent/SKILL-SPEC.md` for the installable Skill contract.
- Shared [`REPO-SPEC.md`](https://github.com/fatecannotbealtered/ai-native-cli-spec/blob/main/REPO-SPEC.md) for repository layout and release documentation.

Before release, Functional Contract Coverage must remain 100%: every public README / Skill / reference / help / context / doctor / changelog / update behavior needs command-level tests.

Release readiness must be explicit: `reference.release_readiness` and `doctor` declare `stable`, `beta`, or `unpublishable`; `stable` requires recorded live smoke/E2E evidence.
