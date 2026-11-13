import * as THREE from "three";
import type { SolidDisplaySettings } from "./display-settings";

/** Apply only to a solid's primitives, leaving its parent/tree visibility intact.
 * Wireframe uses CAD edges, not the triangles of the display tessellation. */
export function applySolidDisplaySettings(group: THREE.Group, settings: SolidDisplaySettings, topologyPick?:'VERTEX'|'EDGE'): void {
  const curve=group.userData.geometryDomain==='WIRE_SET',point=group.userData.geometryDomain==='POINT_SET',sheet=group.userData.geometryDomain==='SHEET_SET'||group.userData.geometryDomain==='MIXED_SET';
  for (const object of group.children) {
    if(object.userData.cadWideEdges){
      object.visible=curve||topologyPick==='EDGE'||settings.mode==="wireframe"||(group.userData.appearanceEdges??settings.edges);
      for(const material of Array.isArray((object as THREE.Mesh).material)?(object as THREE.Mesh).material as THREE.Material[]:[(object as THREE.Mesh).material as THREE.Material]){
        material.depthTest=settings.mode!=="wireframe";
        const uniforms=(material as import('three/addons/lines/LineMaterial.js').LineMaterial).uniforms;
        if(uniforms?.uCadEdgeNatural){
          uniforms.uCadEdgeNatural.value=!curve&&settings.mode!=='wireframe'&&topologyPick!=='EDGE'&&settings.edgeStyle!=='uniform'&&group.userData.edgePresentation!=='uniform'?1:0;
          uniforms.uCadEdgeOpacity.value.set(settings.smoothEdgeOpacity??.12,.88);
          uniforms.uCadEdgeAngles.value.set(Math.cos(5*Math.PI/180),Math.cos((settings.sharpEdgeAngle??35)*Math.PI/180));
        }
      }
    }
    else if (object instanceof THREE.Mesh) {
      object.visible = settings.mode !== "wireframe";
      // A retained construction sheet can coincide with a thickened Body.
      // Bias only its display surface so the shared skin does not z-fight.
      if(sheet)for(const material of Array.isArray(object.material)?object.material:[object.material]){
        material.side=THREE.DoubleSide;material.polygonOffset=true;material.polygonOffsetFactor=1;material.polygonOffsetUnits=1;
      }
    }
    else if (object instanceof THREE.LineSegments) {
      object.visible = curve||topologyPick==='EDGE' || settings.mode === "wireframe" || (group.userData.appearanceEdges??settings.edges);
      for (const material of Array.isArray(object.material) ? object.material : [object.material]) {
        material.depthTest = settings.mode !== "wireframe";
      }
    } else if (object instanceof THREE.Points && !object.userData.cadTopologyOverlay) {
      // Rendering and pickability differ: a hidden marker is still hoverable,
      // but hiding its owning body/occurrence still removes it from picking.
      object.visible = true;
      for (const material of Array.isArray(object.material) ? object.material : [object.material]) material.visible = point||settings.vertices;
    }
  }
}
