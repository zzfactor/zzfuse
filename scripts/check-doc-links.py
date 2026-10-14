#!/usr/bin/env python3
"""Check local targets in maintained guides without grading historical examples."""

import re
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
documents = [ROOT / name for name in ("README.md", "AGENTS.md", "CLAUDE.md", "CONTRIBUTING.md", "SECURITY.md", "TASKS.md", "CHANGELOG.md")]
documents.extend(sorted((ROOT / "docs").glob("*.md")))
documents.extend([ROOT / "services/README.md", ROOT / "frontend/README.md"])
errors = []
checked = 0

for document in documents:
    if not document.exists():
        continue
    in_fence = False
    for line_number, line in enumerate(document.read_text().splitlines(), 1):
        if line.lstrip().startswith(("```", "~~~")):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        for match in re.finditer(r"\]\(([^\s)]+)(?:\s+[^)]*)?\)", line):
            target = match.group(1).strip("<>")
            parts = urlsplit(target)
            if parts.scheme or target.startswith(("#", "/")):
                continue
            path = unquote(parts.path)
            if not path:
                continue
            checked += 1
            if not (document.parent / path).exists():
                errors.append(f"{document.relative_to(ROOT)}:{line_number}: {target}")

if errors:
    print("Broken local documentation targets:")
    print("\n".join(errors))
    raise SystemExit(1)
print(f"Checked {checked} local targets in maintained documentation.")
