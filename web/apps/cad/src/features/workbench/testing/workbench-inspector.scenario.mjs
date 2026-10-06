import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const modules = new Map();
// Load the actual component and its local helpers without a browser or a mock UI.
function load(file) {
  if (modules.has(file)) return modules.get(file).exports;
  const module = { exports: {} };
  modules.set(file, module);
  const js = ts.transpileModule(readFileSync(file, "utf8").replaceAll("import.meta.env", "({})"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX, esModuleInterop: true },
    fileName: file,
  }).outputText;
  const localRequire = createRequire(file);
  new Function("require", "module", "exports", js)((name) => {
    if (!name.startsWith(".")) return localRequire(name);
    const path = resolve(dirname(file), name);
    const target = [path, `${path}.ts`, `${path}.tsx`].find(existsSync);
    assert.ok(target, `cannot resolve ${name} from ${file}`);
    return load(target);
  }, module, module.exports);
  return module.exports;
}
const { Properties } = load(fileURLToPath(new URL("../workbench-inspector.tsx", import.meta.url)));
const { CAD_WORKBENCHES } = load(fileURLToPath(new URL("../../../cad/workbench/cad-workbench.ts", import.meta.url)));
const view = { document: { id: "part", type: "PART", name: "New Part", versionId: "revision", permission: "OWNER" }, part: { bodies:[], activeBodyId:"", features: [] } };
for (const primitives of [null, undefined, []]) {
  const diagnostics = {
    artifacts: [{ visualization: { schemaVersion: 1, referenceGeometry: { datumPlanes: [], axisSystems: [] }, primitives } }],
    aggregate: { artifactCount: 1, solidCount: 0, vertexCount: 0, glbBytes: 0, brepBytes: 0 }, worker: { available: false },
  };
  const html = renderToStaticMarkup(React.createElement(Properties, {
    view, selection: null, diagnostics, workbench: Object.keys(CAD_WORKBENCHES)[0], activeTool: "select", navigationProfile: "cad",
  }));
  assert.ok(html.includes("New Part"));
  assert.ok(html.includes("Non-solid Geometry"));
  if (primitives == null) assert.ok(html.includes("按需从 GLB 加载"), "unloaded data must not be reported as zero primitives");
}
console.log("Properties renders lightweight Artifact descriptors with null, omitted and empty primitives");

const { PartBodies }=load(fileURLToPath(new URL("../workbench-inspector.tsx",import.meta.url)));
view.part.bodies=[{id:"a",name:"Body.1",visible:true,geometryKey:"ga"},{id:"b",name:"Body.2",visible:false,geometryKey:"gb"}];view.part.activeBodyId="a";
view.artifacts=Object.fromEntries(["ga","gb"].map(key=>[key,{representations:Object.fromEntries(["BREP","VISUAL","NAMING"].map(role=>[role,{objectId:`${key}-${role}`,digest:"d".repeat(64),schemaVersion:2,size:123}]))}]));
const files=renderToStaticMarkup(React.createElement(PartBodies,{view}));
for(const text of ["Body.1","Body.2","mesh.glb","naming.pb","BREP","123 B"]) assert.ok(files.includes(text));
assert.equal((files.match(/>Download</g)??[]).length,6);
assert.ok(files.includes("versionId=revision&amp;bodyId=b"));
console.log("Part Files lists both frozen Body artifact groups and scoped download links");

assert.ok(!/<button|<input|contenteditable|ant-typography-edit/.test(files), "Body properties must be read-only");

view.part.datumAxes=[{id:"axis-custom",name:"Inspection axis",origin:[3,4,5],direction:[0,1,0]}];
const axisHTML=renderToStaticMarkup(React.createElement(Properties,{view,selection:{kind:"axis",axis:"DATUM",entityId:"axis-custom",id:"root:axis-custom"},workbench:Object.keys(CAD_WORKBENCHES)[0],activeTool:"select",navigationProfile:"cad"}));
assert.ok(axisHTML.includes("Inspection axis") && axisHTML.includes("3, 4, 5") && axisHTML.includes("0, 1, 0"),"custom axis inspector displays exact origin and direction");
view.part.bodies[0]={...view.part.bodies[0],geometryKey:undefined,displayFallback:{geometryKey:"ga",sourceVersionId:"successful-revision"}};
const failedHTML=renderToStaticMarkup(React.createElement(PartBodies,{view}));
assert.ok(failedHTML.includes("上次成功结果"));
assert.equal((failedHTML.match(/>Download</g)??[]).length,3,"failed body has no authoritative artifact downloads");

