import * as THREE from "three";
import {groundLevel,type SceneSettings} from './scene-settings';

export type GroundFootprint = { center: THREE.Vector2; halfSize: number };
export type SceneGridDisplay = { ground:boolean; world:boolean; sketch:boolean; height:number; center:[number,number]; halfSize:number; sketchGridColor:number };

/** A large, origin-anchored viewing plane, initialized once per document. */
export function groundFootprint(bounds: THREE.Box3, spacing: number): GroundFootprint {
  const step = Number.isFinite(spacing) && spacing > 0 ? spacing : 10;
  const reach=bounds.isEmpty()?0:Math.max(...bounds.min.toArray().map(Math.abs),...bounds.max.toArray().map(Math.abs));
  const required=Math.max(10000,step*1000,reach*4);
  return {center:new THREE.Vector2(),halfSize:10**Math.ceil(Math.log10(required))};
}

/** Display placement is independent of subsequent modeling/motion bounds. */
export class SceneGroundPlacement {
  private documentId?:string;
  private initialHeight?:number;
  private footprint?:GroundFootprint;
  resetHeight():void {this.initialHeight=undefined;}
  resolve(documentId:string,bounds:THREE.Box3,settings:SceneSettings):{height:number;modelHeight:number;footprint:GroundFootprint} {
    if(this.documentId!==documentId){this.documentId=documentId;this.initialHeight=undefined;this.footprint=undefined;}
    const min=bounds.isEmpty()?undefined:bounds.min.z;
    const modelHeight=groundLevel({...settings,groundMode:'AUTO',groundLocked:false,groundOffset:0},min,bounds.getSize(new THREE.Vector3()));
    if(!bounds.isEmpty()){
      this.initialHeight??=modelHeight;
      this.footprint??=groundFootprint(bounds,settings.gridSpacing);
    }
    const height=settings.groundMode==='AUTO'&&!settings.groundLocked?(this.initialHeight??modelHeight)+settings.groundOffset:groundLevel(settings,min);
    return {height,modelHeight,footprint:this.footprint??groundFootprint(new THREE.Box3(),settings.gridSpacing)};
  }
}

/** Only sketch geometry contributes: labels, screen-sized handles and constraints do not. */
export function includeVisibleSketchBounds(bounds: THREE.Box3, content: THREE.Object3D): void {
  content.updateWorldMatrix(true, true);
  content.traverseVisible(object => {
    const data = object.userData;
    if (data.kind !== "visual" || !data.featureId || !["POINT", "CURVE"].includes(data.visualType)) return;
    const geometry = (object as THREE.Mesh).geometry;
    if (!geometry) return;
    if (!geometry.boundingBox) geometry.computeBoundingBox();
    if (geometry.boundingBox) bounds.union(geometry.boundingBox.clone().applyMatrix4(object.matrixWorld));
  });
}

export function sceneGroundVisible(environment: boolean, sceneGrid: boolean, sketchEditing: boolean): boolean {
  return environment && sceneGrid && !sketchEditing;
}
