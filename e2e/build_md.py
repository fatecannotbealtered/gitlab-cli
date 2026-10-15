"""Write docs/evidence/<name>.md from the run's JSON record, after teardown (python e2e/build_md.py).

It also patches the JSON record: its finish time (the last call's) and the cleanup report. The section "Defects and
observations" is <run>/notes.md when that file exists: write what the run found there (a numbered Markdown list)
before running this script.
"""

from __future__ import annotations

import datetime as dt
import json

import live
from build_evidence import SECTIONS, keyring_name, paths_of_record
from live import OUT, RUN, ctx

NAME, JSON_OUT, MD_OUT = paths_of_record()
rec = json.loads(JSON_OUT.read_text(encoding="utf-8"))
c = ctx()
last = OUT / "calls" / (rec["calls"][-1]["id"] + ".out")
rec["finished_at"] = dt.datetime.fromtimestamp(last.stat().st_mtime, dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
rec["cleanup"] = live.hide_run(c.get("teardown"))
raw = json.dumps(rec, indent=1, ensure_ascii=False)
assert not live.leaks(raw)
JSON_OUT.write_bytes((raw + "\n").encode("utf-8"))

calls = rec["calls"]
S = rec["summary"]
OWNER, REVIEWER, PROFILE, GROUP = live.OWNER, live.REVIEWER, live.PROFILE, live.GROUP
KEYRING = keyring_name(c)
UP = rec["upstream"]
MINOR = ".".join((UP.get("version") or "?").replace("-", ".").split(".")[:2])


def doc_of(call):
    try:
        return json.loads(call["stdout"].strip().split("\n")[-1])
    except (ValueError, IndexError):
        return None


def short(argv, n=110):
    s = " ".join(a if (" " not in a and a) else json.dumps(a, ensure_ascii=False) for a in argv)
    s = s.replace("\n", "\\n")
    if "--confirm" in argv:
        i = argv.index("--confirm")
        s = s.replace(argv[i + 1], "<token>")
    s = live.hide_run(s)
    return s if len(s) <= n else s[:n - 1] + "…"


def fact(call):
    d = doc_of(call)
    if call["envelope_valid"] is None:
        return f"{call.get('not_an_envelope', 'not an envelope')} (by design); exit 0"
    if not isinstance(d, dict):
        return "no envelope"
    if d.get("ok") is False:
        e = d["error"]
        det = e.get("details") or {}
        bits = [e["code"]]
        for k in ("conflict", "problem"):
            if det.get(k):
                bits.append(det[k])
        if det.get("parameter"):
            bits.append(f"names `{det['parameter']}`")
        if (det.get("scope") or {}).get("status"):
            bits.append(f"scope {det['scope']['status']}")
        if (det.get("scope") or {}).get("grant_example"):
            bits.append("with grant_example")
        if det.get("operation_id") and e["code"] == "E_OUTCOME_UNKNOWN":
            bits.append("operation id given")
        return ", ".join(bits)
    data = d.get("data") or {}
    if call["command"].startswith("operation") and not isinstance(data.get("items"), list):
        return f"status `{data.get('status')}`" + (f", resolution `{data['resolution']['outcome']}`"
                                                   if data.get("resolution") else "") + (
            "; read-back " + json.dumps(call["read_back"]) if "read_back" in call else "")
    if call["command"] == "job log":
        keys = [k for k in ("line_count", "lines", "total_lines", "truncated") if k in data]
        return ", ".join(f"{k} {data[k]}" for k in keys) or f"`{data.get('ref')}`"
    if "confirm_token" in data:
        return "preview can apply; token issued"
    if "receipt" in data or "summary" in data and "items" in data:
        if "receipt" in data:
            o = data["receipt"]["outcome"]
            out = f"{o.get('change') if o.get('change') != 'not_applicable' else o.get('persistence')}, verification " \
                  f"{o.get('verification')}"
            if data["receipt"].get("replayed"):
                out = "replayed (no token, nothing sent)"
        else:
            s = data["summary"]
            out = f"batch: {s.get('succeeded')}/{s.get('total')} applied"
        if "read_back" in call:
            rb = json.dumps(call["read_back"], ensure_ascii=False)
            out += "; read-back " + (rb if len(rb) <= 140 else rb[:139] + "…")
        return out
    if "items" in data and "receipt" not in data:
        return f"{data.get('count', len(data['items']))} item(s)" + (", has_more" if data.get("has_more") else "")
    cmd = call["command"]
    if cmd == "--version":
        return f"version `{data.get('version')}`"
    if cmd == "context":
        return (f"account {data.get('account') or 'none'}, credentials {data['credentials']['source']}, scope "
                f"{data['authorization']['status']}")
    if cmd == "doctor":
        return "; ".join(f"{x['check']} {x['status']}" for x in data["checks"])
    if cmd == "reference":
        return f"{len(call['stdout'].strip())} characters"
    if cmd == "changelog":
        return f"{data.get('count')} entries, current {data.get('current_version')}"
    if cmd == "auth status":
        return f"signs in as {data.get('account')} ({data.get('credentials_source')})"
    if cmd.startswith("operation"):
        return f"status `{data.get('status')}`" + (f", resolution {data['resolution']['outcome']}"
                                                   if data.get("resolution") else "")
    if data.get("ref"):
        return f"`{data['ref']}`"
    return "ok"


def stdout_chars(cid):
    hit = next((x for x in calls if x["id"] == cid), None)
    return len(hit["stdout"].strip()) if hit else None


dev = "-" in rec["version"] or "+" in rec["version"]
not_env = S["not_envelopes_by_design"]
lines = []
w = lines.append
w(f"# Live validation: gitlab-cli on {UP['product']} {MINOR}")
w("")
w(f"The VER-005 live record of gitlab-cli against a real {UP['product']}. Its machine-readable twin is "
  f"[`{NAME}.json`]({NAME}.json): every call with its argv, exit code, stdout as printed (paths inside the run "
  "directory shown as `<run>`), duration, contract check and, for each confirmed write, a read-back made outside "
  "the tool through the admin API.")
w("")
w("| | |")
w("|---|---|")
w(f"| Tool | gitlab-cli `{rec['version']}`: a {'development ' if dev else ''}build of `{rec['commit'][:7]}` "
  f"(branch `{rec['branch']}`) |")
CHECKED = ("the spec's Python validator (`validate_envelope`, `validate_stream`)"
           if rec["spec"]["validator"].startswith("template/python/aicli/validate.py") else
           "`e2e/live.py`'s ContractLite (the envelope and error rules of `contract.json`; the spec's validator was "
           "not configured)")
w(f"| Spec | ai-native-cli-spec `{rec['spec']['version']}`; every stdout checked with {CHECKED}, and every exit code "
  "against `contract.json` |")
w(f"| Upstream | {UP['product']} {UP['version']} (revision {UP['revision']}, license: {UP['license']}) at {UP['url']}, "
  f"with {UP['runner']} |")
w(f"| Ran | {rec['ran_at']} to {rec['finished_at']} |")
w(f"| Accounts | {OWNER} (owner of the sandbox group) through the keyring profile `{PROFILE}`; {REVIEWER} (developer) "
  "through `GITLAB_CLI_TOKEN` to approve the merge request; the administrator only through the admin API, outside "
  "the tool |")
w("")
w("## Result")
w("")
w(f"- {S['calls']} calls, {S['passed']} as expected, {len(S['failed'])} not"
  + (f": {', '.join(S['failed'])}." if S["failed"] else "."))
w(f"- {S['envelopes_valid']} of {S['envelopes_checked']} stdout envelopes are valid, and every exit code matches "
  "`contract.json`" + (f"; the other {len(not_env)} calls ({', '.join(not_env)}) ask for `--format raw` and print "
                       "the bytes." if not_env else "."))
w(f"- {S['tool_commands_verified']} of the {S['tool_commands']} tool commands passed live"
  + (f"; the {S['tool_commands_not_run']} not run: {', '.join('`' + x + '`' for x in rec['commands_not_run'])}."
     if rec["commands_not_run"] else "."))
if rec["commands_failed"]:
    w(f"- Commands with a failed call: {', '.join('`' + x + '`' for x in rec['commands_failed'])}.")
scan = rec.get("secret_scan") or {}
w(f"- No token or variable value appears in any argv, stdout or stderr ({scan.get('secrets_checked')} secrets "
  f"against {scan.get('files_checked')} recorded files), nor in this record.")
w("")
w("## Defects and observations")
w("")
notes = RUN / "notes.md"
w(live.hide_run(notes.read_text(encoding="utf-8").strip()) if notes.exists() else "None recorded for this run.")
w("")
w("## How it ran")
w("")
w("- **Isolation.** `USERPROFILE`, `HOME`, `APPDATA`, `LOCALAPPDATA` and `TEMP` pointed at a fresh folder in the "
  "run directory, and `GITLAB_CLI_STATE_DIR` at a folder inside it (settings, scope, journal, confirm key); all "
  f"of it was deleted after the run. The only keyring entry was `gitlab-cli` / `profile:{PROFILE}` in the "
  f"{KEYRING}, written by `auth login` and removed by `auth logout`.")
w(f"- **Writes went only to the sandbox**: group `{GROUP}` (admin API, with a second project `{GROUP}/twin/sandbox`) "
  f"and project `{GROUP}/sandbox` (the tool's own `project create`). The scope granted `change`, `configure`, "
  f"`artifact` and `dangerous` on the targets `{GROUP}` (subtree) and `profiles/{PROFILE}` (exact), with `outputs` "
  "limited to the run's artifacts folder.")
w("- **CI data.** `.gitlab-ci.yml` (committed by the tool) has a `build` job that leaves `out/result.txt` as an "
  "artifact and a `flaky` job that fails, so pipelines fail with one failed job and one artifact.")
w("- **Every write** ran as a dry-run and then a confirm with its token; dangerous commands carried `--dangerous` "
  "in both. Each confirmed write is followed by a read-back through the admin API (`read_back` in the JSON).")
w("")
V = live.ContractLite(live.REPO / "contract" / "contract.json")  # the budgets only
B03, B04 = stdout_chars("B03"), stdout_chars("B04")
IB, CB = V.INDEX_BUDGET_CHARS, V.COMMAND_BUDGET_CHARS
intro = {
    "builtins": "aicli's built-ins against the live host: `doctor`'s connectivity and credential checks pass; "
                f"`reference --index` and `reference --command \"mr create\"` stay within their budgets ({B03:,} of "
                f"{IB:,} and {B04:,} of {CB:,} characters)." if B03 and B04 else "aicli's built-ins against the live host.",
    "auth": f"Sign-in stores {OWNER}'s token, read from stdin (`--token-file -`), in the {KEYRING} and makes "
            f"`{PROFILE}` the default profile; sign-out removes the entry.",
    "project": f"P05 reads the new project by its numeric ID; P06 asks for it by its bare name, which the second "
               f"project setup made (`{GROUP}/twin/sandbox`) shares.",
    "repo": "R01 is the first commit of the new, empty repository. R15 is the dangerous branch delete without "
            "`--dangerous`.",
    "issue": "I03 repeats an applied create with the same `--idempotency-key`: the gate replays it at the dry-run, "
             "without a token and without a request.",
    "mr": f"The merge request goes from a branch through review to the dangerous merge; {REVIEWER} approves it.",
    "pipeline": "C02 reads the CI commit's pipeline (failed: `flaky` fails); C10 retries the README commit's "
                "pipeline.",
    "job": "C06 is the log tail as JSON, C07 the whole log raw; C08 saves the `build` job's artifacts archive as a "
           "new file inside the scope's `outputs` folder, and the read-back opens the zip.",
    "variable": "The values come from files (`--value-file`), so no argv holds them; previews, receipts and results "
                "show `***`. The read-back compares GitLab's value with the file inside the harness, and the run's "
                "outputs (stdout and stderr of every call) were searched for both values.",
    "search": "The searches run on the sandbox's own issues, merge request, files and commits.",
    "gate": "The gate, live, on issue #1: a bare call (X01); a confirm used twice (X03, `token_used`, nothing "
            "sent); a confirm with another title (X04, `token_mismatch` naming `--title`); the issue changed "
            "through the admin API between dry-run and confirm (X05, `state_drift` with a new plan); no scope file "
            "(X06, `E_FORBIDDEN` with `grant_example`); a scope with `limits.max_operations: 1` after its one "
            f"operation (X08, `budget_spent`); and `--profile {PROFILE}` at the dry-run but `{PROFILE}-alt` at the "
            "confirm, the same token in `GITLAB_CLI_TOKEN` (X09, `token_mismatch` naming `profile`). The dangerous "
            "command without `--dangerous` is R15, the idempotent replay I03.",
    "lost-reply": "The spec's fault proxy (`tools/agent-eval/harness/faultproxy.py`) sat between the tool and "
                  "GitLab, armed for one `POST …/issues/:iid/notes`: it let the comment reach GitLab and closed the "
                  "connection without a reply. The confirm (LR01) is `E_OUTCOME_UNKNOWN` with an operation id and is "
                  "not retried; the operation shows as `unknown` (LR02); another write to the same issue is held "
                  "back (LR03); the comment is there (LR04); `operation resolve --outcome applied --evidence …` "
                  "records it (LR05, LR06); the admin API counts the probe comments; and a new write to the issue "
                  "works again (LR07). The proxy's log is `fault_proxy` in the JSON.",
}
by_area = {}
for call in calls:
    by_area.setdefault(call["area"], []).append(call)
for anchor in SECTIONS:
    rows = by_area.get(anchor, [])
    if not rows:
        continue
    w(f"## {anchor}")
    w("")
    if anchor in intro:
        w(intro[anchor])
        w("")
    w("| Call | Command line | Exit | Result |")
    w("|---|---|---|---|")
    for call in rows:
        mark = "" if call["pass"] else " **(unexpected)**"
        w(f"| {call['id']} | `{short(call['argv'][1:])}` | {call['exit']} | {fact(call)}{mark} |")
    w("")
if c.get("lost_reply", "").startswith("skipped"):
    w("## lost-reply")
    w("")
    w("Not run: `GITLAB_E2E_FAULTPROXY` was not set.")
    w("")
w("## Commands")
w("")
w(f"Verified here ({len(rec['verified_commands'])}): " + ", ".join(f"`{x}`" for x in rec["verified_commands"]) + ".")
w("")
text = "\n".join(lines) + "\n"
assert not live.leaks(text)
MD_OUT.write_bytes(text.encode("utf-8"))
print("wrote", MD_OUT.relative_to(live.REPO).as_posix(), len(text), "chars")
