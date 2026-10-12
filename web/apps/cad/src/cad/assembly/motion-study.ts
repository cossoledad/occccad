import type {DocumentView,AssemblyGeometryRef,AssemblyConstraint} from '../../types';
export type MotionQuantity={value:number;unit:'deg'|'rad'|'mm'|'m'};
export type MotionPose={translation:[number,number,number];rotation:[number,number,number,number]};
export type JointEndpoint={instanceId:string;frame:MotionPose;axis?:AssemblyGeometryRef;plane?:AssemblyGeometryRef;point?:AssemblyGeometryRef;capturedX?:[number,number,number]};
export type JointKind='GROUND'|'REVOLUTE'|'PRISMATIC'|'CYLINDRICAL'|'PLANAR'|'RIGID'|'SPHERICAL';
export type JointCoordinate={id:string;role:'AXIAL_ROTATION'|'AXIAL_TRANSLATION';dimension:'ANGLE'|'LENGTH';direction:number;zero:MotionQuantity;lower?:MotionQuantity;upper?:MotionQuantity};
export type MechanismJoint={id:string;name:string;kind:JointKind;first:JointEndpoint;second?:JointEndpoint;coordinates?:JointCoordinate[];orientationAngle?:MotionQuantity;axisDirection?:'SAME'|'OPPOSITE';axialOffset?:MotionQuantity;axialDirection?:'SAME'|'OPPOSITE';sources?:{constraintId:string;baseline:string}[];constraints?:AssemblyConstraint[]};
export type Mechanism={id:string;name:string;unitIds:string[];joints:MechanismJoint[];supplementalConstraintIds?:string[];poses?:Record<string,MotionPose>;coordinates?:Record<string,number>;evaluation?:{relativeDof:number;gaugeDof:number;driverCount:number}};
export type MotionViewpoint={position:[number,number,number];rotation:[number,number,number,number];up:[number,number,number];target:[number,number,number];zoom:number;top:number;bottom:number};
export type MotionKeyframe={id:string;timeSeconds:number;values:Record<string,MotionQuantity>;viewpoint?:MotionViewpoint};
export type MotionStudy={id:string;name:string;mechanismId:string;activeDriverIds:string[];durationSeconds:number;frames:number;budgetMs:number;sourceRevisionId?:string;basisDigest?:string;geometryDigest?:string;references?:Record<string,MotionQuantity>;initialPoses?:Record<string,MotionPose>;initialCoordinates?:Record<string,number>;environmentPoses?:Record<string,MotionPose>;keyframes?:MotionKeyframe[];animateViewpoint?:boolean};
export type MotionDriver={id:string;name:string;mechanismId:string;jointId:string;coordinateId:string;reference?:MotionQuantity;referenceDigest?:string};
export type MotionAssemblyMapping={mechanismId:string;jointId:string;role:string;constraintId:string;jointBaseline:string;constraintBaseline:string};
export type KinematicsDefinitions={mechanisms:Mechanism[];studies:MotionStudy[];drivers?:MotionDriver[];associations?:MotionAssemblyMapping[]};
export type MotionApplyRequest={mechanismId?:string;jobId?:string;frameIndex:number;timeSeconds?:number;lockCoordinateIds?:string[];resolutions?:Record<string,string>;planDigest?:string};
export type MechanismApplicationRequest={baseRevisionId:string;mechanismId?:string;sourceMechanismId?:string;sourceMechanismIds?:string[];resume?:boolean};
export type MechanismApplication={baseRevisionId:string;mechanism?:Mechanism;instancePoses:Array<MotionPose&{instanceId:string}>};
export type MotionApplyPlan={baseRevisionId:string;digest:string;ready:boolean;items:{jointId?:string;role:string;constraintId:string;action:string;detail?:string}[];poseChanges:string[];solverStatus?:string;degreesOfFreedom:number};
export type MotionRunRequest={baseRevisionId:string;requestId:string;studyId:string};
export type MotionFrame={keyframeId?:string;timeSeconds:number;commandValues:Record<string,number>;unitPoses:Record<string,MotionPose>;coordinates:Record<string,number>;kinematicValid:boolean};
export type MotionRun={schema:2;snapshot:{documentId:string;revisionId:string;digest:string;view:DocumentView;mechanism:Mechanism;study:MotionStudy};status:string;frames:MotionFrame[];failure?:{code:string;detail:string;timeSeconds:number;commandValues:Record<string,number>;replayObjectId?:string};completed:boolean;elapsedMs:number;solveCalls:number;solverBuild:string};
export type MechanismSessionBegin={ownerId:string;generation:number;baseRevisionId:string;mechanismId:string;activeDriverIds?:string[];studyId?:string;revalidate?:boolean;sourceMechanismIds?:string[]};
export type MechanismSessionFrame={basisDigest:string;geometryDigest:string;initialPoses:Record<string,MotionPose>;activeDriverIds:string[];qualified:boolean;sessionId:string;baseRevisionId:string;inputDigest:string;sequence:number;accepted:boolean;frame:MotionFrame;failure?:{code:string;message:string;diagnosticId?:string};solveMs:number;interaction?:import('./assembly-interaction').AssemblyInteractionFrame['interaction']};
export type MechanismSessionOpened=MechanismSessionFrame&{drivers:MotionDriver[];qualificationMessage?:string;basisDigest:string;geometryDigest:string;initialPoses:Record<string,MotionPose>;study?:MotionStudy};
export type MechanismSessionUpdate={activeDriverIds?:string[];sequence:number;values?:Record<string,MotionQuantity>;spatialTarget?:import('./assembly-interaction').AssemblyDragTarget};
// The clock is presentation only. frame always remains an authoritative sample.
export type MotionPlaybackClock={startedAt:number;timeSeconds:number;frames:readonly MotionFrame[]};
export type MotionPlayback={view:DocumentView;frame:MotionFrame;revisionId:string;clock?:MotionPlaybackClock;study?:MotionStudy;samples?:readonly MotionFrame[];timeSeconds?:number};
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
 const frame=run.frames[index];if(!frame?.kinematicValid||run.snapshot.view.document.id!==run.snapshot.documentId||run.snapshot.view.document.versionId!==run.snapshot.revisionId)return;
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
