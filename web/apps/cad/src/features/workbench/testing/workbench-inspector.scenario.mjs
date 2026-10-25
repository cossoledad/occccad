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
  if(file.endsWith(".json"))return JSON.parse(readFileSync(file,"utf8"));
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
const {builtinCatalog}=load(fileURLToPath(new URL("../../../cad/command/workbench-catalog.ts",import.meta.url)));
const view = { document: { id: "part", type: "PART", name: "New Part", versionId: "revision", permission: "OWNER" }, part: { bodies:[], activeBodyId:"", features: [] } };
for (const primitives of [null, undefined, []]) {
  const diagnostics = {
    artifacts: [{ visualization: { schemaVersion: 1, referenceGeometry: { datumPlanes: [], axisSystems: [] }, primitives } }],
    aggregate: { artifactCount: 1, solidCount: 0, vertexCount: 0, glbBytes: 0, brepBytes: 0 }, worker: { available: false },
  };
  const html = renderToStaticMarkup(React.createElement(Properties, {
    view, selection: null, diagnostics, workbench: builtinCatalog.tabs[0].workbench, activeTool: "select", navigationProfile: "cad",
  }));
  assert.ok(html.includes("New Part"));
  assert.doesNotMatch(html,/Non-solid Geometry|GLB|完整对象数据/);
}
console.log("Properties renders lightweight Artifact descriptors with null, omitted and empty primitives");

const { PartBodies }=load(fileURLToPath(new URL("../workbench-inspector.tsx",import.meta.url)));
view.part.bodies=[{id:"a",name:"Body.1",visible:true,geometryKey:"ga"},{id:"b",name:"Body.2",visible:false,geometryKey:"gb"}];view.part.activeBodyId="a";
view.artifacts=Object.fromEntries(["ga","gb"].map(key=>[key,{representations:Object.fromEntries(["BREP","VISUAL","NAMING"].map(role=>[role,{objectId:`${key}-${role}`,digest:"d".repeat(64),schemaVersion:2,size:123}]))}]));
const files=renderToStaticMarkup(React.createElement(PartBodies,{view}));
for(const text of ["Body.1","Body.2","活动实体","隐藏"])assert.ok(files.includes(text));
assert.doesNotMatch(files,/mesh.glb|naming.pb|versionId|Download/);
console.log("Body properties show current engineering visibility and output state");

assert.ok(!/<button|<input|contenteditable|ant-typography-edit/.test(files), "Body properties must be read-only");

view.part.datumAxes=[{id:"axis-custom",name:"Inspection axis",origin:[3,4,5],direction:[0,1,0]}];
const axisHTML=renderToStaticMarkup(React.createElement(Properties,{view,selection:{kind:"axis",axis:"DATUM",entityId:"axis-custom",id:"root:axis-custom"},workbench:builtinCatalog.tabs[0].workbench,activeTool:"select",navigationProfile:"cad"}));
assert.ok(axisHTML.includes("Inspection axis") && axisHTML.includes("3, 4, 5") && axisHTML.includes("0, 1, 0"),"custom axis inspector displays exact origin and direction");
view.part.bodies[0]={...view.part.bodies[0],geometryKey:undefined,displayFallback:{geometryKey:"ga",sourceVersionId:"successful-revision"}};
const failedHTML=renderToStaticMarkup(React.createElement(PartBodies,{view}));
assert.ok(failedHTML.includes("上次成功结果"));
assert.equal((failedHTML.match(/>Download</g)??[]).length,0,"internal artifacts are not engineering properties");

const dimension={Length:1,Mass:0,Time:0,Current:0,Temperature:0,Amount:0,Luminous:0,Semantic:''};
const parameter={parameterId:'parameter-stable',key:'p111',label:'Plane offset',ownerFeatureId:'plane-stable',propertySlot:'datum:distance',lifecycle:'DATUM_REQUIRED',valueType:'QUANTITY',dimension,displayUnit:'mm',role:'DRIVING',
  source:{expression:{sourceText:'r_1 + 4',reads:['parameter:upstream-stable'],ast:{kind:'binary',operator:'+',left:{parameterId:'upstream-stable'}}}},evaluatedValue:{siValue:0.0123456789012345,dimension}};
