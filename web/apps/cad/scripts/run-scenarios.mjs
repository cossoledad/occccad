import { readdir } from "node:fs/promises";
import { spawn } from "node:child_process";
import { resolve } from "node:path";

const args = process.argv.slice(2);
const verbose = args.includes("--verbose");
const listOnly = args.includes("--list");
const filters = args.filter((arg) => !arg.startsWith("--")).map((arg) => arg.toLowerCase());

async function scenarios(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const found = [];
  for (const entry of entries) {
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) found.push(...await scenarios(path));
    else if (entry.name.endsWith(".scenario.mjs")) found.push(path);
  }
  return found;
}

const discovered = (await scenarios(resolve("src"))).sort();
const selected = discovered.filter((scenario) =>
  filters.length === 0 || filters.some((filter) => scenario.toLowerCase().includes(filter))
);

if (listOnly) {
  for (const scenario of selected) console.log(scenario);
  process.exit(0);
}

if (selected.length === 0) {
  console.error(`No scenarios matched: ${filters.join(", ")}`);
  process.exit(2);
}

for (const scenario of selected) {
  await new Promise((accept, reject) => {
    const child = spawn(process.execPath, [scenario], {
      stdio: verbose ? "inherit" : ["ignore", "pipe", "pipe"],
    });
    const stdout = [];
    const stderr = [];
    child.stdout?.on("data", (chunk) => stdout.push(chunk));
    child.stderr?.on("data", (chunk) => stderr.push(chunk));
    child.once("error", reject);
    child.once("exit", (code) => {
      if (code === 0) return accept();
      if (!verbose) {
        if (stdout.length > 0) process.stdout.write(Buffer.concat(stdout));
        if (stderr.length > 0) process.stderr.write(Buffer.concat(stderr));
      }
      reject(new Error(`${scenario} failed with ${code}`));
    });
  });
}

const selection = filters.length === 0 ? "all" : filters.join(",");
console.log(`web scenarios PASS (${selected.length}/${discovered.length}; ${selection})`);
