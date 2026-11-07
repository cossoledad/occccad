import * as THREE from "three";
import type {LineMaterial} from "three/addons/lines/LineMaterial.js";

type OpaqueDraw={groupOrder:number;renderOrder:number;object:THREE.Object3D;material:{id:number};z:number;id:number};
/** Preserve explicit layers; CAD opaque ties use stable occurrence/Body keys.
 * Transparent sorting, geometry and model-selection identity remain untouched. */
export function stableCadOpaqueSort(a:OpaqueDraw,b:OpaqueDraw):number {
  if(a.groupOrder!==b.groupOrder)return a.groupOrder-b.groupOrder;
  if(a.renderOrder!==b.renderOrder)return a.renderOrder-b.renderOrder;
  const ak=a.object.userData.cadSurfaceKey as string|undefined,bk=b.object.userData.cadSurfaceKey as string|undefined;
  if(ak&&bk&&ak!==bk)return ak<bk?-1:1;
  if(!!ak!==!!bk)return ak?1:-1;
  return a.material.id-b.material.id||a.z-b.z||a.id-b.id;
}

/** Eight bounded samples, only on opaque surfaces. Not a physical illumination
 * solution: the optional view aid deliberately rejects far-depth silhouettes. */
const CAD_AO_GLSL=`
uniform sampler2D uCadSurfaceDepth,uCadLinearDepth;
uniform vec2 uCadDepthSize,uCadAONearFar;
uniform bool uCadAOEnabled;
uniform float uCadAOIntensity,uCadAOScale,uCadAOPerspective,uCadAOPixelRadius;
float cadAmbientOcclusion(float center){
 if(!uCadAOEnabled)return 1.0;
 vec2 uv=gl_FragCoord.xy/uCadDepthSize;
 float radius=uCadAOScale*uCadAOPixelRadius*mix(1.0,center,uCadAOPerspective);
 vec2 gradient=vec2(dFdx(center),dFdy(center));
 float sum=0.0;
 for(int i=0;i<8;i++){
  float angle=float(i)*2.39996323,scale=0.45+0.55*float(i+1)/8.0;
  vec2 offset=floor(gl_FragCoord.xy+vec2(cos(angle),sin(angle))*scale*uCadAOPixelRadius)+0.5-gl_FragCoord.xy;
  vec2 p=uv+offset/uCadDepthSize;
  if(any(lessThan(p,vec2(0.0)))||any(greaterThan(p,vec2(1.0))))continue;
  float sampleDepth=texture2D(uCadSurfaceDepth,p).r;
  if(sampleDepth>=0.999999)continue;
  float change=dot(gradient,offset);
  float expected=mix(center+change,center/max(0.05,1.0-change/center),uCadAOPerspective);
  float sampled=mix(uCadAONearFar.x,uCadAONearFar.y,unpackRGBAToDepth(texture2D(uCadLinearDepth,p))),delta=expected-sampled;
  sum+=smoothstep(radius*0.06,radius*0.65,delta)*(1.0-smoothstep(radius,radius*2.0,abs(center-sampled)));
 }
 return 1.0-uCadAOIntensity*sum/8.0;
}`;

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
  private opaqueTarget?:THREE.WebGLRenderTarget;
  // Bounded raster-depth tie, not a model-space weld tolerance. Covers float
  // transform/interpolation differences between coincident tessellations.
  readonly opaqueUniforms={uCadOpaqueDepth:{value:this.target.depthTexture as THREE.DepthTexture|null},uCadOpaqueResolve:{value:false},uCadDepthEpsilon:{value:16/(2**24-1)}};
  readonly aoUniforms={uCadAOEnabled:{value:false},uCadAOIntensity:{value:.35},uCadAOScale:{value:1},uCadAONearFar:{value:new THREE.Vector2(.1,10000)},uCadAOPerspective:{value:1},uCadAOPixelRadius:{value:12}};
  readonly uniforms:{uCadSurfaceDepth:THREE.IUniform;uCadDepthSize:THREE.IUniform<THREE.Vector2>;uCadDepthEnabled:THREE.IUniform<boolean>};
  constructor(){
    this.target.depthTexture=new THREE.DepthTexture(1,1,THREE.UnsignedIntType);
    this.uniforms={uCadSurfaceDepth:{value:this.target.depthTexture},uCadDepthSize:{value:new THREE.Vector2(1,1)},uCadDepthEnabled:{value:false}};
    // Reuse the depth target's RGBA attachment for packed linear view depth.
    // This avoids near/far cancellation in CAD's deliberately broad clip range
    // without requiring float render-target extensions or another geometry pass.
    this.visibleMaterial.onBeforeCompile=shader=>{
      shader.uniforms.uCadAONearFar=this.aoUniforms.uCadAONearFar;
      shader.vertexShader='varying float vCadViewDepth;\n'+shader.vertexShader.replace('#include <project_vertex>','#include <project_vertex>\nvCadViewDepth=-mvPosition.z;');
      shader.fragmentShader='varying float vCadViewDepth; uniform vec2 uCadAONearFar;\n'+shader.fragmentShader.replace('gl_FragColor = vec4( vec3( 1.0 - fragCoordZ ), opacity );','gl_FragDepth=fragCoordZ; gl_FragColor=packDepthToRGBA(clamp((vCadViewDepth-uCadAONearFar.x)/(uCadAONearFar.y-uCadAONearFar.x),0.0,1.0));');
    };
    this.visibleMaterial.customProgramCacheKey=()=>'cad-linear-view-depth-v2';
  }
  setSectionPlane(enabled:boolean,plane:THREE.Plane):void {
    this.visibleMaterial.clippingPlanes=enabled?[plane]:null;this.visibleMaterial.needsUpdate=true;
  }
  configureLine(material:LineMaterial):void {
    material.onBeforeCompile=shader=>{
      Object.assign(shader.uniforms,this.uniforms);
      if(material.userData.cadContour||material.userData.cadEdgeAppearance){
        shader.uniforms.uCadLineDepthBias=this.opaqueUniforms.uCadDepthEpsilon;
        shader.vertexShader='varying vec2 vCadLineClipZW;\n'+shader.vertexShader.replace('#include <clipping_planes_vertex>','vCadLineClipZW=gl_Position.zw;\n#include <clipping_planes_vertex>');
        // A half-pixel slope allowance keeps the stroked edge on its own face
        // without turning hidden edges into an x-ray overlay.
        shader.fragmentShader='varying vec2 vCadLineClipZW; uniform float uCadLineDepthBias;\n'+shader.fragmentShader.replace('#include <clipping_planes_fragment>',`#include <clipping_planes_fragment>
          float lineDepth=0.5*vCadLineClipZW.x/vCadLineClipZW.y+0.5;
          gl_FragDepth=max(0.0,lineDepth-max(uCadLineDepthBias,0.5*fwidth(lineDepth)));`);
      }
      if(material.userData.cadEdgeAppearance||material.userData.cadSilhouette)shader.vertexShader='attribute vec3 instanceEdgeNormalA,instanceEdgeNormalB; varying float vCadContour;\n'+shader.vertexShader.replace('void main() {',`void main() {
        vec3 a=normalMatrix*instanceEdgeNormalA,b=normalMatrix*instanceEdgeNormalB;
        vec3 eye=projectionMatrix[2][3]==-1.0?normalize(-(modelViewMatrix*vec4(instanceStart,1.0)).xyz):vec3(0.0,0.0,1.0);
        float fa=dot(a,eye),fb=dot(b,eye);
        vCadContour=${material.userData.cadSilhouette?'length(b)<0.1?1.0:step(fa*fb,0.0)':'length(a)<0.1?0.65:(fa*fb<0.0?1.0:1.0-smoothstep(0.0,0.45,min(abs(fa),abs(fb))))'};`);
      shader.fragmentShader=(material.userData.cadEdgeAppearance||material.userData.cadSilhouette?'varying float vCadContour;\n':'')+EDGE_OCCLUSION_GLSL+shader.fragmentShader.replace("gl_FragColor = vec4( diffuseColor.rgb, alpha );",`gl_FragColor = vec4( diffuseColor.rgb, alpha * ${material.userData.cadContour?'1.0':'cadEdgeVisibility()'} ${material.userData.cadSilhouette?'* vCadContour':material.userData.cadEdgeAppearance?'* mix(0.40,0.90,vCadContour)':''} );`);
    };
    material.customProgramCacheKey=()=>"cad-edge-depth-facing-v4-"+Boolean(material.userData.cadEdgeAppearance)+'-'+Boolean(material.userData.cadSilhouette)+'-'+Boolean(material.userData.cadContour);
  }
  configureSurface(material:THREE.MeshStandardMaterial):void{
    material.onBeforeCompile=shader=>{
      Object.assign(shader.uniforms,this.uniforms,this.aoUniforms,this.opaqueUniforms);
      shader.uniforms.uCadLinearDepth={value:this.target.texture};
      // Match the depth pass at fragment centres; gl_FragCoord.z can differ
      // under multisampling, especially for sloped orthographic surfaces.
      shader.vertexShader='varying vec2 vCadClipZW;\n'+shader.vertexShader.replace('#include <project_vertex>','#include <project_vertex>\nvCadClipZW=gl_Position.zw;');
      shader.fragmentShader='varying vec2 vCadClipZW; uniform sampler2D uCadOpaqueDepth; uniform bool uCadOpaqueResolve; uniform float uCadDepthEpsilon;\n'+shader.fragmentShader.replace('#include <packing>','#include <packing>\n'+CAD_AO_GLSL+'\n').replace('#include <clipping_planes_fragment>',`#include <clipping_planes_fragment>
        float cadFragmentDepth=0.5*vCadClipZW.x/vCadClipZW.y+0.5;
        gl_FragDepth=cadFragmentDepth;
        if(uCadOpaqueResolve&&opacity>=1.0){
          float nearest=texture2D(uCadOpaqueDepth,gl_FragCoord.xy/uCadDepthSize).r;
          if(nearest<1.0&&abs(cadFragmentDepth-nearest)<=uCadDepthEpsilon)gl_FragDepth=nearest;
        }`).replace('#include <opaque_fragment>','outgoingLight *= cadAmbientOcclusion(vViewPosition.z);\n#include <opaque_fragment>');
    };
    material.customProgramCacheKey=()=>'cad-surface-ao-stable-depth-v3';
  }
  render(renderer:THREE.WebGLRenderer,source:THREE.Scene,camera:THREE.Camera,ao=false,intensity=.35):void {
    const meshes:THREE.Mesh[]=[];let transparent=false;
    source.updateMatrixWorld(true);
    source.traverseVisible(object=>{
      if(!(object instanceof THREE.Mesh))return;
      const materials=Array.isArray(object.material)?object.material:[object.material];
      if(!materials.some(m=>m.visible&&m.userData.cadMaterial==="surface"&&m.opacity>0))return;
      meshes.push(object);transparent ||= materials.some(m=>m.visible&&m.opacity>0&&m.opacity<1);
    });
    this.uniforms.uCadDepthEnabled.value=transparent;
    const opaque=(m:THREE.Material)=>m.visible&&m.opacity>=1&&!m.transparent;
    const resolve=meshes.filter(mesh=>(Array.isArray(mesh.material)?mesh.material:[mesh.material]).some(opaque)).length>1;
    this.opaqueUniforms.uCadOpaqueResolve.value=resolve;
    this.aoUniforms.uCadAOEnabled.value=ao&&!transparent;
    this.visibleMaterial.colorWrite=ao&&!transparent;
    this.aoUniforms.uCadAOIntensity.value=intensity;
    this.scene.clear();
    if(!transparent&&!ao&&!resolve)return;
    const size=renderer.getDrawingBufferSize(new THREE.Vector2());
    if(!size.equals(this.uniforms.uCadDepthSize.value)){this.target.setSize(size.x,size.y);this.uniforms.uCadDepthSize.value.copy(size)}
    const c=camera as THREE.PerspectiveCamera|THREE.OrthographicCamera;
    this.aoUniforms.uCadAONearFar.value.set(c.near,c.far);
    this.aoUniforms.uCadAOPerspective.value=c instanceof THREE.PerspectiveCamera?1:0;
    this.aoUniforms.uCadAOScale.value=c instanceof THREE.PerspectiveCamera?2*Math.tan(THREE.MathUtils.degToRad(c.fov/2))/c.zoom/size.y:(c.top-c.bottom)/c.zoom/size.y;
    this.aoUniforms.uCadAOPixelRadius.value=12*renderer.getPixelRatio();
    for(const mesh of meshes){
      let proxy=this.proxies.get(mesh);
      if(!proxy){proxy=new THREE.Mesh();proxy.matrixAutoUpdate=false;proxy.frustumCulled=false;this.proxies.set(mesh,proxy)}
      proxy.geometry=mesh.geometry;
      const depth=(m:THREE.Material)=>m.visible&&m.opacity>0?this.visibleMaterial:this.invisibleMaterial;
      proxy.material=Array.isArray(mesh.material)?mesh.material.map(depth):depth(mesh.material);
      proxy.matrix.copy(mesh.matrixWorld);this.scene.add(proxy);
    }
    const target=renderer.getRenderTarget(),viewport=renderer.getViewport(new THREE.Vector4()),scissor=renderer.getScissor(new THREE.Vector4()),scissorTest=renderer.getScissorTest();
    try {
      renderer.setRenderTarget(this.target);renderer.setScissorTest(false);renderer.clear(true,true,false);renderer.render(this.scene,camera);
      this.opaqueUniforms.uCadOpaqueDepth.value=this.target.depthTexture;
      if(resolve&&transparent){
        if(!this.opaqueTarget){this.opaqueTarget=new THREE.WebGLRenderTarget(1,1,{minFilter:THREE.NearestFilter,magFilter:THREE.NearestFilter,depthBuffer:true});this.opaqueTarget.depthTexture=new THREE.DepthTexture(1,1,THREE.UnsignedIntType);}
        if(this.opaqueTarget.width!==size.x||this.opaqueTarget.height!==size.y)this.opaqueTarget.setSize(size.x,size.y);
        for(const mesh of meshes){const proxy=this.proxies.get(mesh)!;const depth=(m:THREE.Material)=>opaque(m)?this.visibleMaterial:this.invisibleMaterial;proxy.material=Array.isArray(mesh.material)?mesh.material.map(depth):depth(mesh.material);}
        this.visibleMaterial.colorWrite=false;renderer.setRenderTarget(this.opaqueTarget);renderer.clear(true,true,false);renderer.render(this.scene,camera);this.opaqueUniforms.uCadOpaqueDepth.value=this.opaqueTarget.depthTexture;
      }
    }
    finally {renderer.setRenderTarget(target);renderer.setViewport(viewport);renderer.setScissor(scissor);renderer.setScissorTest(scissorTest)}
  }
  dispose():void {this.scene.clear();this.target.dispose();this.target.depthTexture?.dispose();this.opaqueTarget?.dispose();this.opaqueTarget?.depthTexture?.dispose();this.visibleMaterial.dispose();this.invisibleMaterial.dispose()}
}
