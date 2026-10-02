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
