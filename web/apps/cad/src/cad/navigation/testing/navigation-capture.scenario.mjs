import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule("three"),{InputManager}=await server.ssrLoadModule("/src/cad/input/input-manager.ts"),{NavigationController}=await server.ssrLoadModule("/src/cad/navigation/navigation-controller.ts");
 class Surface extends EventTarget{
  style={};captured=new Set();setAttribute(){}focus(){}getBoundingClientRect(){return{left:0,top:0}}
  setPointerCapture(id){this.captured.add(id)}hasPointerCapture(id){return this.captured.has(id)}
  releasePointerCapture(id){this.captured.delete(id);this.emit("lostpointercapture",0,-1)}
  emit(type,buttons,button=-1,x=400,y=300){const e=new Event(type,{cancelable:true});Object.assign(e,{pointerId:1,pointerType:"mouse",buttons,button,clientX:x,clientY:y});this.dispatchEvent(e)}
 }
 globalThis.HTMLElement=Surface;globalThis.window=new EventTarget();globalThis.document=new EventTarget();document.visibilityState="visible";
 const camera=new T.OrthographicCamera(-20,20,15,-15,.1,1000);camera.position.set(0,0,100);
 const nav=new NavigationController(camera,()=>({width:800,height:600}),()=>{},{pickNearest:()=>({point:new T.Vector3(),source:"raycast"})},()=>new T.Vector3(),"catia");
 const surface=new Surface(),input=new InputManager(surface,nav,{retainNavigationCapture:e=>nav.retainsMouseCapture(e.pointerId,e.buttons)});
 surface.emit("pointerdown",4,1);surface.emit("pointermove",6,2);surface.emit("pointermove",4,2);
 assert.equal(nav.activeAction,"zoom");surface.captured.delete(1);surface.emit("lostpointercapture",4);
 assert.equal(nav.activeAction,"zoom","releasing the auxiliary button cannot cancel the held leader");assert.equal(surface.hasPointerCapture(1),true);
 const zoom=camera.zoom;surface.emit("pointermove",4,-1,400,270);assert.ok(camera.zoom>zoom);
 window.dispatchEvent(new Event("blur"));assert.equal(nav.activeAction,"none");assert.equal(surface.captured.size,0);
 surface.emit("pointerdown",4,1);surface.emit("pointercancel",4);assert.equal(nav.snapshot.catia.showPivot,false);
 surface.emit("pointerdown",4,1);surface.emit("pointerup",0,1);assert.equal(surface.captured.size,0);input.dispose();
}finally{await server.close()}
