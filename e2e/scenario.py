"""gitlab-cli's live suite against a real GitLab, phase by phase (docs/E2E.md).

    python e2e/scenario.py run                  setup, every phase, then teardown (also after a failure)
    python e2e/scenario.py setup|teardown       create or delete the sandbox
    python e2e/scenario.py all                  every phase, in order
    python e2e/scenario.py <phase> [<phase>...] some phases (they build on the earlier ones)
    python e2e/scenario.py --help               this text and the environment variables

The tool writes only inside the sandbox group, which setup creates with the admin API (with a second project
named like the tool's, for the ambiguous-name check) and teardown deletes. The run's scope file grants the tool
that group and its own keyring profile, nothing else.
"""

from __future__ import annotations

import json
import os
import secrets as _secrets
import shutil
import subprocess
import sys
import time
import urllib.parse
import zipfile
from pathlib import Path

if __name__ == "__main__" and (len(sys.argv) < 2 or any(a in ("-h", "--help") for a in sys.argv[1:])):
    # help needs no configuration: live.py only resolves this path, it creates nothing
    os.environ.setdefault("GITLAB_E2E_RUN_DIR", str(Path.home() / "gitlab-cli-e2e-help"))

import live  # noqa: E402
from live import (HOST, OUT, OWNER, PROFILE, REVIEWER, RUN, STATE, api, call, ctx, ctx_set,  # noqa: E402
                  err_code, ok_true, stdout_doc, token_of, utc_now, write_scope)

G = live.GROUP
P = f"{G}/sandbox"
PQ = urllib.parse.quote(P, safe="")
TWIN = f"{G}/twin/sandbox"
ALT_PROFILE = f"{PROFILE}-alt"
ARTIFACTS = OUT / "artifacts"
SCRATCH = RUN / "scratch"
SETTINGS = STATE / "settings.json"


def nonce() -> str:
    return ctx().get("nonce") or ctx_set(nonce=_secrets.token_hex(3))["nonce"]


def k(cid, argv, user=None, **kw):
    """A tool call as the keyring profile (user None) unless a user is named."""
    return call(cid, argv, user, **kw)


def data(cid):
    return (stdout_doc(cid) or {}).get("data") or {}


def dry_ok(doc, code):
    if not doc or doc.get("ok") is not True or code != 0:
        return False, f"dry-run failed: exit {code}"
    d = doc.get("data") or {}
    if not d.get("confirm_token"):
        return False, "no confirm_token"
    pv = d.get("preview") or {}
    if pv.get("can_apply") is not True:
        return False, f"preview cannot apply: {pv.get('blocked_by')}"
    return True, ""


def applied(doc, code):
    if not doc or doc.get("ok") is not True or code != 0:
        return False, f"confirm failed: exit {code}"
    d = doc.get("data") or {}
    r = d.get("receipt")
    if r is None:  # a batch
        s = d.get("summary") or {}
        bad = {kk: v for kk, v in s.items() if kk in ("failed", "unknown", "not_applied", "blocked") and v}
        return (not bad), f"batch summary {s}"
    o = r.get("outcome") or {}
    if o.get("change") == "applied" or o.get("persistence") == "saved":
        return True, f"verification={o.get('verification')}"
    return False, f"outcome {o}"


def pair(cid, argv, *, area, user=None, dangerous=False, expect="", read_back=None, env=None, stdin=None,
         host=None, check_conf=None):
    flags = ["--dangerous"] if dangerous else []
    k(f"{cid}-dry", argv + flags + ["--dry-run"], user, area=area, env=env, stdin=stdin, host=host,
      expect="dry-run: a preview that can apply and a confirm token", check=dry_ok)
    c, _ = k(cid, argv + flags + ["--confirm", f"@tok:{cid}-dry"], user, area=area, env=env, stdin=stdin, host=host,
             expect=expect or "the confirmed write is applied", check=check_conf or applied, read_back=read_back)
    return c


def gl(path, **q):
    s, d = api("GET", path, query=q or None)
    return d if s == 200 else {"status": s}


def pick(d, *keys):
    return {kk: d.get(kk) for kk in keys} if isinstance(d, dict) else d


def keyring_entry_exists() -> bool | None:
    """Whether the Windows Credential Manager holds the profile's entry (cmdkey lists names, never secrets; it
    echoes the target in its header even when nothing is stored, so an entry shows the name twice). None on other
    systems, where no command lists an entry without reading its secret."""
    if os.name != "nt":
        return None
    target = f"gitlab-cli:profile:{PROFILE}"
    out = subprocess.run(["cmdkey", f"/list:{target}"], capture_output=True).stdout
    return out.count(target.encode()) >= 2


def problem_is(want):
    """details.problem (E_CONFIRMATION_REQUIRED) or details.conflict (E_CONFLICT) is want."""
    def chk(doc, code):
        e = (doc or {}).get("error") or {}
        d = e.get("details") or {}
        p = d.get("problem") or d.get("conflict")
        return p == want, f"code={e.get('code')} {('conflict' if 'conflict' in d else 'problem')}={p} exit={code}"
    return chk


# ---------------------------------------------------------------- built-ins, auth, project

def builtins_pre():
    a = "builtins"
    k("B01", ["--version"], area=a, expect="prints the version", check=lambda d, c: (c == 0, ""))
    k("B02", ["context"], area=a, expect="context before sign-in: no credentials, scope missing", check=ok_true)
    v = live.validator()

    def budget(limit_name):
        lim = getattr(v, limit_name)

        def chk(doc, code):
            raw = json.dumps(doc, separators=(",", ":"), ensure_ascii=False)
            return (code == 0 and len(raw) <= lim), f"{len(raw)} chars, budget {lim}"
        return chk
    k("B03", ["reference", "--index"], area=a, expect="index within INDEX_BUDGET_CHARS",
      check=budget("INDEX_BUDGET_CHARS"))
    k("B04", ["reference", "--command", "mr create"], area=a, expect="one command within COMMAND_BUDGET_CHARS",
      check=budget("COMMAND_BUDGET_CHARS"))
    k("B05", ["changelog"], area=a, expect="changelog within the success budget", check=ok_true)


