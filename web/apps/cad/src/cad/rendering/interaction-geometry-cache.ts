import * as THREE from 'three';
import {LineSegmentsGeometry} from 'three/addons/lines/LineSegmentsGeometry.js';
import type {MeshData} from '../../types';
import {meshTopologyIndex} from './mesh-topology-index';

type Entry={owner:WeakRef<MeshData>;key:string;geometry:THREE.BufferGeometry;bytes:number;references:number;used:number};
export type GeometryLease={geometry:THREE.BufferGeometry;release:()=>void};

/** Owns derived display buffers. Occurrences borrow geometry independently;
 * releasing one hover cannot dispose a selected sibling's GPU buffers. */
export class InteractionGeometryCache {
  private byMesh=new WeakMap<MeshData,Map<string,Entry>>();
  private readonly entries=new Set<Entry>();private clock=0;private bytes=0;
  constructor(private readonly budget=64*1024*1024){}
  acquire(mesh:MeshData,kind:'face'|'edge',ids?:ReadonlySet<number>):GeometryLease {
    const key=kind+':'+(ids?[...ids].sort((a,b)=>a-b).join(','):'whole');
    let map=this.byMesh.get(mesh);if(!map){map=new Map();this.byMesh.set(mesh,map);}
    let entry=map.get(key);
    if(!entry){
      const index=meshTopologyIndex(mesh);
      const geometry=new LineSegmentsGeometry();
      if(kind==='face'){
        const data=index.contours(ids);geometry.setPositions(data.positions);
        geometry.setAttribute('instanceEdgeNormalA',new THREE.InstancedBufferAttribute(data.first,3));
        geometry.setAttribute('instanceEdgeNormalB',new THREE.InstancedBufferAttribute(data.second,3));
      }else geometry.setPositions(index.edgeSegments(ids));
      const arrays=new Set<ArrayBufferLike>();
      for(const attribute of Object.values(geometry.attributes)){const a=attribute as THREE.InterleavedBufferAttribute;arrays.add(a.isInterleavedBufferAttribute?a.data.array.buffer:attribute.array.buffer);}
      entry={owner:new WeakRef(mesh),key,geometry,bytes:[...arrays].reduce((sum,b)=>sum+b.byteLength,0),references:0,used:0};
      map.set(key,entry);this.entries.add(entry);this.bytes+=entry.bytes;
    }
    entry.references++;entry.used=++this.clock;this.trim();let released=false;const borrowed=entry;
    return {geometry:entry.geometry,release:()=>{if(released)return;released=true;borrowed.references--;this.trim();}};
  }
  private trim():void {
    while(this.bytes>this.budget){let oldest:Entry|undefined;for(const entry of this.entries)if(!entry.references&&(!oldest||entry.used<oldest.used))oldest=entry;if(!oldest)break;this.remove(oldest);}
  }
  private remove(entry:Entry):void {const owner=entry.owner.deref();if(owner)this.byMesh.get(owner)?.delete(entry.key);this.entries.delete(entry);this.bytes-=entry.bytes;entry.geometry.dispose();}
  dispose():void {for(const entry of [...this.entries])this.remove(entry);this.byMesh=new WeakMap();}
  get retainedBytes():number{return this.bytes;}
}
