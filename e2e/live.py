"""The harness of gitlab-cli's live suite (docs/E2E.md): configuration, isolation, tool calls, contract checks.

Everything is configured by environment variables (see CONFIG below, or `python e2e/scenario.py --help`). Tokens
are read from the variables named in TOKEN_VARS only: they are held in memory, given to the tool through
GITLAB_CLI_TOKEN in the child's environment, and never printed, logged or written.

Every tool call runs isolated: USERPROFILE, HOME, APPDATA, LOCALAPPDATA and TEMP point into the run directory and
GITLAB_CLI_STATE_DIR at a folder inside it, so the settings, scope, journal and confirm key of a run are its own.
Each recorded call is appended to <run>/out/calls.jsonl with its raw stdout and stderr under
<run>/out/calls/<id>.out|.err, and its stdout is checked against the contract: with the spec's Python validator
when GITLAB_E2E_VALIDATOR names it, else with ContractLite below.

Ad hoc, unrecorded: python e2e/live.py [--as admin|owner|reviewer|none] -- <gitlab-cli arguments>
"""

from __future__ import annotations

import datetime as dt
import importlib
import importlib.machinery
import importlib.util
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import NamedTuple

E2E = Path(__file__).resolve().parent
REPO = E2E.parent
EXE = ".exe" if os.name == "nt" else ""

# name -> (default, meaning); `python e2e/scenario.py --help` prints this table.
CONFIG = {
    "GITLAB_E2E_URL": ("http://localhost:8929", "GitLab's URL (docker-compose.e2e.yml serves http://localhost:8929)"),
    "GITLAB_E2E_RUN_DIR": ("", "required: the run directory, outside the repository (binary, isolated home, records)"),
    "GITLAB_E2E_ADMIN_TOKEN": ("", "required: an administrator's token (api scope); only the harness uses it"),
    "GITLAB_E2E_OWNER": ("eval-bot", "the account the tool signs in as; it owns the sandbox group"),
    "GITLAB_E2E_OWNER_TOKEN": ("", "required: that account's token (api scope)"),
    "GITLAB_E2E_REVIEWER": ("bob", "a second account: developer in the sandbox, approves the merge request"),
    "GITLAB_E2E_REVIEWER_TOKEN": ("", "required: that account's token (api scope)"),
    "GITLAB_E2E_GROUP": ("gitlab-cli-e2e", "the sandbox group setup creates and teardown deletes"),
    "GITLAB_E2E_PROFILE": ("e2e", "the tool profile auth login creates in the OS keyring and auth logout removes"),
    "GITLAB_E2E_BIN": ("", "optional: the gitlab-cli binary to test; default: built from this checkout"),
    "GITLAB_E2E_VALIDATOR": ("", "optional: the spec's template/python/aicli/validate.py; default: ContractLite"),
    "GITLAB_E2E_FAULTPROXY": ("", "optional: the spec's tools/agent-eval/harness/faultproxy.py (lost_reply phase)"),
    "GITLAB_E2E_GRANTED_BY": ("the person who ran the live suite", "who grants the run's scope file"),
    "GITLAB_E2E_EVIDENCE_NAME": ("", "optional: the record's name; default <date>-gitlab-<ce|ee>-<major.minor>"),
}
TOKEN_VARS = {"admin": "GITLAB_E2E_ADMIN_TOKEN", "owner": "GITLAB_E2E_OWNER_TOKEN",
              "reviewer": "GITLAB_E2E_REVIEWER_TOKEN"}


def setting(name: str) -> str:
    return (os.environ.get(name) or "").strip() or CONFIG[name][0]


def _run_dir() -> Path:
    raw = setting("GITLAB_E2E_RUN_DIR")
    if not raw:
        raise SystemExit("set GITLAB_E2E_RUN_DIR to a new folder outside the repository (docs/E2E.md)")
    run = Path(raw).expanduser().resolve()
    if run == REPO or REPO in run.parents:
        raise SystemExit("GITLAB_E2E_RUN_DIR must be outside the repository: the run's records are not committed")
    return run


