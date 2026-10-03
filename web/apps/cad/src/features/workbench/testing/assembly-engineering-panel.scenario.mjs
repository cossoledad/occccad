import assert from "node:assert/strict";
import {createRequire} from "node:module";
import {readFileSync,existsSync} from "node:fs";
import {dirname,resolve} from "node:path";
import {fileURLToPath} from "node:url";
const require=createRequire(import.meta.url),React=require("react"),{renderToStaticMarkup}=require("react-dom/server"),ts=require("typescript");
// Same actual-component CommonJS loader as the neighboring inspector scenario;
// Antd's CJS distribution avoids extensionless icon ESM imports in Node SSR.
const modules=new Map();
function load(file){
 if(modules.has(file))return modules.get(file).exports;
 const module={exports:{}};modules.set(file,module);
 const js=ts.transpileModule(readFileSync(file,"utf8").replaceAll("import.meta.env","({})"),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022,jsx:ts.JsxEmit.ReactJSX,esModuleInterop:true},fileName:file}).outputText;
 const localRequire=createRequire(file);
 new Function("require","module","exports",js)(name=>{
   if(!name.startsWith("."))return localRequire(name);
   const path=resolve(dirname(file),name),target=[path,`${path}.ts`,`${path}.tsx`].find(existsSync);
   assert.ok(target,`cannot resolve ${name}`);return load(target);
 },module,module.exports);return module.exports;
}
 const {App}=require("antd");
 const base=dirname(fileURLToPath(import.meta.url));
 const {AssemblyConflictPanel}=load(resolve(base,"../assembly-conflict-panel.tsx"));
 const {mockToolbarCatalog}=load(resolve(base,"../../../api/mock-toolbar-catalog.ts"));
 const {searchCommands,contextualToolbars}=load(resolve(base,"../workbench-command-model.ts"));
 const {CommandRegistry}=load(resolve(base,"../../../cad/command/command-registry.ts"));
 const registry=new CommandRegistry();let opened=0;
 registry.register({id:"assembly.analyze",execute:()=>opened++,isVisible:()=>true,isEnabled:()=>true});
 const commands=searchCommands(contextualToolbars(mockToolbarCatalog.toolbars,"ASSEMBLY_DESIGN"),"装配约束分析",id=>registry.state(id));
 assert.equal(commands.length,1);await registry.execute(commands[0].commandId);assert.equal(opened,1);
 const constraint={id:"technical-uuid",kind:"DISTANCE",name:"安装间隙",first:{instanceId:"a"},second:{instanceId:"b"},evaluationStatus:"NOT_UPDATED"};
 const report={status:"UNKNOWN",baseRevisionId:"technical-revision",solverBuildPolicy:"technical-build",probeCount:12,elapsedMs:4,scopeConstraintIds:[constraint.id],scopeBodyIds:["a","b"],items:[{kind:"LOCALIZED_SUSPECT",oracle:"UNKNOWN",evidence:"technical-oracle",reason:"algorithm text",members:[{constraintId:constraint.id,first:constraint.first,repairActions:["EDIT"]}]}]};
 const view={document:{id:"p",versionId:"technical-revision",name:"机架"},product:{instances:[{id:"a",name:"支架"},{id:"b",name:"螺栓"}],constraints:[constraint]}};
 const html=renderToStaticMarkup(React.createElement(App,null,React.createElement(AssemblyConflictPanel,{open:true,pending:false,current:true,canEdit:true,view,report,onLocate(){},onLocateInstance(){},onRepair(){},onAnalyze(){},onStop(){},onClose(){}})));
 const defaultView=html.replace(/<details>[\s\S]*?<\/details>/g,"").replace(/<[^>]*>/g,"");
 assert.match(html,/data-row-key="technical-uuid"/,"stable business ID remains the row identity, not visible text");
 assert.match(defaultView,/待处理问题/);assert.match(defaultView,/安装间隙/);assert.match(defaultView,/支架/);
 assert.doesNotMatch(defaultView,/technical-uuid|technical-revision|technical-build|technical-oracle|algorithm text|探测上限|Oracle|UUID/);
 assert.doesNotMatch(defaultView,/technical-uuid/,"technical evidence not eagerly rendered into default worklist");
 assert.equal((html.match(/cad-command-dialog-footer/g)??[]).length,0,"analysis has one existing close/Esc path, no duplicate footer actions");
 for(const count of [0,1,100,500,1000]){
  const largeView={...view,product:{...view.product,constraints:Array.from({length:count},(_,i)=>({...constraint,id:`stable-${i}`,name:`检查项 ${i}`}))}};
  const markup=renderToStaticMarkup(React.createElement(App,null,React.createElement(AssemblyConflictPanel,{open:true,pending:false,current:true,canEdit:true,view:largeView,onLocate(){},onLocateInstance(){},onRepair(){},onAnalyze(){},onStop(){},onClose(){}})));
  assert.equal((markup.match(/class="ant-table-row ant-table-row-level-0"/g)??[]).length,Math.min(count,10),"fixed page length, not full expanded lists");
  if(count>10)assert.doesNotMatch(markup,/>检查项 99</,"unselected/off-page detail is not eagerly rendered");
 }
 for(const count of [0,1,100,500,1000]){
  const motionView={...view,product:{instances:Array.from({length:count},(_,i)=>({id:`unit-${i}`,name:`组件 ${i}`})),constraints:[]}};
  const markup=renderToStaticMarkup(React.createElement(App,null,React.createElement(AssemblyConflictPanel,{open:true,defaultTab:"motion",pending:false,current:true,canEdit:true,view:motionView,ownerOccurrence:"nested-occurrence",onLocate(){},onLocateInstance(){},onRepair(){},onAnalyze(){},onStop(){},onClose(){}})));
  assert.equal((markup.match(/class="ant-table-row ant-table-row-level-0"/g)??[]).length,Math.min(count,10));
  assert.match(markup,/剩余运动/);assert.doesNotMatch(markup,/raw direction|相对运动 3|axisPoint/);
  if(count)assert.match(markup,/p\/nested-occurrence\/unit-0/,"row identity includes owning occurrence");
 }
 // Execute the actual panel hooks/handlers with controlled state (no DOM or
 // browser acceptance claim). SSR alone cannot prove useEffect projection.
 const saved={useState:React.useState,useMemo:React.useMemo,useEffect:React.useEffect};
 const slots=[],shown=[],located=[];let cursor=0,effects=[];
 React.useState=initial=>{const i=cursor++;if(!(i in slots))slots[i]=initial;return [slots[i],value=>slots[i]=typeof value==="function"?value(slots[i]):value];};
 React.useMemo=fn=>fn();React.useEffect=fn=>effects.push(fn);
 const props={open:true,defaultTab:"motion",pending:false,current:true,canEdit:true,view,
  evidence:{documentId:"p",revisionId:"technical-revision",available:true,referenceSemantics:"STATIC_RELATIVE",coordinateFrame:"OWNING_PRODUCT",lengthUnit:"mm",components:[{bodyIds:["a","b"],solved:true,gaugeDof:0,freedoms:[{bodyId:"b",kind:2,translationDof:1,rotationDof:0,translationDirections:[[1,0,0]],rotations:[],linearizationPose:{translation:[0,0,0],rotation:[0,0,0,1]}}]}]},
  onMotion:m=>shown.push(m),onLocateInstance:id=>located.push(id),onLocate(){},onRepair(){},onAnalyze(){},onStop(){},onClose(){}};
 function renderControlled(){cursor=0;effects=[];const tree=AssemblyConflictPanel(props);for(const fn of effects)fn();return tree;}
 function find(tree,type){if(!tree)return;if(Array.isArray(tree)){for(const child of tree){const result=find(child,type);if(result)return result;}}else if(tree.type===type)return tree;else return find(tree.props?.children,type);}
 try{
  const {Table,Tabs}=require("antd");let tree=renderControlled(),table=find(tree,Table);
  assert.equal(tree.props.size,"L");assert.equal(table.props.scroll.x,720);
  const row=table.props.dataSource.find(r=>r.name==="螺栓");table.props.onRow(row).onClick();tree=renderControlled();assert.equal(shown.at(-1).bodyId,"b");
  assert.doesNotMatch(JSON.stringify(tree),/显示运动方向|方向详情/);
  table=find(tree,Table);table.props.columns.at(-1).render(undefined,row).props.onClick({stopPropagation(){}});renderControlled();assert.equal(shown.at(-1).bodyId,"b");assert.equal(located.at(-1),"b");
  table.props.onRow(row).onKeyDown({key:"Enter",preventDefault(){}});renderControlled();assert.equal(shown.at(-1).bodyId,"b");
  props.view={...view,document:{...view.document,versionId:"changed"}};renderControlled();assert.equal(shown.at(-1),undefined);
  props.view=view;tree=renderControlled();find(tree,Tabs).props.onChange("all");renderControlled();assert.equal(shown.at(-1),undefined);
  props.open=false;renderControlled();assert.equal(shown.at(-1),undefined);
 }finally{Object.assign(React,saved);}
