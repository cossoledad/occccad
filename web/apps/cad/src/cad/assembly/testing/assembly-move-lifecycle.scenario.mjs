import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {InputManager}=await server.ssrLoadModule("/src/cad/input/input-manager.ts");
 const {InputResult}=await server.ssrLoadModule("/src/cad/input/input-types.ts");
 const {AssemblyMoveTool}=await server.ssrLoadModule("/src/cad/tool/cad-tool.ts");
 class Surface extends EventTarget{
  style={};captured=new Set();setAttribute(){}focus(){}
  getBoundingClientRect(){return{left:0,top:0,width:1000,height:1000};}
  setPointerCapture(id){this.captured.add(id);}
  hasPointerCapture(id){return this.captured.has(id);}
  releasePointerCapture(id){this.captured.delete(id);this.emit("lostpointercapture",{pointerId:id});}
  emit(type,fields={}){const e=new Event(type,{cancelable:true});Object.assign(e,{clientX:0,clientY:0,pointerId:1,buttons:0,button:0,pointerType:"mouse",...fields});this.dispatchEvent(e);}
 }
 globalThis.HTMLElement=Surface;globalThis.window=new EventTarget();globalThis.document=new EventTarget();
 document.visibilityState="visible";
 const surface=new Surface(),events=[];const tool=new AssemblyMoveTool();
 let dragging=false;
 const context={viewport:{moveManipulatorPointerDown:()=>{dragging=true;events.push("begin");return true;},
  moveManipulatorPointerMove:(_id,x,y)=>{if(dragging)events.push(["target",x,y]);return dragging;},
  moveManipulatorPointerUp:(_id,commit)=>{events.push(commit?"final":"cancel");const was=dragging;dragging=false;return was;}}};
 const manager=new InputManager(surface,{pointerDown:e=>tool.pointerDown(e,context),pointerMove:e=>tool.pointerMove(e,context),
  pointerUp:e=>tool.pointerUp(e,context),pointerCancel:e=>tool.pointerCancel(e,context),cancel:()=>tool.cancel(context)});
 // Real pointerup path must consume pointerup position (not last move), then
 // its normal capture release must NOT issue an asynchronous cancellation.
 surface.emit("pointerdown",{buttons:1});surface.emit("pointermove",{buttons:1,clientX:10,clientY:20});
 surface.emit("pointerup",{clientX:30,clientY:40});
 assert.deepEqual(events,["begin",["target",10,20],["target",30,40],"final"]);
 assert.equal(surface.captured.size,0);
 // Unexpected lost capture, pointercancel, blur, and visibility all cancel.
 for(const kind of ["lostpointercapture","pointercancel","blur","hidden"]){
  events.length=0;surface.emit("pointerdown",{buttons:1});
  if(kind==="blur")window.dispatchEvent(new Event("blur"));
  else if(kind==="hidden"){document.visibilityState="hidden";document.dispatchEvent(new Event("visibilitychange"));document.visibilityState="visible";}
  else surface.emit(kind);
  assert.ok(events.includes("cancel"),kind);assert.equal(events.includes("final"),false,kind);
 }
 manager.dispose();
 // Explicit tool cancel after pointerup is also delivered: final response may
 // still be in flight even though there is no captured pointer anymore.
 events.length=0;tool.cancel(context);assert.deepEqual(events,["cancel"]);
 assert.equal(InputResult.ReleaseCapture,"release-capture");
}finally{await server.close();}
