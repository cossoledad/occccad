import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule('three');
 const {CadShaderLibrary}=await server.ssrLoadModule('/src/cad/rendering/shader/cad-shader-library.ts');
 const {CadMaterialFactory}=await server.ssrLoadModule('/src/cad/rendering/cad-material-factory.ts');
 const {SketchColorAdapter,sceneSketchColors}=await server.ssrLoadModule('/src/cad/rendering/sketch-colors.ts');
 const {makeSketchOverlayLine}=await server.ssrLoadModule('/src/cad/rendering/interaction-highlight.ts');
 const {DEFAULT_SCENE_SETTINGS}=await server.ssrLoadModule('/src/cad/rendering/scene-settings.ts');
 const factory=new CadMaterialFactory(new CadShaderLibrary()),adapter=new SketchColorAdapter(),colors=sceneSketchColors(DEFAULT_SCENE_SETTINGS);
 const root=new T.Group();root.userData={sketchFeatureID:'sketch',documentId:'part',versionId:'r1',occurrencePath:'a'};
 const line=makeSketchOverlayLine([new T.Vector3(),new T.Vector3(1,0,0)],colors.profile,2.5);line.renderOrder=30;root.add(line);
 const point=new T.Points(new T.BufferGeometry(),factory.sketchPoint(colors.profile));root.add(point);
 const initial={opacity:line.material.opacity,color:line.material.color.getHex(),width:line.material.linewidth,size:point.material.userData.cssPointSize};
 const active={featureId:'sketch',documentId:'part',versionId:'r1',occurrencePath:'a'};
 adapter.applyOwned(root,colors,null);
 assert.equal(line.material.depthTest,true);assert.equal(line.material.depthWrite,false);assert(line.material.opacity<initial.opacity);assert(line.material.linewidth<initial.width);assert.equal(line.material.color.getHex(),colors.inactive);assert.equal(line.renderOrder,2);
 assert(point.material.userData.cssPointSize<initial.size);assert.equal(point.material.uniforms.uMarkerShape.value,1,'inactive rendering never replaces sketch point geometry');
 factory.setInteractionState(root,'hover');adapter.applyOwned(root,colors,null);assert.equal(line.material.color.getHex(),colors.hover);assert.equal(line.material.depthTest,true,'explicit feedback remains spatially occluded');
 factory.setInteractionState(root,'default');adapter.applyOwned(root,colors,null);assert.equal(line.material.color.getHex(),colors.inactive);
 adapter.applyOwned(root,colors,active);assert.equal(line.material.depthTest,false);assert.equal(line.material.linewidth,initial.width);assert.equal(line.material.opacity,initial.opacity);assert.equal(line.material.color.getHex(),initial.color);assert.equal(line.renderOrder,30);assert.equal(point.material.userData.cssPointSize,initial.size);
 adapter.applyOwned(root,colors,{...active,occurrencePath:'b'});assert.equal(line.material.depthTest,true,'editing one occurrence never promotes another occurrence');
 const light=sceneSketchColors({...DEFAULT_SCENE_SETTINGS,background:'SOLID',backgroundTop:'#fff'});adapter.applyOwned(root,light,null);assert.equal(line.material.color.getHex(),light.inactive);adapter.applyOwned(root,light,active);assert.equal(line.material.color.getHex(),light.profile,'theme changes retain the original semantic colour');
 const topology=factory.topologyPoint();assert.equal(topology.uniforms.uMarkerShape.value,2);assert.equal(topology.depthWrite,false);assert.equal(factory.point().uniforms.uMarkerShape.value,0,'sketch endpoint crosses remain unchanged');
}finally{await server.close()}
