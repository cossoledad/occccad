import * as THREE from 'three';
import type {MeshData} from '../../types';
import {LineSegmentsGeometry} from 'three/addons/lines/LineSegmentsGeometry.js';

export const NATURAL_EDGE_DEFAULTS={smoothAngle:5,sharpAngle:35,smoothOpacity:.12,sharpOpacity:.88} as const;
export type EdgePresentation='uniform'|'basic'|'natural';
export type EdgeBuffers={positions:Float32Array;aStart:Float32Array;aEnd:Float32Array;bStart:Float32Array;bEnd:Float32Array;kinds:Float32Array;quality:Float32Array;metadata?:Uint32Array;direct?:{a:Float32Array;b:Float32Array}};
const preparation={builds:0,sharedHits:0,attributeBytes:0,buildMs:0};
export function edgePreparationStats(reset=false){const result={...preparation};if(reset)Object.assign(preparation,{builds:0,sharedHits:0,attributeBytes:0,buildMs:0});return result;}
export function edgeSharpness(a:THREE.Vector3,b:THREE.Vector3):number {
  if(!Number.isFinite(a.lengthSq()+b.lengthSq())||a.lengthSq()<1e-24||b.lengthSq()<1e-24)return 1;
  const c=a.clone().normalize().dot(b.clone().normalize());
  return 1-THREE.MathUtils.smoothstep(c,Math.cos(35*Math.PI/180),Math.cos(5*Math.PI/180));
}

function legacyEdgePositions(mesh:MeshData):Float32Array {
  const segments:Array<[number[],number[]]>=[];
  for(const edge of mesh.edges??[])for(let i=1;i<edge.points.length;i++){
    const a=edge.points[i-1],b=edge.points[i];
    if(!a.every(Number.isFinite)||!b.every(Number.isFinite)||Math.hypot(b[0]-a[0],b[1]-a[1],b[2]-a[2])<=1e-12)continue;
    segments.push([a,b]);
  }
  const positions=new Float32Array(segments.length*6);
  segments.forEach(([a,b],i)=>{positions.set(a,i*6);positions.set(b,i*6+3)});
  return positions;
}
/** No coordinate/normal guessing: missing exact evidence remains structural.
 * Render samples follow original chords; picking continues using mesh.edges. */
export function buildEdgeBuffers(mesh:MeshData,direct=false):EdgeBuffers {
  const started=performance.now();preparation.builds++;
  const profile=mesh.edgeProfile;
  const positions=profile?.positions??legacyEdgePositions(mesh);
  const count=positions.length/6,normalCount=direct&&profile?0:count*3,aStart=new Float32Array(normalCount),aEnd=new Float32Array(normalCount),bStart=new Float32Array(normalCount),bEnd=new Float32Array(normalCount),kinds=new Float32Array(count),quality=new Float32Array(count);
  if(profile)for(let i=0;i<count;i++){
    if(!direct){aStart.set(profile.normalA.subarray(i*6,i*6+3),i*3);aEnd.set(profile.normalA.subarray(i*6+3,i*6+6),i*3);
      bStart.set(profile.normalB.subarray(i*6,i*6+3),i*3);bEnd.set(profile.normalB.subarray(i*6+3,i*6+6),i*3);}
    const bits=profile.metadata[i*4+3];kinds[i]=bits&255;quality[i]=bits>>>8;
  }
  preparation.attributeBytes+=aStart.byteLength+aEnd.byteLength+bStart.byteLength+bEnd.byteLength+kinds.byteLength+quality.byteLength;
  preparation.buildMs+=performance.now()-started;
  return {positions,aStart,aEnd,bStart,bEnd,kinds,quality,metadata:profile?.metadata,direct:direct&&profile?{a:profile.normalA,b:profile.normalB}:undefined};
}
export function edgeLineGeometry(data:EdgeBuffers):LineSegmentsGeometry {
  const geometry=new LineSegmentsGeometry().setPositions(data.positions);
  if(data.direct){
    for(const [side,values] of [['A',data.direct.a],['B',data.direct.b]] as const){
      const buffer=new THREE.InstancedInterleavedBuffer(values,6,1);
      geometry.setAttribute(`instanceEdge${side}Start`,new THREE.InterleavedBufferAttribute(buffer,3,0));
      geometry.setAttribute(`instanceEdge${side}End`,new THREE.InterleavedBufferAttribute(buffer,3,3));
    }
  }else for(const [key,values] of Object.entries({instanceEdgeAStart:data.aStart,instanceEdgeAEnd:data.aEnd,instanceEdgeBStart:data.bStart,instanceEdgeBEnd:data.bEnd}))
      geometry.setAttribute(key,new THREE.InstancedBufferAttribute(values,3));
  geometry.setAttribute('instanceEdgeKind',new THREE.InstancedBufferAttribute(data.kinds,1));
  geometry.setAttribute('instanceEdgeQuality',new THREE.InstancedBufferAttribute(data.quality,1));
  return geometry;
}