HOST = setting("GITLAB_E2E_URL").rstrip("/")
OWNER = setting("GITLAB_E2E_OWNER")
REVIEWER = setting("GITLAB_E2E_REVIEWER")
GROUP = setting("GITLAB_E2E_GROUP")
PROFILE = setting("GITLAB_E2E_PROFILE")
RUN = _run_dir()
BIN = Path(setting("GITLAB_E2E_BIN")).expanduser().resolve() if setting("GITLAB_E2E_BIN") \
    else RUN / "bin" / f"gitlab-cli{EXE}"
HOME = RUN / "home"
STATE = HOME / "state"
OUT = RUN / "out"
CALLS = OUT / "calls.jsonl"
CALLDIR = OUT / "calls"
CTX = OUT / "ctx.json"


# ---------------------------------------------------------------- secrets (never printed)

def _role(user: str) -> str | None:
    return "admin" if user == "admin" else "owner" if user == OWNER else "reviewer" if user == REVIEWER else None


def token_of(user: str) -> str:
    role = _role(user)
    if role is None:
        raise SystemExit(f"no token is configured for {user}")
    name = TOKEN_VARS[role]
    tok = (os.environ.get(name) or "").strip()
    if not tok:
        raise SystemExit(f"set {name} (docs/E2E.md, 'Accounts and tokens'); the suite reads the token from it only")
    return tok


EXTRA_SECRETS: list = []  # values the run generates that must never be printed (CI/CD variable values)


def all_secrets() -> list:
    toks = [(os.environ.get(name) or "").strip() for name in TOKEN_VARS.values()]
    return [s for s in [*toks, *EXTRA_SECRETS] if s]


def scrub(text: str) -> str:
    for s in all_secrets():
        text = text.replace(s, "<REDACTED>")
    return text


def leaks(text: str) -> bool:
    return any(s and s in text for s in all_secrets())


# ---------------------------------------------------------------- admin API (urllib), outside the tool

def api(method: str, path: str, data=None, user: str = "admin", raw: bool = False, query: dict | None = None):
    url = HOST + "/api/v4" + path
    if query:
        url += "?" + urllib.parse.urlencode(query, doseq=True)
    body = None
    headers = {"PRIVATE-TOKEN": token_of(user)}
    if data is not None:
        body = json.dumps(data).encode()
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=body, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            b = r.read()
            if raw:
                return r.status, b
            return r.status, (json.loads(b) if b else None)
    except urllib.error.HTTPError as e:
        b = e.read()
        try:
            return e.code, json.loads(b)
        except ValueError:
            return e.code, scrub(b.decode("utf-8", "replace"))[:500]


# ---------------------------------------------------------------- the contract check (VER-002)

class CodeRow(NamedTuple):
    exit: int
    retryable: bool
    meaning: str