def auth_login():
    a = "auth"
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    scope = write_scope(scope_id=f"e2e-{nonce()}", effects=["change", "configure", "artifact"], dangerous=True,
                        targets=[{"key": G}, {"key": f"profiles/{PROFILE}", "match": "exact"}],
                        outputs=[str(ARTIFACTS)],
                        notes=f"gitlab-cli live suite: the sandbox group {G} and the profile {PROFILE} only.")
    ctx_set(scope=json.loads(scope.read_text(encoding="utf-8")))  # for the record: teardown deletes the file
    tok = token_of(OWNER).encode()

    def rb(doc):
        s = json.loads(SETTINGS.read_text(encoding="utf-8")) if SETTINGS.exists() else None
        return {"settings.json": s, "credential_manager_entry": keyring_entry_exists()}
    pair("A01", ["auth", "login", "--profile", PROFILE, "--host", HOST, "--token-file", "-"], area=a,
         dangerous=True, stdin=tok, expect=f"token stored in the keyring for profile {PROFILE}", read_back=rb)
    k("A02", ["auth", "status"], area=a, expect=f"the keyring token signs in as {OWNER}", check=ok_true)
    k("A03", ["context"], area="builtins", expect="context: keyring credentials, scope granted", check=ok_true)
    k("A04", ["doctor"], area="builtins", expect="doctor: connectivity and credentials pass against the live host",
      check=lambda d, c: (c == 0 and all(x["status"] != "fail" for x in d["data"]["checks"]) and
                          any(x["check"] == "connectivity" and x["status"] == "pass" for x in d["data"]["checks"]),
                          "checks: " + ",".join(f"{x['check']}={x['status']}" for x in d["data"]["checks"])))


def project():
    a = "project"
    pair("P01", ["project", "create", "--name", "sandbox", "--namespace", G, "--visibility", "private",
                 "--description", "gitlab-cli live suite"], area=a,
         expect=f"project {P} created",
         read_back=lambda d: pick(gl(f"/projects/{PQ}"), "id", "path_with_namespace", "visibility", "description"))
    pid = gl(f"/projects/{PQ}").get("id")
    ctx_set(project_id=pid)
    k("P02", ["project", "get", P], area=a, expect="reads the new project", check=ok_true)
    k("P03", ["project", "list", "--membership", "--limit", "5"], area=a, expect="lists projects", check=ok_true)
    k("P04", ["project", "members", P], area=a, expect="lists members with membership", check=ok_true)
    k("P05", ["project", "get", str(pid)], area=a, expect="reads the project by its numeric ID",
      check=lambda d, c: (ok_true(d, c)[0] and (d.get("data") or {}).get("ref") == P, f"exit {c}"))
    k("P06", ["project", "get", "sandbox"], area=a,
      expect=f"a name two projects share ({P}, {TWIN}) is E_AMBIGUOUS", check=err_code("E_AMBIGUOUS"))


# ---------------------------------------------------------------- repository

CI_YML = """# gitlab-cli live suite: one job leaves an artifact, one fails on purpose.
build:
  script:
    - echo "e2e build of $CI_COMMIT_SHORT_SHA"
    - mkdir -p out
    - echo "artifact of job $CI_JOB_ID" > out/result.txt
  artifacts:
    paths:
      - out/
    expire_in: 1 day

flaky:
  script:
    - echo "this job fails on purpose"
    - exit 1
"""


def file_rb(path, ref="main"):
    def rb(doc):
        f = gl(f"/projects/{PQ}/repository/files/{urllib.parse.quote(path, safe='')}", ref=ref)
        return pick(f, "file_path", "size", "last_commit_id", "content_sha256")
    return rb


def repo():
    a = "repo"
    SCRATCH.mkdir(parents=True, exist_ok=True)
    ci = SCRATCH / "gitlab-ci.yml"
    ci.write_text(CI_YML, encoding="utf-8", newline="\n")
    pair("R01", ["repo", "file", "create", "README.md", "--project", P, "--branch", "main", "--commit-message",
                 "docs: add README", "--content", "# sandbox\n\nLive validation of gitlab-cli.\n"],
         area=a, expect="first commit of an empty repository", read_back=file_rb("README.md"))
    pair("R02", ["repo", "file", "create", ".gitlab-ci.yml", "--project", P, "--branch", "main", "--commit-message",
                 "ci: one job with an artifact, one that fails", "--content-file", str(ci)],
         area=a, expect="CI file committed from --content-file", read_back=file_rb(".gitlab-ci.yml"))
    pair("R03", ["repo", "file", "update", "README.md", "--project", P, "--branch", "main", "--commit-message",
                 "docs: say what the sandbox is for",
                 "--content", "# sandbox\n\nLive validation of gitlab-cli.\nDeleted after the run.\n"],
         area=a, expect="file updated in a new commit", read_back=file_rb("README.md"))
    k("R04", ["repo", "tree", "--project", P], area=a, expect="tree of the default branch", check=ok_true)
    k("R05", ["repo", "file", "get", "README.md", "--project", P], area=a, expect="file metadata", check=ok_true)
    k("R06", ["repo", "file", "content", "README.md", "--project", P], area=a, expect="file content window",
      check=ok_true)
    k("R07", ["repo", "file", "get", "README.md", "--project", P, "--format", "raw"], area=a,
      expect="raw bytes of the file", check=lambda d, c: (c == 0, ""))
    k("R08", ["repo", "branch", "list", "--project", P], area=a, expect="lists branches", check=ok_true)
    k("R09", ["repo", "branch", "get", "main", "--project", P], area=a, expect="reads one branch", check=ok_true)
    k("R10", ["repo", "commit", "list", "--project", P], area=a, expect="lists commits", check=ok_true)
    sha = data("R10")["items"][0]["sha"] if data("R10").get("items") and "sha" in data("R10")["items"][0] else None
    if sha is None:
        sha = gl(f"/projects/{PQ}/repository/commits", ref_name="main")[0]["id"]
    ctx_set(readme_sha=sha)
    k("R11", ["repo", "commit", "get", sha, "--project", P], area=a, expect="reads one commit", check=ok_true)
    k("R12", ["repo", "commit", "diff", sha, "--project", P], area=a, expect="the commit's diff", check=ok_true)
    pair("R13", ["repo", "branch", "create", "feature/live", "--project", P, "--ref", "main"], area=a,
         expect="branch created",
         read_back=lambda d: pick(gl(f"/projects/{PQ}/repository/branches/feature%2Flive"), "name", "protected"))
    pair("R14", ["repo", "branch", "create", "tmp/delete-me", "--project", P, "--ref", "main"], area=a,
         expect="branch created", read_back=lambda d: pick(gl(f"/projects/{PQ}/repository/branches/tmp%2Fdelete-me"),
                                                            "name"))
    k("R15", ["repo", "branch", "delete", "tmp/delete-me", "--project", P, "--dry-run"], area="gate",
      expect="a dangerous command without --dangerous: exit 5 dangerous_flag_required",
      check=problem_is("dangerous_flag_required"))
    pair("R16", ["repo", "branch", "delete", "tmp/delete-me", "--project", P], area=a, dangerous=True,
         expect="branch deleted",
         read_back=lambda d: {"status": api("GET", f"/projects/{PQ}/repository/branches/tmp%2Fdelete-me")[0]})
    pair("R17", ["repo", "commit", "create", "--project", P, "--branch", "feature/live", "--message",
                 "docs: add the live validation page",
                 "--action", "create:path=docs/live.md;content=# Live validation\n\nWritten by gitlab-cli.\n"],
         area=a, expect="atomic commit on the feature branch", read_back=file_rb("docs/live.md", "feature/live"))


