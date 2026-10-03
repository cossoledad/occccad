import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
  const React=await import(require.resolve("react"));
  const {renderToStaticMarkup}=await import(require.resolve("react-dom/server"));
  const {App}=await import(require.resolve("antd"));
  const {CommandProvider}=await server.ssrLoadModule("/src/cad/command/command-context.tsx");
  const {UIHelpProvider}=await server.ssrLoadModule("/src/cad/help/ui-help-context.tsx");
  const {WorkbenchCommands}=await server.ssrLoadModule("/src/features/workbench/workbench-commands.tsx");
  const {CommandRegistry}=await server.ssrLoadModule("/src/cad/command/command-registry.ts");
  const {mockToolbarCatalog}=await server.ssrLoadModule("/src/api/mock-toolbar-catalog.ts");
  const {contextualToolbars,searchCommands}=await server.ssrLoadModule("/src/features/workbench/workbench-command-model.ts");
  const registry=new CommandRegistry();
  const toolbars=contextualToolbars(mockToolbarCatalog.toolbars,"SKETCHER");
  let executions=0;
  for(const toolbar of toolbars)for(const item of toolbar.items)registry.register({id:item.commandId,execute:()=>executions++});
  const advanced=toolbars.flatMap(toolbar=>toolbar.items.filter(item=>item.groupKey.startsWith("variants:")));
  assert(advanced.some(item=>item.commandId==="sketch.circle.three_point"));
  assert(advanced.some(item=>item.commandId==="sketch.spline.control"));
  assert(!toolbars.some(toolbar=>toolbar.items.some(item=>item.commandId==="sketch.slot")));
  const html=renderToStaticMarkup(React.createElement(App,null,React.createElement(UIHelpProvider,null,
    React.createElement(CommandProvider,{registry},React.createElement(WorkbenchCommands,{toolbars,workbench:"SKETCHER"})))));
  assert(!html.includes("草图轮廓更多方式"));assert(html.includes("矩形"));assert(html.includes("矩形方式"));assert(html.includes("圆方式"));assert(html.includes("样条线方式"));assert(html.includes("cad-split-tool-button"));
  assert(!html.includes("三点圆"),"creation variants are discoverable through their family split buttons");
  assert(!html.includes("控制点样条"));
  const result=searchCommands(toolbars,"三点圆",id=>registry.state(id));
  assert(result.some(item=>item.commandId==="sketch.circle.three_point"),"grouped creation variants remain searchable");
  await registry.execute("sketch.circle.three_point");assert.equal(executions,1);
  const {CadSplitToolButton}=await server.ssrLoadModule("/src/cad/overlay/cad-split-tool-button.tsx");
  const family=advanced.filter(item=>item.groupKey==="variants:circle");
  const renderSplit=()=>renderToStaticMarkup(React.createElement(App,null,React.createElement(UIHelpProvider,null,
    React.createElement(CommandProvider,{registry},React.createElement(CadSplitToolButton,{items:family,defaultId:"sketch.circle",familyName:"圆",toolbarName:"轮廓",onDefaultChange(){throw new Error("render must not rewrite remembered defaults");}})))));
  registry.register({id:"sketch.circle.three_point",isActive:()=>true,execute:()=>executions++});
  assert(renderSplit().includes('aria-label="三点圆"'),'shortcut activation is shown even when a different default is remembered');
  registry.register({id:"sketch.circle.three_point",isActive:()=>false,execute:()=>executions++});
  registry.register({id:"sketch.circle",isEnabled:()=>false,execute:()=>executions++});
  const disabledDefault=renderSplit(),mainButton=disabledDefault.match(/<button[^>]*aria-label="圆"[^>]*>/)[0],arrowButton=disabledDefault.match(/<button[^>]*aria-label="圆方式"[^>]*>/)[0];
  assert(mainButton.includes('disabled'));assert(!arrowButton.includes('disabled'),'another legal variant keeps the arrow available');
  assert.equal(executions,1,'presentation does not execute domain commands');
  console.log("Sketch split-button rendering and grouped command discoverability passed");
}finally{await server.close();}
