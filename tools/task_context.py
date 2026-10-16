"""Bounded task packets and measurable discovery, without a repository index."""
from __future__ import annotations

import json
from pathlib import Path
import subprocess
import time
from uuid import uuid4

from tools.task_recipes import RECIPES


def audit(root: Path) -> list[str]:
    errors = []
    for name, recipe in RECIPES.items():
        for path, symbol in recipe["sources"]:
            target = root / path
            if not target.is_file():
                errors.append(f"{name}: missing source {path}")
            elif symbol not in target.read_text():
                errors.append(f"{name}: missing symbol {symbol} in {path}")
        for path in recipe["documents"]:
            if not (root / path).is_file():
                errors.append(f"{name}: missing knowledge route {path}")
    return errors


def packet(root: Path, name: str, symbol=None, read=False, output=None) -> dict:
    if name not in RECIPES:
        raise ValueError("unknown recipe; choose " + ", ".join(RECIPES))
    started = time.monotonic()
    recipe = RECIPES[name]
    sources, snippets = [], []
    searches = 0
    for path, entry in recipe["sources"]:
        query = symbol or entry
        result = subprocess.run(["rg", "-n", "-m", "3", "-F", "--", query, path],
                                cwd=root, capture_output=True, text=True)
        searches += 1
        if result.returncode not in (0, 1):
            raise ValueError(f"cannot search source {path}")
        hits = result.stdout.splitlines()
        locations = [int(hit.split(":", 1)[0]) for hit in hits]
        sources.append({"path": path, "entry": entry, "query": query, "lines": locations})
        if read and locations and len(snippets) < 2:
            # Only bounded source excerpts enter the packet. Disk I/O and
            # source bytes returned are different metrics.
            lines = (root / path).read_text().splitlines()
            first, last = max(0, locations[0] - 9), min(len(lines), locations[0] + 16)
            excerpt = "\n".join(f"{i + 1}: {lines[i]}" for i in range(first, last))
            encoded = excerpt.encode()
            excerpt = encoded[:5000].decode("utf-8", errors="ignore")
            snippets.append({"path": path, "text": excerpt, "truncated": len(encoded) > 5000})
    report = {"schema": "occccad.task-context.v1", "recipe": name, "goal": recipe["goal"],
              "sources": sources, "documents": recipe["documents"], "obligations": recipe["obligations"],
              "validation": {"focused": recipe["focused"], "complete": recipe["complete"],
                             "escalate": recipe["escalate"]}, "snippets": snippets,
              "metrics": {"searchCalls": searches, "returnedSourceBytes": sum(len(s["text"].encode()) for s in snippets),
                          "discoverySeconds": round(time.monotonic() - started, 6), "modelTokens": None,
                          "boundary": "this packet only; excludes agent reasoning, subsequent reads and implementation"}}
    destination = Path(output) if output else root / "build/agent-workflow" / (time.strftime("%Y%m%dT%H%M%S") + "-" + uuid4().hex[:8] + ".json")
    destination.parent.mkdir(parents=True, exist_ok=True)
    report["artifact"] = str(destination)
    destination.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")
    return report
