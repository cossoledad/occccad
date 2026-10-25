import * as THREE from "three";
import { colorNumber, palette } from "../../design/visual-tokens";
import { sceneBackgroundIsLight, type SceneSettings } from "./scene-settings";

// Same semantic roles on both backgrounds; solver states never change meaning.
const dark = {
  profile:palette.sketch, construction:palette.construction, external:palette.external,
  solved:palette.solved, invalid:palette.invalid, redundant:palette.redundant,
  dimension:palette.dimensionText, reference:palette.dimensionReference, grid:palette.sketchGrid,
  x:palette.axisX, y:palette.axisY, z:palette.axisZ, vertex:palette.vertex,
  selected:palette.selected, hover:palette.hover, preview:palette.preview, snap:palette.snap,
  pointHalo:palette.sketchPointHalo,
};
const light:Record<keyof typeof dark,string> = {
  profile:"#20384f", construction:"#526d8b", external:"#076b86",
  solved:"#146344", invalid:"#b7203f", redundant:"#7936a0",
  dimension:"#6e5016", reference:"#56627a", grid:"#526f87",
  x:"#b5283e", y:"#1b6d45", z:"#2e56bb", vertex:"#243a51",
  selected:"#9b5600", hover:"#007386", preview:"#5344bd", snap:"#986000",
  pointHalo:"#f7fbff",
};
export type SketchColors = Record<keyof typeof dark,number>;
export function sceneSketchColors(settings:SceneSettings):SketchColors {
  return Object.fromEntries(Object.entries(sceneBackgroundIsLight(settings)?light:dark).map(([key,color])=>[key,colorNumber(color)])) as SketchColors;
}

/** Adapt only sketch-owned materials; solid/user appearance colors remain authoritative. */
export class SketchColorAdapter {
  private readonly applied = new WeakMap<THREE.Material,SketchColors>();
  private readonly roles=new Map<number,keyof typeof dark>();
  constructor(){
    for(const colors of [dark,light])for(const [role,color] of Object.entries(colors))this.roles.set(colorNumber(color),role as keyof typeof dark);
  }
  apply(root:THREE.Object3D,colors:SketchColors):void {root.traverse(object=>this.applyObject(object,colors));}
  applyOwned(root:THREE.Object3D,colors:SketchColors):void {
    const visit=(object:THREE.Object3D,inherited:boolean)=>{
      const data=object.userData;
      const owned=inherited||Boolean(data.sketchFeatureID)||data.kind==="sketch-constraint";
      if(owned)this.applyObject(object,colors);
      for(const child of object.children)visit(child,owned);
    };
    visit(root,false);
  }
  applyObject(object:THREE.Object3D,colors:SketchColors):void {
    const material=(object as THREE.Mesh).material;
    for(const m of Array.isArray(material)?material:material?[material]:[]){
      if(this.applied.get(m)===colors||m instanceof THREE.MeshStandardMaterial)continue;
      const convert=(value:number)=>{const role=this.roles.get(value);return role?colors[role]:value;};
      if("color" in m&&m.color instanceof THREE.Color)m.color.setHex(convert(m.color.getHex()));
      for(const key of ["baseColor","dimensionDiagnosticColor"])if(typeof m.userData[key]==="number")m.userData[key]=convert(m.userData[key]);
      if(m instanceof THREE.ShaderMaterial)for(const key of ["uColor","uSelectedColor","uCenterColor"]){const value=m.uniforms[key]?.value;if(value instanceof THREE.Color)value.setHex(convert(value.getHex()));}
      this.applied.set(m,colors);
    }
  }
}
