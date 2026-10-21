import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {freezeScope,maxFrozenSelections}=await server.ssrLoadModule("/src/features/assistant/frozen-scope.ts");
 let release,started;const waiting=new Promise(resolve=>release=resolve),entered=new Promise(resolve=>started=resolve);
 const persistent={schemaVersion:2,sourceDocumentId:"part",sourceBodyId:"body",anchor:{featureId:"feature",outputSlot:"edge:line"},expectedType:"EDGE",selector:{kind:"SELF"},creationEvidence:{}};
 const edge={kind:"edge",id:"display:mesh-index",topologyId:17,geometryKey:"geometry",documentId:"part",versionId:"historical",bodyId:"body"};
 const source={rootDocumentId:"product",documentId:"part",revisionId:"current",workingBodyId:"body",selections:[edge]};
 let calls=[];
 const pending=freezeScope(source,async(...args)=>{calls.push(args);started();await waiting;return {persistentSelection:persistent}});
 await entered;edge.topologyId=99;edge.versionId="new-head";source.selections.length=0;release();
 const result=await pending;assert.equal(result.selectionCount,1);assert.equal(result.complete,true);assert.deepEqual(calls,[["part","geometry","EDGE",17,"historical"]]);
 assert.equal(result.selections[0].sourceVersionId,"historical","binding must use the actual observed revision, not current Head");assert.deepEqual(result.selections[0].selection,persistent);
 assert.equal(JSON.stringify(result).includes("mesh-index"),false);assert.equal("topologyId" in result.selections[0],false,"local topology numbers never become persistent message identity");
 persistent.selector.kind="CHANGED";assert.equal(result.selections[0].selection.selector.kind,"SELF","the sent formal reference is immutable after lookup");
 const occurrence=(id,revision)=>({rootDocumentId:"product",segments:[{instanceId:id,referencedDocumentId:"part",resolvedVersionId:revision}],canonical:id});
 const two=await freezeScope({...source,documentId:"product",revisionId:"root-revision",selections:[{kind:"instance",id:"same-name",entityId:"part",documentId:"part",instancePath:occurrence("first","pinned-revision")},{kind:"instance",id:"same-name",entityId:"part",documentId:"part",instancePath:occurrence("second","accepted-follow-revision")}]},async()=>{throw Error("unexpected topology lookup")});
 assert.deepEqual(two.selections.map(item=>item.instancePath),[["first"],["second"]]);assert.deepEqual(two.selections.map(item=>item.id),["first","second"]);
 assert.deepEqual(two.selections.map(item=>item.sourceVersionId),["pinned-revision","accepted-follow-revision"],"each occurrence preserves its actual accepted source snapshot; Root Revision cannot substitute a Part Revision");
 await assert.rejects(()=>freezeScope({...source,selections:[{kind:"tree",id:"document/root/display/group"}]},async()=>{}),/正式对象身份/);
 const body=await freezeScope({...source,selections:[{kind:"body",id:"occurrence:display:body",bodyId:"stable-body",documentId:"part"}]},async()=>{});assert.equal(body.selections[0].id,"stable-body","display prefixes never replace a formal Body identity");
 await assert.rejects(()=>freezeScope({...source,selections:[{...edge,versionId:"historical"}]},async()=>({persistentSelection:{...persistent,sourceDocumentId:"wrong"}})),/稳定引用/);
 calls=[];await assert.rejects(()=>freezeScope({...source,selections:Array.from({length:maxFrozenSelections+1},()=>edge)},async()=>{calls.push(true)}),/尚未发送/);assert.equal(calls.length,0,"an oversized complete selection is rejected explicitly before network binding, never truncated");
 console.log("Frozen selection binding, historical revision, occurrence identity and complete-set limits passed.");
}finally{await server.close()}
