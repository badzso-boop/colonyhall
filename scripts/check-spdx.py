#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Ujj Norbert

"""Fail when a source file has no SPDX licence header (CONTRIBUTING.md).

Checks every tracked Go, TypeScript, JavaScript, shell and Python file:
within its first lines (after a #! line) it must say
`SPDX-License-Identifier: AGPL-3.0-only`. Generated files, vendored
third-party code and test data are skipped: they are not ours to label,
or a generator rewrites them.

    scripts/check-spdx.py          # check, list what is missing
    scripts/check-spdx.py --fix    # add the header where it is missing
"""

import subprocess
import sys

EXTENSIONS = (".go", ".ts", ".tsx", ".js", ".mjs", ".cjs", ".sh", ".py")
SKIP_PREFIXES = (
    "internal/httpserver/web/vendor/",  # Monaco, third party
    "web/src/shared/api/schema.d.ts",  # openapi-typescript output
)
SKIP_PARTS = ("/testdata/", "/node_modules/", "/dist/")
GENERATED_MARKERS = ("Code generated", "DO NOT EDIT", "auto-generated", "This file was automatically generated")
ID = "SPDX-License-Identifier: AGPL-3.0-only"
HEAD_LINES = 6


def header_for(path):
    c = "#" if path.endswith((".sh", ".py")) else "//"
    return f"{c} {ID}\n{c} Copyright (C) 2026 Ujj Norbert\n"


def wanted(path, text):
    if not path.endswith(EXTENSIONS) or path.startswith(SKIP_PREFIXES):
        return False
    if any(p in "/" + path for p in SKIP_PARTS):
        return False
    head = "\n".join(text.splitlines()[:HEAD_LINES])
    return not any(m in head for m in GENERATED_MARKERS)


def has_header(text):
    return ID in "\n".join(text.splitlines()[:HEAD_LINES])


def add_header(path, text):
    lines = text.splitlines(keepends=True)
    if lines and lines[0].startswith("#!"):
        return lines[0] + header_for(path) + ("\n" if len(lines) > 1 and lines[1].strip() else "") + "".join(lines[1:])
    return header_for(path) + "\n" + text


def main():
    fix = "--fix" in sys.argv[1:]
    files = subprocess.run(["git", "ls-files", "-z"], capture_output=True, check=True).stdout.decode().split("\0")
    missing = []
    for path in filter(None, files):
        try:
            with open(path, encoding="utf-8") as f:
                text = f.read()
        except (OSError, UnicodeDecodeError):
            continue
        if wanted(path, text) and not has_header(text):
            missing.append(path)
            if fix:
                with open(path, "w", encoding="utf-8") as f:
                    f.write(add_header(path, text))
    if missing and not fix:
        print("missing SPDX header (see CONTRIBUTING.md; scripts/check-spdx.py --fix adds it):")
        for p in missing:
            print("  " + p)
        return 1
    if fix and missing:
        print(f"added the header to {len(missing)} file(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
