import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const T=await server.ssrLoadModule("three");
 const {convertProjection,fitPerspectiveView,updatePerspectiveClipping}=await server.ssrLoadModule("/src/cad/navigation/projection-camera.ts");
 const {saveView,restoreView}=await server.ssrLoadModule("/src/cad/navigation/orthographic-view.ts");
 const {captureDocumentCamera,restoreDocumentCamera}=await server.ssrLoadModule("/src/cad/navigation/document-camera-state.ts");
 const {NavigationHUD}=await server.ssrLoadModule("/src/cad/navigation/hud/navigation-hud.ts");
 const {worldUnitsPerCssPixel}=await server.ssrLoadModule("/src/cad/rendering/viewport-metrics.ts");
 const target=new T.Vector3(120,-35,20),ortho=new T.OrthographicCamera(-200,200,150,-150,.001,1e6);
 ortho.position.copy(target).add(new T.Vector3(300,-300,300));ortho.up.set(0,0,1);ortho.lookAt(target);ortho.zoom=3;ortho.updateProjectionMatrix();ortho.updateMatrixWorld(true);
 const perspective=convertProjection(ortho,target,"PERSPECTIVE",4/3),metrics={cssWidth:800,cssHeight:600,devicePixelRatio:1};
 const units=c=>worldUnitsPerCssPixel(c,target,metrics);
 assert.ok(Math.abs(units(ortho)-units(perspective))<1e-10,"switch preserves apparent scale at focus");
 assert.deepEqual(ortho.quaternion.toArray(),perspective.quaternion.toArray());
 const roundtrip=convertProjection(perspective,target,"ORTHOGRAPHIC",4/3);assert.ok(Math.abs(units(roundtrip)-units(ortho))<1e-10);
 const saved=captureDocumentCamera(perspective,target);restoreDocumentCamera(ortho,new T.Vector3(),saved,4/3);assert.ok(Math.abs(units(ortho)-units(perspective))<1e-10,"document pose restores into global projection");
 const pose=saveView(perspective,target);restoreView(ortho,new T.Vector3(),pose);assert.ok(Math.abs(units(ortho)-units(perspective))<1e-10,"sketch return restores scale across projection changes");
 const bounds=new T.Box3(new T.Vector3(-40,-20,-15),new T.Vector3(50,90,55));fitPerspectiveView(perspective,target,bounds);const before=perspective.position.clone();updatePerspectiveClipping(perspective,target,bounds);assert.deepEqual(perspective.position,before,"clipping must not move perspective eye");
 for(let i=0;i<8;i++){const p=new T.Vector3(i&1?bounds.max.x:bounds.min.x,i&2?bounds.max.y:bounds.min.y,i&4?bounds.max.z:bounds.min.z).project(perspective);assert.ok(Math.abs(p.x)<=1&&Math.abs(p.y)<=1&&Math.abs(p.z)<=1,"fit includes all 8 corners")}
 const hud=new NavigationHUD();hud.update({catia:{showPivot:true,pivot:new T.Vector3(999,999,999),pointer:{x:100,y:200}}},perspective,800,600);assert.deepEqual(hud.screenPosition,{x:400,y:300},"HUD anchor independent of picked depth/position");
 const marker=hud.pivotMarker,p=marker.geometry.attributes.position,c=marker.geometry.attributes.color;
 assert.equal(p.count,10);assert.ok(p.getX(1)<0&&p.getX(2)>0,"X axis has a central gap");assert.ok(p.getY(5)<0&&p.getY(6)>0,"Y axis has a central gap");assert.ok(p.getZ(9)-p.getZ(8)<p.getX(3)-p.getX(0),"Z is shorter");assert.ok(c.getX(8)>.9&&c.getY(8)>.6&&c.getZ(8)<.2,"Z is yellow");hud.dispose();
}finally{await server.close()}
