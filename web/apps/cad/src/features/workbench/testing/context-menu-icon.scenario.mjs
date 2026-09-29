import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer } from "vite";
import { readFile } from "node:fs/promises";

const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { ContextMenuIcon } = await server.ssrLoadModule("/src/components/context-menu-icon.tsx");
  const blank = renderToStaticMarkup(createElement(ContextMenuIcon));
  const populated = renderToStaticMarkup(createElement(ContextMenuIcon, null, createElement("svg", { "aria-label": "ignored" })));
  assert.match(blank, /class="context-menu-icon-slot"/);
  assert.match(blank, /aria-hidden="true"/);
  assert(!blank.includes("?"), "blank slots contain no fallback symbol");
  assert.match(populated, /aria-hidden="true"/, "decorative icons do not change the menu item's accessible name");
  const treeSource = await readFile(new URL("../specification-tree.tsx", import.meta.url), "utf8");
  const menuBlock = treeSource.slice(treeSource.indexOf("menu={{ items:"), treeSource.indexOf("] }}>{row}</Dropdown>"));
  const actionableItems = [...menuBlock.matchAll(/\{\s*key:/g)].length;
  const slots = [...menuBlock.matchAll(/icon:\s*<ContextMenuIcon/g)].length;
  assert.equal(slots, actionableItems, "every actionable context-menu row has an icon slot");
  console.log("Context-menu icon and blank-slot accessibility/layout contract passed");
} finally { await server.close(); }