# ---------------------------------------------------------------- labels and milestones

def label_milestone():
    a = "label"
    pair("L01", ["label", "create", "--project", P, "--name", "e2e", "--color", "#1f78d1", "--description",
                 "Made by the live suite"], area=a, expect="label created",
         read_back=lambda d: [pick(x, "name", "color", "description") for x in gl(f"/projects/{PQ}/labels")
                              if x["name"] == "e2e"])
    pair("L02", ["label", "create", "--project", P, "--name", "obsolete", "--color", "#cccccc"], area=a,
         expect="label created")
    k("L03", ["label", "list", "--project", P], area=a, expect="lists labels", check=ok_true)
    k("L04", ["label", "get", "e2e", "--project", P], area=a, expect="reads one label", check=ok_true)
    pair("L05", ["label", "update", "e2e", "--project", P, "--description", "Live suite label (updated)"],
         area=a, expect="label updated",
         read_back=lambda d: [pick(x, "name", "description") for x in gl(f"/projects/{PQ}/labels")
                              if x["name"] == "e2e"])
    pair("L06", ["label", "delete", "obsolete", "--project", P], area=a, dangerous=True, expect="label deleted",
         read_back=lambda d: {"obsolete_exists": any(x["name"] == "obsolete" for x in gl(f"/projects/{PQ}/labels"))})
    a = "milestone"
    pair("M01", ["milestone", "create", "--project", P, "--title", "Sprint 1", "--due-date",
                 time.strftime("%Y-%m-%d", time.gmtime(time.time() + 14 * 86400))],
         area=a, expect="milestone created",
         read_back=lambda d: [pick(x, "iid", "title", "due_date", "state") for x in gl(f"/projects/{PQ}/milestones")])
    k("M02", ["milestone", "list", "--project", P], area=a, expect="lists milestones", check=ok_true)
    k("M03", ["milestone", "get", "Sprint 1", "--project", P], area=a, expect="reads a milestone by title",
      check=ok_true)
    pair("M04", ["milestone", "update", "1", "--project", P, "--description", "Live suite sprint"], area=a,
         expect="milestone updated",
         read_back=lambda d: [pick(x, "iid", "description") for x in gl(f"/projects/{PQ}/milestones")])


# ---------------------------------------------------------------- issues

def issue_rb(iid, *keys):
    keys = keys or ("iid", "title", "state", "labels", "updated_at")
    return lambda d: pick(gl(f"/projects/{PQ}/issues/{iid}"), *keys)


def issues():
    a = "issue"
    pair("I01", ["issue", "create", "--project", P, "--title", "Live validation issue A", "--description",
                 "Created by gitlab-cli during the live validation.", "--label", "e2e", "--milestone", "Sprint 1"],
         area=a, expect="issue created with a label and a milestone",
         read_back=issue_rb(1, "iid", "title", "labels", "milestone"))
    key = f"live-idem-{nonce()}"
    pair("I02", ["issue", "create", "--project", P, "--title", "Live validation issue B", "--idempotency-key", key],
         area=a, expect="issue created with an idempotency key", read_back=issue_rb(2, "iid", "title"))
    k("I03", ["issue", "create", "--project", P, "--title", "Live validation issue B", "--idempotency-key", key,
              "--dry-run"], area="gate", expect="the same key repeated: replayed at the dry-run, no token",
      check=lambda d, c: ((d or {}).get("ok") is True and ((d.get("data") or {}).get("receipt") or {}).get("replayed")
                          is True and "confirm_token" not in (d.get("data") or {}), f"exit {c}"))
    k("I04", ["issue", "list", "--project", P], area=a, expect="lists open issues", check=ok_true)
    k("I05", ["issue", "get", f"{P}#1"], area=a, expect="reads one issue by ref", check=ok_true)
    pair("I06", ["issue", "update", f"{P}#1", "--title", "Live validation issue A (updated)"], area=a,
         expect="title updated", read_back=issue_rb(1, "iid", "title"))
    pair("I07", ["issue", "comment", "create", f"{P}#1", "--body", "First comment from the live validation."],
         area=a, expect="comment added",
         read_back=lambda d: [pick(n, "id", "body") for n in gl(f"/projects/{PQ}/issues/1/notes") if not n["system"]])
    k("I08", ["issue", "comment", "list", f"{P}#1"], area=a, expect="lists comments", check=ok_true)
    pair("I09", ["issue", "bulk", "label", "--project", P, "--ids", "1,2", "--add", "e2e"], area=a,
         expect="bulk: both issues carry the label",
         read_back=lambda d: [pick(gl(f"/projects/{PQ}/issues/{i}"), "iid", "labels") for i in (1, 2)])
    pair("I10", ["issue", "close", f"{P}#2"], area=a, expect="issue closed", read_back=issue_rb(2, "iid", "state"))
    pair("I11", ["issue", "reopen", f"{P}#2"], area=a, expect="issue reopened", read_back=issue_rb(2, "iid", "state"))
    pair("I12", ["issue", "assign", f"{P}#1", "--assignee", REVIEWER], area=a, expect=f"issue assigned to {REVIEWER}",
         read_back=lambda d: {"assignees": [x["username"] for x in gl(f"/projects/{PQ}/issues/1")["assignees"]]})
    note_id = [n for n in gl(f"/projects/{PQ}/issues/1/notes") if not n["system"]][0]["id"]
    pair("I13", ["issue", "comment", "delete", f"{P}#1", "--note-id", str(note_id)], area=a, dangerous=True,
         expect="comment deleted",
         read_back=lambda d: {"note_status": api("GET", f"/projects/{PQ}/issues/1/notes/{note_id}")[0]})
    pair("I14", ["milestone", "close", "1", "--project", P], area="milestone", expect="milestone closed",
         read_back=lambda d: [pick(x, "iid", "state") for x in gl(f"/projects/{PQ}/milestones")])


