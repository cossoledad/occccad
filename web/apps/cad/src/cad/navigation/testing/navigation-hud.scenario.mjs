import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const {sphereCursorVertices,pivotMarkerGeometry,SPHERE_CURSOR_SPAN:span,SPHERE_CURSOR_SEGMENTS:segments}=await server.ssrLoadModule("/src/cad/navigation/hud/navigation-hud-geometry.ts");
 const length=positions=>{let total=0;for(let i=0;i<positions.length;i+=6)total+=Math.hypot(positions[i+3]-positions[i],positions[i+4]-positions[i+1]);return total};
 for(const radius of [120,288,480,737])for(const [u,v] of [[0,0],[.9,0],[0,.9],[.65,.65],[1,0],[.707,.707],[1.3,1.2]]){
  const vertices=sphereCursorVertices(u*radius,v*radius,radius);
  assert.equal(vertices.length,segments*12);assert.ok(vertices.every(Number.isFinite));
  for(const arc of [vertices.slice(0,segments*6),vertices.slice(segments*6)]){
   assert.ok(Math.abs(length(arc)-span)<.01,"screen arc length stays constant, including the rim and outside");
   const middle=(segments/2)*6;assert.ok(Math.abs(arc[middle]-u*radius)<1e-9&&Math.abs(arc[middle+1]-v*radius)<1e-9,"cross stays under pointer");
  }
 }
 const top=sphereCursorVertices(0,250,288),right=sphereCursorVertices(250,0,288);
 assert.ok(top[1]<250-1,"latitude bends towards sphere centre");
 assert.ok(right[segments*6]<250-1,"longitude bends towards sphere centre");
 const before=sphereCursorVertices(288-.00001,0,288),after=sphereCursorVertices(288+.00001,0,288);
 assert.ok(Math.max(...before.map((v,i)=>Math.abs(v-after[i])))<.0001,"curvature is continuous across sphere rim");
 const geometry=pivotMarkerGeometry(),p=geometry.attributes.position,c=geometry.attributes.color;
 assert.ok(p.getY(7)-p.getY(4)<p.getX(3)-p.getX(0),"Y is shorter than X");
 assert.ok(c.getX(0)<c.getX(2)&&c.getX(4)<c.getX(6),"negative X/Y halves are darker");
 for(let i=10;i<18;i+=2){assert.equal(p.getX(i+1),22);assert.ok(p.getX(i)<22,"arrow wings point to positive X")}
 for(let i=18;i<26;i+=2){assert.equal(p.getY(i+1),17);assert.ok(p.getY(i)<17,"arrow wings point to positive Y")}
 geometry.dispose();
 const {navigationCursor}=await server.ssrLoadModule("/src/cad/navigation/navigation-cursor.ts");
 assert.equal(navigationCursor({action:"none"}),"");
 for(const action of ["orbit","zoom","pan","roll"])for(const light of [false,true]){
  const cursor=navigationCursor({action},light),svg=decodeURIComponent(cursor.match(/data:image\/svg\+xml,([^\"]+)/)[1]);
  assert.equal((svg.match(/<g /g)||[]).length,1,"no duplicate halo stroke");
  assert.ok(!svg.includes('stroke="white"'));assert.ok(svg.includes(light?"#253b50":"#f3bd71"));assert.ok(cursor.endsWith("16 16, move"));
 }
}finally{await server.close()}
