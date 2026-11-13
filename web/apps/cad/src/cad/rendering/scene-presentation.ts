import * as THREE from "three";
import type { SceneSettings } from "./scene-settings";
import { CATIA_VISUAL_THEME } from "./cad-visual-theme";

/** Only geometry above the viewing plane can cast onto it. Cover its projection
 * along the light, with a filter guard and outward-rounded world-space edges.
 * The large viewing grid has its own fixed footprint; no camera enters here. */
export function shadowReceiverBounds(bounds:THREE.Box3,height:number,direction:THREE.Vector3,margin:number,step:number):THREE.Box2|undefined {
  if(bounds.isEmpty()||bounds.max.z<height||direction.z<=1e-6)return;
  const projected=new THREE.Box2(),minimumZ=Math.max(bounds.min.z,height);
  for(let i=0;i<8;i++){
    const z=i&4?bounds.max.z:minimumZ,distance=(z-height)/direction.z;
    projected.expandByPoint(new THREE.Vector2((i&1?bounds.max.x:bounds.min.x)-direction.x*distance,(i&2?bounds.max.y:bounds.min.y)-direction.y*distance));
  }
  projected.expandByScalar(margin);
  for(const axis of ['x','y'] as const){projected.min[axis]=Math.floor(projected.min[axis]/step)*step;projected.max[axis]=Math.ceil(projected.max[axis]/step)*step;}
  return [...projected.min.toArray(),...projected.max.toArray()].every(Number.isFinite)?projected:undefined;
}

/** Owns viewing illumination and a shadow receiver, never modeling geometry. */
export class ScenePresentation {
  private readonly floor = new THREE.Mesh(new THREE.PlaneGeometry(1,1),new THREE.ShadowMaterial({opacity:.18,depthWrite:false}));
  private readonly hemisphere:THREE.HemisphereLight;
  private readonly key:THREE.DirectionalLight;
  private readonly fill:THREE.DirectionalLight;
  private readonly rim:THREE.DirectionalLight;
  private settings?:SceneSettings;
  private height=0;
  private editing=false;
  private shadowDefinition='';
  private cacheShadows=true;
  private epoch=0;
  private readonly restoreShadow=()=>this.requestShadowUpdate();
  private readonly casters=new Map<THREE.Mesh,{epoch:number;geometry:THREE.BufferGeometry;position:unknown;index:unknown;positionVersion:number;indexVersion:number;drawRange:string;material:string;matrix:Float64Array}>();
  constructor(private readonly scene:THREE.Scene,lights:THREE.Group,environment:THREE.Group,private readonly renderer:THREE.WebGLRenderer){
    this.hemisphere=lights.children[0] as THREE.HemisphereLight;
    [this.key,this.fill,this.rim]=lights.children.slice(1) as THREE.DirectionalLight[];
    this.floor.name="场景地面";this.floor.receiveShadow=true;this.floor.renderOrder=-10;
    this.floor.userData.sceneEnvironment=true;environment.add(this.floor);
    this.key.shadow.mapSize.set(2048,2048);this.key.shadow.bias=-.00045;
    this.key.shadow.radius=3;
    this.renderer.shadowMap.type=THREE.PCFShadowMap;
    this.renderer.shadowMap.autoUpdate=false;
    this.key.shadow.autoUpdate=false;
    this.scene.add(this.key.target);
    this.renderer.domElement?.addEventListener("webglcontextrestored",this.restoreShadow);
  }
  update(settings:SceneSettings,bounds:THREE.Box3,height:number,sketchEditing=false):void {
    this.settings=settings;this.height=height;this.editing=sketchEditing;
    const theme=CATIA_VISUAL_THEME;
    this.hemisphere.intensity=theme.hemisphereIntensity*settings.ambient*settings.brightness;
    this.key.intensity=theme.keyIntensity*settings.brightness;
    this.fill.intensity=theme.fillIntensity*settings.fill*settings.brightness;
    this.rim.intensity=theme.rimIntensity*settings.brightness;
    this.scene.environmentIntensity=settings.reflection*settings.brightness;
    const center=bounds.isEmpty()?new THREE.Vector3():bounds.getCenter(new THREE.Vector3());
    const radius=bounds.isEmpty()?100:Math.max(.001,bounds.getSize(new THREE.Vector3()).length()*.5);
    const a=THREE.MathUtils.degToRad(settings.azimuth),e=THREE.MathUtils.degToRad(settings.elevation);
    const direction=new THREE.Vector3(Math.cos(a)*Math.cos(e),Math.sin(a)*Math.cos(e),Math.sin(e));
    this.key.target.position.copy(center);this.key.position.copy(center).addScaledVector(direction,radius*3);
    const shadows=!sketchEditing&&settings.shadows&&!bounds.isEmpty();
    this.floor.material.opacity=.55;
    this.key.shadow.intensity=settings.shadowOpacity;
    this.renderer.shadowMap.enabled=shadows;this.key.castShadow=shadows;
    const camera=this.key.shadow.camera;
    camera.left=camera.bottom=-radius*1.15;camera.right=camera.top=radius*1.15;camera.near=Math.max(radius*.001,.00001);camera.far=radius*6;camera.updateProjectionMatrix();
    this.key.shadow.normalBias=radius/2048*2;
    const texel=radius*2.3/this.key.shadow.mapSize.x;
    const margin=(texel*(this.key.shadow.radius+2)+this.key.shadow.normalBias+Math.abs(this.key.shadow.bias)*(camera.far-camera.near))/direction.z;
    const receiverHeight=height-1e-5;
    const receiver=shadowReceiverBounds(bounds,receiverHeight,direction,margin,texel);
    this.floor.visible=shadows&&settings.groundMode!=="OFF"&&!!receiver;
    if(receiver){const center=receiver.getCenter(new THREE.Vector2()),size=receiver.getSize(new THREE.Vector2());this.floor.position.set(center.x,center.y,receiverHeight);this.floor.scale.set(size.x,size.y,1);}
    // Camera, background colour, exposure and shadow strength do not change
    // directional-light depth. Only the definition of that depth map does.
    const definition=JSON.stringify([shadows,center.toArray(),radius,direction.toArray(),height,this.key.shadow.mapSize.toArray(),this.key.shadow.bias,this.key.shadow.normalBias,this.key.shadow.radius]);
    if(definition!==this.shadowDefinition){this.shadowDefinition=definition;this.requestShadowUpdate();}
  }
  private requestShadowUpdate():void {
    this.renderer.shadowMap.needsUpdate=true;this.key.shadow.needsUpdate=true;
  }

