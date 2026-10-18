import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const T=await server.ssrLoadModule("three");
 const {groundFootprint,includeVisibleSketchBounds,sceneGroundVisible}=await server.ssrLoadModule("/src/cad/rendering/scene-ground.ts");
 const {DEFAULT_SCENE_SETTINGS,sceneBackgroundIsLight,colorLuminance}=await server.ssrLoadModule("/src/cad/rendering/scene-settings.ts");
 const {sceneSketchColors,SketchColorAdapter}=await server.ssrLoadModule("/src/cad/rendering/sketch-colors.ts");
 const {PlaneGrid}=await server.ssrLoadModule("/src/cad/rendering/plane-grid.ts");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const content=new T.Group(),occurrence=new T.Group();content.add(occurrence);
 occurrence.position.set(100,200,-50);occurrence.rotation.x=Math.PI/2;
 const curve=new T.Line(new T.BufferGeometry().setFromPoints([new T.Vector3(-20,-10,0),new T.Vector3(20,10,0)]));
 curve.userData={kind:"visual",visualType:"CURVE",featureId:"sketch",sketchFeatureID:"sketch"};occurrence.add(curve);
 const label=new T.Mesh(new T.PlaneGeometry(10000,10000));label.userData={kind:"sketch-constraint",featureId:"sketch"};occurrence.add(label);
 let bounds=new T.Box3();includeVisibleSketchBounds(bounds,content);
 assert.deepEqual(bounds.min.toArray(),[80,200,-60]);assert.deepEqual(bounds.max.toArray(),[120,200,-40]);
 occurrence.visible=false;bounds.makeEmpty();includeVisibleSketchBounds(bounds,content);assert(bounds.isEmpty());occurrence.visible=true;
 const engine=Object.create(CadViewportEngine.prototype);
 Object.assign(engine,{content:new T.Group(),helpers:content,groundBounds:new T.Box3(),solidBindings:new Map(),sceneSettings:{...DEFAULT_SCENE_SETTINGS},groundFrame:{origin:new T.Vector3(),normal:new T.Vector3(0,0,1),u:new T.Vector3(1,0,0),v:new T.Vector3(0,1,0)},groundGrid:new PlaneGrid(0xffffff),worldGrid:new PlaneGrid(0xffffff),scenePresentation:{update(...args){engine.presentation=args}}});
 engine.refreshSceneGround();assert(engine.groundHeight<-60,"a sketch-only scene positions its floor below the sketch");
 const footprint=groundFootprint(engine.groundBounds,10);assert.deepEqual(footprint.center.toArray(),[100,200]);assert(footprint.halfSize>=80);
 for(const [origin,size,spacing] of [[[45,-5,0],[100,10,80],10],[[-73,-128,0],[17,32,5],3],[[1e9+23,-2e9+17,0],[15,3,6],.2],[[0,0,0],[100,100,100],10]]){
  const box=new T.Box3().setFromCenterAndSize(new T.Vector3(...origin),new T.Vector3(...size));
  const square=groundFootprint(box,spacing),major=spacing*10;
  const originalHalf=Math.ceil(Math.max(...size,spacing*5)*2/spacing)*spacing;
  const limits=[square.center.x-square.halfSize,square.center.x+square.halfSize,square.center.y-square.halfSize,square.center.y+square.halfSize];
  for(const edge of limits)assert(Math.abs(edge/major-Math.round(edge/major))<1e-6,'every edge ends on a document-origin major grid line');
  assert(limits[0]<=origin[0]-originalHalf&&limits[1]>=origin[0]+originalHalf&&limits[2]<=origin[1]-originalHalf&&limits[3]>=origin[1]+originalHalf,'alignment only expands the original footprint');
  const cells=Math.max(Math.ceil((origin[0]+originalHalf)/major)-Math.floor((origin[0]-originalHalf)/major),Math.ceil((origin[1]+originalHalf)/major)-Math.floor((origin[1]-originalHalf)/major));
  assert.equal(square.halfSize*2,cells*major,'the aligned square is no larger than needed');
 }
 const empty=groundFootprint(new T.Box3(),10);assert.deepEqual(empty.center.toArray(),[0,0]);assert.equal(empty.halfSize,100);
 const camera=new T.PerspectiveCamera(35,1,.001,1e6);camera.up.set(0,0,1);
 for(const distance of [1,100,1e4]){
  camera.position.set(100,200-distance,200);camera.lookAt(100,200,-50);camera.updateProjectionMatrix();
  engine.groundGrid.update(camera,600,engine.groundFrame);
  const u=engine.groundGrid.object.material.uniforms;
  assert.equal(u.uSpacing.value,10);assert.equal(u.uBounds.value.z-u.uBounds.value.x,footprint.halfSize*2,"zoom does not resize the square");
 }
 engine.sketchPlane={};engine.refreshSceneGround();assert.equal(engine.presentation.at(-1),true,"the shadow receiver also hides while editing");
 assert.equal(sceneGroundVisible(true,true,true),false);assert.equal(sceneGroundVisible(true,true,false),true);assert.equal(sceneGroundVisible(false,true,false),false);
 for(const background of ["#ffffff","#f2f4f8","#dce5ef"])assert(sceneBackgroundIsLight({...DEFAULT_SCENE_SETTINGS,background:"SOLID",backgroundTop:background}));
 const dark=sceneSketchColors(DEFAULT_SCENE_SETTINGS),light=sceneSketchColors({...DEFAULT_SCENE_SETTINGS,background:"SOLID",backgroundTop:"#ffffff"});
 const contrast=(color,background)=>{const a=colorLuminance('#'+color.toString(16).padStart(6,'0')),b=colorLuminance(background);return (Math.max(a,b)+.05)/(Math.min(a,b)+.05)};
 for(const role of ["profile","construction","external","solved","invalid","redundant","dimension","reference","grid"])assert(contrast(light[role],"#ffffff")>=3,`${role} has a visible light-background counterpart`);
 const adapter=new SketchColorAdapter(),root=new T.Group();root.userData.sketchFeatureID="sketch";
 const material=new T.LineBasicMaterial({color:dark.solved});material.userData.baseColor=dark.solved;curve.material=material;root.add(curve);
 const surface=new T.Mesh(new T.BoxGeometry(),new T.MeshStandardMaterial({color:dark.solved}));root.add(surface);
 adapter.applyOwned(root,light);assert.equal(material.color.getHex(),light.solved);assert.equal(material.userData.baseColor,light.solved);assert.equal(surface.material.color.getHex(),dark.solved,"user surface colours are preserved");
 material.color.setHex(light.selected);adapter.applyOwned(root,dark);assert.equal(material.color.getHex(),dark.selected);assert.equal(material.userData.baseColor,dark.solved,"palette changes preserve semantic state and selection");
 engine.groundGrid.dispose();engine.worldGrid.dispose();
} finally {await server.close();}