class ContractLite:
    """The check used when GITLAB_E2E_VALIDATOR is not set, from this repository's contract/contract.json: the
    envelope's exact key sets, schema_version, meta, the error object, core codes with their retryability and the
    always-required details, and every line of a stream. The spec's validate.py checks far more (previews,
    receipts, pages, tokens, untrusted paths); a run that records evidence should use it."""

    class ContractViolation(ValueError):
        def __init__(self, problems: list) -> None:
            super().__init__("; ".join(problems))
            self.problems = list(problems)

    def __init__(self, path: Path) -> None:
        c = json.loads(path.read_text(encoding="utf-8"))
        env = c["envelope"]
        self.schema_version = env["schema_version_value"]
        self.keys = {True: set(env["success_keys"]), False: set(env["error_keys"])}
        self.error_keys = set(env["error_object_keys"])
        self.meta_required = set(env["meta_required_keys"])
        self.meta_allowed = self.meta_required | set(env["meta_optional_keys"])
        self.details_required = list(c["error_details"]["always_required"])
        self.dispatch_states = set(c["error_details"]["dispatch_state"])
        self.CORE_CODES = {code: CodeRow(row["exit"], row["retryable"], row.get("meaning", ""))
                           for code, row in c["error_codes"]["core"].items()}
        budgets = c["size_budgets"]
        self.INDEX_BUDGET_CHARS = budgets["reference_index_chars"]
        self.COMMAND_BUDGET_CHARS = budgets["reference_command_chars"]

    @staticmethod
    def _load(raw) -> tuple:
        try:
            return json.loads(raw.decode("utf-8") if isinstance(raw, bytes) else raw), None
        except (UnicodeDecodeError, ValueError) as e:
            return None, f"not JSON: {e}"

    def _envelope(self, doc) -> list:
        if not isinstance(doc, dict):
            return ["the envelope is not an object"]
        ok = doc.get("ok")
        if not isinstance(ok, bool):
            return ["ok must be true or false"]
        p = []
        if set(doc) != self.keys[ok]:
            p.append(f"keys {sorted(doc)} must be {sorted(self.keys[ok])}")
        if doc.get("schema_version") != self.schema_version:
            p.append(f"schema_version must be {self.schema_version!r}")
        meta = doc.get("meta")
        if not isinstance(meta, dict) or not self.meta_required <= set(meta) or not set(meta) <= self.meta_allowed:
            p.append(f"meta must have {sorted(self.meta_required)} and only {sorted(self.meta_allowed)}")
        if not ok:
            err = doc.get("error")
            if not isinstance(err, dict) or set(err) != self.error_keys:
                return p + [f"error must have exactly {sorted(self.error_keys)}"]
            code = err.get("code")
            if not isinstance(code, str) or not re.fullmatch(r"E_[A-Z0-9][A-Z0-9_]*", code):
                p.append("error.code must be E_ and upper case")
            if not isinstance(err.get("message"), str) or not err["message"]:
                p.append("error.message must be text")
            row = self.CORE_CODES.get(code)
            if not isinstance(err.get("retryable"), bool) or (row and err["retryable"] != row.retryable):
                p.append(f"error.retryable of {code} must be {row.retryable if row else 'a boolean'}")
            details = err.get("details")
            if not isinstance(details, dict):
                p.append("error.details must be an object")
            else:
                p += [f"error.details.{k} is required" for k in self.details_required if k not in details]
                if "dispatch_state" in details and details["dispatch_state"] not in self.dispatch_states:
                    p.append(f"error.details.dispatch_state must be one of {sorted(self.dispatch_states)}")
        return p

    def validate_envelope(self, raw) -> None:
        doc, bad = self._load(raw)
        problems = [bad] if bad else self._envelope(doc)
        if problems:
            raise self.ContractViolation(problems)

    def validate_stream(self, raw) -> None:
        text = raw.decode("utf-8", "replace") if isinstance(raw, bytes) else raw
        lines = [line for line in text.split("\n") if line.strip()]
        if not lines:
            raise self.ContractViolation(["the stream is empty"])
        problems = []
        for i, line in enumerate(lines[:-1], 1):
            doc, bad = self._load(line)
            if bad or not isinstance(doc, dict) or not isinstance(doc.get("type"), str):
                problems.append(f"line {i} must be a JSON object with a type")
        doc, bad = self._load(lines[-1])
        problems += [f"last line: {x}" for x in ([bad] if bad else self._envelope(doc))]
        if problems:
            raise self.ContractViolation(problems)


_PKG = "_e2e_aicli"
_V = None


def _spec_package() -> Path | None:
    raw = setting("GITLAB_E2E_VALIDATOR")
    if not raw:
        return None
    p = Path(raw).expanduser().resolve()
    pkg = p if p.is_dir() else p.parent
    if not (pkg / "validate.py").is_file():
        raise SystemExit("GITLAB_E2E_VALIDATOR must name the spec's template/python/aicli/validate.py")
    return pkg


def validator_label() -> str:
    pkg = _spec_package()
    if pkg is None:
        return ("e2e/live.py ContractLite against contract/contract.json (envelope and error keys, core codes, "
                "exit codes); the spec's validate.py was not configured")
    try:
        version = json.loads((pkg / "MANIFEST.json").read_text(encoding="utf-8"))["spec_version"]
    except (OSError, ValueError, KeyError):
        version = "(version unknown)"
    return (f"template/python/aicli/validate.py of ai-native-cli-spec v{version} (validate_envelope / "
            "validate_stream, exit codes against contract.json)")


