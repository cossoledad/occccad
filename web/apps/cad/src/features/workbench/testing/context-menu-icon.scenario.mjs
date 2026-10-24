import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer } from "vite";

const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { ContextMenuIcon } = await server.ssrLoadModule("/src/components/context-menu-icon.tsx");
  const blank = renderToStaticMarkup(createElement(ContextMenuIcon));
  const populated = renderToStaticMarkup(createElement(ContextMenuIcon, null, createElement("svg", { "aria-label": "ignored" })));
  assert.match(renderToStaticMarkup(createElement(ContextMenuIcon,{className:"ant-dropdown-menu-item-icon"})),/context-menu-icon-slot ant-dropdown-menu-item-icon/,"Antd layout class reaches the actual slot");
  assert.match(blank, /class="context-menu-icon-slot"/);
  assert.match(blank, /aria-hidden="true"/);
  assert(!blank.includes("?"), "blank slots contain no fallback symbol");
  assert.match(populated, /aria-hidden="true"/, "decorative icons do not change the menu item's accessible name");
  // Actual configured tree-menu rows are exercised by constraint-set-menu;
  // declaration/helper source spelling is not the icon accessibility contract.
  console.log("Context-menu icon and blank-slot accessibility/layout contract passed");
} finally { await server.close(); }
