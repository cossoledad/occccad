import assert from "node:assert/strict";
import {createRequire} from "node:module";
import {readFile} from "node:fs/promises";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
  const {offsetCommandFields,offsetInitialFields}=await server.ssrLoadModule("/src/cad/assembly/assembly-offset.ts");
  const {displayLengthToMillimeters,millimetersToDisplayLength}=await server.ssrLoadModule("/src/state/ui-preferences.ts");
  const first={instanceId:"same-part-occurrence-a",kind:"PLANE"};
  const second={instanceId:"same-part-occurrence-b",kind:"PLANE"};
  const definition={first,second,kind:"DISTANCE",value:-2,directionRelation:"UNORIENTED",distanceRelation:"SELECTED_PLANE_NORMAL_V1",mode:"MEASURED",
    offsetParameter:{parameterId:"offset:gap",key:"Gap",source:{expression:{sourceText:"Base - 5 mm"}}}};
  const reopened=offsetInitialFields(definition);
  assert.equal(reopened.distanceRelation,"SELECTED_PLANE_NORMAL_V1");
  assert.equal(reopened.offsetExpression,"Base - 5 mm");
  assert.equal(reopened.constraintMode,"MEASURED");
  assert.deepEqual(offsetCommandFields(reopened),{offsetExpression:"Base - 5 mm",offsetKey:"Gap",constraintMode:"MEASURED"});
  assert.deepEqual(definition.first,first);assert.deepEqual(definition.second,second);
  assert.equal(definition.value,-2);assert.equal(definition.directionRelation,"UNORIENTED");
  assert.equal(offsetInitialFields(undefined,[first,second]).distanceRelation,"SELECTED_PLANE_NORMAL_V1");
  assert.equal(offsetInitialFields(undefined,[{kind:"POINT"},{kind:"AXIS"}]).distanceRelation,"UNSIGNED");
  assert.equal(offsetInitialFields({...definition,distanceRelation:"ALONG_SECOND_NORMAL"}).distanceRelation,"ALONG_SECOND_NORMAL","old Revision cannot be silently reinterpreted");
  for(const unit of ["mm","cm","m","in"]) for(const mm of [-2,0,5]) {
    assert.ok(Math.abs(displayLengthToMillimeters(millimetersToDisplayLength(mm,unit),unit)-mm)<1e-12);
  }
  const source=await readFile(new URL("../../../features/workbench/workbench.tsx",import.meta.url),"utf8");
  assert.ok((source.match(/offsetCommandFields\(/g)||[]).length===3,"Preview/create/edit must consume the same intent adapter");
  assert.ok(source.includes('constraint.directionRelation ?? "UNORIENTED"'),"opening editor must not resolve Undefined from camera/placement");
} finally {await server.close();}
