import type {SketchConstraint,SketchGeometryRef,SketchOperation,SketchPointCapture} from '../../types';

// Capture evidence belongs to a drawing gesture. Only the resulting ordinary
// relationships, with clean stable references, enter a Sketch definition.
export type CapturedSketchReference=SketchGeometryRef&{capture?:SketchPointCapture};
export function cleanCaptureReference(value:CapturedSketchReference):SketchGeometryRef{
 const {capture,...reference}=value;return reference;
}
export function normalizeSketchCaptureOperations(operations:SketchOperation[],existing:readonly SketchConstraint[]=[]):SketchOperation[]{
 const parent=new Map<string,string>();
 const key=(r:SketchGeometryRef)=>JSON.stringify([r.target,r.pointId??r.entityId,r.pointId?'POINT':r.subElement,r.controlPointId??r.controlPointIndex]);
 const root=(k:string):string=>{const p=parent.get(k);if(!p||p===k)return k;const value=root(p);parent.set(k,value);return value};
 const join=(a:SketchGeometryRef,b:SketchGeometryRef)=>parent.set(root(key(a)),root(key(b)));
 const plain=operations.flatMap(op=>op.type==='ADD_CONSTRAINT'&&!op.constraint.references.some(r=>(r as CapturedSketchReference).capture)?[op.constraint]:[]);
 for(const c of [...existing,...plain])if(!c.suppressed&&c.kind==='COINCIDENT'&&c.references.length===2)join(c.references[0],c.references[1]);
 const seen=new Set<string>();
 const relationKey=(kind:string,refs:SketchGeometryRef[])=>JSON.stringify([kind,...refs.map((r,i)=>i===0?root(key(r)):key(r))]);
 for(const c of [...existing,...plain])if(!c.suppressed){seen.add(relationKey(c.kind,c.references));if(c.kind==='MIDPOINT')seen.add(relationKey('POINT_ON_OBJECT',c.references));}
 return operations.flatMap(op=>{
  if(op.type==='ADD_RECTANGLE'||op.type==='CREATE_POLYGON'){
   const a=op.firstReference as CapturedSketchReference|undefined,b=op.type==='ADD_RECTANGLE'?op.secondReference as CapturedSketchReference|undefined:undefined;
   return [{...op,...(a?{firstReference:cleanCaptureReference(a),firstCapture:a.capture}:{}),...(b?{secondReference:cleanCaptureReference(b),secondCapture:b.capture}:{})}];
  }
  if(op.type!=='ADD_CONSTRAINT'||op.constraint.kind!=='COINCIDENT')return [op];
  const target=op.constraint.references[1] as CapturedSketchReference|undefined,capture=target?.capture;if(!capture)return [op];
  const first=cleanCaptureReference(op.constraint.references[0]),result:SketchOperation[]=[];
  for(const [index,support] of capture.references.entries()){
   const refs=[first,cleanCaptureReference(support)],kind=capture.kind==="POINT_ON_OBJECT"&&!["WHOLE","DIRECTION"].includes(support.subElement)?"COINCIDENT":capture.kind;
   if(kind==='COINCIDENT'){if(root(key(first))===root(key(refs[1])))continue;join(first,refs[1]);}
   else {const k=relationKey(kind,refs);if(seen.has(k))continue;seen.add(k);}
   result.push({type:'ADD_CONSTRAINT',constraint:{...op.constraint,id:index?`${op.constraint.id}-${index}`:op.constraint.id,kind,references:refs}});
  }
  return result;
 });
}
