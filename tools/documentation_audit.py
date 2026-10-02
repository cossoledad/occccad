"""Owned Markdown navigation and measurable context sizes (no token estimates)."""
from pathlib import Path
import re
import subprocess
from urllib.parse import unquote


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
        text = prose(raw)
        links = re.findall(r"!?\[[^\]]*\]\(([^)]+)\)", text)
        links += re.findall(r"^\s*\[[^\]]+\]:\s*(\S+)", text, re.M)
        for link in links:
            link = link.strip().split(' "', 1)[0].strip("<>")
            if re.match(r"[a-z][a-z0-9+.-]*:", link, re.I) or link.startswith("/"):
                continue
            name, _, anchor = unquote(link).partition("#")
            target = (source.parent / name).resolve() if name else source
            if not target.exists():
                errors.append(f"broken local link: {source.relative_to(root)} -> {link}")
            elif anchor and target.suffix == ".md" and anchor not in anchors(target.read_text()):
                errors.append(f"broken anchor: {source.relative_to(root)} -> {link}")
    return errors


def sizes(root: Path) -> dict[str, int]:
    files = owned_markdown(root)
    return {str(p.relative_to(root)): p.stat().st_size for p in files}


if __name__ == "__main__":
    import json
    import argparse
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", help="Compare byte sizes with this Git revision, without modifying files")
    args=parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    errors = audit(root)
    current=sizes(root)
    report={"errors": errors, "bytes": current, "markdownCount":len(current), "totalBytes":sum(current.values())}
    if args.baseline:
        names=subprocess.check_output(["git","ls-tree","-r","--name-only",args.baseline],cwd=root).decode().splitlines()
        before={p:len(subprocess.check_output(["git","show",f"{args.baseline}:{p}"],cwd=root)) for p in names if p.endswith(".md")}
        guides=("kernel/assembly/AGENTS.md","kernel/occt/AGENTS.md","workers/geometry/AGENTS.md","services/AGENTS.md","web/apps/cad/AGENTS.md")
        chain=lambda data:{g:sum(data[p] for p in ("AGENTS.md","docs/README.md",g)) for g in guides}
        report["comparison"]={"baseline":args.baseline,"beforeCount":len(before),"beforeTotalBytes":sum(before.values()),"defaultEntryBytesBefore":chain(before),"defaultEntryBytesAfter":chain(current),"deleted":sorted(set(before)-set(current)),"added":sorted(set(current)-set(before))}
    print(json.dumps(report, ensure_ascii=False, indent=2))
    raise SystemExit(bool(errors))
