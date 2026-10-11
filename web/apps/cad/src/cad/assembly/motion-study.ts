import type {DocumentView,AssemblyGeometryRef,AssemblyConstraint} from '../../types';
export type MotionQuantity={value:number;unit:'deg'|'rad'|'mm'|'m'};
export type MotionPose={translation:[number,number,number];rotation:[number,number,number,number]};
export type JointEndpoint={instanceId:string;frame:MotionPose;axis?:AssemblyGeometryRef;plane?:AssemblyGeometryRef;capturedX?:[number,number,number]};
export type MechanismJoint={id:string;name:string;kind:'GROUND'|'REVOLUTE';first:JointEndpoint;second?:JointEndpoint;zero:MotionQuantity;direction:number;axialOffset?:MotionQuantity;axialDirection?:'SAME'|'OPPOSITE';sources?:{constraintId:string;baseline:string}[];lower?:MotionQuantity;upper?:MotionQuantity;constraints?:AssemblyConstraint[]};
export type Mechanism={id:string;name:string;unitIds:string[];joints:MechanismJoint[];supplementalConstraintIds?:string[];poses?:Record<string,MotionPose>;evaluation?:{relativeDof:number;gaugeDof:number;drivenDof:number;driverCount:number}};
export type MotionLaw={id:string;name:string;mechanismId:string;driverId:string;start:MotionQuantity;end:MotionQuantity;durationSeconds:number};
export type MotionStudy={id:string;name:string;mechanismId:string;lawId:string;driverId?:string;driverJointId?:string;start:MotionQuantity;end:MotionQuantity;durationSeconds:number;frames:number;budgetMs:number};
export type MotionDriver={id:string;name:string;mechanismId:string;jointId:string};
export type MotionAssemblyMapping={mechanismId:string;jointId:string;role:string;constraintId:string;jointBaseline:string;constraintBaseline:string};
export type KinematicsDefinitions={mechanisms:Mechanism[];studies:MotionStudy[];laws?:MotionLaw[];drivers?:MotionDriver[];associations?:MotionAssemblyMapping[]};
export type MotionApplyRequest={mechanismId?:string;jobId?:string;frameIndex:number;lockAngle:boolean;resolutions?:Record<string,string>;planDigest?:string};
export type MechanismApplicationRequest={baseRevisionId:string;mechanismId?:string;sourceMechanismId?:string;sourceMechanismIds?:string[];resume?:boolean};
export type MechanismApplication={baseRevisionId:string;mechanism?:Mechanism;instancePoses:Array<MotionPose&{instanceId:string}>};
export type MotionApplyPlan={baseRevisionId:string;digest:string;ready:boolean;items:{jointId?:string;role:string;constraintId:string;action:string;detail?:string}[];poseChanges:string[];solverStatus?:string;degreesOfFreedom:number};
export type MotionRunRequest={baseRevisionId:string;requestId:string;studyId:string};
export type MotionFrame={timeSeconds:number;driverValue:number;unitPoses:Record<string,MotionPose>;coordinates:Record<string,number>;kinematicValid:boolean};
export type MotionRun={schema:1;snapshot:{documentId:string;revisionId:string;digest:string;view:DocumentView;mechanism:Mechanism;study:MotionStudy};status:string;frames:MotionFrame[];failure?:{code:string;detail:string;timeSeconds:number;driverValue:number;replayObjectId?:string};completed:boolean;elapsedMs:number;solveCalls:number;solverBuild:string};
// The clock is presentation only. frame always remains an authoritative sample.
export type MotionPlaybackClock={startedAt:number;timeSeconds:number;frames:readonly MotionFrame[]};
export type MotionPlayback={view:DocumentView;frame:MotionFrame;revisionId:string;clock?:MotionPlaybackClock};
export function motionPlaybackTime(clock:MotionPlaybackClock,now:number):number {
 return Math.min(clock.frames.at(-1)?.timeSeconds??0,clock.timeSeconds+Math.max(0,now-clock.startedAt)/1000);
}
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
// Editing and conversion select a complete accepted sample, independently of display interpolation.
export function motionFrameAt(run:MotionRun,time:number):number {
 let result=0;for(let i=0;i<run.frames.length;i++)if(run.frames[i].timeSeconds<=time)result=i;return result;
}

export function quantityIn(q:MotionQuantity,unit:MotionQuantity['unit']):number {
 if((q.unit==='rad'||q.unit==='deg')!==(unit==='rad'||unit==='deg'))throw new Error('Motion quantity dimension mismatch');
 const factor=(u:MotionQuantity['unit'])=>u==='deg'?Math.PI/180:u==='m'?1000:1;
 return q.value*factor(q.unit)/factor(unit);
}