# ---------------------------------------------------------------- the gate, live

def gate():
    a = "gate"
    tgt = f"{P}#1"
    k("X01", ["issue", "update", tgt, "--title", "Gate: bare call"], area=a,
      expect="a bare write: exit 5 confirmation_required with a preview and a token",
      check=lambda d, c: (c == 5 and problem_is("confirmation_required")(d, c)[0] and
                          bool(((d or {}).get("error") or {}).get("details", {}).get("confirm_token")), f"exit {c}"))
    pair("X02", ["issue", "update", tgt, "--title", "Gate: confirmed once"], area=a, expect="applied once",
         read_back=issue_rb(1, "iid", "title"))
    k("X03", ["issue", "update", tgt, "--title", "Gate: confirmed once", "--confirm", "@tok:X02-dry"], area=a,
      expect="the same token again: token_used, nothing sent", check=problem_is("token_used"),
      read_back=issue_rb(1, "iid", "title", "updated_at"))
    k("X04-dry", ["issue", "update", tgt, "--title", "Gate: planned title", "--dry-run"], area=a,
      expect="dry-run", check=dry_ok)
    k("X04", ["issue", "update", tgt, "--title", "Gate: other title", "--confirm", "@tok:X04-dry"], area=a,
      expect="a mutated confirm: token_mismatch naming --title",
      check=lambda d, c: (problem_is("token_mismatch")(d, c)[0] and
                          "title" in str(((d or {}).get("error") or {}).get("details", {}).get("parameter")),
                          str(((d or {}).get("error") or {}).get("details", {}).get("parameter"))),
      read_back=issue_rb(1, "iid", "title"))
    k("X05-dry", ["issue", "update", tgt, "--title", "Gate: drift", "--dry-run"], area=a, expect="dry-run",
      check=dry_ok)
    s, _ = api("PUT", f"/projects/{PQ}/issues/1", {"description": f"Changed by the admin API between dry-run and "
                                                                 f"confirm ({nonce()})."})
    assert s == 200, s
    k("X05", ["issue", "update", tgt, "--title", "Gate: drift", "--confirm", "@tok:X05-dry"], area=a,
      expect="the issue changed after the dry-run (admin API): state_drift, nothing sent",
      check=problem_is("state_drift"), read_back=issue_rb(1, "iid", "title", "description"))
    missing = STATE / "no-such-scope.json"
    k("X06", ["issue", "create", "--project", P, "--title", "No scope", "--dry-run"], area=a,
      env={"GITLAB_CLI_SCOPE_FILE": str(missing)},
      expect="no scope file: E_FORBIDDEN with details.scope.grant_example",
      check=lambda d, c: (err_code("E_FORBIDDEN")(d, c)[0] and
                          bool(((d or {}).get("error") or {}).get("details", {}).get("scope", {}).get("grant_example")),
                          f"exit {c}"))
    limited = write_scope("scope-limited.json", scope_id=f"e2e-limited-{nonce()}", effects=["change"],
                          dangerous=False, targets=[{"key": G}], limits={"max_operations": 1})
    env = {"GITLAB_CLI_SCOPE_FILE": str(limited)}
    pair("X07", ["issue", "comment", "create", tgt, "--body", "Within a one-operation scope."], area=a, env=env,
         expect="the one operation the scope allows")
    k("X08", ["issue", "comment", "create", tgt, "--body", "Over the budget.", "--dry-run"], area=a, env=env,
      expect="limits.max_operations reached: E_FORBIDDEN budget_spent",
      check=lambda d, c: (err_code("E_FORBIDDEN")(d, c)[0] and
                          "budget" in json.dumps((d or {}).get("error", {}).get("details", {})), f"exit {c}"))
    # two profiles in settings.json (a person's edit): the confirm names the other one
    s = json.loads(SETTINGS.read_text(encoding="utf-8"))
    s["profiles"][ALT_PROFILE] = {"host": HOST}
    SETTINGS.write_text(json.dumps(s, indent=1), encoding="utf-8")
    k("X09-dry", ["issue", "update", tgt, "--title", "Gate: profile", "--profile", PROFILE, "--dry-run"],
      OWNER, host="", area=a, expect=f"dry-run under profile {PROFILE}", check=dry_ok)
    k("X09", ["issue", "update", tgt, "--title", "Gate: profile", "--profile", ALT_PROFILE, "--confirm",
              "@tok:X09-dry"], OWNER, host="", area=a,
      expect="--profile switched at confirm: token_mismatch naming the profile",
      check=lambda d, c: (problem_is("token_mismatch")(d, c)[0] and
                          "profile" in str(((d or {}).get("error") or {}).get("details", {}).get("parameter")),
                          str(((d or {}).get("error") or {}).get("details", {}).get("parameter"))))


# ---------------------------------------------------------------- merge requests

def mr_rb(iid, *keys):
    keys = keys or ("iid", "title", "state", "labels")
    return lambda d: pick(gl(f"/projects/{PQ}/merge_requests/{iid}"), *keys)


