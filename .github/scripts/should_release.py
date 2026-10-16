"""Decide whether VERSION.md should publish a GitHub release.

Prints publish=true when the file version is greater than every existing
vX.Y.Z release. Equal or lower versions publish nothing.
"""

import json
import os
import re
import subprocess
import sys

SEMVER = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")
TAG = re.compile(r"^v(\d+)\.(\d+)\.(\d+)$")


def parse_version(text: str) -> str:
    for line in text.splitlines():
        match = SEMVER.match(line.strip())
        if match:
            return match.group(0)
    raise SystemExit("VERSION.md must contain a line with major.minor.patch")


def as_tuple(version: str) -> tuple[int, int, int]:
    major, minor, patch = version.split(".")
    return int(major), int(minor), int(patch)


def should_publish(current: str, tags: list[str]) -> bool:
    current_parts = as_tuple(current)
    released = []
    for tag in tags:
        match = TAG.match(tag)
        if match:
            released.append((int(match.group(1)), int(match.group(2)), int(match.group(3))))
    if not released:
        return True
    return current_parts > max(released)


def existing_tags() -> list[str]:
    result = subprocess.run(
        ["gh", "release", "list", "--limit", "200", "--json", "tagName"],
        check=True,
        capture_output=True,
        text=True,
    )
    payload = json.loads(result.stdout or "[]")
    return [item["tagName"] for item in payload]


def main() -> None:
    version = parse_version(open("VERSION.md", encoding="utf-8").read())
    publish = should_publish(version, existing_tags())
    line = f"version={version}\npublish={'true' if publish else 'false'}\n"
    output = os.environ.get("GITHUB_OUTPUT")
    if output:
        with open(output, "a", encoding="utf-8") as handle:
            handle.write(line)
    print(line, end="")
    if not publish:
        print(f"v{version} is not newer than the latest release; skipping", file=sys.stderr)


if __name__ == "__main__":
    main()
