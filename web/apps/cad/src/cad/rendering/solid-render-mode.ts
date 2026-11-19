import * as THREE from "three";
import type { SolidDisplaySettings } from "./display-settings";
import { NATURAL_EDGE_DEFAULTS } from "./natural-edges";

/** Apply only to a solid's primitives, leaving its parent/tree visibility intact.
 * Wireframe uses CAD edges, not the triangles of the display tessellation. */
export function applySolidDisplaySettings(group: THREE.Group, settings: SolidDisplaySettings, topologyPick?:'VERTEX'|'EDGE'): void {
  const curve=group.userData.geometryDomain==='WIRE_SET',point=group.userData.geometryDomain==='POINT_SET',sheet=group.userData.geometryDomain==='SHEET_SET'||group.userData.geometryDomain==='MIXED_SET';
  let opaqueBounds: THREE.Box3 | null = null;
  if (group.userData.geometryDomain === 'SOLID_SET' && settings.mode === 'shaded' && topologyPick !== 'EDGE') {
    const surface = group.children.find((child): child is THREE.Mesh => child instanceof THREE.Mesh && !child.userData.cadWideEdges);
    if (surface) {
      const materials = Array.isArray(surface.material) ? surface.material : [surface.material];
      if (materials.every(material => !material.transparent && material.opacity >= 1 && material.depthWrite)) {
        if (!surface.geometry.boundingBox) surface.geometry.computeBoundingBox();
        opaqueBounds = surface.geometry.boundingBox;
      }
    }
  }
  for (const object of group.children) {
    if(object.userData.cadWideEdges){
      object.visible=curve||topologyPick==='EDGE'||settings.mode==="wireframe"||(group.userData.appearanceEdges??settings.edges);
      for(const material of Array.isArray((object as THREE.Mesh).material)?(object as THREE.Mesh).material as THREE.Material[]:[(object as THREE.Mesh).material as THREE.Material]){
        material.depthTest=settings.mode!=="wireframe";
        const uniforms=(material as import('three/addons/lines/LineMaterial.js').LineMaterial).uniforms;
        if(uniforms?.uCadEdgeNatural){
          uniforms.uCadEdgeNatural.value=!curve&&settings.mode!=='wireframe'&&topologyPick!=='EDGE'&&settings.edgeStyle!=='uniform'&&group.userData.edgePresentation!=='uniform'?1:0;
          const defaults = NATURAL_EDGE_DEFAULTS;
          const sharpOpacity = uniforms.uCadEdgeExplicitColor.value ? defaults.explicitSharpOpacity : defaults.sharpOpacity;
          uniforms.uCadEdgeOpacity.value.set(settings.smoothEdgeOpacity??defaults.smoothOpacity,sharpOpacity);
          uniforms.uCadEdgeAngles.value.set(Math.cos(defaults.smoothAngle*Math.PI/180),Math.cos((settings.sharpEdgeAngle??defaults.sharpAngle)*Math.PI/180));
          const span = uniforms.uCadEdgeSolidSpan.value as THREE.Vector3;
          if (opaqueBounds) opaqueBounds.getSize(span);
          else span.set(0,0,0);
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
