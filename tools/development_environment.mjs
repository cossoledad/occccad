// Node development servers and Live tests read the same literal .env as Go.
// Backend values stay in the Node process; Vite exposes only its VITE_ prefix.
import {readFileSync} from "node:fs";
import {resolve} from "node:path";

export function readDevelopmentEnvironment(root, exported = process.env) {
  const explicit = exported.OCCCCAD_ENV_FILE?.trim();
  let content = "";
  try {
    content = readFileSync(explicit ? resolve(root, explicit) : resolve(root, ".env"), "utf8");
  } catch (error) {
    if (explicit || error.code !== "ENOENT") throw error;
  }
  const values = Object.create(null);
  for (let line of content.split(/\r?\n/)) {
    line = line.trim();
    if (!line || line.startsWith("#")) continue;
    line = line.replace(/^export /, "").trim();
    const separator = line.indexOf("=");
    if (separator < 0) continue;
    const key = line.slice(0, separator).trim();
    let value = line.slice(separator + 1).trim();
    if (!/^OCCCCAD_[A-Z0-9_]+$/.test(key) || key in values) continue;
    if (value.length >= 2 && ["'", '"'].includes(value[0]) && value.at(-1) === value[0]) value = value.slice(1, -1);
    values[key] = value;
  }
  for (const [key, value] of Object.entries(exported)) {
    if (key.startsWith("OCCCCAD_") && value !== undefined) values[key] = value;
  }
  return values;
}
