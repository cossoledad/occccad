import {LineSegments2} from "three/addons/lines/LineSegments2.js";
import {LineSegmentsGeometry} from "three/addons/lines/LineSegmentsGeometry.js";
import {LineMaterial} from "three/addons/lines/LineMaterial.js";
import {registerScreenLineUpdate,updateScreenDashDistances} from "./screen-space-lines";
import * as THREE from "three";
import type { GraphicProperties, ResolvedAppearance, SurfaceStyle } from "../../types";
import {CATIA_VISUAL_THEME} from './cad-visual-theme';
import {acquireEdgeGeometry} from './natural-edges';

export function scopedSurfaceAppearance(appearance:ResolvedAppearance|undefined,geometryKey:string):ResolvedAppearance|undefined {
  return appearance&&appearance.geometryKey!==geometryKey?{...appearance,faces:undefined,faceProperties:undefined,faceInherit:undefined}:appearance;
}

// Keep index order intact: faceIndex remains the exact GLB triangle locator.
// A palette entry includes all surface coefficients, including explicit zero.
export function surfaceAppearanceMaterials(
  geometry: THREE.BufferGeometry, faceIds: readonly number[] | Uint32Array, appearance: ResolvedAppearance | undefined,
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
  const engineering=group.userData.cadEngineeringCurve||group.userData.cadEngineeringPoint;
  const primitiveDefault=group.userData.cadEngineeringPoint?CATIA_VISUAL_THEME.engineeringPoint:defaultColor;
  const primitiveColor=engineering?appearance?.color??primitiveDefault:defaultColor;
  const edgeColor=engineering?primitiveColor:appearance?.edgeColor??primitiveColor;
  group.userData.appearanceEdges=appearance?.edges;
  group.userData.appearancePickable=appearance?.pickable;
  const opacity=group.userData.cadEngineeringCurve||group.userData.cadEngineeringPoint?appearance?.opacity??1:appearance?.edgeOpacityMode==="CLEAR"?1:appearance?.opacity??1;
  const width=appearance?.edgeWidth??(group.userData.cadEngineeringCurve?2.1:1.35);
  let wide=group.children.find(c=>c.userData.cadWideEdges) as LineSegments2|undefined;
  const source=group.children.find(c=>c instanceof THREE.LineSegments) as THREE.LineSegments|undefined;
  const data=source?.geometry.userData.cadMesh as import('../../types').MeshData|undefined;
  const shared=group.userData.edgePresentation==='natural'&&appearance?.lineStyle!=='DASHED';
  // CAD edges keep CSS-pixel width at DPR 1/2. The original lines remain the
  // stable picking primitives; their transparent shader does not draw twice.
  if(!wide){
    const positions=source?.geometry.getAttribute("position");
    if(positions){
      const material=new LineMaterial({color:defaultColor,linewidth:width,transparent:true,depthTest:true,depthWrite:false});
      material.userData.cadEdgeAppearance=true;configureLine?.(material);
      const lease=data?acquireEdgeGeometry(data,shared):undefined;
      const geometry=lease?.geometry??new LineSegmentsGeometry().setPositions(Array.from(positions.array));
      wide=new LineSegments2(geometry,material);
      wide.renderOrder=1;
      wide.userData.cadWideEdges=true;wide.raycast=()=>{};group.add(wide);
      wide.userData.cadGeometryRelease=lease?.release;wide.userData.cadEdgeShared=shared;
      registerScreenLineUpdate(wide,(camera,w,h)=>{material.resolution.set(w,h);if(material.dashed)updateScreenDashDistances(wide!,camera,w,h)});
    }
  }
  if(wide){
    if(data&&wide.userData.cadEdgeShared!==shared){
      if(wide.userData.cadGeometryRelease)wide.userData.cadGeometryRelease();else wide.geometry.dispose();
      const lease=acquireEdgeGeometry(data,shared);wide.geometry=lease.geometry;wide.userData.cadGeometryRelease=lease.release;wide.userData.cadEdgeShared=shared;
    }
    if(wide.material.uniforms.uCadEdgeExplicitColor)wide.material.uniforms.uCadEdgeExplicitColor.value=appearance?.edgeColor!==undefined?1:0;
    if(wide.material.uniforms.uCadEdgeNatural)wide.material.uniforms.uCadEdgeNatural.value=engineering?0:1;
    wide.material.color.set(edgeColor);wide.material.linewidth=width;wide.material.opacity=opacity;
    wide.material.userData.baseColor=wide.material.color.getHex();wide.material.userData.baseLinewidth=width;wide.material.userData.baseOpacity=opacity;
    const dashed=appearance?.lineStyle==='DASHED';if(wide.material.dashed!==dashed){wide.material.dashed=dashed;wide.material.needsUpdate=true;wide.computeLineDistances()}
    wide.material.dashSize=4;wide.material.gapSize=3;
  }
  for(const child of group.children){
    if(!(child instanceof THREE.LineSegments||child instanceof THREE.Points))continue;
    for(const material of Array.isArray(child.material)?child.material:[child.material]){
      if(material instanceof THREE.ShaderMaterial){
        if(child instanceof THREE.LineSegments)material.visible=!(wide&&group.userData.edgePresentation==='natural');
        if(material.uniforms.uOpacity)material.uniforms.uOpacity.value=wide&&child instanceof THREE.LineSegments?0:opacity;
        if(material.uniforms.uColor&&(child instanceof THREE.LineSegments||group.userData.cadEngineeringPoint))material.uniforms.uColor.value.set(edgeColor);
        if(child instanceof THREE.Points&&group.userData.cadEngineeringPoint)applyPointAppearance(material,appearance);
      }
      material.userData.cadPickable=appearance?.pickable!==false&&opacity>0;
    }
  }
}

