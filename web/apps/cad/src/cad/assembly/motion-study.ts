import type {DocumentView,AssemblyGeometryRef,AssemblyConstraint} from '../../types';
export type MotionQuantity={value:number;unit:'deg'|'rad'|'mm'|'m'};
export type MotionPose={translation:[number,number,number];rotation:[number,number,number,number]};
export type JointEndpoint={instanceId:string;frame:MotionPose;axis?:AssemblyGeometryRef;plane?:AssemblyGeometryRef;capturedX?:[number,number,number]};
export type MechanismJoint={id:string;name:string;kind:'GROUND'|'REVOLUTE';first:JointEndpoint;second?:JointEndpoint;zero:MotionQuantity;direction:number;axialOffset?:MotionQuantity;sources?:{constraintId:string;baseline:string}[];lower?:MotionQuantity;upper?:MotionQuantity;constraints?:AssemblyConstraint[]};
export type Mechanism={id:string;name:string;unitIds:string[];joints:MechanismJoint[];supplementalConstraintIds?:string[];poses?:Record<string,MotionPose>};
export type MotionStudy={id:string;name:string;mechanismId:string;driverId?:string;driverJointId?:string;start:MotionQuantity;end:MotionQuantity;durationSeconds:number;frames:number;budgetMs:number};
export type MotionDriver={id:string;name:string;mechanismId:string;jointId:string};
export type MotionAssemblyMapping={mechanismId:string;jointId:string;role:string;constraintId:string;jointBaseline:string;constraintBaseline:string};
export type KinematicsDefinitions={mechanisms:Mechanism[];studies:MotionStudy[];drivers?:MotionDriver[];associations?:MotionAssemblyMapping[]};
export type MotionApplyRequest={jobId:string;frameIndex:number;lockAngle:boolean;resolutions?:Record<string,string>;planDigest?:string};
export type MotionApplyPlan={baseRevisionId:string;digest:string;ready:boolean;items:{jointId?:string;role:string;constraintId:string;action:string;detail?:string}[];poseChanges:string[];solverStatus?:string;degreesOfFreedom:number};
export type MotionRunRequest={baseRevisionId:string;requestId:string;studyId:string};
export type MotionFrame={timeSeconds:number;driverValue:number;unitPoses:Record<string,MotionPose>;coordinates:Record<string,number>;kinematicValid:boolean};
export type MotionRun={schema:1;snapshot:{documentId:string;revisionId:string;digest:string;view:DocumentView;mechanism:Mechanism;study:MotionStudy};status:string;frames:MotionFrame[];failure?:{code:string;detail:string;timeSeconds:number;driverValue:number;replayObjectId?:string};completed:boolean;elapsedMs:number;solveCalls:number;solverBuild:string};
export type MotionPlayback={view:DocumentView;frame:MotionFrame;revisionId:string};
export function validMotionPose(value:unknown):value is MotionPose {
 if(!value||typeof value!=='object')return false;
 const pose=value as Partial<MotionPose>;
 return Array.isArray(pose.translation)&&pose.translation.length===3&&Array.isArray(pose.rotation)&&pose.rotation.length===4
  &&[...pose.translation,...pose.rotation].every(Number.isFinite)
  &&Math.abs(pose.rotation.reduce((sum,v)=>sum+v*v,0)-1)<1e-8;
}
export function motionPlayback(run:MotionRun,index:number):MotionPlayback|undefined{
 const frame=run.frames[index];if(!frame||run.snapshot.view.document.id!==run.snapshot.documentId||run.snapshot.view.document.versionId!==run.snapshot.revisionId)return;
 const ids=run.snapshot.view.product?.instances.map(i=>i.id)??[];
 if(!frame.unitPoses||ids.some(id=>!validMotionPose(frame.unitPoses[id])))return;
 return {view:run.snapshot.view,frame,revisionId:run.snapshot.revisionId};
}
// Time selects a complete accepted frame; never interpolate individual units.
export function motionFrameAt(run:MotionRun,time:number):number {
 let result=0;for(let i=0;i<run.frames.length;i++)if(run.frames[i].timeSeconds<=time)result=i;return result;
}

export function quantityIn(q:MotionQuantity,unit:MotionQuantity['unit']):number {
 if((q.unit==='rad'||q.unit==='deg')!==(unit==='rad'||unit==='deg'))throw new Error('Motion quantity dimension mismatch');
 const factor=(u:MotionQuantity['unit'])=>u==='deg'?Math.PI/180:u==='m'?1000:1;
 return q.value*factor(q.unit)/factor(unit);
}
