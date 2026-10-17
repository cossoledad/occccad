import {LineSegments2} from "three/addons/lines/LineSegments2.js";
import {LineSegmentsGeometry} from "three/addons/lines/LineSegmentsGeometry.js";
import {LineMaterial} from "three/addons/lines/LineMaterial.js";
import {registerScreenLineUpdate} from "./screen-space-lines";
import * as THREE from "three";
import type { GraphicProperties, ResolvedAppearance, SurfaceStyle } from "../../types";

export function scopedSurfaceAppearance(appearance:ResolvedAppearance|undefined,geometryKey:string):ResolvedAppearance|undefined {
  return appearance&&appearance.geometryKey!==geometryKey?{...appearance,faces:undefined,faceProperties:undefined,faceInherit:undefined}:appearance;
}

// Keep index order intact: faceIndex remains the exact GLB triangle locator.
// A palette entry includes all surface coefficients, including explicit zero.
export function surfaceAppearanceMaterials(
  geometry: THREE.BufferGeometry, faceIds: readonly number[], appearance: ResolvedAppearance | undefined,
  defaultColor: number, create: (color: number, properties?:GraphicProperties) => THREE.Material,
): THREE.Material | THREE.Material[] {
  geometry.clearGroups();
  const base = appearance?.color ? Number.parseInt(appearance.color.slice(1), 16) : defaultColor;
  const properties:GraphicProperties={opacity:appearance?.opacity,roughness:appearance?.roughness,metalness:appearance?.metalness,pickable:appearance?.pickable};
  if (!Object.keys(appearance?.faces??{}).length&&!Object.keys(appearance?.faceProperties??{}).length) return create(base,properties);
  const styles:Array<{color:number;properties:GraphicProperties}>=[], indices = new Map<string, number>();
  let previous = -1;
  faceIds.forEach((id, triangle) => {
    const value = appearance?.faces?.[id];
    const color = value ? Number.parseInt(value.slice(1), 16) : base;
    const style={...properties,...appearance?.faceProperties?.[id]};
    const key=JSON.stringify([color,style.opacity,style.roughness,style.metalness,style.pickable]);
    let material = indices.get(key);
    if (material === undefined) { material = styles.length; styles.push({color,properties:style}); indices.set(key, material); }
    if (previous === material) geometry.groups.at(-1)!.count += 3;
    else { geometry.addGroup(triangle * 3, 3, material); previous = material; }
  });
  return styles.map(s=>create(s.color,s.properties));
}

export function materialAllowsPick(material:THREE.Material|THREE.Material[],materialIndex=0):boolean {
  const m=Array.isArray(material)?material[materialIndex]:material;
  return Boolean(m&&m.opacity>0&&m.userData.cadPickable!==false);
}

export function applyEdgeAppearance(group:THREE.Group,appearance:SurfaceStyle|undefined,defaultColor:number,configureLine?:(material:LineMaterial)=>void):void {
  group.userData.appearanceEdges=appearance?.edges;
  group.userData.appearancePickable=appearance?.pickable;
  const opacity=appearance?.edgeOpacityMode==="CLEAR"?1:appearance?.opacity??1;
  const width=appearance?.edgeWidth??1;
  let wide=group.children.find(c=>c.userData.cadWideEdges) as LineSegments2|undefined;
  const custom=Math.abs(width-1)>1e-6;
  if(wide&&!custom){group.remove(wide);wide.geometry.dispose();wide.material.dispose();wide=undefined}
  if(custom&&!wide){
    const source=group.children.find(c=>c instanceof THREE.LineSegments) as THREE.LineSegments|undefined;
    const positions=source?.geometry.getAttribute("position");
    if(positions){
      const material=new LineMaterial({color:defaultColor,linewidth:width,transparent:true,depthTest:true,depthWrite:false});
      material.userData.cadEdgeAppearance=true;configureLine?.(material);
      wide=new LineSegments2(new LineSegmentsGeometry().setPositions(Array.from(positions.array)),material);
      wide.userData.cadWideEdges=true;wide.raycast=()=>{};group.add(wide);
      registerScreenLineUpdate(wide,(_camera,w,h)=>material.resolution.set(w,h));
    }
  }
  if(wide){
    wide.material.color.set(appearance?.edgeColor??defaultColor);wide.material.linewidth=width;wide.material.opacity=opacity;
    wide.material.userData.baseColor=wide.material.color.getHex();wide.material.userData.baseLinewidth=width;wide.material.userData.baseOpacity=opacity;
  }
  for(const child of group.children){
    if(!(child instanceof THREE.LineSegments||child instanceof THREE.Points))continue;
    for(const material of Array.isArray(child.material)?child.material:[child.material]){
      if(material instanceof THREE.ShaderMaterial){
        if(material.uniforms.uOpacity)material.uniforms.uOpacity.value=custom&&child instanceof THREE.LineSegments?0:opacity;
        if(material.uniforms.uColor&&child instanceof THREE.LineSegments)material.uniforms.uColor.value.set(appearance?.edgeColor??defaultColor);
      }
      material.userData.cadPickable=appearance?.pickable!==false&&opacity>0;
    }
  }
}