def validator():
    """The spec's validate.py, loaded as a package from its own folder (it imports its siblings), else
    ContractLite. The first call of a run records which one in the run's context; a later phase must use the
    same."""
    global _V
    if _V is None:
        pkg = _spec_package()
        if pkg is None:
            _V = ContractLite(REPO / "contract" / "contract.json")
        else:
            if _PKG not in sys.modules:
                spec = importlib.machinery.ModuleSpec(_PKG, None, is_package=True)
                spec.submodule_search_locations = [str(pkg)]
                sys.modules[_PKG] = importlib.util.module_from_spec(spec)
            _V = importlib.import_module(_PKG + ".validate")
        label = validator_label()
        recorded = ctx().get("validator")
        if recorded is None:
            ctx_set(validator=label)
        elif recorded != label:
            raise SystemExit(f"this run's calls were checked with {recorded!r}; set GITLAB_E2E_VALIDATOR the same way")
    return _V


def _flag(argv, name):
    for i, a in enumerate(argv):
        if a == name and i + 1 < len(argv):
            return argv[i + 1]
        if a.startswith(name + "="):
            return a.split("=", 1)[1]
    return None


def _is_stream(raw: bytes) -> bool:
    first = raw.lstrip().split(b"\n", 1)[0]
    try:
        doc = json.loads(first.decode("utf-8"))
    except (UnicodeDecodeError, ValueError):
        return False
    return isinstance(doc, dict) and "type" in doc


def validate(argv, raw: bytes, exit_code: int):
    """(envelope_valid, problems): None when stdout is not an envelope by design (help, raw, text)."""
    v = validator()
    if "--help" in argv or "-h" in argv:
        return None, ["help is text"]
    if "--version" in argv and exit_code == 0 and not raw.lstrip().startswith(b"{"):
        return None, ["--version is text"]
    fmt = _flag(argv, "--format")
    if fmt in ("text", "raw") and not (exit_code != 0 and raw.lstrip().startswith(b"{")):
        return None, [f"--format {fmt}"]
    if not raw.strip():
        return False, ["stdout is empty"]
    try:
        if _is_stream(raw):
            v.validate_stream(raw)
            last = raw.strip().split(b"\n")[-1]
        else:
            v.validate_envelope(raw)
            last = raw
    except v.ContractViolation as e:
        return False, list(e.problems)
    except Exception as e:  # noqa: BLE001
        return False, [f"validator error: {e!r}"]
    doc = json.loads(last.decode("utf-8"))
    if doc.get("ok") is True:
        want, what = 0, "success"
    else:
        code = (doc.get("error") or {}).get("code")
        row = v.CORE_CODES.get(code)
        if row is None:
            return True, []
        want, what = row.exit, code
    if exit_code != want:
        return False, [f"exit {exit_code}, but {what} means exit {want} (contract.json)"]
    return True, []


# ---------------------------------------------------------------- context (ids found along the way)

def ctx() -> dict:
    return json.loads(CTX.read_text(encoding="utf-8")) if CTX.exists() else {}


def ctx_set(**kw):
    c = ctx()
    c.update(kw)
    OUT.mkdir(parents=True, exist_ok=True)
    CTX.write_text(json.dumps(c, indent=1), encoding="utf-8")
    return c


def utc_now() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def git(*args) -> str:
    return subprocess.run(["git", "-C", str(REPO), *args], capture_output=True, text=True).stdout.strip()


# ---------------------------------------------------------------- the tool