type SharedEdge={buffers:EdgeBuffers;geometry?:LineSegmentsGeometry;references:number};
const sharedEdges=new WeakMap<MeshData,SharedEdge>();
/** Immutable solid lines can share GPU buffers across occurrences. Dashed lines
 * keep private geometry because their screen-space distances depend on pose. */
export function acquireEdgeGeometry(mesh:MeshData,shared:boolean):{geometry:LineSegmentsGeometry;release:()=>void}{
  let entry=shared?sharedEdges.get(mesh):undefined;
  if(entry)preparation.sharedHits++;
  else {entry={buffers:buildEdgeBuffers(mesh,shared),references:0};if(shared)sharedEdges.set(mesh,entry);}
  const owner=entry;owner.geometry??=edgeLineGeometry(owner.buffers);owner.references++;
  const geometry=owner.geometry;let released=false;
  return {geometry,release:()=>{if(released)return;released=true;if(--owner.references===0){geometry.dispose();owner.geometry=undefined;}}};
}

export const NATURAL_EDGE_VERTEX=`
attribute vec3 instanceEdgeAStart,instanceEdgeAEnd,instanceEdgeBStart,instanceEdgeBEnd;
attribute float instanceEdgeKind,instanceEdgeQuality;
varying float vCadEdgeSharp,vCadEdgeSuppressed;
uniform vec2 uCadEdgeAngles;
void cadNaturalEdge() {
  vec3 a=normalMatrix*(position.y<0.5?instanceEdgeAStart:instanceEdgeAEnd);
  vec3 b=normalMatrix*(position.y<0.5?instanceEdgeBStart:instanceEdgeBEnd);
  bool valid=dot(a,a)>1.0e-24&&dot(b,b)>1.0e-24&&mod(instanceEdgeQuality,2.0)<0.5;
  float c=valid?clamp(dot(normalize(a),normalize(b)),-1.0,1.0):0.0;
  bool paired=instanceEdgeKind>2.5&&instanceEdgeKind<5.5;
  vCadEdgeSharp=paired&&valid?1.0-smoothstep(uCadEdgeAngles.y,uCadEdgeAngles.x,c):1.0;
  // Validate continuity from normals even when a seam/coplanar tag is present.
  vCadEdgeSuppressed=(instanceEdgeKind>3.5&&instanceEdgeKind<5.5&&valid&&c>0.999998)?1.0:0.0;
}`;
export const NATURAL_EDGE_FRAGMENT=`
varying float vCadEdgeSharp,vCadEdgeSuppressed;
uniform float uCadEdgeNatural,uCadEdgeInteraction,uCadEdgeExplicitColor;
uniform vec2 uCadEdgeOpacity;
vec4 cadNaturalEdgeColor(vec3 hardColor,float coverage) {
  if(uCadEdgeNatural<0.5||uCadEdgeInteraction>0.5)return vec4(hardColor,coverage);
  float sharp=clamp(vCadEdgeSharp,0.0,1.0);
  float strength=mix(uCadEdgeOpacity.x,uCadEdgeOpacity.y,sharp)*(1.0-clamp(vCadEdgeSuppressed,0.0,1.0));
  // Black alpha blending gently multiplies the already-lit framebuffer colour.
  // Structural edges retain the explicit/theme contrast, including on dark parts.
  vec3 ink=hardColor*mix(sharp,1.0,uCadEdgeExplicitColor);
  return vec4(ink,coverage*strength);
}`;
