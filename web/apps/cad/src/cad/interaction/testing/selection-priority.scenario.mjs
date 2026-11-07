import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule('three');
 const {SelectionIndex}=await server.ssrLoadModule('/src/cad/interaction/selection-index.ts');
 const context={documentId:'part',versionId:'r1',bodyId:'body',geometryKey:'shape',occurrencePath:'a'};
 const entries=[['face',20],['edge',40],['vertex',50],['curve',70],['point',75]].map(([kind,priority])=>({
  root:new T.Object3D(),priority,distance:100,
  selection:kind==='curve'||kind==='point'?{...context,kind:'visual',id:kind,featureId:'sketch',entityId:kind,visualType:kind==='point'?'POINT':'CURVE'}:{...context,kind,id:kind,topologyId:1},
  domain:kind==='curve'||kind==='point'?'sketch':undefined,
 }));
 const ray={intersectObjects:roots=>entries.filter(e=>roots.includes(e.root)).map(e=>({object:e.root,distance:e.distance,point:new T.Vector3()}))};
 const policy={preferSolidTopology:true,depthTolerance:.05};
 for(const order of [entries,[...entries].reverse(),[entries[3],entries[0],entries[4],entries[2],entries[1]]]){
  const index=new SelectionIndex();for(const e of order)index.registerPick(e.root,()=>e.selection,e.priority,0,e.domain);
  assert.equal(index.pick(ray).id,'point','sketch editing retains its normal priorities');
  assert.deepEqual(index.pickAll(ray,()=>true,10,policy).map(h=>h.selection.id),['vertex','edge','point','curve','face']);
  assert.equal(index.pick(ray,s=>s.kind==='visual',policy).id,'point','explicit sketch-only command still finds the sketch');
  assert.equal(index.pick(ray,s=>s.kind!=='vertex',policy).id,'edge');
  const hit=index.pickWithIntersection(ray,s=>s.kind==='edge',policy);assert.equal(hit.selection,entries[1].selection);assert.equal(hit.intersection.object,entries[1].root,'retain the original exact hit and occurrence');
  entries[4].distance=99;assert.equal(index.pick(ray,()=>true,policy).id,'point','priority never jumps over a separate front depth band');entries[4].distance=100;
  for(const scale of [.001,1000]){entries.forEach(e=>e.distance=100*scale);assert.equal(index.pick(ray,()=>true,{...policy,depthTolerance:.05*scale}).kind,'vertex');}entries.forEach(e=>e.distance=100);
  const parent=new T.Group();parent.add(entries[1].root,entries[2].root);parent.visible=false;assert.equal(index.pick(ray,()=>true,policy).id,'point','hidden owner excludes solid candidates');parent.visible=true;
 }
 const annotation=new T.Object3D(),annotated=new SelectionIndex();for(const e of entries)annotated.registerPick(e.root,()=>e.selection,e.priority,0,e.domain);
 const glyph={...context,kind:'sketch-constraint',id:'corner-coincidence',featureId:'sketch',constraintId:'corner'};annotated.registerPick(annotation,()=>glyph,90);
 const annotatedRay={intersectObjects:roots=>[...ray.intersectObjects(roots),{object:annotation,distance:100}]};assert.equal(annotated.pick(annotatedRay,()=>true,policy).kind,'vertex');assert.equal(annotated.pick(annotatedRay).kind,'sketch-constraint','active sketch glyph retains its original priority');
 // Anchored depth bands avoid the old pairwise comparator's non-transitivity.
 const index=new SelectionIndex(),chain=entries.slice(0,3);chain.forEach((e,i)=>{e.distance=100+i*.6;index.registerPick(e.root,()=>e.selection,e.priority)});
 assert.deepEqual(index.pickAll(ray,()=>true,10).map(h=>h.selection.id),['edge','face','vertex']);
 const handle=new T.Object3D();index.registerPick(handle,()=>({...context,kind:'body',id:'handle'}),0,10);
 const layers={intersectObjects:roots=>[...ray.intersectObjects(roots),{object:handle,distance:200}]};assert.equal(index.pick(layers,()=>true,policy).id,'handle','explicit interaction layers retain precedence');
}finally{await server.close()}
