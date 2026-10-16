"""Owned Markdown navigation and measurable context sizes (no token estimates)."""
from pathlib import Path
import re
import subprocess
from urllib.parse import unquote


CODE_GUIDES = (
    "kernel/assembly/AGENTS.md",
    "kernel/occt/AGENTS.md",
    "workers/geometry/AGENTS.md",
    "services/AGENTS.md",
    "web/apps/cad/AGENTS.md",
)
KNOWLEDGE_INDEXES = (
    "docs/README.md",
    "docs/architecture/current/README.md",
    "docs/architecture/target/README.md",
    "docs/development/README.md",
    "docs/guides/README.md",
    "docs/references/README.md",
    "docs/validation/README.md",
    "plans/README.md",
)
REQUIRED_ENTRIES = (*KNOWLEDGE_INDEXES, "docs/AGENTS.md",
                    "docs/development/environment.md",
                    "docs/development/agent-workflow.md",
                    "docs/development/documentation.md",
                    "docs/architecture/current/model-history.md",
                    "docs/architecture/current/persistent-naming.md",
                    "docs/architecture/current/worker-contracts.md")


def owned_markdown(root: Path) -> list[Path]:
    paths = subprocess.check_output(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], cwd=root).decode().split("\0")
    return sorted({root / p for p in paths if p.endswith(".md") and (root / p).is_file()})


def slug(text: str) -> str:
    return re.sub(r"[^\w\s\-\u4e00-\u9fff]", "", re.sub(r"<[^>]*>", "", text).lower()).replace(" ", "-")


def prose(text: str) -> str:
    return re.sub(r"^\s*(```|~~~)[\s\S]*?^\s*\1.*$", "", text, flags=re.M)


def anchors(text: str) -> set[str]:
    result, counts = set(), {}
    for heading in re.findall(r"^#{1,6}\s+(.+?)\s*#*\s*$", prose(text), re.M):
        name = slug(heading); count = counts.get(name, 0); counts[name] = count + 1
        result.add(name if count == 0 else f"{name}-{count}")
    result.update(re.findall(r'(?:id|name)=["\']([^"\']+)', text))
    return result


def local_links(source: Path, text: str) -> list[tuple[str, Path, str]]:
    """Return local destinations, shared by link checking and discovery checks."""
    text = prose(text)
    links = re.findall(r"!?\[[^\]]*\]\(([^)]+)\)", text)
    links += re.findall(r"^\s*\[[^\]]+\]:\s*(<[^>]+>|\S+)", text, re.M)
    result = []
    for link in links:
        link = link.strip().split(' "', 1)[0].strip("<>")
        if re.match(r"[a-z][a-z0-9+.-]*:", link, re.I) or link.startswith("/"):
            continue
        name, _, anchor = unquote(link).partition("#")
        target = (source.parent / name).resolve() if name else source.resolve()
        result.append((link, target, anchor))
    return result


def audit(root: Path) -> list[str]:
    errors = []
    for source in owned_markdown(root):
        raw = source.read_text()
        fence = None
        for line in raw.splitlines():
            match = re.match(r"^\s*(`{3,}|~{3,})", line)
            if match:
                mark=match.group(1)
                if fence is None: fence=mark
                elif mark[0]==fence[0] and len(mark)>=len(fence): fence=None
        if fence: errors.append(f"unclosed fenced block: {source.relative_to(root)}")
        for link, target, anchor in local_links(source, raw):
            if not target.exists():
                errors.append(f"broken local link: {source.relative_to(root)} -> {link}")
            elif anchor and target.suffix == ".md" and anchor not in anchors(target.read_text()):
                errors.append(f"broken anchor: {source.relative_to(root)} -> {link}")
    return errors


def navigation_errors(root: Path) -> list[str]:
    """Every owned Markdown page must be discoverable from the two root entries."""
    files = {path.resolve() for path in owned_markdown(root)}
    pending = [root / "README.md", root / "AGENTS.md"]
    seen = set()
    while pending:
        source = pending.pop().resolve()
        if source in seen or source not in files:
            continue
        seen.add(source)
        for _, target, _ in local_links(source, source.read_text()):
            # A link to a knowledge/module directory opens its README.
            if target.is_dir():
                target = target / "README.md"
            if target in files:
                pending.append(target)
    return [f"unreachable Markdown: {path.relative_to(root.resolve())}"
            for path in sorted(files - seen)]


def structure_errors(root: Path) -> list[str]:
    errors = [f"missing knowledge entry: {name}" for name in REQUIRED_ENTRIES
              if not (root / name).is_file()]
    topic_directories = {str(Path(name).parent) for name in KNOWLEDGE_INDEXES
                         if name.startswith("docs/") and name != "docs/README.md"}
    for path in owned_markdown(root):
        name = str(path.relative_to(root))
        if not name.startswith("docs/") or name in {"docs/README.md", "docs/AGENTS.md"}:
            continue
        if str(Path(name).parent) not in topic_directories:
            errors.append(f"unclassified knowledge page: {name}")
        elif path.name not in {"README.md", "AGENTS.md"} and not re.fullmatch(r"[a-z0-9]+(?:-[a-z0-9]+)*\.md", path.name):
            errors.append(f"non-semantic documentation filename: {name}")
    return errors


def sizes(root: Path) -> dict[str, int]:
    files = owned_markdown(root)
    return {str(p.relative_to(root)): p.stat().st_size for p in files}


def entry_sizes(data: dict[str, int]) -> dict:
    """Static paths, not a token estimate or a claim that every task reads the map."""
    root = data["AGENTS.md"]
    project_map = data["docs/README.md"]
    return {
        "rootGuide": root,
        "projectMap": project_map,
        "localTask": {guide: root + data[guide] for guide in CODE_GUIDES},
        "withProjectMap": {guide: root + project_map + data[guide] for guide in CODE_GUIDES},
    }


if __name__ == "__main__":
    import json
    import argparse
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", help="Compare byte sizes with this Git revision, without modifying files")
    args=parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    errors = audit(root) + navigation_errors(root) + structure_errors(root)
    current=sizes(root)
    report={"errors": errors, "bytes": current, "markdownCount":len(current), "totalBytes":sum(current.values()), "entryBytes": entry_sizes(current)}
    if args.baseline:
        names=subprocess.check_output(["git","ls-tree","-r","--name-only",args.baseline],cwd=root).decode().splitlines()
        before={p:len(subprocess.check_output(["git","show",f"{args.baseline}:{p}"],cwd=root)) for p in names if p.endswith(".md")}
        report["comparison"]={"baseline":args.baseline,"beforeCount":len(before),"beforeTotalBytes":sum(before.values()),"entryBytesBefore":entry_sizes(before),"entryBytesAfter":entry_sizes(current),"deleted":sorted(set(before)-set(current)),"added":sorted(set(current)-set(before))}
    print(json.dumps(report, ensure_ascii=False, indent=2))
    raise SystemExit(bool(errors))
