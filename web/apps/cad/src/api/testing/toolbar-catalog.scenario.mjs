import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });

try {
  const { mockToolbarCatalog } = await server.ssrLoadModule("/src/api/mock-toolbar-catalog.ts");
  const toolbars = mockToolbarCatalog.toolbars;
  assert.ok(toolbars.some((toolbar) => toolbar.id === "part-features"));
  assert.ok(toolbars.some((toolbar) => toolbar.id === "sketch-projection"));
  assert.ok(toolbars.some((toolbar) => toolbar.id === "product-interface"));
  assert.ok(toolbars.some((toolbar) => toolbar.id === "view-navigation"));
  assert.ok(toolbars.every((toolbar) => toolbar.items.every(item=>item.groupKey==="primary"||item.groupKey.startsWith("variants:"))),
    "one toolbar must represent one user-intent category");
  const commands = toolbars.flatMap((toolbar) => toolbar.items);
  assert.ok(!commands.some((item) => item.commandId === "sketch.normal"));
  assert.equal(commands.some((item) => item.commandId === "capture.settings"), false,
    "capture settings belong to the global preference center");
  assert.equal(commands.some((item) => item.commandId === "navigation.profile.toggle"), false,
    "mouse navigation belongs to the global preference center");
  assert.equal(commands.find((item) => item.commandId === "part.publications")?.iconKey, "publication");
  assert.equal(commands.find((item) => item.commandId === "product.release")?.iconKey, "release");
  assert.deepEqual(toolbars.filter(bar=>bar.workbench==="PART_DESIGN").flatMap(bar=>bar.items).filter((item) => item.commandId.startsWith("view.")).map((item) => item.commandId),
    ["view.fit", "view.top", "view.front", "view.right", "view.iso", "view.normal", "view.selection-summary"]);
  for (const command of ["assembly.parallel", "assembly.perpendicular"]) {
    assert.ok(toolbars.find(toolbar => toolbar.id === "assembly-constraints").items.some(item => item.commandId === command));
  }
  const {contextTabs,builtinCatalog,matches}=await server.ssrLoadModule("/src/cad/command/workbench-catalog.ts");
  const facts={targetType:'PRODUCT',hostType:'PRODUCT',rootTarget:true,motionActive:false,sketchActive:false,canEdit:true,rootCanEdit:true,busy:false,isMock:true,moveReceiptPending:false,selectionKind:'',selectionCount:0,hasWorkingBody:false};
  assert.deepEqual(contextTabs(builtinCatalog,facts).map(t=>t.name),['装配设计','机构与 DMU','视图','文档与协作','DEBUG']);
  assert(!contextTabs(builtinCatalog,{...facts,rootTarget:false}).some(t=>t.domain==='KINEMATICS_DMU'),'nested Product editing cannot create a host mechanism');
  const motionTab=contextTabs(builtinCatalog,facts).find(t=>t.domain==='KINEMATICS_DMU');
  const motionGroups=toolbars.filter(t=>t.tabIds.includes(motionTab.id));
  const motionCommands=motionGroups.flatMap(g=>g.items).map(v=>v.commandId);
  for(const cmd of builtinCatalog.commands.filter(v=>v.id.startsWith('dmu.'))){
   assert(motionCommands.includes(cmd.id),`${cmd.id} needs a classified placement`);
   assert.equal(matches(cmd.visibleWhen,facts),false);
   assert.equal(matches(cmd.visibleWhen,{...facts,motionActive:true}),true);
  }
  assert.equal(new Set(motionCommands).size,motionCommands.length);
  assert.equal(builtinCatalog.commands.find(v=>v.id==='assembly.motion-study').implementation,'handler','application navigation must not own a form invocation');
  for(const action of ['new','revolute','driver','study','interference','import','apply'])assert.equal(builtinCatalog.commands.find(v=>v.id==='dmu.'+action).implementation,'form');
  for(const action of ['run','check','play','pause','next','reset','restore','cancel'])assert.equal(builtinCatalog.commands.find(v=>v.id==='dmu.'+action).implementation,'handler');
  console.log("Toolbar category and preference-boundary tests passed.");
} finally {
  await server.close();
}
