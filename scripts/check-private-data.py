#!/usr/bin/env python3
"""Fail when the maintainer's private infrastructure shows up in a tracked file.

The maintainer's real addresses, host and user names and paths live outside
this repository (CLAUDE.local.md, a private companion repository, deploy/.env).
This check keeps them out: it splits every tracked text file into tokens and
compares their SHA-256 with a list of hashed private tokens, so the list itself
does not publish them.

    scripts/check-private-data.py            # check the tracked files
    scripts/check-private-data.py --hash X   # print the hash to add for token X

Tokens are runs of [A-Za-z0-9._-], lower-cased, with trailing punctuation
removed; each token is also checked part by part (split on '.', '-', '_'), so
"gitea.<domain>.hu" or "<user>-home" is caught too.
"""

import hashlib
import re
import subprocess
import sys

# SHA-256 of the lower-cased private tokens (IP addresses, the server's user
# name, the private domain, host and distro names, the shared Postgres
# container). Add one with --hash; never write the token itself here.
PRIVATE = {
    "1119b9c3acb56bf35bba81e0d2016f8183d12fc02ee2b36db089064386c7f95c",
    "fcc1e27aa6605aad2eb6e1844b3dd7ac16129f42e40ca05290d7c45dc2d819f6",
    "d7da75db715e9ec489582517601f380b201872bb0a281a5c6b04bca3ddca5cad",
    "218e551cf84988f452469efe7873e3c1f8a4597d15a634fe9724e6d6bdb1530f",
    "7cdb02a6f37c409a8b8d734282550e3b02a576cc4b1272b69a9211582e21331b",
    "2834f1963790670a51836de582aa7c7e841ac51166b750424d3478e1e9a401fe",
    "3946b4a3be907f4c6c76b571a81000930ed409f83bd481ca4cb0faeac2438464",
    "f5e73c8612f072d4b654d7346afd154c15bb11fd298a75b1fa4e02709f4d438a",
    "6c9681d5c12979f5df67c711e158eb9c68644e1ee242cfbf2acf0c376240b3be",
    "5cae49b1fea5b5b969cdc931b8338968940c92309aa2339360cdd5c77be4a4ff",
    "2861a3d6929ed8ade543bc350df10029d1f55c945e329612143651793fd50ea1",
}

# Third-party code we do not edit.
SKIP_PREFIXES = ("internal/httpserver/web/vendor/",)

TOKEN = re.compile(r"[A-Za-z0-9._-]+")
PARTS = re.compile(r"[._-]+")


def digest(token: str) -> str:
    return hashlib.sha256(token.encode()).hexdigest()


def hits(text: str):
    """Yield (line number, token) for every private token in text."""
    for lineno, line in enumerate(text.splitlines(), 1):
        for raw in TOKEN.findall(line):
            token = raw.strip("._-").lower()
            if not token:
                continue
            candidates = {token, *PARTS.split(token)}
            for c in candidates:
                if c and digest(c) in PRIVATE:
                    yield lineno, raw
                    break


def tracked_files():
    out = subprocess.run(["git", "ls-files", "-z"], check=True, capture_output=True).stdout
    return [f for f in out.decode().split("\0") if f]


def main(argv):
    if len(argv) == 3 and argv[1] == "--hash":
        print(digest(argv[2].lower()))
        return 0
    found = 0
    for path in tracked_files():
        if path.startswith(SKIP_PREFIXES):
            continue
        try:
            with open(path, "rb") as f:
                data = f.read()
        except (IsADirectoryError, FileNotFoundError):
            continue  # a symlink to a folder, or deleted in the working tree
        if b"\0" in data[:8192]:
            continue  # binary
        text = data.decode("utf-8", errors="replace")
        for lineno, _ in hits(text):
            # Print the place, never the token: the log is public too.
            print(f"{path}:{lineno}: private infrastructure detail (see CONTRIBUTING.md)")
            found += 1
    if found:
        print(f"{found} private detail(s) found; replace them with example values "
              "(192.0.2.x, 100.64.0.x, example.com, /home/alice).", file=sys.stderr)
        return 1
    print("no private infrastructure details in tracked files")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
