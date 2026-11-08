import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {solidSketchWorkflowMatches,resumeSolidSketchDraft}=await server.ssrLoadModule('/src/features/workbench/solid-sketch-workflow.ts');
 const feature={id:'pad',type:'LINEAR_EXTRUDE',bodyId:'body',profile:'sketch',profileSelection:{mode:'REGIONS',regionIds:['original']}};
 const quantities={length:'width / 2 + 3 mm',length2:'10',angle:'360',diameter:'5',depth:'10',recessDiameter:'10',recessDepth:'3',countersinkAngle:'90'};
 const draft={feature,quantities,unit:'cm',intentId:'same-unsubmitted-command'};
 const workflow={documentId:'part',rootDocumentId:'root',occurrencePath:'a',draft,sketchId:'sketch'};
 const view={document:{id:'part',versionId:'next'},part:{features:[{id:'sketch',type:'SKETCH',sketch:{}},feature]},sketchAnalyses:{sketch:{regionIds:['replacement']}}};
 assert.equal(solidSketchWorkflowMatches(workflow,view,'root','a'),true,'own sketch revisions do not end the owning occurrence');
 assert.equal(solidSketchWorkflowMatches(workflow,view,'root','b'),false);
 assert.equal(solidSketchWorkflowMatches(workflow,view,'different','a'),false);
 assert.equal(solidSketchWorkflowMatches(workflow,{...view,document:{id:'other'}},'root','a'),false);
 assert.deepEqual(resumeSolidSketchDraft(workflow,view),draft,'raw expression, unit, intent and missing original region remain explicit');
 assert.equal(resumeSolidSketchDraft(workflow,{...view,part:{features:[{...feature,type:'REVOLVE'}]}}),undefined);
 assert.equal(resumeSolidSketchDraft(workflow,{...view,part:{features:[]}}),undefined);
 const created=resumeSolidSketchDraft({...workflow,created:true},view);
 assert.deepEqual(created.feature.profileSelection,{mode:'REGIONS',regionIds:['replacement']},'only a newly created input starts with its current regions');
 assert.equal(created.quantities,quantities);assert.equal(created.intentId,draft.intentId);assert.deepEqual(feature.profileSelection.regionIds,['original']);
 assert.equal(resumeSolidSketchDraft({...workflow,created:true,sketchId:'deleted'},view),undefined);
 console.log('PASS: solid/sketch draft ownership, explicit regions and resume across revisions');
}finally{await server.close();}
