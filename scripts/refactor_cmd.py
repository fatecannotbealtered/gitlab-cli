#!/usr/bin/env python3
import re
import pathlib

root = pathlib.Path(__file__).resolve().parents[1] / "cmd"
for f in sorted(root.glob("*.go")):
    if f.name.endswith("_test.go"):
        continue
    t = f.read_text(encoding="utf-8")
    orig = t
    t = re.sub(
        r"output\.Error\(([^)]+)\)\n\s+setExitCode\(ExitBadArgs\)\n\s+return ErrSilent",
        r"return failArg(\1)",
        t,
    )
    t = re.sub(
        r"output\.Error\(([^)]+)\)\n\s+setExitCode\(ExitNotFound\)\n\s+return ErrSilent",
        r"return failNotFound(\1)",
        t,
    )
    t = re.sub(
        r"output\.Error\(([^)]+)\)\n\s+setExitCode\(ExitAuth\)\n\s+return ErrSilent",
        r"return failAuth(\1)",
        t,
    )
    t = re.sub(
        r"output\.Error\(([^)]+)\)\n\s+setExitCode\(ExitNetwork\)\n\s+return ErrSilent",
        r"return failWithCode(\1, ExitNetwork, output.ErrNetwork)",
        t,
    )
    t = re.sub(
        r'if !confirmAction\(([^,]+), ([^)]+)\) \{\n\s+output\.Info\("Aborted\."\)\n\s+return nil\n\s+\}',
        r"if err := requireConfirm(cmd, \1, \2); err != nil {\n\t\t\treturn err\n\t\t}",
        t,
    )
    t = re.sub(
        r"if !confirmAction\(([^,]+), ([^)]+)\) \{\n\s+return nil\n\s+\}",
        r"if err := requireConfirm(cmd, \1, \2); err != nil {\n\t\t\t\treturn err\n\t\t\t}",
        t,
    )
    if t != orig:
        f.write_text(t, encoding="utf-8")
        print("updated", f.name)
