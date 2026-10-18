import {expect,test} from "@playwright/test";

test("bounded ground stays fixed in world space, respects depth and ends at its square boundary",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",error=>errors.push(error.message));page.on("console",message=>{if(/THREE.WebGLProgram|Shader Error|VALIDATE_STATUS/.test(message.text()))errors.push(message.text())});
 await page.goto("/");
 const result=await page.evaluate(async()=>{
  // @ts-ignore Vite production sources and the real WebGL renderer.
  const T=await import("/node_modules/.vite/deps/three.js");
  // @ts-ignore Vite production source.
  const {PlaneGrid}=await import("/src/cad/rendering/plane-grid.ts");
  // @ts-ignore Vite production source.
  const {groundFootprint}=await import("/src/cad/rendering/scene-ground.ts");
  const renderer=new T.WebGLRenderer({antialias:false,preserveDrawingBuffer:true});renderer.setSize(400,400);document.body.append(renderer.domElement);
  renderer.domElement.style.cssText="position:fixed;left:0;top:0;z-index:9000";
  const scene=new T.Scene();scene.background=new T.Color(0);
  const grid=new PlaneGrid(0xffffff);grid.setStyle("#ffffff",1,10);scene.add(grid.object);
  const frame={origin:new T.Vector3(),normal:new T.Vector3(0,0,1),u:new T.Vector3(1,0,0),v:new T.Vector3(0,1,0)};
  const camera=new T.PerspectiveCamera(35,1,1e-7,1e9);camera.up.set(0,1,0);
  const gl=renderer.getContext(),pixels=()=>{const data=new Uint8Array(400*400*4);gl.readPixels(0,0,400,400,gl.RGBA,gl.UNSIGNED_BYTE,data);return data};
  const draw=()=>{camera.updateProjectionMatrix();grid.update(camera,400,frame);renderer.render(scene,camera);return pixels()};
  const lit=(data:Uint8Array,from=0,to=400)=>{let count=0;for(let y=from;y<to;y++)for(let x=0;x<400;x++)if(data[(y*400+x)*4]>5)count++;return count};
  const scales=[];
  for(const distance of [1e-4,1e-3,.01,.1,1,10,100,1000,1e4,1e5]){
   camera.position.set(.17,.23,distance);camera.lookAt(.17,.23,0);const data=draw();
   scales.push({distance,lit:lit(data),base:grid.object.material.uniforms.uSpacing.value});
  }
  const distant=[];grid.setFootprint({center:new T.Vector2(1e9,-2e9),halfSize:100});for(const distance of [1,100]){camera.position.set(1e9+.17,-2e9+.23,distance);camera.lookAt(1e9+.17,-2e9+.23,0);distant.push(lit(draw()))}
  const aligned=groundFootprint(new T.Box3(new T.Vector3(5,15,-5),new T.Vector3(25,35,5)),10);grid.setFootprint(aligned);
  const edgeCamera=new T.OrthographicCamera(-200,200,200,-200,.1,1000);edgeCamera.position.set(50,50,200);edgeCamera.lookAt(50,50,0);edgeCamera.updateProjectionMatrix();
  grid.update(edgeCamera,400,frame);renderer.render(scene,edgeCamera);const edgePixels=pixels();
  const alignedEdges={visible:0,outside:0};for(let i=52;i<348;i++){
   for(const [x,y] of [[50,i],[349,i],[i,50],[i,349]])if(edgePixels[(y*400+x)*4]>10)alignedEdges.visible++;
   for(const [x,y] of [[49,i],[350,i],[i,49],[i,350]])if(edgePixels[(y*400+x)*4]>5)alignedEdges.outside++;
  }
  grid.setFootprint({center:new T.Vector2(),halfSize:40});camera.position.set(0,0,200);camera.lookAt(0,0,0);
  const bounded=draw();const corners=[bounded[(12*400+12)*4],bounded[(388*400+388)*4]];
  camera.position.set(400,0,200);camera.lookAt(400,0,0);const pannedAway=lit(draw());
  grid.setFootprint({center:new T.Vector2(),halfSize:100});
  const mesh=new T.Mesh(new T.BoxGeometry(20,20,20),new T.MeshBasicMaterial({color:0x404040}));scene.add(mesh);
  camera.position.set(0,0,100);camera.lookAt(0,0,0);camera.near=.1;camera.far=120;frame.origin.z=-50;
  const top=draw();grid.object.visible=false;renderer.render(scene,camera);const noGrid=pixels();
  const value=(data:Uint8Array,x:number,y:number)=>data[(y*400+x)*4];
  const occlusion={front:0,model:Math.abs(value(top,200,200)-value(noGrid,200,200)),farGrid:value(top,24,200)-value(noGrid,24,200)};
  camera.position.set(0,0,-100);camera.lookAt(0,0,0);const below=draw();grid.object.visible=false;renderer.render(scene,camera);occlusion.front=value(below,200,200)-value(pixels(),200,200);
  mesh.visible=false;camera.position.set(0,-100,0);camera.up.set(0,0,1);camera.lookAt(0,0,0);const horizon=draw();
  const horizonCounts={below:lit(horizon,0,190),above:lit(horizon,210,400)};
  return {scales,distant,occlusion,horizonCounts,corners,pannedAway,alignedEdges};
 });
 for(const sample of result.scales){expect(sample.base).toBe(10);if(sample.distance<=1000)expect(sample.lit,`distance=${sample.distance}`).toBeGreaterThan(300)}
 expect(result.scales.at(-1)!.lit).toBeLessThan(50);expect(result.corners).toEqual([0,0]);expect(result.pannedAway).toBe(0);
 for(const count of result.distant)expect(count).toBeGreaterThan(300);
 expect(result.alignedEdges.visible).toBe((348-52)*4);expect(result.alignedEdges.outside).toBe(0);
 expect(result.occlusion.model).toBeLessThan(1);expect(result.occlusion.farGrid).toBeGreaterThan(10);expect(result.occlusion.front).toBeGreaterThan(10);
 expect(result.horizonCounts.below).toBeGreaterThan(300);expect(result.horizonCounts.above).toBe(0);
 expect(errors).toEqual([]);await page.screenshot({path:testInfo.outputPath("bounded-ground-horizon.png")});
});
