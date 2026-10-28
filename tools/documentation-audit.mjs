// Owned Markdown links, navigation, layout and context budgets. No token estimate.
import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
export const codeGuides = [
  "kernel/assembly/AGENTS.md",
  "kernel/occt/AGENTS.md",
  "workers/geometry/AGENTS.md",
  "services/AGENTS.md",
  "web/apps/cad/AGENTS.md",
];
export const indexes = [
  "docs/README.md",
  "docs/architecture/current/README.md",
  "docs/architecture/target/README.md",
  "docs/development/README.md",
  "docs/guides/README.md",
  "docs/references/README.md",
  "docs/validation/README.md",
  "plans/README.md",
];
export const entries = [
  ...indexes,
  "docs/AGENTS.md",
  "docs/development/environment.md",
  "docs/development/agent-workflow.md",
  "docs/development/documentation.md",
  "docs/architecture/current/model-history.md",
  "docs/architecture/current/persistent-naming.md",
  "docs/architecture/current/worker-contracts.md",
];
export function owned(root) {
  return [
    ...new Set(
      execFileSync(
        "git",
        ["ls-files", "--cached", "--others", "--exclude-standard", "-z"],
        { cwd: root, encoding: "utf8" },
      )
        .split("\0")
        .filter((p) => p.endsWith(".md") && fs.existsSync(path.join(root, p))),
    ),
  ].sort();
}
export function slug(s) {
  return s
    .replace(/<[^>]*>/g, "")
    .toLowerCase()
    .replace(/[^\p{L}\p{N}_\s-]/gu, "")
    .replaceAll(" ", "-");
}
export function prose(s) {
  let fence = null,
    lines = [];
  for (const l of s.split("\n")) {
    const m = l.match(/^\s*(`{3,}|~{3,})/);
    if (m) {
      if (!fence) fence = m[1];
      else if (m[1][0] === fence[0] && m[1].length >= fence.length)
        fence = null;
      continue;
    }
    if (!fence) lines.push(l);
  }
  return lines.join("\n");
}
export function anchors(s) {
  const counts = {},
    result = new Set();
  for (const m of prose(s).matchAll(/^#{1,6}\s+(.+?)\s*#*\s*$/gm)) {
    const n = slug(m[1]),
      count = counts[n] ?? 0;
    counts[n] = count + 1;
    result.add(count ? `${n}-${count}` : n);
  }
  for (const m of s.matchAll(/(?:id|name)=["']([^"']+)/g)) result.add(m[1]);
  return result;
}
export function links(source, s) {
  s = prose(s);
  return [
    ...s.matchAll(/!?\[[^\]]*\]\(([^)]+)\)/g),
    ...s.matchAll(/^\s*\[[^\]]+\]:\s*(<[^>]+>|\S+)/gm),
  ].flatMap((m) => {
    const link = m[1].trim().split(' "')[0].replace(/^<|>$/g, "");
    if (/^[a-z][a-z0-9+.-]*:/i.test(link) || link.startsWith("/")) return [];
    const [name, anchor = ""] = decodeURIComponent(link).split("#");
    return [
      {
        link,
        target: path.resolve(
          path.dirname(source),
          name || path.basename(source),
        ),
        anchor,
      },
    ];
  });
}
export function entrySizes(bytes) {
  return {
    rootGuide: bytes["AGENTS.md"],
    projectMap: bytes["docs/README.md"],
    localTask: Object.fromEntries(
      codeGuides.map((g) => [g, bytes["AGENTS.md"] + bytes[g]]),
    ),
    withProjectMap: Object.fromEntries(
      codeGuides.map((g) => [
        g,
        bytes["AGENTS.md"] + bytes["docs/README.md"] + bytes[g],
      ]),
    ),
  };
}
// Compare actual file tasks with the Git delivery inventory, not only mise's
// filesystem discovery: ignored tasks work locally but disappear in a checkout.
export function publicTaskErrors(root, markdown = owned(root)) {
  const base = path.join(root, "dev/tasks");
  if (!fs.existsSync(base)) return [];
  const deliverable = new Set(execFileSync("git", ["ls-files", "--cached", "--others", "--exclude-standard", "-z"], {cwd:root, encoding:"utf8"}).split("\0"));
  const names = new Set(), errors = [];
  function visit(dir) {
    for (const e of fs.readdirSync(dir, {withFileTypes:true})) {
      const file = path.join(dir,e.name);
      if (e.isDirectory()) visit(file);
      else if (e.isFile()) {
        const rel = path.relative(root,file).split(path.sep).join("/");
        if (!deliverable.has(rel)) errors.push("public task absent from Git delivery inventory (ignored): " + rel);
        if (!(fs.statSync(file).mode & 0o111)) errors.push("public task is not executable: " + rel);
        names.add(path.relative(base,file).split(path.sep).filter(n => n !== "_default").join(":"));
      }
    }
  }
  visit(base);
  for (const file of markdown) {
    if (file.startsWith("docs/validation/")) continue; // Explicit historical evidence.
    for (const m of fs.readFileSync(path.join(root,file),"utf8").matchAll(/mise run (?:--quiet )?([a-z][a-z:._-]*)/g)) {
      if (!names.has(m[1])) errors.push("undelivered public command: " + file + " -> " + m[1]);
    }
  }
  return errors;
}
export function audit(root) {
  const files = owned(root),
    errors = publicTaskErrors(root, files),
    bytes = {},
    hotspots = [];
  for (const f of files) {
    const p = path.join(root, f),
      s = fs.readFileSync(p, "utf8");
    bytes[f] = Buffer.byteLength(s);
    let fence = null;
    for (const l of s.split("\n")) {
      const m = l.match(/^\s*(`{3,}|~{3,})/);
      if (m) {
        if (!fence) fence = m[1];
        else if (m[1][0] === fence[0] && m[1].length >= fence.length)
          fence = null;
      }
    }
    if (fence) errors.push("unclosed fenced block: " + f);
    for (const { link, target, anchor } of links(p, s)) {
      if (!fs.existsSync(target))
        errors.push(`broken local link: ${f} -> ${link}`);
      else if (
        anchor &&
        target.endsWith(".md") &&
        !anchors(fs.readFileSync(target, "utf8")).has(anchor)
      )
        errors.push(`broken anchor: ${f} -> ${link}`);
    }
  }
  const pending = ["README.md", "AGENTS.md"],
    seen = new Set();
  while (pending.length) {
    const f = pending.pop();
    if (seen.has(f) || !files.includes(f)) continue;
    seen.add(f);
    for (let { target } of links(
      path.join(root, f),
      fs.readFileSync(path.join(root, f), "utf8"),
    )) {
      if (fs.existsSync(target) && fs.statSync(target).isDirectory())
        target = path.join(target, "README.md");
      const rel = path.relative(root, target);
      if (files.includes(rel)) pending.push(rel);
    }
  }
  for (const f of files.filter((f) => !seen.has(f)))
    errors.push("unreachable Markdown: " + f);
  for (const f of entries)
    if (!fs.existsSync(path.join(root, f)))
      errors.push("missing knowledge entry: " + f);
  const dirs = new Set(
    indexes
      .filter((f) => f.startsWith("docs/") && f !== "docs/README.md")
      .map(path.dirname),
  );
  for (const f of files) {
    if (
      f.startsWith("docs/") &&
      !["docs/README.md", "docs/AGENTS.md"].includes(f)
    ) {
      if (!dirs.has(path.dirname(f)))
        errors.push("unclassified knowledge page: " + f);
      else if (
        !["README.md", "AGENTS.md"].includes(path.basename(f)) &&
        !/^[a-z0-9]+(?:-[a-z0-9]+)*\.md$/.test(path.basename(f))
      )
        errors.push("non-semantic documentation filename: " + f);
    }
  }
  for (const f of ["AGENTS.md", ...codeGuides, "docs/AGENTS.md"]) {
    if (!fs.existsSync(path.join(root, f))) {
      errors.push("missing guide: " + f);
      continue;
    }
    const s = fs.readFileSync(path.join(root, f), "utf8");
    if (s.trimEnd().split("\n").length > 100 || Buffer.byteLength(s) > 7000)
      errors.push("oversized guide: " + f);
  }
  if (bytes["docs/README.md"] > 5000) errors.push("project map exceeds 5 KB");
  for (const f of ["save-token.md", "save-token-plan.md"])
    if (fs.existsSync(path.join(root, f)))
      errors.push("superseded root prompt: " + f);
  for (const f of execFileSync(
    "git",
    ["ls-files", "--cached", "--others", "--exclude-standard", "-z"],
    { cwd: root, encoding: "utf8" },
  ).split("\0")) {
    const p = path.join(root, f);
    if (!f || !fs.existsSync(p) || !fs.statSync(p).isFile()) continue;
    if (fs.statSync(p).size >= 40000) {
      const b = fs.readFileSync(p);
      if (!b.subarray(0, 8192).includes(0))
        hotspots.push({ path: f, bytes: b.length });
    }
  }
  const recipesPath = path.join(root, "dev/recipes.json");
  if (fs.existsSync(recipesPath)) {
    for (const [name, r] of Object.entries(
      JSON.parse(fs.readFileSync(recipesPath)),
    )) {
      for (const [p, symbol] of r.sources) {
        const target = path.join(root, p);
        if (!fs.existsSync(target)) errors.push(name + ": missing source " + p);
        else if (!fs.readFileSync(target, "utf8").includes(symbol))
          errors.push(name + ": missing symbol " + symbol + " in " + p);
      }
      for (const p of r.documents)
        if (!fs.existsSync(path.join(root, p)))
          errors.push(name + ": missing knowledge route " + p);
    }
  }
  return {
    errors,
    bytes,
    markdownCount: files.length,
    totalBytes: Object.values(bytes).reduce((a, b) => a + b, 0),
    entryBytes: entrySizes(bytes),
    hotspots: hotspots.sort((a, b) => b.bytes - a.bytes),
  };
}
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const args = process.argv.slice(2);
  const baseIndex = args.indexOf("--baseline");
  const baseline = baseIndex >= 0 ? args[baseIndex + 1] : null;
  const parsed =
    baseIndex >= 0
      ? args.filter((_, i) => i !== baseIndex && i !== baseIndex + 1)
      : args;
  if (parsed.some((x) => !["--json", "--verbose"].includes(x)))
    throw Error("unknown audit argument");
  if (baseIndex >= 0 && !baseline)
    throw Error("--baseline requires a revision");
  const r = audit(root);
  if (baseline) {
    const names = execFileSync(
        "git",
        ["ls-tree", "-rz", "--name-only", baseline],
        { cwd: root, encoding: "utf8" },
      )
        .split("\0")
        .filter((p) => p.endsWith(".md")),
      before = Object.fromEntries(
        names.map((p) => [
          p,
          execFileSync("git", ["show", baseline + ":" + p], { cwd: root })
            .length,
        ]),
      );
    r.comparison = {
      baseline,
      beforeCount: names.length,
      beforeTotalBytes: Object.values(before).reduce((a, b) => a + b, 0),
      entryBytesBefore: entrySizes(before),
      entryBytesAfter: r.entryBytes,
      deleted: names.filter((p) => !(p in r.bytes)),
      added: Object.keys(r.bytes).filter((p) => !(p in before)),
    };
  }
  console.log(
    args.includes("--json")
      ? JSON.stringify(r, null, 2)
      : `${r.errors.length ? "FAIL" : "PASS"} docs: ${r.markdownCount} Markdown pages; ${r.hotspots.length} large text files\n${r.errors.join("\n")}`,
  );
  process.exitCode = r.errors.length ? 1 : 0;
}