def mrs():
    a = "mr"
    mr = f"{P}!1"
    pair("MR01", ["mr", "create", "--project", P, "--source-branch", "feature/live", "--target-branch", "main",
                  "--title", "Add the live validation page", "--description", "Made by gitlab-cli."], area=a,
         expect="merge request created from a branch",
         read_back=mr_rb(1, "iid", "title", "state", "source_branch", "target_branch"))
    k("MR02", ["mr", "list", "--project", P], area=a, expect="lists open merge requests", check=ok_true)
    k("MR03", ["mr", "get", mr], area=a, expect="reads one merge request", check=ok_true)
    k("MR04", ["mr", "diff", mr], area=a, expect="files changed", check=ok_true)
    pair("MR05", ["mr", "update", mr, "--title", "Add the live validation page (reviewed)", "--add-labels", "e2e"],
         area=a, expect="title and label changed", read_back=mr_rb(1, "iid", "title", "labels"))
    pair("MR06", ["mr", "comment", "add", mr, "--body", "Looks good to the live validation."], area=a,
         expect="comment added",
         read_back=lambda d: [pick(n, "id", "body") for n in gl(f"/projects/{PQ}/merge_requests/1/notes")
                              if not n["system"]])
    k("MR07", ["mr", "comment", "list", mr], area=a, expect="lists comments", check=ok_true)
    pair("MR08", ["mr", "discussion", "create", mr, "--new-path", "docs/live.md", "--new-line", "1", "--body",
                  "Is this heading right?"], area=a, expect="inline thread on line 1 of docs/live.md",
         read_back=lambda d: [{"id": x["id"], "position": pick(x["notes"][0].get("position") or {}, "new_path",
                                                                "new_line")}
                              for x in gl(f"/projects/{PQ}/merge_requests/1/discussions") if not x["individual_note"]])
    k("MR09", ["mr", "discussion", "list", mr], area=a, expect="lists threads", check=ok_true)
    disc = [x["id"] for x in gl(f"/projects/{PQ}/merge_requests/1/discussions") if not x["individual_note"]][0]
    ctx_set(discussion_id=disc)
    k("MR10", ["mr", "discussion", "get", mr, "--discussion-id", disc], area=a, expect="reads one thread",
      check=ok_true)
    pair("MR11", ["mr", "discussion", "reply", mr, "--discussion-id", disc, "--body", "Yes, it is."], area=a,
         expect="reply added", read_back=lambda d: {"notes": len(gl(f"/projects/{PQ}/merge_requests/1/discussions/{disc}")
                                                                  ["notes"])})
    pair("MR12", ["mr", "discussion", "resolve", mr, "--discussion-id", disc], area=a, expect="thread resolved",
         read_back=lambda d: {"resolved": all(n.get("resolved") for n in
                                              gl(f"/projects/{PQ}/merge_requests/1/discussions/{disc}")["notes"]
                                              if n.get("resolvable"))})
    pair("MR13", ["mr", "approve", mr], area=a, user=REVIEWER, expect=f"{REVIEWER} approves",
         read_back=lambda d: {"approved_by": [x["user"]["username"] for x in
                                              gl(f"/projects/{PQ}/merge_requests/1/approvals").get("approved_by", [])]})
    pair("MR14", ["mr", "merge", mr], area=a, dangerous=True, expect="merged into main",
         read_back=mr_rb(1, "iid", "state", "merge_commit_sha"))


# ---------------------------------------------------------------- releases

def releases():
    a = "release"
    pair("RL01", ["release", "create", "v0.1.0", "--project", P, "--ref", "main", "--name", "v0.1.0",
                  "--description", "First sandbox release."], area=a, expect="release and tag created",
         read_back=lambda d: pick(gl(f"/projects/{PQ}/releases/v0.1.0"), "tag_name", "name", "description"))
    k("RL02", ["release", "list", "--project", P], area=a, expect="lists releases", check=ok_true)
    k("RL03", ["release", "get", "v0.1.0", "--project", P], area=a, expect="reads one release", check=ok_true)
    pair("RL04", ["release", "update", "v0.1.0", "--project", P, "--description", "First sandbox release (notes "
                                                                                  "updated)."],
         area=a, expect="notes updated",
         read_back=lambda d: pick(gl(f"/projects/{PQ}/releases/v0.1.0"), "tag_name", "description"))


# ---------------------------------------------------------------- pipelines and jobs

def wait_pipeline(pid, limit=240):
    t = time.monotonic()
    while time.monotonic() - t < limit:
        p = gl(f"/projects/{PQ}/pipelines/{pid}")
        if p.get("status") in ("success", "failed", "canceled", "skipped"):
            return p
        time.sleep(3)
    raise SystemExit(f"pipeline {pid} did not finish in {limit}s: is a runner registered (docs/E2E.md)?")


def ci():
    a = "pipeline"
    sha = gl(f"/projects/{PQ}/repository/commits", ref_name="main", per_page=50)
    ci_sha = [c["id"] for c in sha if c["title"].startswith("ci:")][0]
    pls = gl(f"/projects/{PQ}/pipelines", sha=ci_sha)
    if not pls:
        raise SystemExit("no pipeline for the CI commit")
    pid = pls[0]["id"]
    wait_pipeline(pid)
    jobs = gl(f"/projects/{PQ}/pipelines/{pid}/jobs")
    build = [j["id"] for j in jobs if j["name"] == "build"][0]
    flaky = [j["id"] for j in jobs if j["name"] == "flaky"][0]
    ctx_set(pipeline=pid, build_job=build, flaky_job=flaky)
    k("C01", ["pipeline", "list", "--project", P], area=a, expect="lists pipelines", check=ok_true)
    k("C02", ["pipeline", "get", str(pid), "--project", P, "--wait", "90s"], area=a,
      expect="reads a finished pipeline (failed: one job fails)", check=ok_true)
    k("C03", ["pipeline", "jobs", str(pid), "--project", P], area=a, expect="jobs with the failure reason",
      check=ok_true)

    def new_pipeline_rb(d):
        ref = (((d or {}).get("data") or {}).get("result") or {}).get("ref", "")
        new = ref.rsplit("/", 1)[-1]
        return pick(gl(f"/projects/{PQ}/pipelines/{new}"), "id", "ref", "status", "source") if new.isdigit() else {}
    pair("C04", ["pipeline", "create", "--project", P, "--ref", "main"], area=a, expect="pipeline started",
         read_back=new_pipeline_rb)
    a = "job"
    k("C05", ["job", "get", str(build), "--project", P], area=a, expect="reads a job", check=ok_true)
    k("C06", ["job", "log", str(flaky), "--project", P], area=a, expect="log tail as JSON", check=ok_true)
    k("C07", ["job", "log", str(flaky), "--project", P, "--format", "raw"], area=a, expect="the whole log, raw",
      check=lambda d, c: (c == 0, ""))
    out = ARTIFACTS / f"build-{build}.zip"

    def zip_rb(d):
        if not out.exists():
            return {"exists": False}
        with zipfile.ZipFile(out) as z:
            return {"exists": True, "bytes": out.stat().st_size, "entries": z.namelist(),
                    "result_txt": z.read("out/result.txt").decode().strip() if "out/result.txt" in z.namelist()
                    else None}
    pair("C08", ["job", "artifacts", str(build), "--project", P, "--output", str(out)], area=a,
         expect="artifacts archive saved under the scope's outputs directory", read_back=zip_rb)

    def retried_rb(d):
        js = gl(f"/projects/{PQ}/pipelines/{pid}/jobs", include_retried="true")
        return [pick(j, "id", "name", "status", "retried") for j in js if j["name"] == "flaky"]
    pair("C09", ["job", "retry", str(flaky), "--project", P], area=a, expect="failed job runs again",
         read_back=retried_rb)
    # the README update's pipeline (failed: flaky fails) is retried as a whole
    readme_sha = [c["id"] for c in gl(f"/projects/{PQ}/repository/commits", ref_name="main", per_page=50)
                  if c["title"].startswith("docs: say what")][0]
    pid2 = gl(f"/projects/{PQ}/pipelines", sha=readme_sha)[0]["id"]
    wait_pipeline(pid2)
    ctx_set(pipeline_retried=pid2)
    pair("C10", ["pipeline", "retry", str(pid2), "--project", P], area="pipeline",
         expect="the failed pipeline's failed jobs run again",
         read_back=lambda d: {"pipeline": pick(gl(f"/projects/{PQ}/pipelines/{pid2}"), "id", "status"),
                              "jobs": [pick(j, "id", "name", "status") for j in
                                       gl(f"/projects/{PQ}/pipelines/{pid2}/jobs", include_retried="true")]})


