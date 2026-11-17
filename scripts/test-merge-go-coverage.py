#!/usr/bin/env python3
"""Coverage union must mark covered blocks, never count them twice."""
import importlib.util
import pathlib
import tempfile

spec = importlib.util.spec_from_file_location("merger", pathlib.Path(__file__).with_name("merge-go-coverage.py"))
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(module)

with tempfile.TemporaryDirectory() as temp:
    folder = pathlib.Path(temp)
    unit, integration, combined = (folder / name for name in ("unit.out", "integration.out", "combined.out"))
    unit.write_text("mode: atomic\npkg/a.go:1.1,2.2 2 0\npkg/b.go:4.1,7.1 3 1\n")
    integration.write_text("mode: atomic\npkg/a.go:1.1,2.2 2 1\npkg/b.go:4.1,7.1 3 0\n")
    module.merge(unit, integration, combined)
    result = combined.read_text()
    assert result.count("pkg/a.go") == 1, result
    assert result.count("pkg/b.go") == 1, result
    assert "pkg/a.go:1.1,2.2 2 1" in result
    assert "pkg/b.go:4.1,7.1 3 1" in result
    integration.write_text("mode: atomic\npkg/a.go:1.1,2.2 9 1\n")
    try:
        module.merge(unit, integration, combined)
    except ValueError as error:
        assert "mismatched" in str(error)
    else:
        raise AssertionError("mismatched statement counts accepted")
print("PASS: Go coverage union counts unique source blocks")
