import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {matchBooleanRegion}=await server.ssrLoadModule('/src/features/workbench/boolean-region-model.ts');
 const {featureSelectionHit}=await server.ssrLoadModule('/src/cad/interaction/feature-selection.ts');
 const ref={boundaryUnique:true,boundary:['source-face'],inputDigest:'frozen',cellDigest:'left'};
 const cell=(reference,eligible=true)=>({reference,eligible});
 assert.equal(matchBooleanRegion(ref,[cell({...ref,inputDigest:'resized',cellDigest:'new-brep'})]).status,'RESOLVED');
 assert.equal(matchBooleanRegion(ref,[cell({...ref,cellDigest:'split-a',inputDigest:'split'}),cell({...ref,cellDigest:'split-b',inputDigest:'split'})]).status,'AMBIGUOUS');
 assert.equal(matchBooleanRegion(ref,[cell({...ref,boundary:['other']})]).status,'RESOLVED','exact frozen identity can prove an unchanged cell');
 assert.equal(matchBooleanRegion(ref,[cell({...ref,boundary:['other'],inputDigest:'changed'})]).status,'MISSING');
 const ambiguous={...ref,boundaryUnique:false};
 assert.equal(matchBooleanRegion(ambiguous,[cell(ambiguous),cell({...ambiguous,cellDigest:'right'})]).cell.reference.cellDigest,'left');
 assert.equal(matchBooleanRegion(ambiguous,[cell({...ambiguous,inputDigest:'changed',cellDigest:'right'})]).status,'AMBIGUOUS','removal of the original twin must never bind the surviving twin');
 assert.equal(matchBooleanRegion(ref,[cell({...ref,inputDigest:'split',boundaryUnique:false}),cell({...ref,inputDigest:'split',cellDigest:'other',boundaryUnique:false},false)]).status,'AMBIGUOUS','operation filtering must not hide a second lineage match');
 assert.equal(matchBooleanRegion(ref,[cell(ref,false)]).status,'MISSING','operation eligibility remains authoritative');
 const hit={kind:'boolean-region',id:'temporary',regionId:'left',analysisId:'frozen',documentId:'part',versionId:'head',occurrencePath:'a'};
 const session={role:'boolean-region',documentId:'part',versionId:'head',occurrencePath:'a',selections:[],booleanRegions:{cells:[cell(ref)]}};
 assert.equal(featureSelectionHit(hit,session),hit);
 for(const patch of [{regionId:'unknown'},{analysisId:'old'},{versionId:'old'},{occurrencePath:'b'},{documentId:'other'}])assert.equal(featureSelectionHit({...hit,...patch},session),null);
 assert.equal(featureSelectionHit({...hit,kind:'face',topologyId:1},session),null,'final Body faces cannot be mistaken for cells');
}finally{await server.close()}