# ---------------------------------------------------------------- variables

def variables():
    a = "variable"
    v1, v2 = "pv1" + _secrets.token_hex(12), "pv2" + _secrets.token_hex(12)
    live.EXTRA_SECRETS.extend([v1, v2])
    SCRATCH.mkdir(parents=True, exist_ok=True)
    f1, f2 = SCRATCH / "var-value-1.txt", SCRATCH / "var-value-2.txt"
    f1.write_text(v1, encoding="utf-8")
    f2.write_text(v2, encoding="utf-8")

    def var_rb(want):
        def rb(d):
            s, v = api("GET", f"/projects/{PQ}/variables/E2E_SECRET")
            if s != 200:
                return {"status": s}
            return {"key": v["key"], "masked": v["masked"], "protected": v["protected"],
                    "value_matches_file": v["value"] == want}
        return rb
    pair("V01", ["variable", "create", "--project", P, "--key", "E2E_SECRET", "--value-file", str(f1),
                 "--masked"], area=a, dangerous=True, expect="masked variable created; value never shown",
         read_back=var_rb(v1))
    k("V02", ["variable", "list", "--project", P], area=a, expect="lists variables without values", check=ok_true)
    k("V03", ["variable", "get", "E2E_SECRET", "--project", P], area=a, expect="settings, never the value",
      check=ok_true)
    pair("V04", ["variable", "update", "E2E_SECRET", "--project", P, "--value-file", str(f2)], area=a,
         dangerous=True, expect="value replaced; never shown", read_back=var_rb(v2))
    pair("V05", ["variable", "delete", "E2E_SECRET", "--project", P], area=a, dangerous=True,
         expect="variable deleted",
         read_back=lambda d: {"status": api("GET", f"/projects/{PQ}/variables/E2E_SECRET")[0]})
    # the values must appear in no output of the whole run, stderr included
    seen = []
    for f in (OUT / "calls").glob("*"):
        b = f.read_bytes()
        if v1.encode() in b or v2.encode() in b:
            seen.append(f.name)
    ctx_set(variable_values_seen_in=seen)
    print("variable values found in outputs:", seen)


# ---------------------------------------------------------------- search and users

def search_users():
    """Searches run on the sandbox's own data, made by the phases before; the users are the run's accounts."""
    a = "search"
    k("S01", ["search", "projects", "--query", "sandbox"], area=a, expect="projects by name", check=ok_true)
    k("S02", ["search", "issues", "--project", P, "--query", "validation"], area=a,
      expect="issues by text", check=ok_true)
    k("S03", ["search", "mrs", "--project", P, "--query", "validation"], area=a,
      expect="merge requests by text", check=ok_true)
    k("S04", ["search", "code", "--project", P, "--query", "validation"], area=a,
      expect="code on the default branch", check=ok_true)
    k("S05", ["search", "commits", "--project", P, "--query", "README"], area=a,
      expect="commit messages", check=ok_true)
    a = "user"
    k("U01", ["user", "me"], area=a, expect="the signed-in account", check=ok_true)
    k("U02", ["user", "get", f"@{REVIEWER}"], area=a, expect="one user", check=ok_true)
    k("U03", ["user", "search", "--query", REVIEWER], area=a, expect="users by name", check=ok_true)


# ---------------------------------------------------------------- the lost reply (VER-006 scenario)

