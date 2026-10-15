"""Write docs/evidence/<name>.json, the VER-005 live record of a run, from the run's records (<run>/out).

    python e2e/build_evidence.py

<name> is GITLAB_E2E_EVIDENCE_NAME, else <date>-gitlab-<ce|ee>-<major.minor> from the run. Nothing is written if any
secret appears in the record. Paths inside the run directory are written as <run>. Also writes <run>/out/verdict.json
(the commands that passed, failed or did not run), which set_maturity.py reads.
"""

from __future__ import annotations

import json
import subprocess
from collections import OrderedDict, defaultdict

import live
from live import BIN, OUT, REPO, STATE, base_env, ctx, evidence_name, git

BUILTINS = {"context", "doctor", "reference", "changelog", "operation show", "operation resolve", "operation list",
            "update"}
# the section (anchor) each command group is reported under
SECTION_OF = {"auth": "auth", "project": "project", "repo": "repo", "label": "label", "milestone": "milestone",
              "issue": "issue", "mr": "mr", "release": "release", "pipeline": "pipeline", "job": "job",
              "variable": "variable", "search": "search", "user": "user"}
SECTIONS = OrderedDict([
    ("builtins", "Built-ins"), ("auth", "auth"), ("project", "project"), ("repo", "repo"), ("label", "label"),
    ("milestone", "milestone"), ("issue", "issue"), ("mr", "mr"), ("release", "release"), ("pipeline", "pipeline"),
    ("job", "job"), ("variable", "variable"), ("search", "search"), ("user", "user"), ("gate", "gate"),
    ("lost-reply", "lost-reply")])


def paths_of_record():
    """The record's name and where its JSON and Markdown go."""
    name = evidence_name()
    return name, REPO / "docs" / "evidence" / f"{name}.json", REPO / "docs" / "evidence" / f"{name}.md"


def registry_paths() -> list:
    p = subprocess.run([str(BIN), "reference"], capture_output=True, env=base_env(None))
    d = json.loads(p.stdout)["data"]
    return sorted({c["path"] for c in d["commands"]}, key=lambda s: -len(s.split()))


def command_of(argv: list, paths: list) -> str:
    words = []
    for a in argv[1:]:
        if a.startswith("-"):
            break
        words.append(a)
    for p in paths:
        n = len(p.split())
        if words[:n] == p.split():
            return p
    if "--version" in argv:
        return "--version"
    return " ".join(words[:2])


def build_info(version: str) -> tuple:
    """(commit, branch) of the binary: what setup built, else the commit in a Go pseudo-version, else HEAD."""
    b = ctx().get("build") or {}
    if b.get("commit"):
        return b["commit"], b.get("branch") or ""
    tail = version.rsplit("-", 1)[-1].split("+", 1)[0]
    commit = git("rev-parse", "--verify", "--quiet", tail + "^{commit}") if len(tail) >= 7 else ""
    return commit or git("rev-parse", "HEAD"), git("rev-parse", "--abbrev-ref", "HEAD")


def runner_text(c: dict) -> str:
    rs = c.get("runners")
    if rs is None:
        return "no runner recorded"
    if not rs:
        return "no online instance runner"
    return "; ".join(f"instance runner {r.get('description') or '?'} (gitlab-runner {r.get('version') or '?'})"
                     for r in rs)


def keyring_name(c: dict) -> str:
    return {"win32": "Windows Credential Manager", "darwin": "macOS Keychain"}.get(c.get("platform"), "the OS keyring")


