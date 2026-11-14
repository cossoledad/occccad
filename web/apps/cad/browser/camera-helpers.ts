import {expect,type Page} from "@playwright/test";

/** Read the mounted component's engine ref. Do not import another HMR module
 * identity, force a render or move the camera just to obtain the test probe. */
export async function observeViewport(page:Page){
 await expect.poll(()=>page.evaluate(()=>{
  const canvas=document.querySelector('canvas');if(!canvas)return false;
  const current=(window as any).__cameraEngine;
  if(current?.renderer?.domElement===canvas&&!current.disposed)return true;
  const host=canvas.parentElement as any;if(!host)return false;
  const key=Object.getOwnPropertyNames(host).find(key=>key.startsWith('__reactFiber$'));if(!key)return false;
  for(let fiber=host[key];fiber;fiber=fiber.return){
   for(let hook=fiber.memoizedState;hook&&typeof hook==='object'&&'memoizedState' in hook;hook=hook.next){
    const engine=hook.memoizedState?.current;
    if(engine?.renderer?.domElement===canvas&&engine.navigation&&engine.camera&&!engine.disposed){(window as any).__cameraEngine=engine;return true;}
   }
  }
  return false;
 })).toBeTruthy();
}
export async function worldScreen(page:Page,point:number[]){
 await observeViewport(page);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.viewTransition.active)).toBeFalsy();
 return page.evaluate(point=>{const e=(window as any).__cameraEngine,p=e.navigation.target.clone().set(...point).project(e.camera),box=document.querySelector('canvas')!.getBoundingClientRect();return {x:box.x+(p.x+1)*box.width/2,y:box.y+(1-p.y)*box.height/2}},point);
}
export async function setProjection(page:Page,label:"平行"|"透视"){
 await page.getByRole("button",{name:"选项",exact:true}).click();
 const dialog=page.getByRole("dialog",{name:"选项",exact:true});await dialog.getByRole("menuitem",{name:"场景",exact:true}).click();
 await dialog.getByRole("combobox",{name:"投影方式"}).click();await page.locator(".ant-select-dropdown:not(.ant-select-dropdown-hidden)").getByText(label,{exact:true}).click();
 await dialog.getByRole("button",{name:"关闭",exact:true}).click();await expect(dialog).not.toBeVisible();
}

// Tests read the existing engine snapshot directly. No debug strings or JSON
// are inserted into the user interface, and no geometry/state is mocked.
class ViewportProbe{
 constructor(readonly page:Page,readonly kind:string){}
 async snapshot(){await observeViewport(this.page);return this.page.evaluate(()=>{const e=(window as any).__cameraEngine;let result:any;const original=e.callbacks.debugStateChanged;e.callbacks.debugStateChanged=(s:any)=>{result=s};try{e.emitDebugState()}finally{e.callbacks.debugStateChanged=original}return result})}
 async count(){const s=await this.snapshot();return this.kind==='viewport-datum-preview'?Number(!!s.datumPreview):this.kind==='viewport-feature-selection'?Number(!!s.featureSelection):this.kind==='navigation-pivot'?Number(!!s.navigation?.catia):1}
 async getAttribute(name:string){const s=await this.snapshot(),n=s.navigation;let value:unknown;
  if(name==='data-transition-active')value=!!s.viewTransitionActive;
  if(name==='data-camera-state')value=JSON.stringify({position:n.cameraPosition,quaternion:n.cameraQuaternion,zoom:n.cameraZoom,projection:n.projection,fov:n.fov,aspect:n.aspect,span:n.span});
  if(name==='data-state')value=JSON.stringify(s.sceneGrid);
  if(name==='data-kind')value=s.datumPreview?.kind;
  if(name==='data-count')value=this.kind==='viewport-feature-selection'?s.featureSelection?.count:s.selectionKeys?.length??0;
  if(name==='data-highlighted')value=s.highlightedVisible??0;
  if(name==='data-role')value=s.featureSelection?.role;
  if(name==='data-overlays')value=s.featureSelection?.overlays;
  if(name==='data-visible')value=n?.catia?.showPivot;
  return value===undefined?null:String(value);
 }
 async textContent(){const s=await this.snapshot(),n=s.navigation,c=n?.catia;
  if(this.kind==='navigation-camera')return `Camera: ${n.cameraPosition.map((v:number)=>v.toFixed(4)).join(',')} / ${n.cameraQuaternion.map((v:number)=>v.toFixed(4)).join(',')} / zoom ${n.cameraZoom.toPrecision(10)}`;
  if(this.kind==='navigation-projection')return `${n.projection} zoom: ${n.cameraZoom.toPrecision(10)}`;
  if(this.kind==='viewport-selection')return `Selection: ${s.selectionKeys?.join(', ')||'none'}`;
  if(this.kind==='viewport-datum-preview')return s.datumPreview?`Datum: ${JSON.stringify(s.datumPreview)}`:'';
  if(this.kind==='navigation-pivot')return c?`Centre: ${[c.pivot.x,c.pivot.y,c.pivot.z].map(v=>v.toFixed(6)).join(',')} · ${c.hitObject??'existing'}`:'';
  const input=s.input;
  return `L: ${input.buttons.left?'down':'up'} · M: ${input.buttons.middle?'down':'up'} · R: ${input.buttons.right?'down':'up'} Ctrl: ${input.modifiers.ctrl} · Shift: ${input.modifiers.shift} · Alt: ${input.modifiers.alt} Tool: ${s.activeTool} · Nav: ${s.navigationProfile}/${s.navigationAction} Reference: ${n?.solidworks?.reference?.highlight?.kind??'none'} ${c?`CATIA: ${c.state} · Pivot: ${c.pivotSource} P: ${c.pivot.x.toFixed(3)}, ${c.pivot.y.toFixed(3)}, ${c.pivot.z.toFixed(3)} Hit: ${c.hitObject??'none'} · Distance: ${c.cameraDistance.toFixed(3)} HUD: ${c.hudVisible&&s.hudScreen?`${s.hudScreen.x.toFixed(1)}, ${s.hudScreen.y.toFixed(1)}`:'hidden'}`:''}`;
 }
}
export const viewportProbe=(page:Page,kind:string)=>new ViewportProbe(page,kind);
export function expectViewport(value:any){
 if(!(value instanceof ViewportProbe))return expect(value);
 const match=(actual:string|null,want:string|RegExp)=>want instanceof RegExp?want.test(actual??''):actual===want;
 const methods=(negate=false):any=>({get not(){return methods(!negate)},
  toHaveAttribute:async(name:string,want:string|RegExp)=>expect.poll(async()=>match(await value.getAttribute(name),want)).toBe(!negate),
  toHaveText:async(want:string|RegExp)=>expect.poll(async()=>match(await value.textContent(),want)).toBe(!negate),
  toContainText:async(want:string|RegExp)=>expect.poll(async()=>want instanceof RegExp?want.test(await value.textContent()):(await value.textContent()).includes(want)).toBe(!negate),
  toHaveCount:async(count:number)=>expect.poll(async()=>await value.count()===count).toBe(!negate),
  toBeVisible:async()=>expect.poll(async()=>await value.count()>0).toBe(!negate)
 });return methods();
}
