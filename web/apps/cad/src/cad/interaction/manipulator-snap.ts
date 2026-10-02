import * as THREE from "three";
import type {AssemblyGeometryRef,Vec3} from "../../types";
import type {ManipulatorAnchor} from "./assembly-manipulator";

export type SnapDescriptor={Kind:string;Origin:Vec3;Direction:Vec3;Rotation:[number,number,number,number];XDirection:Vec3;LengthUnit?:string};
export type SnapHints={center?:Vec3;boundaryDirection?:Vec3;endFirst?:Vec3;endLast?:Vec3};
export type SupportInspection={documentId:string;versionId:string;supports:Array<{reference:AssemblyGeometryRef;exactType?:string;descriptor?:SnapDescriptor;snapHints?:SnapHints;status:string;diagnostic?:string;diagnosticCode?:string;constraintEligible:boolean;constraintDiagnosticCode?:string;constraintDiagnostic?:string}>};
export type SnapCandidate=ManipulatorAnchor&{role:string;exact:boolean};

/** Main Z follows a normal, main X follows a line. Preserve the prior transverse
 * axis/sign when geometry is symmetric; every result is right handed. */
export function snapFrame(direction:THREE.Vector3,normal:boolean,previous:THREE.Quaternion,secondary?:THREE.Vector3):THREE.Quaternion|undefined {
  if(!direction.toArray().every(Number.isFinite)||direction.lengthSq()<1e-18)return;
  const main=direction.clone().normalize();
  const prior=new THREE.Vector3(normal?1:0,normal?0:1,0).applyQuaternion(previous);
  const project=(v:THREE.Vector3)=>v.clone().addScaledVector(main,-v.dot(main));
  let transverse=secondary?project(secondary):project(prior);
  if(transverse.lengthSq()<1e-10){
    const axes=[new THREE.Vector3(1,0,0),new THREE.Vector3(0,1,0),new THREE.Vector3(0,0,1)];
    axes.sort((a,b)=>Math.abs(a.dot(main))-Math.abs(b.dot(main)));transverse=project(axes[0]);
  }
  transverse.normalize();if(transverse.dot(prior)<0)transverse.negate();
  const x=normal?transverse:main,y=normal?main.clone().cross(x).normalize():transverse,z=normal?main:x.clone().cross(y).normalize();
  return new THREE.Quaternion().setFromRotationMatrix(new THREE.Matrix4().makeBasis(x,y,z)).normalize();
}

export function exactSnapCandidates(descriptor:SnapDescriptor,hints:SnapHints|undefined,bodyWorld:THREE.Matrix4,hitWorld:THREE.Vector3,previous:THREE.Quaternion):SnapCandidate[] {
  if(descriptor.LengthUnit!=="mm"||!descriptor.Origin?.every(Number.isFinite))return [];
  const origin=new THREE.Vector3().fromArray(descriptor.Origin).applyMatrix4(bodyWorld);
  const worldRotation=new THREE.Quaternion().setFromRotationMatrix(bodyWorld);
  const direction=new THREE.Vector3().fromArray(descriptor.Direction??[0,0,0]).applyQuaternion(worldRotation);
  const secondary=hints?.boundaryDirection?new THREE.Vector3().fromArray(hints.boundaryDirection).applyQuaternion(worldRotation):undefined;
  const kind=descriptor.Kind;
  const orientation=kind==="FRAME"?worldRotation.multiply(new THREE.Quaternion().fromArray(descriptor.Rotation)).normalize():
    kind==="POINT"?previous.clone():snapFrame(direction,["PLANE","CIRCLE","CYLINDER","CONE"].includes(kind),previous,secondary);
  const candidates:SnapCandidate[]=[];
  const add=(point:THREE.Vector3,role:string)=>candidates.push({position:point,orientation:orientation?.clone(),role,exact:true});
  for(const [role,value] of Object.entries(hints??{}))if(value&&role!=="boundaryDirection")add(new THREE.Vector3().fromArray(value).applyMatrix4(bodyWorld),role);
  if(["POINT","FRAME","CIRCLE","SPHERE"].includes(kind))add(origin,"center");
  if(kind==="PLANE"&&direction.lengthSq()>0){const n=direction.normalize();add(hitWorld.clone().addScaledVector(n,-hitWorld.clone().sub(origin).dot(n)),"surface");}
  if(["AXIS","LINE","CYLINDER","CONE"].includes(kind)&&direction.lengthSq()>0){const n=direction.normalize();add(origin.clone().addScaledVector(n,hitWorld.clone().sub(origin).dot(n)),"axis");}
  return candidates;
}

/** Bounded snapshot/topology cache. One request per candidate, abort and fence
 * older sources; never install an exact response into a started motion drag. */
export class ManipulatorSnapCache {
  private values=new Map<string,SupportInspection>();private pending?:{key:string;abort:AbortController};private generation=0;
  clear(){this.generation++;this.pending?.abort.abort();this.pending=undefined;this.values.clear();}
  cancelPending(){this.generation++;this.pending?.abort.abort();this.pending=undefined;}
  get(key:string){return this.values.get(key);}
  request(key:string,load:(signal:AbortSignal)=>Promise<SupportInspection>,accept:(result:SupportInspection)=>void){
    const value=this.values.get(key);if(value){accept(value);return;}
    if(this.pending?.key===key)return;
    this.pending?.abort.abort();const abort=new AbortController(),generation=++this.generation;this.pending={key,abort};
    void load(abort.signal).then(result=>{if(abort.signal.aborted||generation!==this.generation)return;
      if(this.values.size>=128)this.values.delete(this.values.keys().next().value!);this.values.set(key,result);accept(result);
    }).catch(()=>{/* picking has a safe approximate fallback; never certify exact */}).finally(()=>{if(this.pending?.abort===abort)this.pending=undefined;});
  }
}