export function applyPointAppearance(material:THREE.ShaderMaterial,appearance:SurfaceStyle|undefined):void {
 if(material.uniforms.uPointSize){const size=appearance?.pointSize??11;material.userData.cssPointSize=size;material.uniforms.uPointSize.value=size}
 if(material.uniforms.uMarkerShape)material.uniforms.uMarkerShape.value=appearance?.pointSymbol==='DOT'?2:appearance?.pointSymbol==='PLUS'?1:3;
}

export function applyReferenceAppearance(group:THREE.Object3D,appearance:SurfaceStyle|undefined):void {
 group.userData.appearancePickable=appearance?.pickable!==false&&(appearance?.opacity??1)>0;
 group.traverse(child=>{
  const primitive=child as THREE.Mesh;
  if(!(primitive.isMesh||(child as THREE.Line).isLine||(child as THREE.Points).isPoints))return;
  for(const material of Array.isArray(primitive.material)?primitive.material:[primitive.material]){
   const shader=(material as THREE.ShaderMaterial).isShaderMaterial?material as THREE.ShaderMaterial:undefined;
   const line=(material as LineMaterial).isLineMaterial?material as LineMaterial:undefined;
   const colored=material as THREE.Material&{color?:THREE.Color};
   if(!material.userData.referenceBase){material.userData.referenceBase={color:shader?.uniforms.uColor?.value?.getHex()??colored.color?.getHex(),opacity:shader?.uniforms.uOpacity?.value??material.opacity,transparent:material.transparent,width:line?.linewidth,dashed:line?.dashed??false}}
   const base=material.userData.referenceBase,color=appearance?.color??base.color,opacity=(appearance?.opacity??1)*base.opacity;
   if(shader?.uniforms.uColor)shader.uniforms.uColor.value.set(color);else if(color!==undefined)colored.color?.set(color);
   if(shader?.uniforms.uOpacity)shader.uniforms.uOpacity.value=opacity;else {material.opacity=opacity;material.transparent=base.transparent||opacity<1}
   if(color!==undefined)material.userData.baseColor=colored.color?.getHex()??shader?.uniforms.uColor?.value?.getHex();
   material.userData.cadPickable=appearance?.pickable!==false;
   if(line){
    line.linewidth=appearance?.edgeWidth??base.width;
    const dashed=appearance?.lineStyle?appearance.lineStyle==='DASHED':base.dashed;
    if(line.dashed!==dashed){line.dashed=dashed;material.needsUpdate=true;if((child as LineSegments2).isLineSegments2)(child as LineSegments2).computeLineDistances()}
    material.userData.baseLinewidth=line.linewidth;material.userData.baseOpacity=opacity;
   }
   if(shader&&(child as THREE.Points).isPoints)applyPointAppearance(shader,appearance);
  }
 });
}
