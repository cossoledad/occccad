import {expect,type Page} from "@playwright/test";

export async function worldScreen(page:Page,point:number[]){
 await expect(page.getByTestId("navigation-camera")).toBeVisible();
 return page.evaluate(async point=>{
  // @ts-ignore Vite serves the real viewport dependency.
  const T=await import("/node_modules/.vite/deps/three.js");
  const state=JSON.parse(document.querySelector('[data-testid="navigation-camera"]')!.getAttribute("data-camera-state")!),box=document.querySelector("canvas")!.getBoundingClientRect(),aspect=box.width/box.height;
  const span=state.span??300;
  const camera=state.projection==="perspective"?new T.PerspectiveCamera(state.fov,aspect,.000001,1e12):new T.OrthographicCamera(-span*aspect/2,span*aspect/2,span/2,-span/2,0,1e12);
  camera.position.fromArray(state.position);camera.quaternion.fromArray(state.quaternion);camera.zoom=state.zoom;camera.updateProjectionMatrix();camera.updateMatrixWorld(true);
  const p=new T.Vector3(...point).project(camera);return{x:box.x+(p.x+1)*box.width/2,y:box.y+(1-p.y)*box.height/2};
 },point);
}
export async function setProjection(page:Page,label:"平行（无透视）"|"透视（近大远小）",commit=true){
 await page.getByRole("button",{name:"选项",exact:true}).click();
 const dialog=page.getByRole("dialog",{name:"选项",exact:true});await dialog.getByRole("menuitem",{name:"场景",exact:true}).click();
 await dialog.getByRole("combobox",{name:"投影方式"}).click();await page.getByText(label,{exact:true}).click();
 await dialog.getByRole("button",{name:commit?/确\s*定/:/取\s*消/}).click();await expect(dialog).not.toBeVisible();
}
