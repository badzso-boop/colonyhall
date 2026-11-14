#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Ujj Norbert

"""Write THIRD_PARTY_NOTICES.md: the licences of everything shipped in the
binaries and the container image (round 48).

- Go: every module linked into cmd/colonyhall and cmd/colony (go list -deps).
- Web: the PWA's production dependency tree (web/node_modules), which the
  server embeds (web/dist).
- Vendored: the Monaco editor of the developer page.

Identical licence texts are printed once, with the packages that use them.
Run from the repository root; `make gen` runs it and `make gen-check`
fails when the file is out of date. Needs `go` and `npm ci` in web/.
"""

import json
import os
import re
import subprocess
import sys

OUT = "THIRD_PARTY_NOTICES.md"
LICENSE_NAMES = re.compile(r"^(LICEN[CS]E|COPYING|NOTICE)(\.(md|txt|markdown))?$", re.I)


def license_files(directory):
    try:
        names = sorted(os.listdir(directory))
    except FileNotFoundError:
        return []
    return [os.path.join(directory, n) for n in names if LICENSE_NAMES.match(n)]


def read(path):
    with open(path, encoding="utf-8", errors="replace") as f:
        return f.read().replace("\r\n", "\n").strip() + "\n"


def spdx_guess(text):
    t = " ".join(text.split()).lower()
    if "third-party software notices" in t[:300]:
        return "several licences"
    if "apache license" in t and "version 2.0" in t:
        return "Apache-2.0"
    if "mozilla public license" in t and "2.0" in t:
        return "MPL-2.0"
    if "sil open font license" in t:
        return "OFL-1.1"
    if "permission is hereby granted, free of charge" in t:
        return "MIT" if "the above copyright notice and this permission notice shall be included" in t else "MIT-0"
    if "redistribution and use in source and binary forms" in t:
        if "neither the name" in t or "names of its contributors" in t:
            return "BSD-3-Clause"
        return "BSD-2-Clause"
    if re.search(r"permission to use, copy, modify, and(/or)? distribute this software", t):
        return "ISC" if "provided that the above copyright notice" in t else "0BSD"
    m = re.match(r"\(No licence file in the package; package\.json declares ([^)]+?)\.\)", text)
    if m:
        return m.group(1)
    return "see text"


def go_modules():
    out = subprocess.run(
        ["go", "list", "-deps", "-json=Module,Standard", "./cmd/colonyhall", "./cmd/colony"],
        check=True, capture_output=True, text=True,
    ).stdout
    mods = {}
    for chunk in re.split(r"(?<=\n})\n(?={)", out.strip()):
        p = json.loads(chunk)
        m = p.get("Module")
        if p.get("Standard") or not m or m.get("Main"):
            continue
        mods[m["Path"]] = m
    for path in sorted(mods):
        m = mods[path]
        d = m.get("Dir") or ""
        files = license_files(d)
        if not files:
            sys.exit(f"{path}: no licence file in {d}")
        yield f"{path} {m.get('Version', '')}".strip(), "\n".join(read(f) for f in files)


def npm_packages(root="web"):
    with open(os.path.join(root, "package.json"), encoding="utf-8") as f:
        top = json.load(f).get("dependencies", {})
    seen = {}
    todo = [(name, os.path.join(root, "node_modules")) for name in top]
    while todo:
        name, base = todo.pop()
        # Node's lookup: base, then every node_modules further up, inside web/.
        cand = None
        d = base
        top_nm = os.path.join(root, "node_modules")
        while True:
            if os.path.basename(d) == "node_modules" and os.path.isfile(os.path.join(d, name, "package.json")):
                cand = os.path.join(d, name)
                break
            if os.path.abspath(d) == os.path.abspath(top_nm):
                break
            d = os.path.dirname(d)
        if cand is None:
            sys.exit(f"npm package {name} not found from {base} (run npm ci in web/)")
        real = os.path.realpath(cand)
        if real in seen:
            continue
        with open(os.path.join(cand, "package.json"), encoding="utf-8") as f:
            pkg = json.load(f)
        seen[real] = pkg
        for dep in list(pkg.get("dependencies", {})) + list(pkg.get("optionalDependencies", {})):
            todo.append((dep, os.path.join(cand, "node_modules")))
    for real, pkg in sorted(seen.items(), key=lambda kv: (kv[1]["name"], kv[1].get("version", ""))):
        files = license_files(real)
        text = "\n".join(read(f) for f in files) if files else ""
        declared = pkg.get("license")
        if isinstance(declared, dict):
            declared = declared.get("type")
        if not text:
            if not declared:
                sys.exit(f"npm package {pkg['name']}: no licence file and no licence field")
            text = f"(No licence file in the package; package.json declares {declared}.)\n"
        yield f"{pkg['name']} {pkg.get('version', '')}".strip(), text


def vendored():
    d = "internal/httpserver/web/vendor/monaco"
    files = license_files(d)
    if not files:
        sys.exit("vendored Monaco: no licence file")
    yield "monaco-editor 0.52.2 (vendored in internal/httpserver/web/vendor/monaco)", "\n".join(read(f) for f in files)
    # The minified editor bundles other packages (e.g. marked); the npm
    # package lists them in ThirdPartyNotices.txt, kept next to it.
    yield "monaco-editor 0.52.2: bundled third-party code", read(os.path.join(d, "ThirdPartyNotices.txt"))


def section(title, items):
    groups = {}
    for name, text in items:
        groups.setdefault(text, []).append(name)
    lines = [f"## {title}", ""]
    for text, names in sorted(groups.items(), key=lambda kv: kv[1][0].lower()):
        lines.append(f"### {spdx_guess(text)}: {', '.join(names)}")
        lines.append("")
        lines.append("```text")
        lines.append(text.rstrip().replace("```", "'''"))
        lines.append("```")
        lines.append("")
    return lines


def main():
    lines = [
        "# Third-party notices",
        "",
        "<!-- Generated by scripts/third-party-notices.py (make gen); do not edit. -->",
        "",
        "Colonyhall is licensed under the GNU Affero General Public License v3.0 only",
        "(see LICENSE and NOTICE). The binaries and the container image also contain",
        "the following third-party software, under their own licences.",
        "",
    ]
    lines += section("Go modules (linked into colonyhall and colony)", go_modules())
    lines += section("Web app (embedded in the server)", npm_packages())
    lines += section("Vendored", vendored())
    with open(OUT, "w", encoding="utf-8") as f:
        f.write("\n".join(lines).rstrip() + "\n")


if __name__ == "__main__":
    main()
