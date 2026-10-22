import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(import.meta.url);
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {debugSelectionSource,freezeDebugQuery,nextDebugQuery}=await server.ssrLoadModule("/src/features/assistant/automation-debug-model.ts");
 const view={document:{id:"part",versionId:"head-new"},part:{activeBodyId:"body"}};
 const selections=[{kind:"edge",id:"display-only",documentId:"part",versionId:"old-exact",bodyId:"body",topologyId:9,geometryKey:"old-key"}];
 const source=debugSelectionSource(view,selections);
 assert.equal(source.revisionId,"old-exact","historical input must not inherit current Head");
 selections[0].versionId="moved-selection";
 assert.equal(source.selections[0].versionId,"old-exact","captured selection mutated after inspection");
 const draft={scope:{documentId:source.documentId,revisionId:source.revisionId,workingBodyId:source.workingBodyId,selections:[],selectionCount:0,complete:true},tool:"cad_query_topology",arguments:{kind:"EDGE",limit:1},selectedOnly:false};
 const executed=freezeDebugQuery(draft);
 draft.scope.revisionId="edited-draft";draft.arguments.kind="FACE";
 const next=nextDebugQuery(executed,"snapshot-bound-cursor");
 assert.equal(next.scope.revisionId,"old-exact");assert.equal(next.arguments.kind,"EDGE");assert.equal(next.arguments.cursor,"snapshot-bound-cursor");
 assert.equal(executed.arguments.cursor,undefined,"pagination must not mutate the prior trace entry");
 assert.throws(()=>freezeDebugQuery({...executed,scope:{...executed.scope,revisionId:""}}),/Revision/);
 assert.throws(()=>debugSelectionSource(view,[{kind:"edge",id:"a",documentId:"foreign",bodyId:"body",topologyId:1}]),/Revision/);
 assert.throws(()=>debugSelectionSource(view,[{...source.selections[0],bodyId:"body"},{...source.selections[0],bodyId:"other-body"}]),/Body/);
 assert.throws(()=>debugSelectionSource(view,[{...source.selections[0],contextVariantKey:"variant"}]),/变体/);
 assert.throws(()=>debugSelectionSource(view,[source.selections[0],{...source.selections[0],versionId:"newer"}]),/Revision/);
 assert.equal(debugSelectionSource(view,[{kind:"feature",id:"exact-feature",documentId:"part",versionId:"old-exact"}]).workingBodyId,undefined,"a selected object without Body evidence must not inherit an unrelated active Body");
 console.log("Debug input and pagination remain bound to the captured document/Revision/Body despite live selection, Head or draft changes");
}finally{await server.close()}
