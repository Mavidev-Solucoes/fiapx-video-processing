#!/usr/bin/env python3
"""Union of atomic Go unit-test and instrumented-container coverage by source block.

No packages or source blocks are removed. Blocks covered by either suite are
counted once in the denominator and marked as executed.
"""
from __future__ import annotations

import pathlib
import sys


def read_profile(path: pathlib.Path):
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0].strip() != "mode: atomic":
        raise ValueError(f"{path}: expected an atomic Go coverage profile")
    result = {}
    for line_number, line in enumerate(lines[1:], start=2):
        if not line.strip():
            continue
        fields = line.split()
        if len(fields) != 3:
            raise ValueError(f"{path}:{line_number}: invalid entry")
        location, nstmt, count = fields
        try:
            statements, executions = int(nstmt), int(count)
        except ValueError as error:
            raise ValueError(f"{path}:{line_number}: invalid count") from error
        if statements < 0 or executions < 0 or ":" not in location or "," not in location:
            raise ValueError(f"{path}:{line_number}: invalid block")
        if location in result:
            previous_statements, previous_count = result[location]
            if previous_statements != statements:
                raise ValueError(f"{path}:{line_number}: inconsistent statement count")
            result[location] = statements, max(previous_count, executions)
        else:
            result[location] = statements, executions
    if not result:
        raise ValueError(f"{path}: empty Go coverage profile")
    return result


def merge(unit_path: pathlib.Path, integration_path: pathlib.Path, output: pathlib.Path):
    unit = read_profile(unit_path)
    integration = read_profile(integration_path)
    combined = dict(unit)
    overlaps = 0
    for block, (statements, count) in integration.items():
        if block in combined:
            previous_statements, previous_count = combined[block]
            if previous_statements != statements:
                raise ValueError(f"mismatched statement counts at {block}")
            combined[block] = statements, max(previous_count, count)
            overlaps += 1
        else:
            combined[block] = statements, count
    if not overlaps:
        raise ValueError("incompatible profiles: no common source blocks")
    covered = sum(statements for statements, count in combined.values() if count > 0)
    total = sum(statements for statements, _ in combined.values())
    if total <= 0:
        raise ValueError("zero-statement coverage profile")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text("mode: atomic\n" + "".join(
        f"{block} {combined[block][0]} {combined[block][1]}\n"
        for block in sorted(combined)
    ), encoding="utf-8")
    print(f"Go statement coverage: {covered}/{total} = {100 * covered / total:.1f}%")
    print(f"Unit blocks={len(unit)}, integration blocks={len(integration)}, overlap={overlaps}")


if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit("usage: merge-go-coverage.py UNIT.out INTEGRATION.out COMBINED.out")
    try:
        merge(*(pathlib.Path(arg) for arg in sys.argv[1:]))
    except (ValueError, OSError) as exc:
        sys.exit(f"coverage merge error: {exc}")