def main():
    name, json_out, _ = paths_of_record()
    calls = [json.loads(line) for line in live.CALLS.read_text(encoding="utf-8").splitlines() if line.strip()]
    c = ctx()
    paths = registry_paths()
    version = json.loads((OUT / "calls" / "B01.out").read_text(encoding="utf-8"))["data"]["version"]
    commit, branch = build_info(version)
    scope = c.get("scope") or (json.loads((STATE / "scope.json").read_text(encoding="utf-8"))
                               if (STATE / "scope.json").exists() else None)
    proxy = json.loads((OUT / "faultproxy.json").read_text(encoding="utf-8")) if (OUT / "faultproxy.json").exists() \
        else None
    owner, reviewer, profile, group = live.OWNER, live.REVIEWER, live.PROFILE, live.GROUP

    who = {"keyring": f"{owner} (keyring profile {profile})", owner: f"{owner} (GITLAB_CLI_TOKEN)",
           reviewer: f"{reviewer} (GITLAB_CLI_TOKEN)", "admin": f"{c.get('admin_user') or 'admin'} (GITLAB_CLI_TOKEN)"}
    entries = []
    for x in calls:
        cmd = command_of(x["argv"], paths)
        e = OrderedDict(id=x["id"], area=x["area"], command=cmd, argv=x["argv"], exit=x["exit"],
                        duration_ms=x["duration_ms"], envelope_valid=x["envelope_valid"])
        if x["envelope_valid"] is None:
            e["not_an_envelope"] = (x.get("validation") or [""])[0]
        elif x.get("validation"):
            e["validation_problems"] = x["validation"]
        e["expect"] = x["expect"]
        e["pass"] = x["pass"]
        if not x["pass"] and x.get("why"):
            e["why"] = x["why"]
        e["as"] = who.get(x["as"], x["as"])
        if x.get("via_proxy"):
            e["via_fault_proxy"] = True
        if "read_back" in x:
            e["read_back"] = x["read_back"]
        e["stdout"] = x["stdout"]
        entries.append(e)

    # per command: verified when every live call of it passed with a valid envelope (or a by-design non-envelope)
    per_cmd = defaultdict(list)
    for e in entries:
        per_cmd[e["command"]].append(e)
    verdict = {}
    for cmd, es in per_cmd.items():
        ok = all(e["pass"] and e["envelope_valid"] is not False for e in es)
        verdict[cmd] = {"calls": [e["id"] for e in es], "passed": ok,
                        "section": SECTION_OF.get(cmd.split()[0], "builtins") if cmd not in BUILTINS else "builtins"}
    tool_cmds = sorted(p for p in paths if p not in BUILTINS)
    verified = sorted(cmd for cmd, v in verdict.items() if v["passed"] and cmd in tool_cmds)
    not_run = sorted(p for p in tool_cmds if p not in verdict)
    failed = sorted(cmd for cmd, v in verdict.items() if not v["passed"])

    summary = OrderedDict(
        calls=len(entries), passed=sum(e["pass"] for e in entries),
        failed=[e["id"] for e in entries if not e["pass"]],
        envelopes_checked=sum(e["envelope_valid"] is not None for e in entries),
        envelopes_valid=sum(e["envelope_valid"] is True for e in entries),
        not_envelopes_by_design=[e["id"] for e in entries if e["envelope_valid"] is None],
        tool_commands=len(tool_cmds), tool_commands_verified=len(verified), tool_commands_not_run=len(not_run))

    gl = c.get("gitlab") or {}
    record = OrderedDict(
        record="live", schema="VER-005 live record", tool="gitlab-cli", version=version, commit=commit,
        branch=branch, spec=OrderedDict(version=(REPO / ".agent" / "SPEC_VERSION").read_text().strip(),
                                        validator=c.get("validator") or live.validator_label()),
        upstream=OrderedDict(product="GitLab EE" if gl.get("enterprise") else "GitLab CE", version=gl.get("version"),
                             revision=gl.get("revision"), url=live.HOST, license=c.get("license", "none (Free features)"),
                             runner=runner_text(c)),
        ran_at=c.get("started_at"), finished_at=c.get("finished_at") or entries[-1].get("ran_at"),
        isolation=OrderedDict(
            home="USERPROFILE, HOME, APPDATA, LOCALAPPDATA and TEMP pointed into the run directory; GITLAB_CLI_STATE_DIR "
                 "inside that home (settings, scope, journal, confirm key); deleted after the run",
            credentials=f"keyring profile {profile} only ({keyring_name(c)} gitlab-cli / profile:{profile}, written by "
                        "auth login, removed by auth logout); other identities through GITLAB_CLI_TOKEN; no token in "
                        "any argv, stdout or record (scanned)",
            sandbox=f"group {group} (admin API, with a second project {group}/twin/sandbox) and project "
                    f"{group}/sandbox (the tool's project create); nothing outside the group was written",
            scope=scope),
        secret_scan=c.get("secret_scan"), variable_values_seen_in=c.get("variable_values_seen_in"),
        fault_proxy=OrderedDict(summary=c.get("proxy_summary"), rule=(proxy or {}).get("arm"),
                                requests=[{k: r.get(k) for k in ("seq", "method", "path", "action", "status",
                                                                  "upstream_status", "matched")}
                                          for r in (proxy or {}).get("requests", [])]),
        summary=summary, verified_commands=verified, commands_not_run=not_run, commands_failed=failed,
        calls=entries)

    record = live.hide_run(record)
    raw = json.dumps(record, indent=1, ensure_ascii=False)
    if live.leaks(raw):
        raise SystemExit("a secret appears in the evidence; nothing written")
    json_out.parent.mkdir(parents=True, exist_ok=True)
    json_out.write_bytes((raw + "\n").encode("utf-8"))
    (OUT / "verdict.json").write_text(json.dumps({"record": name, "verified": verified, "not_run": not_run,
                                                   "failed": failed,
                                                   "sections": {k: v["section"] for k, v in verdict.items()}},
                                                  indent=1), encoding="utf-8")
    print(json.dumps(summary, indent=1))
    print("verified", len(verified), "not run", len(not_run), "failed", failed)
    print("wrote", json_out.relative_to(REPO).as_posix())


if __name__ == "__main__":
    main()
