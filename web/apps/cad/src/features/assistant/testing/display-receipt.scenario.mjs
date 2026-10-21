import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {frameDocuments}=await server.ssrLoadModule("/src/viewport/display-receipt.ts");
 const target={view:{document:{id:"part",versionId:"new"}},occurrencePath:"a"};
 const occurrence=(path,revision)=>({occurrencePath:path,instancePath:{segments:[{referencedDocumentId:"part",resolvedVersionId:revision}]}});
 const root={document:{id:"product",versionId:"root"},resolvedInstances:[occurrence("a","old"),occurrence("b","new")]};
 assert.deepEqual(frameDocuments(root,target),[{documentId:"product",revisionId:"root"}],"hydrating the edited Part or showing another occurrence cannot acknowledge an unprojected source revision");
 const visible={...root,resolvedInstances:[occurrence("a","new"),occurrence("b","old")]};
 assert.deepEqual(frameDocuments(visible,target),[{documentId:"product",revisionId:"root"},{documentId:"part",revisionId:"new"}]);
 assert.deepEqual(frameDocuments(visible),[{documentId:"product",revisionId:"root"}],"leaving edit context restores an exact root-only presentation");
 const nested={...root,resolvedInstances:[{occurrencePath:"assembly/a",instancePath:{segments:[{referencedDocumentId:"subproduct",resolvedVersionId:"assembly-version"},{referencedDocumentId:"part",resolvedVersionId:"new"}]}}]};
 assert.deepEqual(frameDocuments(nested,{...target,occurrencePath:"assembly/a"}).at(-1),{documentId:"part",revisionId:"new"});
 assert.deepEqual(frameDocuments(nested,{view:{document:{id:"subproduct",versionId:"assembly-version"}},occurrencePath:"assembly"}).at(-1),{documentId:"subproduct",revisionId:"assembly-version"});
 console.log("Successful-frame receipts bind the exact root and rendered occurrence revision, including shared and nested definitions.");
}finally{await server.close()}
