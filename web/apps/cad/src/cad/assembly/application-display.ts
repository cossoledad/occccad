import type {DocumentView} from '../../types';
import {validMotionPose,type Mechanism,type MotionPose,type MotionPlayback} from './motion-study';

export type ApplicationDisplay={documentId:string;revisionId:string;application:'PRODUCT'|'MECHANISM';mechanism?:Mechanism;mode:'ACCEPTED'|'DRAFT'|'PLAYBACK';poses:Record<string,MotionPose>;playback?:MotionPlayback};

export function memberPoses(m:Mechanism,poses:Record<string,MotionPose>):Record<string,MotionPose>{
 return Object.fromEntries(m.unitIds.flatMap(id=>validMotionPose(poses[id])?[[id,poses[id]]]:[]));
}

// A remembered multi-mechanism environment is valid only at its saved Head.
// After another editor advances Head, nonmembers use the current Product.
export function restoredApplicationSources(view:DocumentView,m:Mechanism,params:URLSearchParams):string[]{
 const live=new Set(view.product?.kinematics?.mechanisms.map(v=>v.id));
 const sources=params.get('applicationRevision')===view.document.versionId?(params.get('mechanismSources')??'').split(','):[];
 return [...new Set([...sources.filter(id=>live.has(id)&&id!==m.id),m.id])];
}

// The scene always receives one complete frame. Member snapshots never supply
// nonmember positions; those belong to the current display document.
export function applicationDisplay(view:DocumentView,m?:Mechanism,playback?:MotionPlayback,draft?:Array<MotionPose&{instanceId:string}>,accepted?:Array<MotionPose&{instanceId:string}>):ApplicationDisplay {
 const poses:Record<string,MotionPose>={};
 for(const v of view.product?.instances??[])poses[v.id]={translation:v.translation,rotation:v.rotation??[0,0,0,1]};
 if(accepted)for(const v of accepted)if(v.instanceId in poses&&validMotionPose(v))poses[v.instanceId]={translation:v.translation,rotation:v.rotation};
 if(m?.poses)Object.assign(poses,memberPoses(m,m.poses));
 if(draft)for(const v of draft)if(v.instanceId in poses&&validMotionPose(v))poses[v.instanceId]={translation:v.translation,rotation:v.rotation};
 if(playback){
  if(playback.view.document.id!==view.document.id||playback.revisionId!==view.document.versionId)throw new Error('Application playback revision mismatch');
  for(const id of Object.keys(poses)){if(!validMotionPose(playback.frame.unitPoses[id]))throw new Error('Incomplete application playback frame');poses[id]=playback.frame.unitPoses[id]}
 }
 return {documentId:view.document.id,revisionId:view.document.versionId,application:m?'MECHANISM':'PRODUCT',mechanism:m,mode:playback?'PLAYBACK':draft?'DRAFT':'ACCEPTED',poses,playback};
}
