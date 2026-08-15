#!/usr/bin/env python3
"""Consolidates every alpha entry since the last official release into one
entry for the version being cut, dropping the individual alpha headings.

develop's CHANGELOG.md accumulates one heading per alpha release-please
cuts. main should show only official releases - each one summarizing
everything that happened since the previous official release, not the
individual alphas along the way.
"""
import re
import sys

ALPHA_HEADING = re.compile(r"^## (?:\[[^\]]+\]\([^)]*\)|\S+) \(\d{4}-\d{2}-\d{2}\)\s*$")
IS_ALPHA = re.compile(r"-alpha\.\d+")
SECTION_HEADING = re.compile(r"^### (.+)$")


def split_blocks(text):
    lines = text.splitlines(keepends=True)
    blocks = []
    current = []
    for line in lines:
        if ALPHA_HEADING.match(line) and current:
            blocks.append("".join(current))
            current = [line]
        else:
            current.append(line)
    if current:
        blocks.append("".join(current))
    return blocks


def block_heading_line(block):
    for line in block.splitlines():
        if ALPHA_HEADING.match(line):
            return line
    return None


def parse_sections(block):
    """Returns {section_name: [bullet_lines]} for a single release block."""
    sections = {}
    current_section = None
    for line in block.splitlines():
        m = SECTION_HEADING.match(line)
        if m:
            current_section = m.group(1)
            sections.setdefault(current_section, [])
            continue
        if current_section is not None and line.startswith("* "):
            sections[current_section].append(line)
    return sections


def main():
    if len(sys.argv) != 3:
        print("usage: consolidate-changelog.py CHANGELOG.md X.Y.Z", file=sys.stderr)
        sys.exit(1)
    path, version = sys.argv[1], sys.argv[2]

    with open(path, encoding="utf-8") as f:
        text = f.read()

    blocks = split_blocks(text)
    preamble = blocks[0] if blocks and block_heading_line(blocks[0]) is None else ""
    rest = blocks[1:] if preamble else blocks

    alpha_blocks = []
    remaining = []
    for i, block in enumerate(rest):
        heading = block_heading_line(block)
        if heading and IS_ALPHA.search(heading):
            alpha_blocks.append(block)
        else:
            remaining = rest[i:]
            break
    else:
        remaining = []

    if not alpha_blocks:
        print("no alpha entries to consolidate", file=sys.stderr)
        sys.exit(0)

    merged = {}
    order = []
    for block in alpha_blocks:
        for section, bullets in parse_sections(block).items():
            if section not in merged:
                merged[section] = []
                order.append(section)
            merged[section].extend(bullets)

    date = re.search(r"\((\d{4}-\d{2}-\d{2})\)", block_heading_line(alpha_blocks[0])).group(1)

    new_block = f"## {version} ({date})\n\n\n"
    for section in order:
        new_block += f"### {section}\n\n"
        new_block += "\n".join(merged[section]) + "\n\n\n"
    new_block = new_block.rstrip("\n") + "\n\n"

    with open(path, "w", encoding="utf-8") as f:
        f.write(preamble)
        f.write(new_block)
        f.write("".join(remaining))


if __name__ == "__main__":
    main()
