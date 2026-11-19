import { expect, test } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { createHash } from "node:crypto";
import { edgeFixtures, prepareEdgeFixtures } from "./edge-fixtures";

test.beforeAll(async () => { test.setTimeout(180_000); await prepareEdgeFixtures(); });
test("native curved highlights keep visible strokes and occlusion", async ({ page }) => {
  const output = path.resolve("../../../build/agent-work/browser-polish/native"); fs.mkdirSync(output, { recursive: true });
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  page.on("console", m => { if(/THREE.WebGLProgram|Shader Error/.test(m.text())) errors.push(m.text()); });
  await page.route("**/edge-fixtures/*.glb", route => route.fulfill({ body: fs.readFileSync(path.join(edgeFixtures, path.basename(new URL(route.request().url()).pathname))), contentType: "model/gltf-binary" }));
  await page.goto("/browser/natural-edges.html"); await page.waitForFunction(() => Boolean((window as any).edgeHarness));
  const reports = [];
  for(const name of ["cylinder", "fillet"]){
    const data = fs.readFileSync(path.join(edgeFixtures, name+".glb")), digest=createHash("sha256").update(data).digest("hex");
    await page.evaluate(async args => (window as any).edgeHarness.load(...args), [name,digest,data.length,"#98a8b8",1,"natural"]);
    for(const [projection,dpr] of [["perspective",1],["orthographic",1],["orthographic",2]] as const) for(const state of ["selected","hover"]){
      await page.evaluate(async ({projection,dpr})=>{
        const {engine:e,frame}=(window as any).edgeHarness;e.switchProjection(projection);e.renderer.setPixelRatio(dpr);e.renderer.setSize(1280,900);await frame();
      },{projection,dpr});
      const report=await page.evaluate(async state=>{
        const {engine:e,THREE:T,frame}= (window as any).edgeHarness,b=[...e.solidBindings.values()][0] as any;
        e.select(null,false); e.preselect(null); if(state==="selected")e.select(b.group.userData,false);else e.preselect(b.group.userData);
        await frame();
        const overlays=state==="selected"?e.selectedOverlays:e.preselectedOverlays;
        const ray=new T.Raycaster(),size=new T.Vector2();e.renderer.getSize(size);
        const points=[];const scale=new T.Box3().setFromObject(b.mesh).getSize(new T.Vector3()).length();
        for(const edge of b.artifact.mesh.edges??[])for(let i=1;i<edge.points.length;i++){
          const p=new T.Vector3().fromArray(edge.points[i-1]).lerp(new T.Vector3().fromArray(edge.points[i]),.5).applyMatrix4(b.group.matrixWorld),q=p.clone().project(e.camera);
          ray.setFromCamera(new T.Vector2(q.x,q.y),e.camera);const hit=ray.intersectObject(b.mesh)[0];
          const distance=p.clone().sub(ray.ray.origin).dot(ray.ray.direction);
          if(hit&&Math.abs(hit.distance-distance)<scale*.002)points.push([(q.x+1)*size.x/2,(1-q.y)*size.y/2]);
        }
        // Keep one reference to the production hook per material for a fair
        // prior-coverage recipe and for diagnostic depth-only comparison.
        for(const line of overlays){const m=line.material;m.userData.acceptanceHook??=m.onBeforeCompile;m.userData.acceptanceKey??=m.customProgramCacheKey;}
        return {state,points,overlays:overlays.length,digest:b.artifact.representations.VISUAL.digest,bytes:e.interactionGeometry.retainedBytes,
          rendererDpr:e.renderer.getPixelRatio(),deviceDpr:devicePixelRatio,drawingBuffer:e.renderer.getDrawingBufferSize(new T.Vector2()).toArray()};
      },state);
      expect(report.overlays).toBe(2); expect(report.points.length).toBeGreaterThan(10);expect(report.rendererDpr).toBe(dpr);
      const screenshots:Record<string,Buffer>={},calls:Record<string,number>={};
      for(const mode of ["prior-coverage","current","depth-reference"]){
        calls[mode]=await page.evaluate(async mode=>{
          const {engine:e,frame}=(window as any).edgeHarness;
          for(const line of [...e.selectedOverlays,...e.preselectedOverlays]){
            const m=line.material,original=m.userData.acceptanceHook;
            m.depthTest=mode!=="depth-reference";m.alphaToCoverage=mode==="prior-coverage";
            m.onBeforeCompile=(shader:any)=>{original(shader);if(mode==="prior-coverage")shader.fragmentShader=shader.fragmentShader
              .replace("#define CAD_ANALYTIC_LINE_COVERAGE\n","").replaceAll("CAD_ANALYTIC_LINE_COVERAGE","USE_ALPHA_TO_COVERAGE")
              .replace("float alpha = 1.0;","float alpha = opacity;").replace("gl_FragColor.a *= opacity;\n","")
              .replace("max(max(uCadLineDepthBias,0.5*fwidth(lineDepth)),vCadLineSupportDepthBias)","max(uCadLineDepthBias,0.5*fwidth(lineDepth))");};
            m.customProgramCacheKey=()=>m.userData.acceptanceKey()+"/"+mode;m.needsUpdate=true;
          }
          return (await frame()).calls;
        },mode);
        screenshots[mode]=await page.screenshot({path:path.join(output,`${name}-${state}-${projection}-${dpr}-${mode}.png`)});
        if(mode==="current"){
          await page.evaluate(async()=>{await(window as any).edgeHarness.frame()});
          expect((await page.screenshot()).equals(screenshots[mode]),"stationary highlight is stable").toBe(true);
        }
      }
      expect(calls.current).toBe(calls['prior-coverage']);
      const coverage=await page.evaluate(async({images,points,state,dpr})=>{
        const arrays=[];
        for(const src of images){const image=new Image();image.src=src;await image.decode();const canvas=document.createElement('canvas');canvas.width=image.width;canvas.height=image.height;const c=canvas.getContext('2d')!;c.drawImage(image,0,0);arrays.push({data:c.getImageData(0,0,canvas.width,canvas.height).data,width:canvas.width})}
        const samples=[],pixelScale=arrays[0].width/1280;
        for(const [x,y] of points){const sums=arrays.map(({data,width})=>{let sum=0;for(let dx=-2*pixelScale;dx<=2*pixelScale;dx++)for(let dy=-2*pixelScale;dy<=2*pixelScale;dy++){
          const i=((Math.round(y*pixelScale)+dy)*width+Math.round(x*pixelScale)+dx)*4,R=data[i],G=data[i+1],B=data[i+2];sum+=state==='selected'?Math.max(0,R-B):Math.max(0,Math.min(G,B)-R);
        }return sum});if(sums[2]>20)samples.push([sums[0]/sums[2],sums[1]/sums[2]])}
        const quantile=(index:number,p:number)=>samples.map(s=>s[index]).sort((a,b)=>a-b)[Math.floor(samples.length*p)];
        return {samples:samples.length,priorP10:quantile(0,.1),p10:quantile(1,.1),priorMedian:quantile(0,.5),median:quantile(1,.5)};
      },{images:['prior-coverage','current','depth-reference'].map(key=>'data:image/png;base64,'+screenshots[key].toString('base64')),points:report.points,state,dpr});
      const diagnostic=JSON.stringify({name,state,projection,dpr,coverage});
      expect(coverage.samples,diagnostic).toBeGreaterThan(10);
      expect(coverage.p10,diagnostic).toBeGreaterThanOrEqual(Math.min(coverage.priorP10+.02,.98));
      expect(coverage.p10,diagnostic).toBeGreaterThanOrEqual(coverage.priorP10-.01);
      // A separate foreground entity must occlude both highlighters. This
      // compares complete pixels, rather than merely checking depthTest=true.
      await page.evaluate(async()=>{
        const h=(window as any).edgeHarness,{engine:e,THREE:T,frame}=h,b=[...e.solidBindings.values()][0] as any;
        for(const line of [...e.selectedOverlays,...e.preselectedOverlays]){const m=line.material;m.onBeforeCompile=m.userData.acceptanceHook;m.customProgramCacheKey=m.userData.acceptanceKey;m.depthTest=true;m.alphaToCoverage=false;m.needsUpdate=true}
        const box=new T.Box3().setFromObject(b.mesh),scale=box.getSize(new T.Vector3()).length(),center=box.getCenter(new T.Vector3()),direction=e.camera.position.clone().sub(center).normalize();
        const cover=new T.Mesh(new T.BoxGeometry(scale*2,scale*2,scale*.05),e.materials.surface(0x718294));cover.position.copy(center).addScaledVector(direction,scale*.7);cover.quaternion.copy(e.camera.quaternion);e.content.add(cover);e.refreshContentBounds();h.highlightCover=cover;await frame();
      });
      const occluded=await page.screenshot();
      await page.evaluate(async()=>{const {engine:e,frame}=(window as any).edgeHarness;for(const line of [...e.selectedOverlays,...e.preselectedOverlays])line.visible=false;await frame()});
      expect((await page.screenshot()).equals(occluded),'foreground surface still hides highlighted geometry').toBe(true);
      await page.evaluate(async()=>{const h=(window as any).edgeHarness,c=h.highlightCover;c.removeFromParent();c.geometry.dispose();c.material.dispose();h.engine.refreshContentBounds();for(const line of [...h.engine.selectedOverlays,...h.engine.preselectedOverlays])line.visible=true;await h.frame()});
      reports.push({name,digest,projection,dpr,state,overlays:report.overlays,bytes:report.bytes,calls,coverage,deviceDpr:report.deviceDpr,rendererDpr:report.rendererDpr,drawingBuffer:report.drawingBuffer});
    }
  }
  fs.writeFileSync(path.join(output,"highlights.json"),JSON.stringify({conditions:"Native OCCT fixtures through VisualRepository and viewport; same camera, geometry, lighting and material. Depth-reference intentionally exposes hidden edges for diagnosis only.",reports,errors},null,2));
  expect(errors).toEqual([]);
});
