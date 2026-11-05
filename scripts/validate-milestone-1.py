#!/usr/bin/env python3
"""Validate the dispatch graph without touching GitHub or live processes."""
import json
import sys
from pathlib import Path


def validate(path):
    graph = json.loads(path.read_text())
    tasks = graph["tasks"]
    ids = [task["issue"] for task in tasks]
    if len(ids) != len(set(ids)) or not all(isinstance(i, int) for i in ids):
        raise ValueError("task issue IDs must be unique integers")
    if set(ids) & set(graph["not_m1_blockers"]):
        raise ValueError("M1 tasks cannot also be outside its critical path")
    if set(ids) & {task["issue"] for task in graph["later"]}:
        raise ValueError("follow-on tasks cannot be included in M1")
    owners = [task["owner"] for task in tasks]
    if len(owners) != len(set(owners)):
        raise ValueError("work ownership must be distinct")
    dependencies = {}
    for task in tasks:
        deps = task["depends_on"]
        if len(deps) != len(set(deps)) or not set(deps) <= set(ids):
            raise ValueError(f"invalid dependency for #{task['issue']}")
        if task["priority"] not in {"P0", "P1", "P2"}:
            raise ValueError("unknown priority")
        dependencies[task["issue"]] = set(deps)
    done = set()
    waves = []
    while len(done) < len(ids):
        ready = sorted(i for i in ids if i not in done and dependencies[i] <= done)
        if not ready:
            raise ValueError("dependency cycle prevents dispatch")
        waves.append(ready)
        done.update(ready)
    if waves[0] != [10] or waves[-1] != [12]:
        raise ValueError("M1 must begin with contract #10 and end with acceptance #12")
    ancestors = set()
    remaining = [12]
    while remaining:
        node = remaining.pop()
        if node not in ancestors:
            ancestors.add(node)
            remaining.extend(dependencies[node])
    if ancestors != set(ids):
        raise ValueError("every M1 task must feed the acceptance gate")
    return {"success": True, "tasks": len(ids), "waves": waves,
            "max_implementation_workers": graph["max_implementation_workers"],
            "note": "Waves show precedence, not current issue/merge status."}


if __name__ == "__main__":
    default = Path(__file__).resolve().parents[1] / "docs" / "milestone-1.tasks.json"
    try:
        result = validate(Path(sys.argv[1]) if len(sys.argv) > 1 else default)
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(json.dumps({"success": False, "error": str(error)}))
        sys.exit(1)
    print(json.dumps(result))
