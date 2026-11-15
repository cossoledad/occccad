import { expect, test } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";

test("curved highlight joins retain coverage across colours, widths and pixel ratios", async ({ page }, info) => {
  const errors: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  page.on("console", message => { if (/THREE.WebGLProgram|Shader Error/.test(message.text())) errors.push(message.text()); });
  await page.goto("/");
  const results = await page.evaluate(async () => {
    // @ts-ignore Production modules; Vite owns resolution and its scoped cache.
    const { T, LineSegments2, LineSegmentsGeometry, LineMaterial, makeHighlightLineMaterial, SurfaceDepthPass } = await import("/browser/fixtures/highlight-joins.ts");
    const host = document.createElement("main"); host.style.cssText = "position:absolute;top:0;left:0;right:0;min-height:100%;z-index:10000;background:#e9eef3;padding:12px;display:grid;grid-template-columns:repeat(4,320px);gap:12px;align-content:start"; document.body.style.overflow="auto"; document.body.append(host);
    const results = [], size = 320, radius = 106.4;
    for (const dpr of [1, 2]) {
      const renderer = new T.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true });
      renderer.setPixelRatio(dpr); renderer.setSize(size, size); renderer.outputColorSpace = T.SRGBColorSpace;
      const camera = new T.OrthographicCamera(-size/2, size/2, size/2, -size/2, .1, 100); camera.position.z = 10; camera.updateMatrixWorld(true);
      const pass = new SurfaceDepthPass(), gl = renderer.getContext();
      for (const [background, color] of [["#f4f5f7", 0xeaa02d], ["#17232f", 0xeaa02d], ["#4f7195", 0x45c6d4]] as const) for (const width of [1.75, 2.25]) {
        const samples: Record<string, number[]> = {};
        for (const legacy of [true, false]) {
          const scene = new T.Scene(); scene.background = new T.Color(background);
          // The legacy reference uses the previous material recipe exactly:
          // transparent blending AND alphaToCoverage, without the new helper.
          const material = legacy ? new LineMaterial({ color, linewidth: width, worldUnits: false, transparent: true, opacity: .96, depthTest: false, depthWrite: false, toneMapped: false }) : makeHighlightLineMaterial(color, width);
          material.userData.cadContour = true; pass.configureLine(material); material.alphaToCoverage = legacy;
          material.resolution.set(size, size);
          const positions = [], points = [];
          // Deliberately short segments expose cap derivatives at tessellated bends.
          for (let i = 0; i <= 192; i++) {
            const angle = -.3 + i / 192 * Math.PI * 1.7;
            points.push([Math.cos(angle)*radius+.37, Math.sin(angle)*radius+.23, 0]);
          }
          for (let i=1; i<points.length; i++) positions.push(...points[i-1], ...points[i]);
          const geometry = new LineSegmentsGeometry().setPositions(positions), line = new LineSegments2(geometry, material); scene.add(line);
          renderer.render(scene, camera);
          const pixels = new Uint8Array(size*size*dpr*dpr*4); gl.readPixels(0,0,size*dpr,size*dpr,gl.RGBA,gl.UNSIGNED_BYTE,pixels);
          const bg = [parseInt(background.slice(1,3),16),parseInt(background.slice(3,5),16),parseInt(background.slice(5,7),16)], fg = [color>>16,(color>>8)&255,color&255];
          const vector = fg.map((v,i)=>v-bg[i]), denominator = vector.reduce((a,v)=>a+v*v,0), strength = [];
          for (const point of points.slice(2,-2)) {
            const x=(point[0]+size/2)*dpr, y=(point[1]+size/2)*dpr; let best=0;
            for(let dy=-dpr;dy<=dpr;dy++)for(let dx=-dpr;dx<=dpr;dx++){
              const offset=(Math.floor(y+dy)*size*dpr+Math.floor(x+dx))*4;
              best=Math.max(best,vector.reduce((a,v,i)=>a+(pixels[offset+i]-bg[i])*v,0)/denominator);
            }
            strength.push(best);
          }
          samples[legacy?"legacy":"current"]=strength.sort((a,b)=>a-b);
          const figure=document.createElement("figure");figure.style.cssText="margin:0;font:12px system-ui;color:#253849";
          const image=new Image();image.src=renderer.domElement.toDataURL();image.style.cssText="width:320px;height:320px;display:block";figure.append(image,`${legacy?"原方案":"本次"} · DPR ${dpr} · ${width} px · ${background}`);host.append(figure);
          geometry.dispose(); material.dispose();
        }
        results.push({dpr,background,width,legacyMinimum:samples.legacy[0],minimum:samples.current[0],legacyP10:samples.legacy[Math.floor(samples.legacy.length*.1)],p10:samples.current[Math.floor(samples.current.length*.1)]});
      }
      pass.dispose(); renderer.dispose();
    }
    return results;
  });
  const output = path.resolve("../../../build/agent-work/browser-polish"); fs.mkdirSync(output, { recursive: true });
  fs.writeFileSync(path.join(output, "highlight-joins.json"), JSON.stringify({ conditions: "Chromium SwiftShader, 192 short curved segments; nearest centreline pixel strength, not a recognition or GPU performance measurement", results, errors }, null, 2));
  await page.screenshot({ path: info.outputPath("highlight-joins.png"), fullPage: true });
  for (const result of results) {
    expect(result.minimum, JSON.stringify(result)).toBeGreaterThan(.65);
    expect(result.p10, JSON.stringify(result)).toBeGreaterThanOrEqual(result.legacyP10 - .02);
  }
  expect(errors).toEqual([]);
});