view.part.parameters=[parameter];
const props={view,workbench:builtinCatalog.tabs[0].workbench,activeTool:'select',navigationProfile:'cad'};
const render=selection=>renderToStaticMarkup(React.createElement(Properties,{...props,selection}));
const parameterHTML=render({kind:'parameter',id:'parameter-stable',entityId:'parameter-stable',documentId:'part',versionId:'revision'});
for(const value of ['Plane offset','r_1 + 4','12.35'])assert(parameterHTML.includes(value),`engineering property missing: ${value}`);
assert.doesNotMatch(parameterHTML,/parameter-stable|datum:distance|DATUM_REQUIRED|upstream-stable|0.0123456789012345|完整对象数据/);
assert(!/<button|<input|contenteditable/.test(parameterHTML),'parameter summary is read-only');
view.part.datumAxes[0].definition={source:{kind:'AXIS_SYSTEM',entityId:'frame-stable',axis:'Z'},angle:25,distance:13};
assert(!render({kind:'axis',axis:'DATUM',id:'axis-custom',entityId:'axis-custom'}).includes('frame-stable'));
view.part.features=[{id:'feature-stable',type:'LOFT',name:'Loft',sections:[{sketchId:'section-stable'}],evaluationStatus:'FAILED',diagnostic:'exact-diagnostic'}];
const featureHTML=render({kind:'feature',id:'feature-stable'});assert.match(featureHTML,/Loft|计算失败/);assert.doesNotMatch(featureHTML,/section-stable|exact-diagnostic/);

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

const face={kind:'face',id:'face',topologyId:7,geometryKey:'selected-key',documentId:'part',versionId:'revision',associatedFeatureIds:['feature-stable']};
const response={kind:'FACE',localId:7,geometryKey:'selected-key',properties:{exactPrecision:0.0123456789012345},persistentSelection:{anchor:{featureId:'feature-stable',outputSlot:'root-face'}}};
assert.equal(inspectorObjectData(view,face,undefined,response).object,response);
for(const stale of [{...response,geometryKey:'other-key'},{...response,localId:8},{...response,kind:'EDGE'}])assert.equal(inspectorObjectData(view,face,undefined,stale).object,undefined,'topology details must match the selected geometry snapshot');
const errorHTML=renderToStaticMarkup(React.createElement(Properties,{...props,selection:face,readErrors:{topology:'specific-topology-error'}}));
assert(!errorHTML.includes('specific-topology-error')&&!errorHTML.includes('selected-key'),'failed read has a user message; raw context remains in developer data');
assert.equal(inspectorObjectData(view,{kind:'parameter',id:'parameter-stable',entityId:'parameter-stable',documentId:'part',versionId:'revision'}).object.evaluatedValue.siValue,0.0123456789012345,'engineering display never changes exact values');

const surface={...response,geometryType:'CONE',properties:{area:12.3456789,referenceRadius:4.56789,semiAngle:Math.PI/6,axis:[0,0,1],exactPrecision:0.0123456789012345}};
const surfaceHTML=renderToStaticMarkup(React.createElement(Properties,{...props,selection:face,topology:surface}));
for(const text of ['面积 (mm²)','12.35','参考半径 (mm)','4.57','半锥角 (°)','30','Loft'])assert(surfaceHTML.includes(text),`geometric property missing: ${text}`);
assert.equal(surface.properties.semiAngle,Math.PI/6,'display units never change precise geometry');
const splitHTML=renderToStaticMarkup(React.createElement(Properties,{...props,selection:face,topology:{...surface,persistentSelection:{anchor:{featureId:'later-shell',outputSlot:'SPLIT_FROM/original'}}}}));
assert(splitHTML.includes('Loft')&&!splitHTML.includes('later-shell'),'a split anchor is not the feature owning the current boundary');
const unknownOwnerHTML=renderToStaticMarkup(React.createElement(Properties,{...props,selection:{...face,associatedFeatureIds:undefined},topology:surface}));
assert(unknownOwnerHTML.includes('生成步骤暂不可用'),'missing contribution evidence must not be replaced by the semantic anchor');

view.part.features.push({id:'fillet-input',type:'FILLET',name:'外边圆角',bodyId:'a',length:3.14159265});
const filletHTML=render({kind:'feature',id:'fillet-input'});
assert(filletHTML.includes('圆角半径')&&filletHTML.includes('3.14 mm')&&filletHTML.includes('Body.1'),'feature properties retain engineering dimensions and ownership');
assert.equal(view.part.features.at(-1).length,3.14159265);
