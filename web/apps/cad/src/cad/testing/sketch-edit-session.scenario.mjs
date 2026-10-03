import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const {SketchEditTool}=await server.ssrLoadModule('/src/cad/tool/sketch-edit-tool.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 const {SelectTool}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const entities=[{id:'a',kind:'LINE',role:'PROFILE',start:{x:0,y:0},end:{x:10,y:0}}];
 const selected={kind:'visual',id:'a',entityId:'a',featureId:'sketch',ownerDocumentId:'part'};
 const states=[],prompts=[],sent=[];
 let manager;
 const context={viewport:{hasActiveSketch:()=>true,currentSketchEntities:()=>entities,currentSketchConstraints:()=>[],currentSketchIdentity:()=>({documentId:'part',sketchId:'sketch',versionId:'v1'}),currentSelections:()=>[],
 selectionAt:()=>selected,sketchEntityAt:()=>selected,sketchPlacementPoint:(x,y)=>[x,y],showSketchEntityPreview:()=>{},clearToolPreview:()=>{},clearReferencePreview:()=>{},setToolPrompt:s=>prompts.push(s),setSketchCommandState:s=>states.push(s),finishToolUse:()=>manager.activate('select'),commitSketchOperations:async ops=>sent.push(ops)}};
 manager=new ToolManager(context);manager.register(new SelectTool());manager.register(new SketchEditTool('trim'));
 manager.activate('sketch.edit.trim');manager.selectionInput([selected]);
 manager.keyDown({key:'Escape',code:'Escape',editableTarget:false,state:{modifiers:{}}});
 assert.equal(manager.activeToolID,'select','one Escape must end selected uncommitted trim, not just clear its private selection');
 assert.equal(sent.length,0);
 console.log('sketch edit session PASS');
} finally {await server.close();}