  /** A controlled rendering comparison, not a model or viewing preference. */
  setShadowCaching(enabled:boolean):void {this.cacheShadows=enabled;this.requestShadowUpdate();}

  /** Check object state, never vertices/edges. This also covers animated poses,
   * visibility, previews and clipping without trusting every UI event to dirty
   * a shadow. The framebuffer and camera are deliberately absent from the key. */
  prepareShadows(camera:THREE.Camera):boolean {
    if(!this.settings?.shadows||this.editing){this.casters.clear();return false;}
    let changed=false;const epoch=++this.epoch;
    this.scene.traverseVisible(object=>{
      if(!(object instanceof THREE.Mesh)||!object.castShadow||!object.layers.test(camera.layers))return;
      const materials:THREE.Material[]=Array.isArray(object.material)?object.material:[object.material];
      if(!materials.some(m=>m.visible&&m.opacity>0&&m.userData.cadMaterial==='surface'))return;
      const geometry=object.geometry,position=geometry.getAttribute('position'),index=geometry.index;
      const positionVersion=position instanceof THREE.InterleavedBufferAttribute?position.data.version:position?.version??0;
      const indexVersion=index?.version??0,drawRange=`${geometry.drawRange.start}:${geometry.drawRange.count}`;
      const material=materials.map(m=>[m.visible,m.opacity,m.side,m.shadowSide,m.alphaTest,m.clipShadows,m.clipIntersection,...(m.clippingPlanes??[]).flatMap(p=>[...p.normal.toArray(),p.constant])].join(',')).join(';');
      let state=this.casters.get(object);
      if(!state){state={epoch,geometry,position,index,positionVersion,indexVersion,drawRange,material,matrix:Float64Array.from(object.matrixWorld.elements)};this.casters.set(object,state);changed=true;return;}
      state.epoch=epoch;
      if(state.geometry!==geometry||state.position!==position||state.index!==index||state.positionVersion!==positionVersion||state.indexVersion!==indexVersion||state.drawRange!==drawRange||state.material!==material||object.matrixWorld.elements.some((v,i)=>v!==state!.matrix[i])){
        if(state.position!==position||state.positionVersion!==positionVersion)geometry.computeBoundingBox();
        Object.assign(state,{geometry,position,index,positionVersion,indexVersion,drawRange,material});state.matrix.set(object.matrixWorld.elements);changed=true;
      }
    });
    for(const [mesh,state] of this.casters)if(state.epoch!==epoch){this.casters.delete(mesh);changed=true;}
    if(changed){
      const bounds=new THREE.Box3(),box=new THREE.Box3();
      this.scene.traverseVisible(object=>{
        if(!(object instanceof THREE.Mesh))return;
        const materials:THREE.Material[]=Array.isArray(object.material)?object.material:[object.material];
        if(!materials.some(m=>m.visible&&m.opacity>0&&m.userData.cadMaterial==='surface'))return;
        if(!object.geometry.boundingBox)object.geometry.computeBoundingBox();
        if(object.geometry.boundingBox)bounds.union(box.copy(object.geometry.boundingBox).applyMatrix4(object.matrixWorld));
      });
      this.update(this.settings,bounds,this.height,this.editing);
    }
    if(changed||!this.cacheShadows)this.requestShadowUpdate();
    return this.renderer.shadowMap.enabled&&this.renderer.shadowMap.needsUpdate;
  }
  dispose():void {this.renderer.domElement?.removeEventListener("webglcontextrestored",this.restoreShadow);this.casters.clear();this.key.shadow.dispose();this.scene.remove(this.key.target);}
}
