import assert from "node:assert/strict";
import {createRequire} from "node:module";
import {readFileSync,existsSync} from "node:fs";
import {dirname,resolve} from "node:path";
import {fileURLToPath} from "node:url";
const require=createRequire(import.meta.url),React=require("react"),{renderToStaticMarkup}=require("react-dom/server"),ts=require("typescript"),antd=require("antd"),runtime=require("react/jsx-runtime");
const modules=new Map(),elements=[];
const capture=fn=>(type,props,...rest)=>{const element=fn(type,props,...rest);elements.push(element);return element;};
function load(file){
 if(modules.has(file))return modules.get(file).exports;
 const module={exports:{}};modules.set(file,module);
 const js=ts.transpileModule(readFileSync(file,"utf8").replaceAll("import.meta.env","({})"),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022,jsx:ts.JsxEmit.ReactJSX,esModuleInterop:true},fileName:file}).outputText;
 const localRequire=createRequire(file);
 new Function("require","module","exports",js)(name=>{
  if(name==="react/jsx-runtime")return {...runtime,jsx:capture(runtime.jsx),jsxs:capture(runtime.jsxs)};
  if(!name.startsWith("."))return localRequire(name);
  const path=resolve(dirname(file),name),target=[path,`${path}.ts`,`${path}.tsx`].find(existsSync);assert(target,`missing ${name}`);return load(target);
 },module,module.exports);return module.exports;
}
const base=dirname(fileURLToPath(import.meta.url));
const {SketchInlineParameterInput,SketchParameterValueInput,defaultSketchToolMode}=load(resolve(base,"../../../cad/sketch/sketch-inline-parameter-input.tsx"));
const {WorkbenchStatus}=load(resolve(base,"../workbench-status.tsx"));
const {UIHelpProvider}=load(resolve(base,"../../../cad/help/ui-help-context.tsx"));
const state={toolId:"sketch.edit.fillet",operation:"圆角",phase:"definition",presentation:"inline",input:{id:"stable-focus-request",fieldIndex:1,anchor:[120,80]},role:"输入半径",count:{accepted:2,required:2},completion:{label:"完成角点"},selectedIds:["private-entity-uuid"],references:[],fields:[{label:"长度",value:"4",unit:"length"},{label:"半径",value:"2 * width",unit:"length"}],options:[{name:"internal-option",label:"高级选项",value:"a",choices:[]}],canConfirm:true,next:"确认当前角点"};
const actions=[];
const render=(component,props)=>{elements.length=0;return renderToStaticMarkup(React.createElement(antd.App,null,React.createElement(UIHelpProvider,null,React.createElement(component,props))));};
const inlineHTML=render(SketchInlineParameterInput,{state,lengthUnit:"in",onAction:action=>actions.push(action)});
assert(inlineHTML.includes("半径")&&inlineHTML.includes(" in"));assert.match(inlineHTML,/2 \* width/);assert(inlineHTML.includes("<svg"));assert(elements.some(element=>element.type===antd.Input&&element.props.autoFocus),"current parameter immediately enters borderless numeric editing");assert(!inlineHTML.includes("高级选项"));assert(!inlineHTML.includes("长度"));assert(!inlineHTML.includes("private-entity-uuid"));
const inline=elements.find(element=>element.props?.["data-cad-parameter-annotation"]);
render(SketchParameterValueInput,{field:state.fields[1],pending:false,onChange:value=>actions.push({type:"field",index:1,value})});
const input=elements.find(element=>element.type===antd.Input);assert(input.props.autoFocus);assert.equal(input.props.variant,"borderless");assert.equal(input.props.onBlur,undefined);input.props.onChange({target:{value:"3 * height + 2 in"}});assert.deepEqual(actions.pop(),{type:"field",index:1,value:"3 * height + 2 in"});
const key=(value,overrides={})=>{const event={key:value,repeat:false,shiftKey:false,nativeEvent:{isComposing:false,keyCode:0},defaultPrevented:false,preventDefault(){this.defaultPrevented=true;},stopPropagation(){},...overrides};inline.props.onKeyDownCapture(event);return event;};
key("Enter",{nativeEvent:{isComposing:true,keyCode:229}});key("Enter",{repeat:true});assert.equal(actions.length,0);assert(key("Enter").defaultPrevented);key("Enter");assert.deepEqual(actions,[{type:"confirm"}],"production inline confirm uses a synchronous latch");actions.length=0;
key("5");assert.equal(actions.length,0,"typing belongs to the focused native input");
key("Tab");assert.deepEqual(actions.pop(),{type:"input",index:0});key("Escape");assert.deepEqual(actions.pop(),{type:"cancel"},"Esc exits the whole owning tool, not just its input");
for(const phase of ["committing","unknown"]){render(SketchInlineParameterInput,{state:{...state,phase},onAction:action=>actions.push(action)});const blockValue=elements.find(element=>element.type===antd.Input),block=elements.find(element=>element.props?.["data-cad-parameter-annotation"]);assert(blockValue.props.disabled);block.props.onKeyDownCapture({key:"Enter",repeat:false,nativeEvent:{isComposing:false},preventDefault(){},stopPropagation(){}});assert.equal(actions.length,0);}
// Real component pointer handlers emit placement only, with frozen gesture anchor.
render(SketchInlineParameterInput,{state:{...state,input:{...state.input,dimension:true}},onAction:action=>actions.push(action)});
const dragGroup=elements.find(element=>element.props?.["data-cad-parameter-annotation"]),dragCapture={setPointerCapture(){},hasPointerCapture(){return true;},releasePointerCapture(){}};
const event=(x,y)=>({pointerId:7,button:0,clientX:x,clientY:y,currentTarget:dragCapture,target:{closest(){return null;}},stopPropagation(){}});
dragGroup.props.onPointerDown(event(10,10));dragGroup.props.onPointerMove(event(18,15));dragGroup.props.onPointerUp(event(18,15));
assert.deepEqual(actions.pop(),{type:"placement",position:[128,85]},"annotation drag changes placement, never numeric source");
assert(!render(SketchInlineParameterInput,{state:{...state,input:undefined},onAction(){}}).includes("data-cad-parameter-annotation"));assert(!render(SketchInlineParameterInput,{state:{...state,presentation:"advanced"},onAction(){}}).includes("data-cad-parameter-annotation"));
const frozenUnitHTML=render(SketchInlineParameterInput,{state:{...state,fields:state.fields.map(field=>({...field,displayUnit:"in"}))},lengthUnit:"mm",onAction(){}});assert(frozenUnitHTML.includes(" in"));assert(!frozenUnitHTML.includes(" mm"),"an active draft keeps its actor-owned display unit when the document preference changes");
const statusHTML=render(WorkbenchStatus,{busy:false,canEdit:true,selectionCount:0,toolName:"圆角",lengthUnit:"in",continuous:true,sketchCommand:state,onSketchAction:action=>actions.push(action)});
assert.match(statusHTML,/当前草图动作/);assert.match(statusHTML,/输入半径/);assert.match(statusHTML,/2 \/ 2/);assert.match(statusHTML,/完成角点/);assert(!statusHTML.includes("private-entity-uuid"));
const statusButtons=elements.filter(element=>element.type===antd.Button);
statusButtons.find(element=>element.props.children==="半径").props.onClick();assert.deepEqual(actions.pop(),{type:"input",index:1});statusButtons.find(element=>element.props.children==="完成角点").props.onClick();assert.deepEqual(actions.pop(),{type:"confirm"});
render(WorkbenchStatus,{busy:false,canEdit:true,selectionCount:0,toolName:"修剪",lengthUnit:"mm",continuous:false,sketchCommand:{...state,input:undefined,error:"尺寸引用需要处理",recovery:{label:"解除受影响关系（1）",action:"release"}},onSketchAction:action=>actions.push(action)});
assert(elements.some(e=>e.props?.role==="alert"&&e.props.children==="尺寸引用需要处理"),"no-input failures remain visible in the status bar");
elements.find(e=>e.type===antd.Button&&e.props.children==="解除受影响关系（1）").props.onClick();assert.deepEqual(actions.pop(),{type:"release"});
// Options are an explicit secondary popup; changing them stays in the actor.
const mirrorOption={name:"mirrorMode",label:"镜像关系",value:"LINKED",choices:[{value:"LINKED",label:"关联镜像"},{value:"INDEPENDENT",label:"独立副本"}]};
render(WorkbenchStatus,{busy:false,canEdit:true,selectionCount:0,toolName:"镜像",lengthUnit:"mm",continuous:false,sketchCommand:{...state,input:undefined,fields:[],options:[mirrorOption]},onSketchAction:action=>actions.push(action)});
const popup=elements.find(element=>element.type===antd.Popover&&element.props.title==="工具选项");assert(popup);assert.equal(popup.props.trigger,"click");
const optionGroup=elements.find(element=>element.type===antd.Space&&element.props["aria-label"]==="工具选项");assert.equal(optionGroup.props.role,"dialog");let stopped=false;optionGroup.props.onKeyDown({key:"Escape",nativeEvent:{},stopPropagation(){stopped=true;}});assert(stopped,"options Escape belongs to the popup, not the active tool");
const mirrorSelect=elements.find(element=>element.type===antd.Select&&element.props["aria-label"]==="镜像关系");mirrorSelect.props.onChange("INDEPENDENT");assert.deepEqual(actions.pop(),{type:"option",name:"mirrorMode",value:"INDEPENDENT"});
for(const [mode,label,unit,value] of [["TWO_LENGTHS","第二距离","length","7 mm"],["LENGTH_ANGLE","角度","angle","45"]]){
 const chamfer={...state,toolId:"sketch.edit.chamfer",operation:"倒角",input:{id:`chamfer-${mode}`,fieldIndex:1},fields:[{label:"第一距离",value:"5",unit:"length"},{label,value,unit}],options:[{name:"chamferMode",label:"倒角定义",value:mode,choices:[{value:"TWO_LENGTHS",label:"双距离"},{value:"LENGTH_ANGLE",label:"距离与角度"}]}]};
 render(WorkbenchStatus,{busy:false,canEdit:true,selectionCount:0,toolName:"倒角",lengthUnit:"mm",continuous:true,sketchCommand:chamfer,onSketchAction:action=>actions.push(action)});
 const modeSelect=elements.find(element=>element.type===antd.Select&&element.props["aria-label"]==="倒角定义");modeSelect.props.onChange(mode);assert.deepEqual(actions.pop(),{type:"option",name:"chamferMode",value:mode});
 elements.find(element=>element.type===antd.Button&&element.props.children===label).props.onClick();assert.deepEqual(actions.pop(),{type:"input",index:1});
 const html=render(SketchInlineParameterInput,{state:chamfer,lengthUnit:"mm",onAction:action=>actions.push(action)});assert(html.includes(label)&&html.includes(unit==="angle"?"deg":"mm"));render(SketchParameterValueInput,{field:chamfer.fields[1],pending:false,onChange:value=>actions.push({type:"field",index:1,value})});elements.find(element=>element.type===antd.Input).props.onChange({target:{value:"2 * parameter"}});assert.deepEqual(actions.pop(),{type:"field",index:1,value:"2 * parameter"});
}
for(const phase of ["committing","unknown"]){render(WorkbenchStatus,{busy:false,canEdit:true,selectionCount:0,toolName:"镜像",lengthUnit:"mm",continuous:false,sketchCommand:{...state,phase,options:[mirrorOption]},onSketchAction(){}});assert(elements.find(element=>element.type===antd.Select&&element.props["aria-label"]==="镜像关系").props.disabled);assert(elements.find(element=>element.type===antd.Button&&element.props.children==="选项").props.disabled);}
render(WorkbenchStatus,{busy:false,canEdit:true,selectionCount:0,toolName:"圆角",lengthUnit:"mm",continuous:true,sketchCommand:{...state,fields:[state.fields[1]],options:[]},onSketchAction(){}});assert(!elements.some(element=>element.type===antd.Popover&&element.props.title==="工具选项"),"single radius does not add an empty options popup");
render(WorkbenchStatus,{busy:false,canEdit:true,selectionCount:0,toolName:"高级工具",lengthUnit:"mm",continuous:false,sketchCommand:{...state,presentation:"advanced"},onSketchAction(){}});assert(!elements.some(element=>element.type===antd.Popover&&element.props.title==="工具选项"),"advanced options remain in their explicitly opened panel");
assert.equal(defaultSketchToolMode("sketch.edit.mirror"),"once");assert.equal(defaultSketchToolMode("sketch.edit.mirror",true),"continuous");for(const command of ["trim","quick_trim","split","fillet","chamfer"])assert.equal(defaultSketchToolMode(`sketch.edit.${command}`),"once");assert.equal(defaultSketchToolMode("sketch.dimension.linear"),"once");assert.equal(defaultSketchToolMode("sketch.line"),"once");
render(WorkbenchStatus,{busy:false,canEdit:true,selectionCount:0,toolName:"选择",lengthUnit:"mm",continuous:false,sketchReceipt:{status:"unknown"},onSketchReceiptCheck:()=>actions.push("receipt")});elements.find(element=>element.type===antd.Button&&element.props.children==="确认草图结果").props.onClick();assert.equal(actions.pop(),"receipt");
console.log("PASS production inline/status React UI, focus request, raw units/expressions, field actions, whole-tool Escape, single Enter and pending barrier");
