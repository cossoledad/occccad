import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {sketchDiagnosticComponents:components,sketchConstraintIssues:issues}=await server.ssrLoadModule('/src/cad/sketch/sketch-diagnostics.ts');
 const sketch={entities:[{id:'a'},{id:'b'}],constraints:[{id:'conflict'},{id:'redundant'},{id:'suppressed',suppressed:true}],solve:{components:[{entityIds:['b','a','unavailable'],constraintIds:['conflict','solver-internal'],status:'CONFLICTING',definitionStatus:'UNRESOLVED',degreesOfFreedom:0}],conflictingConstraintIds:['conflict'],redundantConstraintIds:['redundant']}};
 const result=components(sketch)[0];assert.equal(result.known,false,'unresolved zero is not displayed as fully constrained');assert.deepEqual(result.entityIds,['b','a']);assert.deepEqual(result.constraintIds,['conflict']);
 assert.equal(components({...sketch,solve:{...sketch.solve,components:[{...sketch.solve.components[0],entityIds:['a','unavailable','b']}]}})[0].key,result.key,'display order does not become component identity');
 assert.deepEqual(issues(sketch).map(item=>item.issue),['CONFLICTING','REDUNDANT','SUPPRESSED']);
 assert.equal(components({...sketch,solve:{components:[{entityIds:['a'],constraintIds:[],status:'REDUNDANT',definitionStatus:'FULLY_CONSTRAINED',degreesOfFreedom:0}]}})[0].known,true,'redundancy remains orthogonal to definition status');
 console.log('PASS: accepted diagnostic scopes, stable component keys and honest DOF availability');
}finally{await server.close();}
