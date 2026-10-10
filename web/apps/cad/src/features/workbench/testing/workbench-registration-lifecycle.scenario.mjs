import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {readFileSync,existsSync,statSync} from 'node:fs';
import {dirname,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
const require=createRequire(import.meta.url),React=require('react'),ts=require('typescript');
const modules=new Map();
function load(file){
 if(file.endsWith('.json'))return JSON.parse(readFileSync(file,'utf8'));
 if(modules.has(file))return modules.get(file).exports;
 const module={exports:{}};modules.set(file,module);
 const js=ts.transpileModule(readFileSync(file,'utf8').replaceAll('import.meta.env','({})'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022,jsx:ts.JsxEmit.ReactJSX,esModuleInterop:true},fileName:file}).outputText;
 const localRequire=createRequire(file);
 new Function('require','module','exports',js)(name=>{
  if(!name.startsWith('.'))return localRequire(name);
  const path=resolve(dirname(file),name),target=[path,`${path}.ts`,`${path}.tsx`,`${path}/index.ts`].find(path=>existsSync(path)&&statSync(path).isFile());
  assert(target,`cannot resolve ${name}`);return load(target);
 },module,module.exports);return module.exports;
}
const base=dirname(fileURLToPath(import.meta.url));
const {CommandRegistry,CommandOperation}=load(resolve(base,'../../../cad/command/command-registry.ts'));
const {builtinCatalog}=load(resolve(base,'../../../cad/command/workbench-catalog.ts'));
const {useWorkbenchCommandRegistration}=load(resolve(base,'../workbench-command-registration.ts'));
const {structureCommands,resolveStructureNode}=load(resolve(base,'../commands/structure-commands.ts'));
const saved={useMemo:React.useMemo,useRef:React.useRef,useLayoutEffect:React.useLayoutEffect,useState:React.useState,useEffect:React.useEffect,useSyncExternalStore:React.useSyncExternalStore};
const refs=[],effects=[];let refCursor=0,effectCursor=0,queue=[];
React.useRef=value=>{const index=refCursor++;return refs[index]??(refs[index]={current:value});};
React.useLayoutEffect=(fn,deps)=>{const index=effectCursor++,previous=effects[index];if(!deps||!previous||deps.some((value,index)=>!Object.is(value,previous.deps?.[index])))queue.push(()=>{previous?.dispose?.();effects[index]={deps,dispose:fn()};});};
try {
 let closed=0,opened=0,fitted=0,registrations=0;
 const registry=new CommandRegistry(),register=registry.registerMany.bind(registry);
 registry.registerMany=commands=>{registrations++;return register(commands);};
 const noop=()=>{},store={activeToolID:'select',selections:[],setActiveTool(id){this.activeToolID=id;}};
 const part={document:{id:'part',type:'PART',permission:'OWNER'},part:{features:[],axisSystems:[]}};
 const bindings={store,editingView:part,view:part,canEdit:true,canEditRoot:true,command:{isPending:false},treeNodes:[],viewport:{current:{fit(){fitted++;}}},closePanels(){closed++;},
 startSketch:noop,finishSketch:noop,normalToSelection:noop,openSolidFeature(){opened++;},deleteTreeNodes:noop,executeHistory:noop,setConflictOpen:noop,setParameterManagerOpen:noop,setPublicationManagerOpen:noop,setInsertOpen:noop,setPatternOpen:noop,setReleaseOpen:noop,setVersionOpen:noop,setShareResource:noop,setSolidEditor:noop,setBooleanDialog:noop,setDatumEditor:noop,setEditingDatumId:noop,showMessage:noop};
 let facts={hostType:'PART',targetType:'PART',sketchActive:false,canEdit:true,rootCanEdit:true,busy:false,selectionKind:'',selectionCount:0,hasWorkingBody:true};
 function render(identity){refCursor=effectCursor=0;queue=[];useWorkbenchCommandRegistration(registry,bindings,builtinCatalog,facts,identity);for(const effect of queue)effect();}
 render('A');await registry.execute('part.pad');assert.equal(opened,1);assert.equal(closed,1);
 facts={...facts,selectionCount:1};render('A');assert.equal(closed,1,'selection refresh preserves current command');assert.equal(registrations,1);
 render('B');assert.equal(closed,2,'changed target cancels and cleans previous form');
 await registry.execute('view.fit');bindings.viewport.current={fit(){fitted+=10;}};render('B');await registry.execute('view.fit');assert.equal(fitted,11,'one registration reads current bindings');
 await registry.execute('part.pad');bindings.formActive=true;render('B');assert.equal(registry.state('part.pad').active,true);
 const beforeClose=closed;bindings.formActive=false;render('B');assert.equal(closed,beforeClose+1);assert.equal(registry.state('part.pad').active,false,'form close ends the operation');
 facts={...facts,sketchActive:true};store.sketchPlane={};render('B/sketch');await registry.execute('sketch.constraint.coincident');
 const beforeFinish=closed;store.activeToolID='select';bindings.toolContinuation=true;render('B/sketch');assert.equal(closed,beforeFinish,'tool-to-form continuation keeps its owned resources');
 bindings.toolContinuation=false;render('B/sketch');assert.equal(closed,beforeFinish+1,'finished tool releases resources once');render('B/sketch');assert.equal(closed,beforeFinish+1);
 const node={key:'stable',documentId:'part',entityId:'feature',capabilities:['EDIT'],selection:{versionId:'revision-A'},instancePath:{canonical:'host/occurrence-A'}};
 assert.equal(resolveStructureNode([node],node),node);
 assert.throws(()=>resolveStructureNode([node],{...node,instancePath:{canonical:'host/occurrence-B'}}),/context changed/);
 assert.throws(()=>resolveStructureNode([node],{...node,selection:{versionId:'revision-B'}}),/context changed/);
 let edits=0,editOwner;const commands=structureCommands({onEdit(_node,operation){edits++;editOwner=operation;}},[node]);
 const editOperation=new CommandOperation();commands.find(command=>command.id==='tree.edit').execute({payload:{node},operation:editOperation});assert.equal(edits,1);assert.equal(editOwner,editOperation,'tree form owns the same operation as toolbar forms');
 assert.throws(()=>structureCommands({onEdit:noop},[{...node,capabilities:[]}]).find(command=>command.id==='tree.edit').execute({payload:{node}}),/unavailable/);
 const cleanup=new CommandOperation();let released=0;cleanup.own(()=>released++);cleanup.own(()=>{throw new Error('resource failure');});assert.throws(()=>cleanup.cancel(),AggregateError);assert.equal(released,1);cleanup.cancel();assert.equal(released,1);
 // A root-owned study must not open against the host while a nested Product is active.
 const {workbenchCommands}=load(resolve(base,'../workbench-command-registration.ts'));
 const product={document:{id:'root',type:'PRODUCT'},product:{instances:[]}};
 let motionOpened=0;
 const motionBindings={...bindings,view:product,editingView:product,setMotionOpen:()=>motionOpened++};
 const motionCommand=()=>workbenchCommands(motionBindings).find(c=>c.id==='assembly.motion-study');
 assert.equal(motionCommand().isEnabled(),true);motionCommand().execute();assert.equal(motionOpened,1);
 motionBindings.editingView={document:{id:'nested',type:'PRODUCT'},product:{instances:[]}};
 assert.equal(motionCommand().isEnabled(),false);
 motionBindings.editingView=product;motionBindings.canEditRoot=false;
 assert.equal(motionCommand().isEnabled(),true,'viewer can run read-only frozen research');
 // Run the actual Tab callback with controlled React state, without a browser.
 const {WorkbenchCommands}=load(resolve(base,'../workbench-commands.tsx'));
 const commandContext=load(resolve(base,'../../../cad/command/command-context.tsx'));
 const helpContext=load(resolve(base,'../../../cad/help/ui-help-context.tsx'));
 const feedback=load(resolve(base,'../../../cad/command/operation-feedback.tsx'));
 commandContext.useCommandRegistry=()=>registry;helpContext.useUIHelp=()=>({active:false});feedback.useOperationFeedback=()=>noop;
 const {projectToolbars}=load(resolve(base,'../../../cad/command/workbench-catalog.ts'));
 const toolbars=projectToolbars(builtinCatalog).filter(bar=>bar.workbench==='PART_DESIGN');
 let configuredTabs=builtinCatalog.tabs.filter(tab=>tab.workbench==='PART_DESIGN');
 const state=[];let stateCursor=0;
 React.useState=value=>{const index=stateCursor++;if(!(index in state))state[index]=value;return [state[index],value=>state[index]=typeof value==='function'?value(state[index]):value];};
 React.useMemo=fn=>fn();React.useEffect=React.useLayoutEffect=()=>{};React.useRef=()=>({current:null});React.useSyncExternalStore=(_subscribe,get)=>get();
 const findTabs=tree=>{if(!tree)return;if(Array.isArray(tree)){for(const child of tree){const found=findTabs(child);if(found)return found;}}else if(tree.type===require('antd').Tabs)return tree;else return findTabs(tree.props?.children);};
 const {DocumentSessions}=load(resolve(base,'../../../cad/document/document-session.ts'));
 const documentSessions=new DocumentSessions();documentSessions.open('A');documentSessions.open('B');let hostDocumentId='A';
 const deck=()=>{stateCursor=0;return findTabs(WorkbenchCommands({documentSessions,hostDocumentId,toolbars,workbench:'PART_DESIGN',catalog:builtinCatalog,tabs:configuredTabs}));};
 const beforeTab={closed,opened,fitted};deck().props.onChange('PART_DESIGN.view');assert.equal(deck().props.activeKey,'PART_DESIGN.view');
 configuredTabs=[...configuredTabs];assert.equal(deck().props.activeKey,'PART_DESIGN.view','configuration refresh preserves stable Tab selection');
 hostDocumentId='B';assert.equal(deck().props.activeKey,'PART_DESIGN.model');deck().props.onChange('PART_DESIGN.document');
 hostDocumentId='A';assert.equal(deck().props.activeKey,'PART_DESIGN.view','returning to document restores its own toolbar Tab');
 documentSessions.close('A');documentSessions.open('A');assert.equal(deck().props.activeKey,'PART_DESIGN.model','closing document clears presentation state');
 configuredTabs=configuredTabs.filter(tab=>tab.id==='PART_DESIGN.document');assert.deepEqual(deck().props.items.map(item=>item.key),['PART_DESIGN.document'],'only configured context Tabs are presented');
 assert.deepEqual({closed,opened,fitted},beforeTab,'Tab callback changes presentation without executing a command');
 assert.equal(store.activeToolID,'select');assert.equal(registrations,1);
 // A new SVG ID is rendered directly from configuration; missing IDs share the placeholder.
 Object.assign(React,saved);
 const {CadIcon,CadIconContext}=load(resolve(base,'../../../cad/overlay/cad-icons.tsx'));
 const {renderToStaticMarkup}=require('react-dom/server');
 const icon=name=>renderToStaticMarkup(React.createElement(CadIconContext.Provider,{value:{'extension.glyph':{elements:[{tag:'circle',attributes:{cx:'4',cy:'5',r:'3'}}]}}},React.createElement(CadIcon,{name})));
 assert.match(icon('extension.glyph'),/<circle cx="4" cy="5" r="3"/);assert.match(icon('missing'),/<rect/);
 for(const effect of effects.slice().reverse())effect.dispose?.();assert.equal(registry.has('part.pad'),false);
 console.log('Registration stability, target cancellation, continuation, cleanup and occurrence identity passed.');
}finally{Object.assign(React,saved);}