def ensure_binary() -> None:
    """Build the binary from this checkout into <run>/bin unless GITLAB_E2E_BIN names one; record what was built."""
    if setting("GITLAB_E2E_BIN"):
        if not BIN.is_file():
            raise SystemExit("GITLAB_E2E_BIN does not name a file")
        if "build" not in ctx():
            ctx_set(build={"source": "GITLAB_E2E_BIN"})
        return
    if BIN.is_file() and "build" in ctx():
        return
    BIN.parent.mkdir(parents=True, exist_ok=True)
    p = subprocess.run(["go", "build", "-o", str(BIN), "./cmd/gitlab-cli"], cwd=REPO, capture_output=True, text=True)
    if p.returncode != 0:
        raise SystemExit(f"go build failed:\n{p.stderr}")
    ctx_set(build={"source": "this checkout (go build ./cmd/gitlab-cli)", "commit": git("rev-parse", "HEAD"),
                   "branch": git("rev-parse", "--abbrev-ref", "HEAD"), "dirty": bool(git("status", "--porcelain"))})


def base_env(user: str | None, host: str | None = None, extra: dict | None = None, state: Path | None = None) -> dict:
    keep = ("SYSTEMROOT", "SystemRoot", "WINDIR", "PATH", "PATHEXT", "COMSPEC", "NUMBER_OF_PROCESSORS",
            "PROCESSOR_ARCHITECTURE", "OS", "LANG", "LC_ALL",
            # the OS keyring on Linux (Secret Service over the session bus)
            "DBUS_SESSION_BUS_ADDRESS", "XDG_RUNTIME_DIR")
    env = {k: os.environ[k] for k in keep if k in os.environ}
    for d in ("home", "appdata", "localappdata", "tmp"):
        (RUN / d).mkdir(parents=True, exist_ok=True)
    env.update({
        "USERPROFILE": str(HOME), "HOME": str(HOME),
        "APPDATA": str(RUN / "appdata"), "LOCALAPPDATA": str(RUN / "localappdata"),
        "TEMP": str(RUN / "tmp"), "TMP": str(RUN / "tmp"), "TMPDIR": str(RUN / "tmp"),
        "GITLAB_CLI_STATE_DIR": str(state or STATE),
        "GITLAB_CLI_NO_UPDATE_CHECK": "1",
    })
    if host:
        env["GITLAB_CLI_HOST"] = host
    elif host is None and user is not None:
        env["GITLAB_CLI_HOST"] = HOST
    if user is not None:
        env["GITLAB_CLI_TOKEN"] = token_of(user)
    if extra:
        env.update(extra)
    return env


_SUB = re.compile(r"@(tok|op|json):([A-Za-z0-9_.\-]+)(?::([A-Za-z0-9_.\[\]\-]+))?")


def stdout_doc(call_id: str):
    raw = (CALLDIR / f"{call_id}.out").read_bytes()
    lines = [line for line in raw.strip().split(b"\n") if line.strip()]
    return json.loads(lines[-1].decode("utf-8")) if lines else None


def jpath(doc, path: str):
    cur = doc
    for part in path.split("."):
        m = re.fullmatch(r"([A-Za-z0-9_\-]+)(?:\[(\d+)\])?", part)
        if not m:
            raise KeyError(path)
        cur = cur[m.group(1)]
        if m.group(2) is not None:
            cur = cur[int(m.group(2))]
    return cur


def confirm_token(doc) -> str:
    for p in ("data.confirm_token", "error.details.confirm_token", "data.preview.confirm_token"):
        try:
            v = jpath(doc, p)
            if v:
                return v
        except (KeyError, TypeError, IndexError):
            pass
    raise KeyError("no confirm token")


def op_id(doc) -> str:
    for p in ("error.details.operation_id", "data.receipt.operation_id", "data.operation_id", "data.id"):
        try:
            v = jpath(doc, p)
            if v:
                return v
        except (KeyError, TypeError, IndexError):
            pass
    raise KeyError("no operation id")


def subst(arg: str) -> str:
    """@tok:<call> is that call's confirm token, @op:<call> its operation id, @json:<call>:<path> a value of its
    stdout."""
    def rep(m):
        kind, cid, path = m.group(1), m.group(2), m.group(3)
        doc = stdout_doc(cid)
        if kind == "tok":
            return confirm_token(doc)
        if kind == "op":
            return op_id(doc)
        return str(jpath(doc, path))
    return _SUB.sub(rep, arg)