def lost_reply():
    a = "lost-reply"
    proxy = live.setting("GITLAB_E2E_FAULTPROXY")
    if not proxy:
        print("lost_reply skipped: set GITLAB_E2E_FAULTPROXY to the spec's tools/agent-eval/harness/faultproxy.py")
        ctx_set(lost_reply="skipped: GITLAB_E2E_FAULTPROXY was not set")
        return
    path = Path(proxy).expanduser().resolve()
    if path.name != "faultproxy.py" or not path.is_file():
        raise SystemExit("GITLAB_E2E_FAULTPROXY must name the spec's tools/agent-eval/harness/faultproxy.py")
    sys.path.insert(0, str(path.parent))
    import faultproxy  # standard library only
    log, stop = OUT / "faultproxy.json", OUT / "faultproxy.stop"
    plog = faultproxy.spawn(HOST, {"method": "POST", "path": r"/issues/\d+/notes$"}, log, stop, max_seconds=1200)
    url = plog["tool_url"]
    ctx_set(proxy_url=url, lost_reply="ran")
    tgt = f"{P}#2"
    body = f"Lost-reply probe {nonce()}"
    try:
        k("LR01-dry", ["issue", "comment", "create", tgt, "--body", body, "--dry-run"], OWNER, host=url, area=a,
          expect="dry-run through the armed proxy", check=dry_ok)
        k("LR01", ["issue", "comment", "create", tgt, "--body", body, "--confirm", "@tok:LR01-dry"], OWNER,
          host=url, area=a, expect="the reply is dropped after GitLab applied it: E_OUTCOME_UNKNOWN with an operation id",
          check=lambda d, c: (err_code("E_OUTCOME_UNKNOWN")(d, c)[0] and
                              bool(((d or {}).get("error") or {}).get("details", {}).get("operation_id")), f"exit {c}"))
        op = live.op_id(stdout_doc("LR01"))
        ctx_set(lost_op=op)
        k("LR02", ["operation", "show", op], OWNER, host=url, area=a, expect="the operation is unknown",
          check=ok_true)
        k("LR03", ["issue", "comment", "create", tgt, "--body", "Blocked while unknown?", "--dry-run"], OWNER,
          host=url, area=a, expect="a write to the same target while its outcome is unknown is held back",
          check=lambda d, c: ((d or {}).get("ok") is False, f"exit {c}"))
        k("LR04", ["issue", "comment", "list", tgt], OWNER, host=url, area=a,
          expect="re-read: the comment is there", check=lambda d, c: (
              ok_true(d, c)[0] and sum(1 for it in d["data"]["items"] if body in str(it.get("body", ""))) == 1,
              "comment count"))
        nid = [n["id"] for n in gl(f"/projects/{PQ}/issues/2/notes") if n.get("body") == body]
        k("LR05", ["operation", "resolve", op, "--outcome", "applied", "--evidence",
                   f"issue comment list shows the comment (note {nid[0] if nid else '?'})"], OWNER, host=url,
          area=a, expect="resolved as applied", check=ok_true,
          read_back=lambda d: {"notes_with_probe_body": len([n for n in gl(f"/projects/{PQ}/issues/2/notes")
                                                             if n.get("body") == body])})
        k("LR06", ["operation", "show", op], OWNER, host=url, area=a, expect="the operation is resolved",
          check=ok_true)
        after = f"After the resolution {nonce()}"
        pair("LR07", ["issue", "comment", "create", tgt, "--body", after], user=OWNER,
             host=url, area=a, expect="a further write to the target works again",
             read_back=lambda d: {"probe_notes": len([n for n in gl(f"/projects/{PQ}/issues/2/notes")
                                                      if n.get("body") == body]),
                                  "after_notes": len([n for n in gl(f"/projects/{PQ}/issues/2/notes")
                                                      if n.get("body") == after])})
        k("LR08", ["operation", "list", "--limit", "5"], OWNER, host=url, area=a, expect="operations listed",
          check=ok_true)
    finally:
        final = faultproxy.halt(log, stop)
        ctx_set(proxy_summary=faultproxy.summary(final))
        print("proxy:", json.dumps(faultproxy.summary(final)))


# ---------------------------------------------------------------- sign out

def auth_logout():
    a = "auth"

    def rb(doc):
        return {"credential_manager_entry": keyring_entry_exists()}
    pair("A05", ["auth", "logout", "--profile", PROFILE], area=a, dangerous=True,
         expect=f"the keyring entry of profile {PROFILE} is removed", read_back=rb)
    k("A06", ["auth", "status"], area=a, expect="no token any more: E_AUTH", check=err_code("E_AUTH"))


def final_scan():
    """Every secret of the run (tokens, the variable values) against every kept stdout, stderr and argv."""
    hits = []
    for f in sorted((OUT / "calls").glob("*")):
        b = f.read_bytes().decode("utf-8", "replace")
        if live.leaks(b):
            hits.append(f.name)
    for line in live.CALLS.read_text(encoding="utf-8").splitlines():
        if line.strip() and live.leaks(line):
            hits.append("calls.jsonl:" + json.loads(line)["id"])
    ctx_set(secret_scan={"secrets_checked": len(live.all_secrets()), "files_checked":
                         len(list((OUT / "calls").glob("*"))), "hits": hits})
    print("secret scan:", ctx()["secret_scan"])
    if hits:
        raise SystemExit("a secret appears in the recorded output")


PHASES = {"builtins_pre": builtins_pre, "auth_login": auth_login, "project": project, "repo": repo,
          "label_milestone": label_milestone, "issues": issues, "gate": gate, "mrs": mrs, "releases": releases,
          "ci": ci, "variables": variables, "search_users": search_users, "lost_reply": lost_reply,
          "auth_logout": auth_logout, "final_scan": final_scan}


# ---------------------------------------------------------------- the sandbox

def _whoami(user: str) -> dict:
    s, me = api("GET", "/user", user=user)
    if s != 200:
        raise SystemExit(f"the token for {user} is refused by {HOST} (HTTP {s})")
    return me


def setup():
    """Check the accounts, build the binary, and create the sandbox with the admin API: group G, its owner and
    reviewer, and a second project named sandbox in the subgroup twin (P06 asks for the name both share)."""
    admin = _whoami("admin")
    if not admin.get("is_admin"):
        raise SystemExit("GITLAB_E2E_ADMIN_TOKEN must belong to an administrator")
    for user, var in ((OWNER, "GITLAB_E2E_OWNER_TOKEN"), (REVIEWER, "GITLAB_E2E_REVIEWER_TOKEN")):
        if _whoami(user).get("username") != user:
            raise SystemExit(f"{var} belongs to another account than {user}")
    s, _ = api("GET", f"/groups/{urllib.parse.quote(G, safe='')}")
    if s == 200:
        raise SystemExit(f"group {G} exists already; run teardown first, or set GITLAB_E2E_GROUP")
    if live.CALLS.exists():
        raise SystemExit(f"{live.CALLS} holds an earlier run's calls; use a new GITLAB_E2E_RUN_DIR")
    live.ensure_binary()
    s, g = api("POST", "/groups", {"name": G, "path": G, "visibility": "private",
                                   "description": "gitlab-cli live suite sandbox (deleted after the run)"})
    assert s == 201, (s, g)
    ctx_set(group_id=g["id"])
    for name, level in ((OWNER, 50), (REVIEWER, 30)):
        uid = api("GET", "/users", query={"username": name})[1][0]["id"]
        s, m = api("POST", f"/groups/{g['id']}/members", {"user_id": uid, "access_level": level})
        assert s == 201, (s, m)
    s, sub = api("POST", "/groups", {"name": "twin", "path": "twin", "parent_id": g["id"], "visibility": "private"})
    assert s == 201, (s, sub)
    s, twin = api("POST", "/projects", {"name": "sandbox", "path": "sandbox", "namespace_id": sub["id"],
                                        "visibility": "private"})
    assert s == 201, (s, twin)
    meta = api("GET", "/metadata")[1]
    s, lic = api("GET", "/license")
    license_text = lic.get("plan") if s == 200 and isinstance(lic, dict) and lic.get("plan") else "none (Free features)"
    runners = []
    for r in api("GET", "/runners/all", query={"type": "instance_type", "status": "online"})[1] or []:
        version = (api("GET", f"/runners/{r['id']}")[1] or {}).get("version")
        if not version:  # GitLab 17+ reports it per runner manager
            managers = api("GET", f"/runners/{r['id']}/managers")[1]
            version = managers[0].get("version") if isinstance(managers, list) and managers else None
        runners.append({"description": r.get("description"), "version": version})
    if not runners:
        print("warning: no online instance runner; the ci phase needs one (docs/E2E.md)")
    ctx_set(gitlab={"version": meta.get("version"), "revision": meta.get("revision"),
                    "enterprise": meta.get("enterprise")}, license=license_text, admin_user=admin.get("username"),
            runners=runners, platform=sys.platform, started_at=utc_now())
    print("group", g["id"], "gitlab", meta.get("version"), "runners", len(runners))


