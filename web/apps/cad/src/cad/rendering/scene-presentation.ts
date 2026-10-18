import * as THREE from "three";
import type { SceneSettings } from "./scene-settings";
import { CATIA_VISUAL_THEME } from "./cad-visual-theme";
import { groundFootprint } from "./scene-ground";

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
    this.key.shadow.mapSize.set(1024,1024);this.key.shadow.bias=-.0002;
    this.renderer.shadowMap.type=THREE.PCFSoftShadowMap;
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
    const extent=bounds.isEmpty()?100:Math.max(1,bounds.getSize(new THREE.Vector3()).length());
    const a=THREE.MathUtils.degToRad(settings.azimuth),e=THREE.MathUtils.degToRad(settings.elevation);
    const direction=new THREE.Vector3(Math.cos(a)*Math.cos(e),Math.sin(a)*Math.cos(e),Math.sin(e));
    this.key.target.position.copy(center);this.key.position.copy(center).addScaledVector(direction,extent*2);
    const footprint=groundFootprint(bounds,settings.gridSpacing);
    this.floor.position.set(footprint.center.x,footprint.center.y,height-Math.max(extent*1e-6,1e-5));this.floor.scale.setScalar(footprint.halfSize*2);
    this.floor.visible=!sketchEditing&&settings.groundMode!=="OFF"&&settings.shadows&&!bounds.isEmpty();
    this.floor.material.opacity=settings.shadowOpacity;
    this.renderer.shadowMap.enabled=this.floor.visible;this.key.castShadow=this.floor.visible;
    const camera=this.key.shadow.camera;
    camera.left=camera.bottom=-extent*1.5;camera.right=camera.top=extent*1.5;camera.near=Math.max(extent*.001,.001);camera.far=extent*6;camera.updateProjectionMatrix();
    this.key.shadow.needsUpdate=true;
  }
  dispose():void {this.key.shadow.dispose();this.scene.remove(this.key.target);}
}
