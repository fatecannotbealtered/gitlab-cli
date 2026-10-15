"""Raise the commands that passed live to Maturity verified, citing the run's record (CLI-027, VER-005).

    python e2e/set_maturity.py [--dry]

Reads <run>/out/verdict.json (build_evidence.py) and the source location of every command's RunE
(`go run ./e2e/maturitymap`), then edits the command's Meta, the first `Maturity:` line after its RunE: an
experimental command that passed becomes aicli.Verified and its Evidence gains liveEvidence("<section>"). Commands
that share one RunE, or whose Meta a helper builds, are listed for a manual edit.

liveEvidence cites the record cmd/evidence.go names (liveRecord). When that is not this run's record, nothing is
edited: point liveRecord at docs/evidence/<name>.md first, and the script lists the verified commands this run did
not pass, which the new record would no longer support (lower them to experimental, or keep the old record).
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from collections import Counter, defaultdict
from pathlib import Path

from live import BIN, OUT, REPO, base_env

dry = "--dry" in sys.argv
verdict = json.loads((OUT / "verdict.json").read_text(encoding="utf-8"))
record = f"docs/evidence/{verdict['record']}.md"

evidence_go = (REPO / "cmd" / "evidence.go").read_text(encoding="utf-8")
m = re.search(r'const liveRecord = "([^"]+)"', evidence_go)
cited = m.group(1) if m else None
if cited != record:
    p = subprocess.run([str(BIN), "reference"], capture_output=True, env=base_env(None))
    verified_now = sorted(c["path"] for c in json.loads(p.stdout)["data"]["commands"]
                          if c.get("maturity") == "verified")
    unsupported = [c for c in verified_now if c not in verdict["verified"]]
    print(f"cmd/evidence.go cites {cited}, not this run's record {record}; nothing edited.")
    print("Point liveRecord at the new record, then run this again. Verified commands this run did not pass "
          f"({len(unsupported)}): {', '.join(unsupported) or 'none'}")
    sys.exit(1)

p = subprocess.run(["go", "run", "./e2e/maturitymap"], cwd=REPO, capture_output=True, text=True)
if p.returncode != 0:
    sys.exit(f"go run ./e2e/maturitymap failed:\n{p.stderr}")
mm = {}
for line in p.stdout.splitlines():
    path, file, ln = line.split("\t")
    mm[path.removeprefix("gitlab-cli ")] = (Path(file), int(ln))
shared = {k for k, v in Counter(mm.values()).items() if v > 1}

edits = defaultdict(dict)  # file -> {line index: (section, cmd)}
manual = []
already = []
for cmd in verdict["verified"]:
    if cmd not in mm:
        manual.append((cmd, "not in the command tree"))
        continue
    file, ln = mm[cmd]
    if (file, ln) in shared:
        manual.append((cmd, "shares its RunE with another command"))
        continue
    lines = file.read_text(encoding="utf-8").splitlines(keepends=True)
    i = ln - 1
    while i < len(lines) and "Maturity:" not in lines[i]:
        i += 1
    if i < len(lines) and "aicli.Verified" in lines[i]:
        already.append(cmd)
        continue
    if i >= len(lines) or "aicli.Experimental" not in lines[i]:
        manual.append((cmd, f"no Maturity: aicli.Experimental after {file.name}:{ln}"))
        continue
    if i in edits[file]:
        manual.append((cmd, f"{file.name}:{i + 1} is already the Meta of {edits[file][i][1]}"))
        continue
    edits[file][i] = (verdict["sections"][cmd], cmd)

for file, at in edits.items():
    lines = file.read_text(encoding="utf-8").splitlines(keepends=True)
    for i, (section, cmd) in sorted(at.items()):
        lines[i] = lines[i].replace("aicli.Experimental", "aicli.Verified", 1)
        for j in range(i, min(i + 4, len(lines))):
            m = re.search(r"Evidence: \[\]string\{([^}]*)\}", lines[j])
            if m:
                lines[j] = lines[j][:m.end() - 1] + f', liveEvidence("{section}")' + lines[j][m.end() - 1:]
                break
        else:
            manual.append((cmd, f"no Evidence near {file.name}:{i + 1}"))
        print(f"{file.name}:{i + 1}: {cmd} -> verified #{section}")
    if not dry:
        file.write_bytes("".join(lines).encode("utf-8"))

print(f"already verified: {len(already)}; manual: {manual}")
if not dry and edits:
    print("now run gofmt -w cmd and go test ./cmd/ (the conformance suite checks maturity against evidence)")
