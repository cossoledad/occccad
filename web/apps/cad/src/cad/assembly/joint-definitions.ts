import type {JointKind,MechanismJoint} from './motion-study';
import type {JointSupportRole} from './mechanism-selection';
import {randomUUID} from '../../utils/random-uuid';

// A thin description of supports/coordinates, shared by creation and the form.
// The exact server compiler and the public assembly registry remain authoritative.
export const jointDefinitions:Record<JointKind,{name:string;roles:JointSupportRole[];coordinates:('AXIAL_ROTATION'|'AXIAL_TRANSLATION')[]}>= {
 GROUND:{name:'固定零件',roles:['first.instance'],coordinates:[]},
 REVOLUTE:{name:'旋转',roles:['first.axis','second.axis','first.plane','second.plane'],coordinates:['AXIAL_ROTATION']},
 PRISMATIC:{name:'棱形',roles:['first.axis','second.axis','first.plane','second.plane'],coordinates:['AXIAL_TRANSLATION']},
 CYLINDRICAL:{name:'圆柱',roles:['first.axis','second.axis'],coordinates:['AXIAL_ROTATION','AXIAL_TRANSLATION']},
 PLANAR:{name:'平面',roles:['first.plane','second.plane'],coordinates:[]},
 RIGID:{name:'刚性',roles:['first.instance','second.instance'],coordinates:[]},
 SPHERICAL:{name:'球面',roles:['first.point','second.point'],coordinates:[]},
};
export function createJointCoordinates(kind:JointKind):MechanismJoint['coordinates']{
 return jointDefinitions[kind].coordinates.map(role=>({id:randomUUID(),role,dimension:role==='AXIAL_ROTATION'?'ANGLE':'LENGTH',direction:1,zero:{value:0,unit:role==='AXIAL_ROTATION'?'rad':'mm'}}));
}
export function jointSupportsComplete(joint:MechanismJoint):boolean{
 return jointDefinitions[joint.kind].roles.every(role=>{const [side,field]=role.split('.') as ['first'|'second','axis'|'plane'|'point'|'instance'];return field==='instance'?Boolean(joint[side]?.instanceId):Boolean(joint[side]?.[field]);});
}