def _existing_ids() -> set:
    if not CALLS.exists():
        return set()
    return {json.loads(line)["id"] for line in CALLS.read_text(encoding="utf-8").splitlines() if line.strip()}


def call(cid: str, argv: list, user: str | None = "admin", *, area: str, expect: str = "", env: dict | None = None,
         stdin: bytes | None = None, host: str | None = None, state: Path | None = None, record: bool = True,
         check=None, quiet: bool = False, show: int = 1800, read_back=None):
    """Run the tool once as user (None: the keyring profile). check(doc, exit) -> (bool, why) decides `pass`;
    read_back(doc) -> dict is a read made outside the tool (admin API) after the call, kept with the record
    (VER-005)."""
    ensure_binary()
    argv = [subst(a) for a in argv]
    if any(leaks(a) for a in argv):
        raise SystemExit(f"{cid}: a secret would appear in argv")
    if record and cid in _existing_ids():
        raise SystemExit(f"call id {cid} already recorded")
    e = base_env(user, host, env, state)
    t0 = time.monotonic()
    p = subprocess.run([str(BIN), *argv], input=stdin, capture_output=True, env=e, timeout=300)
    ms = round((time.monotonic() - t0) * 1000)
    out, err = p.stdout, p.stderr
    if leaks(out.decode("utf-8", "replace")) or leaks(err.decode("utf-8", "replace")):
        # never keep a leaked token: redact before storing and flag the call
        leaked = True
        out = scrub(out.decode("utf-8", "replace")).encode()
        err = scrub(err.decode("utf-8", "replace")).encode()
    else:
        leaked = False
    CALLDIR.mkdir(parents=True, exist_ok=True)
    (CALLDIR / f"{cid}.out").write_bytes(out)
    (CALLDIR / f"{cid}.err").write_bytes(err)
    valid, problems = validate(argv, out, p.returncode)
    try:
        doc = stdout_doc(cid)
    except Exception:  # noqa: BLE001
        doc = None
    ok, why = True, ""
    if check is not None:
        try:
            ok, why = check(doc, p.returncode)
        except Exception as ex:  # noqa: BLE001
            ok, why = False, f"check raised {ex!r}"
    if leaked:
        ok, why = False, "a secret appeared in the output (redacted)"
    rb = None
    if read_back is not None:
        try:
            rb = read_back(doc)
        except Exception as ex:  # noqa: BLE001
            rb = {"error": repr(ex)}
        if leaks(json.dumps(rb)):
            rb = {"error": "read-back held a secret; not kept"}
    entry = {"id": cid, "area": area, "argv": ["gitlab-cli", *argv], "exit": p.returncode,
             "stdout": out.decode("utf-8", "replace"), "duration_ms": ms, "envelope_valid": valid,
             "validation": problems, "expect": expect, "pass": bool(ok) and valid is not False, "why": why,
             "as": user or "keyring", "via_proxy": bool(host) and host != HOST}
    if rb is not None:
        entry["read_back"] = rb
    if record:
        with CALLS.open("a", encoding="utf-8") as f:
            f.write(json.dumps(entry, ensure_ascii=False) + "\n")
    if not quiet:
        s = entry["stdout"]
        mark = "PASS" if entry["pass"] else "FAIL"
        print(f"[{mark}] {cid} exit={p.returncode} valid={valid} {ms}ms :: gitlab-cli {' '.join(argv)}"[:600])
        if problems and valid is False:
            print("   validation:", problems[:4])
        if why:
            print("   why:", why)
        print("   " + (s if len(s) <= show else s[:show] + f"... [{len(s)} chars]"))
        es = err.decode("utf-8", "replace").strip()
        if es:
            print("   stderr:", es[:600])
        if rb is not None:
            print("   read_back:", json.dumps(rb)[:600])
    return doc, p.returncode