def _logout_leftover():
    """A run stopped before auth_logout leaves the profile's keyring entry: remove it with the tool (unrecorded)."""
    if not (STATE / "scope.json").exists() or keyring_entry_exists() is False:
        return
    args = ["auth", "logout", "--profile", PROFILE, "--dangerous"]
    doc, code = call("teardown-logout-dry", args + ["--dry-run"], None, area="teardown", record=False, quiet=True)
    if code == 0 and doc and (doc.get("data") or {}).get("confirm_token"):
        call("teardown-logout", args + ["--confirm", doc["data"]["confirm_token"]], None, area="teardown",
             record=False, quiet=True)


def teardown():
    """Delete what setup and the tool made (admin API) and the run's isolated home; out/ (the records) stays. Only
    the group this run's setup created is deleted. GitLab 18 removes a project at once when it is marked first
    (permanently_remove), but a top-level group only after its deletion delay: the projects go for good, then the
    group is marked for deletion (and renamed)."""
    report = {}
    try:
        _logout_leftover()
    except SystemExit as e:
        report["logout"] = str(e)
    gid = ctx().get("group_id")
    s, g = api("GET", f"/groups/{urllib.parse.quote(G, safe='')}")
    if s == 200 and g.get("id") != gid:
        raise SystemExit(f"group {G} was not created by this run's setup; delete it yourself if it is a leftover")
    if s == 200:
        for p in api("GET", f"/groups/{g['id']}/projects", query={"include_subgroups": "true"})[1]:
            api("DELETE", f"/projects/{p['id']}")
            for _ in range(30):
                st, cur = api("GET", f"/projects/{p['id']}")
                if st == 404 or cur.get("marked_for_deletion_on"):
                    break
                time.sleep(2)
            if st != 404:
                fp = urllib.parse.quote(cur["path_with_namespace"], safe="")
                api("DELETE", f"/projects/{p['id']}?permanently_remove=true&full_path={fp}")
                for _ in range(60):
                    if api("GET", f"/projects/{p['id']}")[0] == 404:
                        break
                    time.sleep(3)
            report[p["path_with_namespace"]] = api("GET", f"/projects/{p['id']}")[0]
        sg, _ = api("DELETE", f"/groups/{g['id']}")
        time.sleep(2)
        st, after = api("GET", f"/groups/{g['id']}")
        report["group"] = {"delete_status": sg, "now": after.get("full_path") if st == 200 else None,
                           "marked_for_deletion_on": after.get("marked_for_deletion_on") if st == 200 else None}
    report["paths_free"] = {G: api("GET", f"/groups/{urllib.parse.quote(G, safe='')}")[0],
                            P: api("GET", f"/projects/{PQ}")[0]}
    report["keyring_entry_left"] = keyring_entry_exists()
    print(json.dumps(report))
    ctx_set(teardown=report)
    for d in ("home", "appdata", "localappdata", "tmp", "scratch"):
        shutil.rmtree(RUN / d, ignore_errors=True)
    ctx_set(finished_at=utc_now())


def run_all():
    for name, phase in PHASES.items():
        print(f"===== {name}", flush=True)
        phase()


def run():
    """setup, every phase, teardown; teardown runs after a failure too, once setup made the sandbox."""
    try:
        print("===== setup", flush=True)
        setup()
        run_all()
    finally:
        if ctx().get("group_id"):
            print("===== teardown", flush=True)
            teardown()


EXTRA = {"setup": setup, "teardown": teardown, "all": run_all, "run": run}


def usage() -> str:
    rows = "\n".join(f"  {name:<26} {meaning}" + (f" (default {default})" if default else "")
                     for name, (default, meaning) in live.CONFIG.items())
    return f"{__doc__}\nphases, in order: {', '.join(PHASES)}\n\nenvironment:\n{rows}\n"


if __name__ == "__main__":
    names = sys.argv[1:]
    if not names or any(a in ("-h", "--help") for a in names):
        print(usage())
        sys.exit(0 if names else 2)
    unknown = [n for n in names if n not in PHASES and n not in EXTRA]
    if unknown:
        raise SystemExit(f"unknown phase {', '.join(unknown)}; phases: {', '.join([*PHASES, *EXTRA])}")

    def recorded() -> list:
        text = live.CALLS.read_text(encoding="utf-8") if live.CALLS.exists() else ""
        return [json.loads(line) for line in text.splitlines() if line.strip()]
    before = len(recorded())
    for name in names:
        if name in PHASES:
            print(f"===== {name}", flush=True)
            PHASES[name]()
        else:
            EXTRA[name]()
    new = recorded()[before:]
    failed = [c["id"] for c in new if not c["pass"]]
    print(f"{len(new)} calls recorded, {len(failed)} not as expected" + (f": {', '.join(failed)}" if failed else ""))
    sys.exit(1 if failed else 0)
