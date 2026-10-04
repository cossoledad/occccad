import assert from "node:assert/strict";
import { createServer } from "vite";
const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const {booleanInputStages, booleanDefinitionReady, solidParameterEdit, solidGeneratorParameters,pickLoftSection} = await server.ssrLoadModule("/src/features/workbench/solid-feature-model.ts");
  const pattern={id:"frames",type:"SKETCH_PATTERN",pattern:{count:6,skippedSlots:[1,4]}};
  let sections=pickLoftSection([],pattern);
  sections=pickLoftSection(sections,pattern);
  assert.deepEqual(sections,[{sketchId:"frames",memberSlot:0},{sketchId:"frames",memberSlot:2}]);
  sections=pickLoftSection(sections,pattern);
  assert.equal(sections[2].memberSlot,3);
  assert.deepEqual(pickLoftSection(sections,{...pattern,pattern:{count:2,skippedSlots:[1]}}),sections,"shrinking must not silently rebind a missing existing section");
  const pointSection={sketchId:"same",point:{sketchId:"same",reference:{target:"ENTITY",entityId:"p",subElement:"POINT"}}};
  assert.deepEqual(pickLoftSection([pointSection],{id:"same",type:"SKETCH"}),[pointSection,{sketchId:"same"}]);
  assert.deepEqual(pickLoftSection([pointSection,{sketchId:"same"}],{id:"same",type:"SKETCH"}),[pointSection],"profile toggle must retain the separately referenced point");
  const {featureSelectionHit}=await server.ssrLoadModule("/src/cad/interaction/feature-selection.ts");
  const session={role:"point",documentId:"part",versionId:"v",sketchIds:["same"]};
  const pick={kind:"visual",visualType:"POINT",id:"p",documentId:"part",versionId:"v",featureId:"same",entityId:"p",sketchReference:pointSection.point.reference};
  assert.deepEqual(featureSelectionHit(pick,session),pick,"point picking must retain the entity reference rather than project to its entire sketch");
  assert.equal(featureSelectionHit({...pick,versionId:"old"},session),null);
  assert.equal(featureSelectionHit({...pick,visualType:"CURVE"},session),null);
  const features = [
    {id:"base",bodyId:"target",type:"LINEAR_EXTRUDE"},
    {id:"loft",bodyId:"tool",type:"LOFT"},
    {id:"fillet",bodyId:"tool",type:"FILLET"},
    {id:"cut",bodyId:"target",type:"BOOLEAN"},
    {id:"future",bodyId:"future-body",type:"CHAMFER"},
  ];
  const stages=booleanInputStages(features,"cut");
  assert.deepEqual(stages.map(f=>f.id),["base","loft","fillet"]);
  const tools=[{bodyId:"tool",featureId:"fillet"}];
  assert.equal(booleanDefinitionReady("target","REMOVE",tools,stages),true);
  assert.equal(booleanDefinitionReady("tool","REMOVE",tools,stages),false);
  assert.equal(booleanDefinitionReady("target","REMOVE",[...tools,{bodyId:"tool",featureId:"loft"}],stages),false);
  assert.equal(booleanDefinitionReady("target","REMOVE",[{bodyId:"future-body",featureId:"future"}],stages),false);
  assert.equal(booleanDefinitionReady("target","INTERSECT",tools,stages),true);
  assert.deepEqual(booleanInputStages(features,"missing"),[]);
  assert.deepEqual(solidParameterEdit("length","12.3456789","12.3456789","mm"),{},"untouched UI must not rewrite exact Quantity");
  assert.deepEqual(solidParameterEdit("length","Width / 2","Width / 2","mm"),{},"untouched expression remains a source");
  assert.equal(solidParameterEdit("length2","2 cm","1","mm").value,20);
  assert.equal(solidParameterEdit("length","Width / 3","1","mm").expression,"Width / 3");
  assert.equal(solidParameterEdit("angle",`${Math.PI} rad`,"90","mm").value,180);
  assert.equal(solidParameterEdit("angle","Alpha / 2","90","mm").expression,"Alpha / 2");
  assert.deepEqual(solidGeneratorParameters({generator:"REVOLVE",angle:"Alpha / 2"},"mm"),{angle:360,parameterExpressions:{angle:"Alpha / 2"}});
  assert.deepEqual(solidGeneratorParameters({generator:"LINEAR_EXTRUDE",extent:"TWO_SIDED",angle:360,length2:"2 cm"},"mm"),{length2:20,parameterExpressions:{}});
  assert.throws(()=>solidParameterEdit("angle","2 cm","90","mm"));
  assert.throws(()=>solidParameterEdit("length","-1","2","mm"));
} finally { await server.close(); }
