import type {AssemblyGeometryRef,ContextInput,DocumentView,InstancePath,Publication} from '../../types';
export type ReplacementPublication=Pick<Publication,'id'|'name'|'type'|'contract'|'compatibilityVersion'|'resolution'>;
export function replacementCompatible(a:ReplacementPublication,b:ReplacementPublication){
 const c=a.contract,d=b.contract;
 const equal=(x:unknown,y:unknown):boolean=>{
  if(x===y)return true;if(!x||!y||typeof x!=='object'||typeof y!=='object')return false;
  const keys=Object.keys(x);return keys.length===Object.keys(y).length&&keys.every(k=>equal((x as Record<string,unknown>)[k],(y as Record<string,unknown>)[k]));
 };
 return a.type===b.type&&a.compatibilityVersion===b.compatibilityVersion&&c.geometryKind===d.geometryKind&&c.valueType===d.valueType&&c.unitPolicy===d.unitPolicy&&c.symmetry===d.symmetry&&equal(c.dimension,d.dimension)&&equal(c.bounds,d.bounds)&&b.resolution.status==='CONNECTED';
}
export function replacementImpact(view:DocumentView,id:string){
 const used=new Set<string>(),bindings:string[]=[],constraints:string[]=[],forwarding:string[]=[],blocked:string[]=[];
 const uses=(p?:InstancePath)=>p?.segments[0]?.instanceId===id;
 const direct=(p?:InstancePath)=>uses(p)&&p?.segments.length===1;
 for(const b of view.product?.contextBindings??[]){
  if(uses(b.owningInstancePath)){bindings.push(b.name);if(!direct(b.owningInstancePath)||!b.sourceInstancePath||b.sourceInstancePath.segments.length!==1)blocked.push('关联使用深层路径，本轮不迁移');if(b.referenceMode==='PINNED')blocked.push('该组件接收固定输入，请先明确处理关联');}
  if(!uses(b.sourceInstancePath))continue;bindings.push(b.name);used.add(b.publication.publicationId);
  if(!direct(b.sourceInstancePath))blocked.push('关联使用深层来源，请先转发为组件公开接口');
  if(b.referenceMode==='PINNED')blocked.push('该组件提供固定输入，请先明确处理关联');
 }
 for(const p of view.product?.publications??[]){if(!uses(p.target.instancePath))continue;forwarding.push(p.name);used.add(p.target.publicationId);if(!direct(p.target.instancePath))blocked.push('转发接口使用深层来源');}
 const refUses=(r?:AssemblyGeometryRef)=>r?.instanceId===id||uses(r?.instancePath);
 for(const c of view.product?.constraints??[]){const refs=[c.first,c.second,c.angleAxis].filter(r=>refUses(r)),members=(c.groupMembers??[]).filter(m=>m.instanceId===id||uses(m.instancePath));if(!refs.length&&!members.length)continue;constraints.push(c.name||'装配约束');
  for(const m of members)if(m.instancePath&&!direct(m.instancePath))blocked.push('固联组使用深层成员，本轮不迁移');
  for(const r of refs){if(r?.instancePath&&!direct(r.instancePath))blocked.push('约束使用深层来源');
   if(r?.publicationRef)used.add(r.publicationRef.publicationId);else if(!(c.kind==='FIX'&&r?.kind==='BODY'&&!r.capturedFrame))blocked.push('约束支撑未使用公开接口，请先重新指定');}
 }
 if(view.product?.kinematics?.mechanisms.some(m=>m.unitIds.includes(id)))blocked.push('该组件属于运动机构，本轮不迁移机构关系');
 const incoming=(view.product?.contextBindings??[]).filter(b=>uses(b.owningInstancePath));
 if(incoming.length&&(view.product?.contextBindings??[]).some(b=>uses(b.sourceInstancePath)))blocked.push('该接收零件同时提供其他关联输入，本轮不迁移');
 const receiverPart=view.contextVariants?.find(v=>v.owningInstancePath.segments.length===1&&v.owningInstancePath.segments[0].instanceId===id)?.part;
 return {incoming,receiverInputs:receiverPart?.contextInputs??[],used:[...used].sort(),bindings,constraints,forwarding,blocked:[...new Set(blocked)]};
}
export function publicDescriptors(view:DocumentView):ReplacementPublication[]{return view.part?.publications??view.product?.publications??[];}

export function replacementInputCompatible(a:ContextInput,b:ContextInput){
 return replacementCompatible({...a,compatibilityVersion:'1',resolution:{status:'CONNECTED'}},{...b,compatibilityVersion:'1',resolution:{status:'CONNECTED'}})&&a.target.kind===b.target.kind;
}
export function receiverMappingComplete(old:ContextInput[],next:ContextInput[],ids:string[],mapping:Record<string,string>){
 const targets=new Set<string>();
 for(const id of ids){const a=old.find(i=>i.id===id),b=next.find(i=>i.id===mapping[id]);if(!a||!b||!replacementInputCompatible(a,b))return false;
  const key=b.target.kind==='SKETCH_EXTERNAL_GEOMETRY'?b.id:`${b.target.kind}:${b.target.targetId}`;if(targets.has(key))return false;targets.add(key);
 }
 return next.every(i=>!i.required||ids.some(id=>mapping[id]===i.id));
}
