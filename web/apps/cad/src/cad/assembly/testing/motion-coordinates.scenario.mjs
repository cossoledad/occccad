import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {motionCommandRange,jointLimitReading,jointLimitValue,drivenJointLimits,jointLimitsIssue}=await server.ssrLoadModule('/src/cad/assembly/motion-coordinates.ts');
 const joint={id:'j',lower:{value:10,unit:'deg'},upper:{value:100,unit:'deg'}};
 const mechanism={joints:[joint]},driver={jointId:'j',reference:{value:40,unit:'deg'}};
 const before=JSON.stringify(joint);
 let range=motionCommandRange(mechanism,driver);
 assert.ok(Math.abs(range.lower*180/Math.PI+30)<1e-10);assert.ok(Math.abs(range.upper*180/Math.PI-60)<1e-10);
 range=motionCommandRange(mechanism,{...driver,reference:{value:60,unit:'deg'}});
 assert.ok(Math.abs(range.lower*180/Math.PI+50)<1e-10);assert.ok(Math.abs(range.upper*180/Math.PI-40)<1e-10);
 assert.equal(JSON.stringify(joint),before,'reset changes the displayed range, not physical stops');
 assert.ok(Math.abs(jointLimitReading(joint.upper,60*Math.PI/180)-40)<1e-10);
 assert.ok(Math.abs(jointLimitValue(40,60*Math.PI/180).value-100)<1e-10,'joint editor and simulation use the same zero');
 assert.deepEqual(drivenJointLimits(joint,1),{lower:joint.lower,upper:joint.upper},'enabling driving preserves existing physical stops');
 const missing={id:'j'};assert.equal(jointLimitsIssue(missing,false),undefined);assert.match(jointLimitsIssue(missing,true),/上下限/);
 const defaults=drivenJointLimits(missing,40*Math.PI/180);
 assert.ok(Math.abs(jointLimitReading(defaults.lower,40*Math.PI/180)+360)<1e-10);
 assert.ok(Math.abs(jointLimitReading(defaults.upper,40*Math.PI/180)-360)<1e-10,'new command limits use the qualified current coordinate');
 assert.equal(jointLimitsIssue({...missing,upper:{value:45,unit:'deg'}},false,0),undefined,'undriven joints support a single stop');
 assert.match(jointLimitsIssue(joint,true,0),/当前角度/);assert.match(jointLimitsIssue({...joint,lower:joint.upper},true),/下限/);
 assert.throws(()=>motionCommandRange({joints:[missing]},driver),/上下限/);
 console.log('joint-owned limits: reset affine range, unchanged stops, optional passive bounds and mandatory driven bounds passed');
}finally{await server.close()}
