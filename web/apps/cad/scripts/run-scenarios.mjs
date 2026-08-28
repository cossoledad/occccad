import { readdir } from "node:fs/promises";
import { spawn } from "node:child_process";
import { resolve } from "node:path";

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

for (const scenario of (await scenarios(resolve("src"))).sort()) {
  await new Promise((accept, reject) => {
    const child = spawn(process.execPath, [scenario], { stdio: "inherit" });
    child.once("error", reject);
    child.once("exit", (code) => code === 0 ? accept() : reject(new Error(`${scenario} failed with ${code}`)));
  });
}
