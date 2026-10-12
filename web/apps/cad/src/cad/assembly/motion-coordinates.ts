import {quantityIn,type Mechanism,type JointCoordinate,type MotionDriver,type MotionQuantity} from './motion-study';
export function driverCoordinate(mechanism:Mechanism,driver:MotionDriver):JointCoordinate|undefined{return mechanism.joints.find(j=>j.id===driver.jointId)?.coordinates?.find(c=>c.id===driver.coordinateId)}
export function coordinateUnit(coordinate:JointCoordinate,display=false):MotionQuantity['unit']{return coordinate.dimension==='ANGLE'?(display?'deg':'rad'):'mm'}
export function motionCommandRange(mechanism:Mechanism,driver:MotionDriver):{lower:number;upper:number}{
 const coordinate=driverCoordinate(mechanism,driver);if(!coordinate?.lower||!coordinate.upper||!driver.reference)throw new Error('驱动参数需要上下限');
 const unit=coordinateUnit(coordinate),reference=quantityIn(driver.reference,unit);
 return {lower:quantityIn(coordinate.lower,unit)-reference,upper:quantityIn(coordinate.upper,unit)-reference};
}
export function jointLimitReading(limit:MotionQuantity|undefined,reference:number,coordinate:JointCoordinate):number|undefined{
 return limit?quantityIn(limit,coordinateUnit(coordinate,true))-quantityIn({value:reference,unit:coordinateUnit(coordinate)},coordinateUnit(coordinate,true)):undefined;
}
export function jointLimitValue(value:number,reference:number,coordinate:JointCoordinate):MotionQuantity{
 const unit=coordinateUnit(coordinate,true);return {value:value+quantityIn({value:reference,unit:coordinateUnit(coordinate)},unit),unit};
}
export function drivenJointLimits(coordinate:JointCoordinate,reference:number):Pick<JointCoordinate,'lower'|'upper'>{
 const span=coordinate.dimension==='ANGLE'?360:100;return {lower:coordinate.lower??jointLimitValue(-span,reference,coordinate),upper:coordinate.upper??jointLimitValue(span,reference,coordinate)};
}
export function jointLimitsIssue(coordinate:JointCoordinate,driven:boolean,current?:number):string|undefined{
 if(driven&&(!coordinate.lower||!coordinate.upper))return '驱动参数需要上下限';
 const unit=coordinateUnit(coordinate),lower=coordinate.lower?quantityIn(coordinate.lower,unit):undefined,upper=coordinate.upper?quantityIn(coordinate.upper,unit):undefined;
 if(lower!==undefined&&!Number.isFinite(lower)||upper!==undefined&&!Number.isFinite(upper))return '限位数值无效';
 if(lower!==undefined&&upper!==undefined&&lower>=upper)return '下限须小于上限';
 if(current!==undefined&&(lower!==undefined&&current<lower-1e-8||upper!==undefined&&current>upper+1e-8))return '当前位置超出接合限制';
}
