import {quantityIn,type Mechanism,type MechanismJoint,type MotionDriver,type MotionQuantity} from './motion-study';

export function motionCommandRange(mechanism:Mechanism,driver:MotionDriver):{lower:number;upper:number} {
 const joint=mechanism.joints.find(j=>j.id===driver.jointId);
 if(!joint?.lower||!joint.upper||!driver.reference)throw new Error('驱动接合需要上下限');
 const reference=quantityIn(driver.reference,'rad');
 return {lower:quantityIn(joint.lower,'rad')-reference,upper:quantityIn(joint.upper,'rad')-reference};
}

export function jointLimitReading(limit:MotionQuantity|undefined,reference:number):number|undefined {
 return limit?quantityIn(limit,'deg')-reference*180/Math.PI:undefined;
}

export function jointLimitValue(value:number,reference:number):MotionQuantity {
 return {value:value+reference*180/Math.PI,unit:'deg'};
}

export function drivenJointLimits(joint:MechanismJoint,reference:number):Pick<MechanismJoint,'lower'|'upper'> {
 return {lower:joint.lower??jointLimitValue(-360,reference),upper:joint.upper??jointLimitValue(360,reference)};
}

export function jointLimitsIssue(joint:MechanismJoint,driven:boolean,current?:number):string|undefined {
 if(driven&&(!joint.lower||!joint.upper))return '驱动接合需要上下限';
 const lower=joint.lower?quantityIn(joint.lower,'rad'):undefined,upper=joint.upper?quantityIn(joint.upper,'rad'):undefined;
 if(lower!==undefined&&!Number.isFinite(lower)||upper!==undefined&&!Number.isFinite(upper))return '限位数值无效';
 if(lower!==undefined&&upper!==undefined&&lower>=upper)return '下限须小于上限';
 if(current!==undefined&&(lower!==undefined&&current<lower-1e-8||upper!==undefined&&current>upper+1e-8))return '当前角度超出接合限制';
}
