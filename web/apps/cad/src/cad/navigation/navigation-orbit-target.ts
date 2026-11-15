import * as THREE from "three";
import type { MeshData, SelectionItem } from "../../types";
import { meshTopologyIndex } from "../rendering/mesh-topology-index";

export type NavigationOrbitTarget = { kind: "point" | "bounds"; point: THREE.Vector3 };
type TopologyDisplay = { data: MeshData; matrixWorld: THREE.Matrix4 };

/** Read-only displayed-element bounds, in world coordinates. Never creates a
 * selection, changes a camera or turns a display locator into model identity. */
export function selectionOrbitTarget(selection: SelectionItem, objects: readonly THREE.Object3D[],
  topology?: TopologyDisplay, sketchPoint?: THREE.Vector3): NavigationOrbitTarget | undefined {
  if(sketchPoint&&sketchPoint.toArray().every(Number.isFinite))return {kind:"point",point:sketchPoint.clone()};
  const box=new THREE.Box3(),point=new THREE.Vector3();
  const expand=(value:ArrayLike<number>)=>box.expandByPoint(point.fromArray(value).applyMatrix4(topology!.matrixWorld));
  if(topology&&selection.kind==='vertex'){
    const vertex=topology.data.topologyVertices?.find(v=>v.localId===selection.topologyId);
    if(vertex){
      const point=new THREE.Vector3().fromArray(vertex.point).applyMatrix4(topology.matrixWorld);
      if(point.toArray().every(Number.isFinite))return {kind:'point',point};
    }
    return;
  }
  if(topology&&selection.kind==='face'){
    const index=meshTopologyIndex(topology.data),positions=index.buffers.positions;
    index.forEachFaceTriangle(selection.topologyId,(a,b,c)=>{for(const vertex of [a,b,c])expand(positions.subarray(vertex*3,vertex*3+3))});
  }else if(topology&&selection.kind==='edge'){
    for(const edge of topology.data.edges??[])if(edge.localId===selection.topologyId)for(const value of edge.points)expand(value);
  }else{
    for(const root of objects)root.traverseVisible(object=>{
      if(!(object instanceof THREE.Mesh||object instanceof THREE.Line||object instanceof THREE.Points))return;
      const geometry=object.geometry;
      if(!geometry.boundingBox)geometry.computeBoundingBox();
      if(geometry.boundingBox)box.union(geometry.boundingBox.clone().applyMatrix4(object.matrixWorld));
    });
  }
  if(box.isEmpty()||![...box.min.toArray(),...box.max.toArray()].every(Number.isFinite))return;
  const isPoint=selection.kind==='datum-point'||selection.kind==='reference-point'||selection.kind==='visual'&&selection.visualType==='POINT';
  return {kind:isPoint?'point':'bounds',point:box.getCenter(new THREE.Vector3())};
}
