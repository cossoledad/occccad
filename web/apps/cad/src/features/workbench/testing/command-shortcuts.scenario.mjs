import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { resolveCommandShortcut, commandShortcutAria } = await server.ssrLoadModule("/src/cad/command/command-shortcuts.ts");
  const base = { key: "z", ctrlKey: true, metaKey: false, shiftKey: false, altKey: false, repeat: false, isComposing: false };
  assert.equal(resolveCommandShortcut(base)?.command, "edit.undo");
  assert.equal(resolveCommandShortcut({ ...base, key: "y" })?.command, "edit.redo");
  assert.equal(resolveCommandShortcut({ ...base, shiftKey: true })?.command, "edit.redo");
  assert.equal(resolveCommandShortcut({ ...base, ctrlKey: false, metaKey: true })?.command, "edit.undo");
  for (const override of [{ altKey: true }, { repeat: true }, { isComposing: true }, { metaKey: true }, { ctrlKey: false }])
    assert.equal(resolveCommandShortcut({ ...base, ...override }), undefined);
  assert.equal(resolveCommandShortcut({ ...base, key: "K" })?.command, "ui.command-search");
  assert.equal(resolveCommandShortcut({ ...base, key: "f", ctrlKey: false })?.command, "view.fit");
  assert.equal(resolveCommandShortcut({ ...base, key: "L", ctrlKey: false })?.command, "sketch.line");
  assert.equal(resolveCommandShortcut({ ...base, key: "?", ctrlKey: false, shiftKey: true })?.command, "ui.shortcut-help");
  assert.equal(resolveCommandShortcut({ ...base, key: "Delete", ctrlKey: false }), undefined);
  assert.equal(resolveCommandShortcut({ ...base, key: "Escape", ctrlKey: false }), undefined);
  assert.match(commandShortcutAria("edit.redo"), /Meta\+Shift\+z/);
  console.log("Shortcut aliases, modifiers, IME, repeat and reserved gesture keys passed.");
} finally { await server.close(); }
