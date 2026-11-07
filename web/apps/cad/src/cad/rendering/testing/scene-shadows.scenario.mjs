import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const T=await server.ssrLoadModule('three');
 const {ScenePresentation,shadowReceiverBounds}=await server.ssrLoadModule('/src/cad/rendering/scene-presentation.ts');
 const {SceneGroundPlacement}=await server.ssrLoadModule('/src/cad/rendering/scene-ground.ts');
 const {DEFAULT_SCENE_SETTINGS:defaults}=await server.ssrLoadModule('/src/cad/rendering/scene-settings.ts');
 const scene=new T.Scene(),lights=new T.Group(),environment=new T.Group(),renderer={shadowMap:{}};
 lights.add(new T.HemisphereLight(),new T.DirectionalLight(),new T.DirectionalLight(),new T.DirectionalLight());
 scene.add(lights,environment);
 const presentation=new ScenePresentation(scene,lights,environment,renderer),placement=new SceneGroundPlacement();
 const bounds=new T.Box3(new T.Vector3(0,0,0),new T.Vector3(100,60,30));
 const ground=placement.resolve('doc',bounds,defaults);
 presentation.update(defaults,bounds,ground.height);
 assert.equal(renderer.shadowMap.enabled,true);assert.equal(environment.children[0].visible,true);
 assert.equal(lights.children[1].castShadow,true);assert.equal(lights.children[1].shadow.intensity,defaults.shadowOpacity);
 const floor=environment.children[0],position=floor.position.clone(),scale=floor.scale.clone(),lightPosition=lights.children[1].position.clone();
 assert(floor.scale.x<ground.footprint.halfSize&&floor.scale.y<ground.footprint.halfSize,'receiver does not inherit the large grid footprint');
 presentation.update({...defaults,gridSpacing:1000},bounds,ground.height);
 assert.deepEqual(floor.position,position);assert.deepEqual(floor.scale,scale,'grid spacing cannot resize the shadow receiver');
 bounds.translate(new T.Vector3(200,-300,400));bounds.expandByScalar(200);
 const moved=placement.resolve('doc',bounds,defaults);
 presentation.update({...defaults,groundMode:'OFF'},bounds,moved.height);
 assert.equal(renderer.shadowMap.enabled,true,'object shadows remain available with the ground hidden');
 assert.equal(floor.visible,false);assert.equal(floor.position.z,position.z,'ground height remains fixed');
 assert.notDeepEqual(floor.position,position,'receiver follows the actual light projection');assert.notDeepEqual(floor.scale,scale);
 assert.deepEqual(moved.footprint,ground.footprint,'the viewing grid remains fixed while geometry moves');
 assert.notDeepEqual(lights.children[1].position,lightPosition,'shadow coverage follows displayed geometry independently');
 presentation.update(defaults,bounds,moved.height,true);
 assert.equal(renderer.shadowMap.enabled,false,'sketch editing disables shadows');assert.equal(floor.visible,false);
 presentation.update({...defaults,shadows:false},bounds,moved.height);
 assert.equal(renderer.shadowMap.enabled,false);assert.equal(lights.children[1].castShadow,false);
 presentation.update(defaults,new T.Box3(),moved.height);
 assert.equal(renderer.shadowMap.enabled,false,'empty scenes do not allocate shadow passes');
 const direction=new T.Vector3(3,4,5).normalize(),box=new T.Box3(new T.Vector3(0,0,0),new T.Vector3(10,10,10));
 const receiver=shadowReceiverBounds(box,0,direction,0,.5);
 assert.deepEqual(receiver.min.toArray(),[-6,-8]);assert.deepEqual(receiver.max.toArray(),[10,10],'covers the light-ray projection, not only the Body footprint');
 const crossing=shadowReceiverBounds(new T.Box3(new T.Vector3(0,0,-5),new T.Vector3(10,10,5)),0,direction,0,.5);
 assert.deepEqual(crossing.min.toArray(),[-3,-4]);assert.deepEqual(crossing.max.toArray(),[10,10],'below-ground geometry cannot extend the projection backwards');
 assert.equal(shadowReceiverBounds(box,11,direction,1,1),undefined,'geometry below the viewing plane cannot cast onto it');
 assert.equal(shadowReceiverBounds(box,0,new T.Vector3(1,0,0),1,1),undefined);
 for(const factor of [1e-4,1,1e6])for(const elevation of [5,30,90]){
  const b=new T.Box3(box.min.clone().multiplyScalar(factor),box.max.clone().multiplyScalar(factor)),height=-factor,light=new T.Vector3(Math.cos(elevation*Math.PI/180),0,Math.sin(elevation*Math.PI/180)),margin=factor*.2;
  const r=shadowReceiverBounds(b,height,light,margin,factor*.03);
  for(let i=0;i<8;i++){const p=new T.Vector3(i&1?b.max.x:b.min.x,i&2?b.max.y:b.min.y,i&4?b.max.z:b.min.z),distance=(p.z-height)/light.z;
   const xy=new T.Vector2(p.x-light.x*distance,p.y-light.y*distance);assert(r.containsPoint(xy));assert(r.containsPoint(xy.clone().addScalar(margin*.99)));assert(r.containsPoint(xy.clone().addScalar(-margin*.99)));
  }
 }
 presentation.update(defaults,box,11);assert.equal(floor.visible,false);assert.equal(renderer.shadowMap.enabled,true,'object shadows do not depend on a receiving ground');
 presentation.dispose();
} finally {await server.close();}
