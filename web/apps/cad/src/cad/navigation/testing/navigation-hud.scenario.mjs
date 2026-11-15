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
   for(let i=0;i<arc.length;i+=3)assert.ok(Math.hypot(arc[i],arc[i+1])<=radius+1e-9,"the whole cross stays inside the sphere disk");
  }
  const factor=1/Math.max(1,Math.hypot(u,v)),middle=(segments/2)*6;
  assert.ok(Math.abs(vertices[middle]-u*radius*factor)<1e-9&&Math.abs(vertices[middle+1]-v*radius*factor)<1e-9,"anchor clamps to the trackball rim");
  if(Math.hypot(u,v)>=1)for(let i=0;i<segments*6;i+=3)
   assert.ok(Math.abs(Math.hypot(vertices[i],vertices[i+1])-radius)<1e-9,"one arm coincides with the outer circle at every rim angle");
 }
 const top=sphereCursorVertices(0,250,288),right=sphereCursorVertices(250,0,288);
 assert.ok(top[1]<250-.1,"latitude bends towards sphere centre");
 assert.ok(right[segments*6]<250-1,"longitude bends towards sphere centre");
 const before=sphereCursorVertices(288-.00001,0,288),after=sphereCursorVertices(288+.00001,0,288);
 assert.ok(Math.max(...before.map((v,i)=>Math.abs(v-after[i])))<.0001,"curvature is continuous across sphere rim");
 const geometry=pivotMarkerGeometry(),p=geometry.attributes.position,c=geometry.attributes.color;
 assert.ok(p.getY(7)-p.getY(4)<p.getX(3)-p.getX(0),"Y is shorter than X");
 assert.ok(c.getX(0)<c.getX(2)&&c.getX(4)<c.getX(6),"negative X/Y halves are darker");
 assert.equal(p.count,10,"only the gapped axes remain, without arrowheads");
 assert.notDeepEqual([c.getX(2),c.getY(2),c.getZ(2)],[c.getX(6),c.getY(6),c.getZ(6)],"positive X and Y have distinct colours");
 geometry.dispose();
 const {navigationCursor}=await server.ssrLoadModule("/src/cad/navigation/navigation-cursor.ts");
 const {palette}=await server.ssrLoadModule("/src/design/visual-tokens.ts");
 const {builtinIcons}=await server.ssrLoadModule("/src/cad/overlay/builtin-icons.ts");
 const {iconSvgElements}=await server.ssrLoadModule("/src/cad/overlay/icon-elements.ts");
 assert.equal(navigationCursor({action:"none"}),"");
 for(const action of ["orbit","zoom","pan","roll"])for(const light of [false,true]){
  const cursor=navigationCursor({action},light),svg=decodeURIComponent(cursor.match(/data:image\/svg\+xml,([^\"]+)/)[1]);
  assert.equal((svg.match(/<g /g)||[]).length,1,"no duplicate halo stroke");
  assert.ok(!svg.includes('stroke="white"'));assert.ok(svg.includes(light?palette.text:palette.chromeText));
  assert.ok(svg.includes(iconSvgElements(builtinIcons[`navigation-${action}`],light?palette.primary:palette.axisZ)),"cursor artwork comes from the shared glyph catalog");
  assert.ok(cursor.endsWith(`16 16, ${action==='pan'?'move':action==='zoom'?'ns-resize':'grabbing'}`));
 }
 const reference={};assert.ok(navigationCursor({action:'none',solidworks:{reference}}).endsWith(', grab'));
 assert.ok(navigationCursor({action:'orbit',solidworks:{reference}}).endsWith(', grabbing'),'ready and active cursor caches stay distinct');
 const {NavigationHUD}=await server.ssrLoadModule('/src/cad/navigation/hud/navigation-hud.ts');
 const T=await server.ssrLoadModule('three'),hud=new NavigationHUD(),camera=new T.PerspectiveCamera();camera.position.z=100;camera.updateMatrixWorld(true);
 let draws=0;const renderer={clearDepth(){},render(){draws++}};
 const circle=hud.circle.geometry.attributes.position;
 const initial=circle.version;hud.update({action:'none'},camera,800,600);hud.render(renderer);
 assert.equal(draws,0,'idle HUD adds no empty scene draw');assert.equal(circle.version,initial,'idle frames do not regenerate the sphere');
 const active={action:'orbit',catia:{showPivot:true,showRotationCircle:true,hudVisible:true,pointer:{x:700,y:300}}};
 hud.update(active,camera,800,600,true);hud.render(renderer);assert.equal(draws,1);
 const baked=circle.version;hud.update(active,camera,800,600,true);assert.equal(circle.version,baked,'sphere buffers and dash distances are cached by radius');
 const scale=hud.pivotMarker.scale.x;hud.update(active,camera,1600,1200,true);assert.equal(hud.pivotMarker.scale.x,scale,'centre marker keeps a fixed CSS pixel size');
 hud.dispose();
}finally{await server.close()}
