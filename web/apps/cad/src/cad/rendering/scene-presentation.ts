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
  constructor(private readonly scene:THREE.Scene,lights:THREE.Group,environment:THREE.Group,private readonly renderer:THREE.WebGLRenderer){
    this.hemisphere=lights.children[0] as THREE.HemisphereLight;
    [this.key,this.fill,this.rim]=lights.children.slice(1) as THREE.DirectionalLight[];
    this.floor.name="场景地面";this.floor.receiveShadow=true;this.floor.renderOrder=-10;
    this.floor.userData.sceneEnvironment=true;environment.add(this.floor);
    this.key.shadow.mapSize.set(2048,2048);this.key.shadow.bias=-.00045;
    this.key.shadow.radius=3;
    this.renderer.shadowMap.type=THREE.PCFShadowMap;
    this.scene.add(this.key.target);
  }
  update(settings:SceneSettings,bounds:THREE.Box3,height:number,sketchEditing=false):void {
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
    this.floor.material.opacity=.4;
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
    this.key.shadow.needsUpdate=true;
  }
  dispose():void {this.key.shadow.dispose();this.scene.remove(this.key.target);}
}
