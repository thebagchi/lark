#!/usr/bin/env python3
"""
fmt_proto.py — Format .proto files by aligning '=' signs.

Aligns assignment operators within each message/enum block (including nested).
"""

import re
import sys
from pathlib import Path


def split_blocks(lines: list[str]) -> list[tuple[str, list[str]]]:
    """Split into (kind, block_lines) where kind is 'message', 'enum', or 'other'."""
    blocks: list[tuple[str, list[str]]] = []
    i = 0
    n = len(lines)
    while i < n:
        line = lines[i]
        m = re.match(r'^\s*(message|enum)\s+\w+', line)
        if m:
            kind = m.group(1)
            block = [line]
            depth = line.count('{') - line.count('}')
            j = i + 1
            while j < n and depth > 0:
                block.append(lines[j])
                depth += lines[j].count('{') - lines[j].count('}')
                j += 1
            blocks.append((kind, block))
            i = j
            continue
        blocks.append(('other', [line]))
        i += 1
    return blocks


def _align_eq_in_block(lines: list[str]) -> list[str]:
    """Align '=' only within this block's field/enum lines."""
    parsed = []
    for line in lines:
        stripped = line.rstrip()
        # Message field: optional/repeated type name = ...
        m = re.match(r'^(\s*(?:optional|required|repeated|map<[^>]+>)?\s*\S+\s+\S+)\s*=\s*(.*)', stripped)
        if m:
            parsed.append((m.group(1), m.group(2), line))
            continue
        # Enum value or oneof field
        m2 = re.match(r'^(\s*\w+)\s*=\s*(.*)', stripped)
        if m2:
            parsed.append((m2.group(1), m2.group(2), line))
            continue
        # Everything else (comments, reserved, options, nested braces, etc.)
        parsed.append((None, None, line))

    # Find max prefix length
    prefixes = [p[0] for p in parsed if p[0] is not None]
    if not prefixes:
        return lines

    max_len = max(len(p) for p in prefixes)
    target = max_len + 1  # space before =

    result = []
    for prefix, suffix, original in parsed:
        if prefix is None:
            result.append(original)
        else:
            pad = ' ' * (target - len(prefix))
            ending = '\n' if original.endswith('\n') else ''
            result.append(f"{prefix}{pad}= {suffix}{ending}")
    return result


def format_block(kind: str, block: list[str]) -> list[str]:
    """Format a message/enum block recursively."""
    if kind not in ('message', 'enum'):
        return block

    header = block[0]
    # Find body and trailer
    body = []
    trailer = []
    depth = header.count('{') - header.count('}')
    for line in block[1:]:
        if depth <= 0:
            trailer.append(line)
        else:
            body.append(line)
            depth += line.count('{') - line.count('}')

    # Recurse on nested blocks
    inner_blocks = split_blocks(body)
    formatted_body = []
    for bkind, blines in inner_blocks:
        formatted_body.extend(format_block(bkind, blines))

    # Align only at this level (non-nested field lines)
    aligned_body = _align_eq_in_block(formatted_body)

    return [header] + aligned_body + trailer


def fmt_proto_file(path: str) -> None:
    p = Path(path)
    if not p.exists():
        print(f"  skipped (not found): {p}")
        return

    original = p.read_text(encoding='utf-8')
    lines = original.splitlines(keepends=True)

    blocks = split_blocks(lines)
    out_lines = []
    for kind, blines in blocks:
        out_lines.extend(format_block(kind, blines))

    result = ''.join(out_lines)
    if result != original:
        p.write_text(result, encoding='utf-8')
        print(f"  ✓ formatted {p}")
    else:
        print(f"  • no changes {p}")


def main() -> None:
    if len(sys.argv) < 2:
        print("Usage: python3 fmt_proto.py <file.proto> [...]")
        print("       python3 fmt_proto.py *.proto")
        sys.exit(1)

    for arg in sys.argv[1:]:
        paths = Path('.').glob(arg) if '*' in arg or '?' in arg else [Path(arg)]
        for path in paths:
            fmt_proto_file(str(path))


if __name__ == '__main__':
    main()