def drop(cid_prefix: str):
    """Remove recorded calls whose id starts with the prefix (to rerun a phase)."""
    if not CALLS.exists():
        return
    keep = [line for line in CALLS.read_text(encoding="utf-8").splitlines()
            if line.strip() and not json.loads(line)["id"].startswith(cid_prefix)]
    CALLS.write_text("\n".join(keep) + ("\n" if keep else ""), encoding="utf-8")


# ---------------------------------------------------------------- checks

def ok_true(doc, code):
    return (doc is not None and doc.get("ok") is True and code == 0), f"ok={doc and doc.get('ok')} exit={code}"


def err_code(want: str, **detail):
    def chk(doc, code):
        if not doc or doc.get("ok") is not False:
            return False, "expected an error envelope"
        e = doc.get("error") or {}
        if e.get("code") != want:
            return False, f"code {e.get('code')} != {want}"
        d = e.get("details") or {}
        for k, v in detail.items():
            got = d.get(k)
            if callable(v):
                if not v(got):
                    return False, f"details.{k}={got!r} failed its check"
            elif got != v:
                return False, f"details.{k}={got!r} != {v!r}"
        return True, ""
    return chk


def write_scope(name: str = "scope.json", **kw) -> Path:
    """The run's scope file in its isolated state directory: the person who runs the suite grants it
    (GITLAB_E2E_GRANTED_BY); it covers the sandbox only."""
    now = dt.datetime.now(dt.timezone.utc)
    doc = {
        "scope_id": kw.pop("scope_id", "e2e"),
        "tool": "gitlab-cli",
        "granted_by": setting("GITLAB_E2E_GRANTED_BY"),
        "granted_at": (now - dt.timedelta(minutes=1)).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "expires_at": (now + dt.timedelta(hours=8)).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "effects": kw.pop("effects", ["change", "configure", "artifact"]),
        "dangerous": kw.pop("dangerous", True),
    }
    doc.update(kw)
    p = STATE / name
    STATE.mkdir(parents=True, exist_ok=True)
    p.write_text(json.dumps(doc, indent=1), encoding="utf-8")
    return p


# ---------------------------------------------------------------- the evidence's name and paths

def evidence_name() -> str:
    """GITLAB_E2E_EVIDENCE_NAME, else <date>-gitlab-<ce|ee>-<major.minor> from the run's context."""
    name = setting("GITLAB_E2E_EVIDENCE_NAME")
    if name:
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]*", name):
            raise SystemExit("GITLAB_E2E_EVIDENCE_NAME may hold letters, digits, '.', '_' and '-' only")
        return name
    c = ctx()
    gl = c.get("gitlab") or {}
    if not c.get("started_at") or not gl.get("version"):
        raise SystemExit("the run has no setup record (out/ctx.json); set GITLAB_E2E_EVIDENCE_NAME")
    minor = ".".join(re.split(r"[.-]", gl["version"])[:2])
    return f"{c['started_at'][:10]}-gitlab-{'ee' if gl.get('enterprise') else 'ce'}-{minor}"


def run_paths() -> list:
    """The run directory as it can appear in argv and in stdout (JSON-escaped, or with forward slashes)."""
    raw = str(RUN)
    forms = {raw, raw.replace("\\", "/"), json.dumps(raw)[1:-1]}
    return sorted((f for f in forms if f), key=len, reverse=True)


def hide_run(obj):
    """obj with every path into the run directory written as <run>: the record holds no machine's paths."""
    if isinstance(obj, str):
        for form in run_paths():
            obj = obj.replace(form, "<run>")
        return obj
    if isinstance(obj, list):
        return [hide_run(x) for x in obj]
    if isinstance(obj, dict):
        return type(obj)((k, hide_run(v)) for k, v in obj.items())
    return obj


if __name__ == "__main__":
    args = sys.argv[1:]
    who = "owner"
    if args and args[0] == "--as":
        who, args = args[1], args[2:]
    if args and args[0] == "--":
        args = args[1:]
    user = {"admin": "admin", "owner": OWNER, "reviewer": REVIEWER, "none": None}.get(who, who)
    call("adhoc", args, user, area="adhoc", record=False, show=int(os.environ.get("SHOW", "6000")))
