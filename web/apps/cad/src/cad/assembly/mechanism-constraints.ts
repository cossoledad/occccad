import type {AssemblyConstraint, AssemblyGeometryRef} from '../../types';
import {quantityIn,type Mechanism} from './motion-study';

// Read-only joint expansion for the existing constraint renderer. These IDs
// identify semantic children of the Joint, never Product constraints or solver IDs.
export function mechanismConstraints(m:Mechanism):AssemblyConstraint[] {
 return m.joints.flatMap(j=>{
  const reference=(side:'first'|'second',field:'axis'|'plane'):AssemblyGeometryRef=>{
   const end=j[side]!;return end[field]??{kind:'BODY',instanceId:end.instanceId};
  };
  const relation=(role:string,kind:AssemblyConstraint['kind'],field:'axis'|'plane'):AssemblyConstraint=>({
   id:j.id+'/'+role,kind,definitionVersion:2,first:reference('first',field),second:j.second?reference('second',field):undefined,
   directionRelation:'SAME',evaluationStatus:'NOT_UPDATED',
  });
  if(j.kind==='GROUND')return [{...relation('ground','FIX','axis'),first:{kind:'BODY',instanceId:j.first.instanceId},fixMode:'SPACE'}];
  if(!j.second)return [];
  // A draft only exposes a relation once both relevant selections exist.
  const axis=relation('axis','COINCIDENT','axis');axis.family='Coincidence';
  const result:AssemblyConstraint[]=j.first.axis&&j.second.axis?[axis]:[];
  if(j.kind!=='PRISMATIC'&&j.first.plane&&j.second.plane){
   const offset=relation('axial-location','DISTANCE','plane');offset.family='Offset';offset.distanceRelation='SELECTED_PLANE_NORMAL_V1';offset.directionRelation='UNORIENTED';offset.value=j.axialOffset?quantityIn(j.axialOffset,'mm'):0;result.push(offset);
  }
  if((j.kind==='RIGID'||j.kind==='PRISMATIC')&&result.length){
   const orientation=relation('rotation','ANGLE','axis');orientation.angleRelation='DIRECTED';orientation.angleAxis=reference('first','axis');
   if(j.first.capturedX)orientation.first={...orientation.first,capturedDirection:j.first.capturedX};
   if(j.second.capturedX)orientation.second={...orientation.second!,capturedDirection:j.second.capturedX};result.push(orientation);
  }
  return result;
 });
}
