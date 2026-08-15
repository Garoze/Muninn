#!/usr/bin/env python3
"""Resolves the expected CHANGELOG.md conflict when merging main back into
develop after a release cut.

main's copy consolidates alpha entries into one official entry per release;
develop's copy keeps accumulating one heading per alpha. A plain merge
conflicts, since the same underlying content is grouped differently on each
side, and comparing against main's content directly does not work either -
consolidation removes the old alpha headings from main entirely, so they
never match anything there even though they are not new.

The reliable boundary is develop's own file at the merge-base commit (the
last point the two branches actually agreed): any heading on develop's
current side that already existed there is old, already accounted for by
main's later consolidation; anything not yet present at the merge-base was
added afterward and is genuinely new.
"""
import re
import sys

HEADING = re.compile(r"^## (?:\[[^\]]+\]\([^)]*\)|\S+) \(.*\)\s*$")


def split_blocks(text):
    lines = text.splitlines(keepends=True)
    blocks = []
    current = []
    for line in lines:
        if HEADING.match(line) and current:
            blocks.append("".join(current))
            current = [line]
        else:
            current.append(line)
    if current:
        blocks.append("".join(current))
    return blocks


def block_heading_line(block):
    for line in block.splitlines():
        if HEADING.match(line):
            return line
    return None


def main():
    if len(sys.argv) != 5:
        print(
            "usage: resolve-changelog-backmerge.py DEVELOP_CURRENT DEVELOP_AT_MERGEBASE MAIN_CURRENT OUT",
            file=sys.stderr,
        )
        sys.exit(1)
    develop_path, base_path, main_path, out_path = sys.argv[1:5]

    with open(develop_path, encoding="utf-8") as f:
        develop_text = f.read()
    with open(base_path, encoding="utf-8") as f:
        base_text = f.read()
    with open(main_path, encoding="utf-8") as f:
        main_text = f.read()

    base_headings = {block_heading_line(b) for b in split_blocks(base_text) if block_heading_line(b)}

    develop_blocks = split_blocks(develop_text)
    rest = develop_blocks[1:] if develop_blocks and block_heading_line(develop_blocks[0]) is None else develop_blocks

    new_blocks = []
    for block in rest:
        heading = block_heading_line(block)
        if heading and heading not in base_headings:
            new_blocks.append(block)
        else:
            break

    main_body = main_text
    if main_body.startswith("# Changelog"):
        main_body = main_body.split("\n", 1)[1].lstrip("\n")

    with open(out_path, "w", encoding="utf-8") as f:
        f.write("# Changelog\n\n")
        f.write("".join(new_blocks))
        f.write(main_body)


if __name__ == "__main__":
    main()
