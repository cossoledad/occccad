import type {SketchFeature} from '../../types';

export function sketchDiagnosticComponents(sketch:SketchFeature){
 const ids=new Set(sketch.entities.map(e=>e.id)),constraints=new Set(sketch.constraints.map(c=>c.id));
 return (sketch.solve.components??[]).map(component=>({
  ...component,key:JSON.stringify([[...component.entityIds].sort(),[...component.constraintIds].sort()]),
  entityIds:component.entityIds.filter(id=>ids.has(id)),constraintIds:component.constraintIds.filter(id=>constraints.has(id)),
  known:component.definitionStatus!=='UNRESOLVED'&&!['INVALID','CONFLICTING','EXECUTION_FAILURE','FAILED','UNRESOLVED_EXTERNAL'].includes(component.status),
 }));
}
export function sketchConstraintIssues(sketch:SketchFeature){
 const conflicting=new Set(sketch.solve.conflictingConstraintIds??[]),redundant=new Set(sketch.solve.redundantConstraintIds??[]);
 return sketch.constraints.map(constraint=>({constraint,issue:conflicting.has(constraint.id)?'CONFLICTING':redundant.has(constraint.id)?'REDUNDANT':constraint.suppressed?'SUPPRESSED':'NONE'}));
}
