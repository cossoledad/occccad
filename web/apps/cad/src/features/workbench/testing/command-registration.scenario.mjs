import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {CommandRegistry}=await server.ssrLoadModule('/src/cad/command/command-registry.ts');
 const {builtinCatalog,contextTabs,projectToolbars,validateCatalog}=await server.ssrLoadModule('/src/cad/command/workbench-catalog.ts');
 const {workbenchCommands}=await server.ssrLoadModule('/src/features/workbench/workbench-command-registration.ts');
 const {selectionSummaryCommand}=await server.ssrLoadModule('/src/features/workbench/commands/selection-summary.ts');
 const catalog=structuredClone(builtinCatalog),registry=new CommandRegistry();
 let fits=0,pads=0,messages=[],active='select',cleared=0;
 const store={activeToolID:active,selections:[],setActiveTool(id){this.activeToolID=id;active=id;},selection:null};
 const part={document:{id:'target-part',type:'PART',permission:'OWNER'},part:{bodies:[{id:'body-A'}],features:[],axisSystems:[]}};
 const host={document:{id:'host-product',type:'PRODUCT',permission:'OWNER'},product:{instances:[]}};
 const noop=()=>{};
 const bindings={store,editingView:part,view:host,canEdit:true,canEditRoot:true,command:{isPending:false},treeNodes:[],viewport:{current:{fit(){fits++;},clearCommandPreview:noop}},workingBodyID:'body-A',
  startSketch:noop,finishSketch:noop,normalToSelection:noop,openSolidFeature(){pads++;assert.equal(bindings.editingView.document.id,'target-part');},deleteTreeNodes:noop,executeHistory:noop,
  setConflictOpen:noop,setParameterManagerOpen:noop,setPublicationManagerOpen:noop,setInsertOpen:noop,setPatternOpen:noop,setReleaseOpen:noop,setVersionOpen:noop,setShareResource:noop,setSolidEditor:noop,setBooleanDialog:noop,setDatumEditor:noop,setEditingDatumId:noop,showMessage:text=>messages.push(text)};
 const commands=workbenchCommands(bindings);
 registry.registerMany(commands);
 let facts={hostType:'PRODUCT',targetType:'PART',sketchActive:false,canEdit:true,rootCanEdit:true,busy:false,selectionKind:'',selectionCount:0,hasWorkingBody:true};
 validateCatalog(catalog,id=>registry.has(id));registry.configure(catalog,()=>facts);
 assert.equal(await registry.execute('view.fit'),true);assert.equal(fits,1);
 assert.equal(await registry.execute('part.pad'),true);assert.equal(pads,1);
 assert.equal(await registry.execute('assembly.coincident'),false,'host Product does not authorize assembly on an edited Part');
 facts={...facts,sketchActive:true};store.sketchPlane={};
 assert.equal(contextTabs(catalog,facts)[0].workbench,'SKETCHER');
 assert.equal(await registry.execute('part.pad'),false);
 assert.equal(await registry.execute('sketch.constraint.coincident'),true);assert.equal(active,'sketch.constraint.coincident');
 facts={...facts,targetType:'PRODUCT',sketchActive:false};bindings.editingView=host;
 assert.equal(contextTabs(catalog,facts)[0].workbench,'ASSEMBLY_DESIGN');
 const productRegistry=new CommandRegistry();productRegistry.registerMany(workbenchCommands(bindings));productRegistry.configure(catalog,()=>facts);
 assert.equal(await productRegistry.execute('assembly.contact'),true);assert.equal(active,'assembly.contact');
 assert.equal(await productRegistry.execute('assembly.fix_together'),true);assert.equal(active,'assembly.fix_together');
 assert.throws(()=>registry.register({id:"invalid"}),/Missing command implementation/);
 assert.throws(()=>registry.register({id:'part.pad',execute:noop}),/Duplicate/);
 assert.throws(()=>registry.registerMany([{id:'new',execute:noop},{id:'part.pad',execute:noop}]),/Duplicate/);assert.equal(registry.has('new'),false,'batch failure is atomic');
 const invalid=structuredClone(catalog);invalid.commands.push({...invalid.commands[0],id:'missing.implementation'});
 assert.throws(()=>registry.configure(invalid,()=>facts),/Missing command implementation/);
 // A new independent tool needs only declaration/placement and implementation registration.
 const extension=new CommandRegistry();const custom=structuredClone(catalog);
 custom.commands.push({...custom.commands.find(c=>c.id==='view.selection-summary'),id:'example.summary'});
 custom.groups.find(group=>group.id==='view-navigation').commands.push({commandId:'example.summary',groupKey:'primary',order:200});
 extension.registerMany(workbenchCommands(bindings));
 extension.register({...selectionSummaryCommand({getContext:()=>({documentId:'target-part',selectionCount:2}),show:text=>messages.push(text)}),id:'example.summary'});
 extension.configure(custom,()=>facts);
 assert(projectToolbars(custom).some(bar=>bar.tabIds.includes('PART_DESIGN.view')&&bar.items.some(item=>item.commandId==='example.summary')));
 assert.equal(await extension.execute('example.summary'),true);assert.match(messages.at(-1),/target-part.*2/);
 let finish;
 const deferred=new Promise(resolve=>finish=resolve),run=new CommandRegistry(),scopeCatalog=structuredClone(catalog);
 const declaration=scopeCatalog.commands.find(command=>command.id==='sketch.constraint.coincident');
 scopeCatalog.commands=[declaration];scopeCatalog.groups=[];scopeCatalog.placements=[];
 run.register({id:declaration.id,async execute(invocation){invocation.operation.own(()=>cleared++);await deferred;if(invocation.operation.current)messages.push('late');}});
 facts={...facts,targetType:'PART',sketchActive:true};run.configure(scopeCatalog,()=>facts);run.setContext('host/A');
 const pending=run.execute(declaration.id);run.setContext('host/B');finish();assert.equal(await pending,false);assert.equal(cleared,1);assert(!messages.includes('late'));run.cancel();assert.equal(cleared,1,'cancel is idempotent');
 console.log('Complete registration, Part/Product/Sketch context, extension and stale invocation checks passed.');
}finally{await server.close();}