const dimension={Length:1,Mass:0,Time:0,Current:0,Temperature:0,Amount:0,Luminous:0,Semantic:''};
const parameter={parameterId:'parameter-stable',key:'p111',label:'Plane offset',ownerFeatureId:'plane-stable',propertySlot:'datum:distance',lifecycle:'DATUM_REQUIRED',valueType:'QUANTITY',dimension,displayUnit:'mm',role:'DRIVING',
  source:{expression:{sourceText:'r_1 + 4',reads:['parameter:upstream-stable'],ast:{kind:'binary',operator:'+',left:{parameterId:'upstream-stable'}}}},evaluatedValue:{siValue:0.0123456789012345,dimension}};
view.part.parameters=[parameter];
const props={view,workbench:Object.keys(CAD_WORKBENCHES)[0],activeTool:'select',navigationProfile:'cad'};
const render=selection=>renderToStaticMarkup(React.createElement(Properties,{...props,selection}));
const parameterHTML=render({kind:'parameter',id:'parameter-stable',entityId:'parameter-stable',documentId:'part',versionId:'revision'});
for(const value of ['p111','datum:distance','DATUM_REQUIRED','DRIVING','QUANTITY','upstream-stable','0.0123456789012345','完整对象数据与上下文'])assert(parameterHTML.includes(value),`parameter field missing: ${value}`);
assert(!/<button|<input|contenteditable/.test(parameterHTML),'complete parameter details are read-only');
assert(parameterHTML.includes('<details class="inspector-diagnostics" open=""'),'complete details are visible initially');
view.part.datumAxes[0].definition={source:{kind:'AXIS_SYSTEM',entityId:'frame-stable',axis:'Z'},angle:25,distance:13};
assert(render({kind:'axis',axis:'DATUM',id:'axis-custom',entityId:'axis-custom'}).includes('frame-stable'),'datum transform references are retained');
view.part.features=[{id:'feature-stable',type:'LOFT',name:'Loft',sections:[{sketchId:'section-stable'}],evaluationStatus:'FAILED',diagnostic:'exact-diagnostic'}];
for(const value of ['section-stable','exact-diagnostic'])assert(render({kind:'feature',id:'feature-stable'}).includes(value),'full feature definition/diagnostic missing');

const {inspectorObjectData}=load(fileURLToPath(new URL('../inspector-object-data.ts',import.meta.url)));
const foreign={kind:'parameter',id:'parameter-stable',documentId:'other-part',versionId:'other-revision',occurrencePath:'instance/b'};
assert.equal(inspectorObjectData(view,foreign).object,undefined,'foreign selection must not fall back to a same-id root parameter');
assert.equal(inspectorObjectData(view,{...foreign,documentId:'part',versionId:'old-revision'}).object,undefined,'historical selection must not use current definitions');
const owner={document:{...view.document,id:'other-part',versionId:'other-revision'},part:{...view.part,parameters:[{...parameter,key:'other_alias'}]}};
assert.equal(inspectorObjectData(view,foreign,undefined,undefined,undefined,owner).object.key,'other_alias');
assert.equal(inspectorObjectData(view,{...foreign,versionId:'pinned-revision'},undefined,undefined,undefined,owner).object,undefined,'latest owner data must not impersonate a pinned revision');
const unrelated=inspectorObjectData(view,{kind:'feature',id:'unknown'});
assert.equal(unrelated.parameters,undefined,'unknown object must not show all unrelated document parameters');
console.log('Complete parameter/feature/datum data, read-only rendering and selected Revision/owner scope passed');

const face={kind:'face',id:'face',topologyId:7,geometryKey:'selected-key',documentId:'part',versionId:'revision'};
const response={kind:'FACE',localId:7,geometryKey:'selected-key',properties:{exactPrecision:0.0123456789012345},persistentSelection:{anchor:{featureId:'feature-stable',outputSlot:'root-face'}}};
assert.equal(inspectorObjectData(view,face,undefined,response).object,response);
for(const stale of [{...response,geometryKey:'other-key'},{...response,localId:8},{...response,kind:'EDGE'}])assert.equal(inspectorObjectData(view,face,undefined,stale).object,undefined,'topology details must match the selected geometry snapshot');
const errorHTML=renderToStaticMarkup(React.createElement(Properties,{...props,selection:face,readErrors:{topology:'specific-topology-error'}}));
assert(errorHTML.includes('specific-topology-error')&&errorHTML.includes('selected-key'),'failed read retains selection context and diagnostic');
