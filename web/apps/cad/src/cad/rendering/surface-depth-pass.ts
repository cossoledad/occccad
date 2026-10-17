import * as THREE from "three";
import type {LineMaterial} from "three/addons/lines/LineMaterial.js";

/** Nearest visible CAD surface, including glass; never changes the colour pass's
 * depth-write rules or the geometry shared with picking/Naming. */
export const EDGE_OCCLUSION_GLSL=`
uniform sampler2D uCadSurfaceDepth;
uniform vec2 uCadDepthSize;
uniform bool uCadDepthEnabled;
float cadEdgeVisibility() {
  if (!uCadDepthEnabled) return 1.0;
  float surfaceDepth=texture2D(uCadSurfaceDepth,gl_FragCoord.xy/uCadDepthSize).r;
  float bias=max(0.000002,2.0*fwidth(gl_FragCoord.z));
  if (gl_FragCoord.z <= surfaceDepth+bias) return 1.0;
  return mix(0.16,0.38,step(0.5,fract((gl_FragCoord.x+gl_FragCoord.y)/8.0)));
}`;

export class SurfaceDepthPass {
  private readonly target=new THREE.WebGLRenderTarget(1,1,{minFilter:THREE.NearestFilter,magFilter:THREE.NearestFilter,depthBuffer:true,stencilBuffer:false});
  private readonly scene=new THREE.Scene();
  private readonly visibleMaterial=new THREE.MeshDepthMaterial({depthPacking:THREE.BasicDepthPacking,side:THREE.DoubleSide,colorWrite:false});
  private readonly invisibleMaterial=new THREE.MeshDepthMaterial({visible:false});
  private readonly proxies=new WeakMap<THREE.Mesh,THREE.Mesh>();
  readonly uniforms:{uCadSurfaceDepth:THREE.IUniform;uCadDepthSize:THREE.IUniform<THREE.Vector2>;uCadDepthEnabled:THREE.IUniform<boolean>};
  constructor(){
    this.target.depthTexture=new THREE.DepthTexture(1,1,THREE.UnsignedIntType);
    this.uniforms={uCadSurfaceDepth:{value:this.target.depthTexture},uCadDepthSize:{value:new THREE.Vector2(1,1)},uCadDepthEnabled:{value:false}};
  }
  setSectionPlane(enabled:boolean,plane:THREE.Plane):void {
    this.visibleMaterial.clippingPlanes=enabled?[plane]:null;this.visibleMaterial.needsUpdate=true;
  }
  configureLine(material:LineMaterial):void {
    material.onBeforeCompile=shader=>{
      Object.assign(shader.uniforms,this.uniforms);
      shader.fragmentShader=EDGE_OCCLUSION_GLSL+shader.fragmentShader.replace("gl_FragColor = vec4( diffuseColor.rgb, alpha );","gl_FragColor = vec4( diffuseColor.rgb, alpha * cadEdgeVisibility() );");
    };
    material.customProgramCacheKey=()=>"cad-edge-depth-v1";
  }
  render(renderer:THREE.WebGLRenderer,source:THREE.Scene,camera:THREE.Camera):void {
    const meshes:THREE.Mesh[]=[];let transparent=false;
    source.updateMatrixWorld(true);
    source.traverseVisible(object=>{
      if(!(object instanceof THREE.Mesh))return;
      const materials=Array.isArray(object.material)?object.material:[object.material];
      if(!materials.some(m=>m.visible&&m.userData.cadMaterial==="surface"&&m.opacity>0))return;
      meshes.push(object);transparent ||= materials.some(m=>m.visible&&m.opacity>0&&m.opacity<1);
    });
    this.uniforms.uCadDepthEnabled.value=transparent;
    this.scene.clear();
    if(!transparent)return;
    const size=renderer.getDrawingBufferSize(new THREE.Vector2());
    if(!size.equals(this.uniforms.uCadDepthSize.value)){this.target.setSize(size.x,size.y);this.uniforms.uCadDepthSize.value.copy(size)}
    for(const mesh of meshes){
      let proxy=this.proxies.get(mesh);
      if(!proxy){proxy=new THREE.Mesh();proxy.matrixAutoUpdate=false;proxy.frustumCulled=false;this.proxies.set(mesh,proxy)}
      proxy.geometry=mesh.geometry;
      const depth=(m:THREE.Material)=>m.visible&&m.opacity>0?this.visibleMaterial:this.invisibleMaterial;
      proxy.material=Array.isArray(mesh.material)?mesh.material.map(depth):depth(mesh.material);
      proxy.matrix.copy(mesh.matrixWorld);this.scene.add(proxy);
    }
    const target=renderer.getRenderTarget(),viewport=renderer.getViewport(new THREE.Vector4()),scissor=renderer.getScissor(new THREE.Vector4()),scissorTest=renderer.getScissorTest();
    try {renderer.setRenderTarget(this.target);renderer.setScissorTest(false);renderer.clear(true,true,false);renderer.render(this.scene,camera)}
    finally {renderer.setRenderTarget(target);renderer.setViewport(viewport);renderer.setScissor(scissor);renderer.setScissorTest(scissorTest)}
  }
  dispose():void {this.scene.clear();this.target.dispose();this.target.depthTexture?.dispose();this.visibleMaterial.dispose();this.invisibleMaterial.dispose()}
}
