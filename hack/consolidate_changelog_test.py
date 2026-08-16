#!/usr/bin/env python3
"""Fixture tests for consolidate-changelog.py.

The script runs once per release, against real history, where a defect is
awkward to undo - a prerelease entry left behind ends up on main outside any
release, and its contents never reach the release they belong to. Both defects
found so far came from assuming the shape of a prerelease version, so the cases
below cover every shape release-please can produce, not only the ones seen.

Run: python3 hack/consolidate_changelog_test.py
"""
import re
import subprocess
import sys
import tempfile
from pathlib import Path

SCRIPT = Path(__file__).with_name("consolidate-changelog.py")
OLDER_RELEASE = "## 0.2.0 (2026-08-15)\n\n\n### Features\n\n* old thing ([aaa](u))\n"


def changelog(*entries):
    text = "# Changelog\n\n"
    for version, section, bullet in entries:
        text += (
            f"## [{version}](https://x/compare/a...b) (2026-08-16)\n\n\n"
            f"### {section}\n\n* {bullet} ([sha](u))\n\n\n"
        )
    return text + OLDER_RELEASE


def consolidate(text, version="0.2.2"):
    with tempfile.TemporaryDirectory() as tmp:
        path = Path(tmp) / "CHANGELOG.md"
        path.write_text(text, encoding="utf-8")
        subprocess.run(
            [sys.executable, str(SCRIPT), str(path), version],
            check=True,
            capture_output=True,
        )
        return path.read_text(encoding="utf-8")


def bullets_before_older_release(text):
    head = text.split("## 0.2.0")[0]
    return [line for line in head.splitlines() if line.startswith("* ")]


CASES = {
    # release-please increments a prerelease only once one carries a number.
    # The first prerelease after a release is a bare -alpha, and matching
    # -alpha.N left exactly that entry behind when v0.2.2 was cut.
    "bare prerelease alone": [("0.2.2-alpha", "Features", "feat")],
    "numbered prereleases": [
        ("0.2.2-alpha.2", "Bug Fixes", "two"),
        ("0.2.2-alpha.1", "Bug Fixes", "one"),
    ],
    "numbered then bare": [
        ("0.2.2-alpha.1", "Bug Fixes", "one"),
        ("0.2.2-alpha", "Features", "feat"),
    ],
    # Nothing emits these today; they fail the same way an -alpha.N pattern
    # fails a bare -alpha, which is why the test is on the version's shape.
    "other prerelease identifiers": [
        ("0.2.2-rc.1", "Bug Fixes", "rc"),
        ("0.2.2-beta", "Features", "beta"),
    ],
}


def check(name, entries):
    source = changelog(*entries)
    result = consolidate(source)

    failures = []
    leftover = [
        line
        for line in result.splitlines()
        if re.match(r"^## \[?0\.2\.2-", line)
    ]
    if leftover:
        failures.append(f"prerelease entries survived: {leftover}")
    if "## 0.2.2 (2026-08-16)" not in result:
        failures.append("no consolidated 0.2.2 entry")
    if "## 0.2.0 (2026-08-15)" not in result:
        failures.append("the previous release was consumed")

    before = len(bullets_before_older_release(source))
    after = len(bullets_before_older_release(result))
    if before != after:
        failures.append(f"{before} bullets in, {after} out")

    return failures


def check_no_prerelease():
    """An official-release-only changelog must be left alone."""
    source = "# Changelog\n\n" + OLDER_RELEASE
    with tempfile.TemporaryDirectory() as tmp:
        path = Path(tmp) / "CHANGELOG.md"
        path.write_text(source, encoding="utf-8")
        subprocess.run(
            [sys.executable, str(SCRIPT), str(path), "0.2.2"],
            check=True,
            capture_output=True,
        )
        if path.read_text(encoding="utf-8") != source:
            return ["the changelog was rewritten with nothing to consolidate"]
    return []


def main():
    failed = 0
    for name, entries in CASES.items():
        failures = check(name, entries)
        print(f"{'FAIL' if failures else 'ok  '}  {name}")
        for failure in failures:
            print(f"        {failure}")
        failed += bool(failures)

    failures = check_no_prerelease()
    print(f"{'FAIL' if failures else 'ok  '}  no prerelease entries")
    for failure in failures:
        print(f"        {failure}")
    failed += bool(failures)

    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
