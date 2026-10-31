import type {DocumentStructureNode,DocumentView,InstancePath,SelectionItem,Vec3} from '../../types';
export type InterferenceScope='ALL'|'SELECTED'|'AGAINST_ALL'|'BETWEEN';
export type StaticInterferenceBody={id:string;name:string;documentId:string;revisionId:string;instancePath:InstancePath;bodyId:string;variantKey?:string;geometryKey?:string;pose:{translation:Vec3;rotation:[number,number,number,number]};diagnosticCode?:string};
export type StaticInterferencePair={firstId:string;secondId:string;classification:string;distanceExact?:boolean;witnessKind?:'NONE'|'DISTANCE'|'CONTACT';detailStatus?:string;containment?:string;sectionPoints?:Vec3[];overlapTriangles?:Vec3[];distanceMm:number;commonVolumeMm3:number;firstWitness:Vec3;secondWitness:Vec3;complete:boolean;clearanceSatisfied:boolean;diagnostic?:string};
export type StaticInterferenceReport={source?:string;mechanismId?:string;frameSequence?:number;documentId:string;revisionId:string;inputDigest:string;policy:string;scope:InterferenceScope;clearanceMm:number;toleranceMm:number;bodies:StaticInterferenceBody[];pairs:StaticInterferencePair[];issues:{name:string;occurrence:string;diagnosticCode:string;diagnostic?:string}[];complete:boolean;checkedPairs:number;layoutParts:number;consumedBodies:number;kernelBuild?:string};
export type StaticInterferenceInput={revisionId:string;scope:InterferenceScope;selected?:InstancePath[];clearanceMm:number;secondSelection?:InstancePath[];detailPair?:string[];thresholdOnly?:boolean;mechanismId?:string};

export function staticPartLeaves(view:DocumentView):Map<string,{name:string;path:InstancePath}>{
 const result=new Map<string,{name:string;path:InstancePath}>();
 const visit=(n:DocumentStructureNode)=>{const path=n.instancePath??n.occurrence?.instancePath;if(['PART','PRODUCT','INSTANCE'].includes(n.kind)&&path?.segments.length)result.set(path.canonical,{path,name:path.display||n.name});n.children?.forEach(visit)};
 if(view.structureTree)visit(view.structureTree);
 // Display rows supplement tree leaves; they never establish analysis scope
 // on the server and never replace an unavailable leaf's accepted definition.
 for(const r of view.resolvedInstances??[])if(!result.has(r.occurrencePath))result.set(r.occurrencePath,{path:r.instancePath,name:r.instancePath.display||r.name});
 return result;
}
export function staticPartPick(view:DocumentView,pick:SelectionItem):InstancePath|undefined{
 if(!['part','product','instance','body','face','edge','vertex'].includes(pick.kind))return;
 const leaf=staticPartLeaves(view).get(pick.occurrencePath??pick.instancePath?.canonical??'');
 if(!leaf)return;
 const last=leaf.path.segments.at(-1)!;
 const owner=pick.ownerDocumentId??pick.documentId;
 if(owner&&!(pick.kind==='instance'?[last.referencedDocumentId,last.ownerDocumentId]:[last.referencedDocumentId]).includes(owner))return;
 if(pick.versionId&&!(pick.kind==='instance'||pick.kind==='part'?[last.resolvedVersionId,last.ownerVersionId]:[last.resolvedVersionId]).includes(pick.versionId))return;
 if(pick.instancePath&&JSON.stringify(pick.instancePath.segments.map(s=>[s.instanceId,s.ownerVersionId,s.resolvedVersionId]))!==JSON.stringify(leaf.path.segments.map(s=>[s.instanceId,s.ownerVersionId,s.resolvedVersionId])))return;
 return leaf.path;
}
export function staticPartSelection(view:DocumentView,path:InstancePath):SelectionItem{
 let nodeId:string|undefined;
 const find=(node:DocumentStructureNode)=>{if(['PART','PRODUCT','INSTANCE'].includes(node.kind)&&(node.instancePath??node.occurrence?.instancePath)?.canonical===path.canonical)nodeId=node.id;node.children?.forEach(find)};
 if(view.structureTree)find(view.structureTree);
 return {kind:'instance',id:path.canonical,visualKey:`occurrence:${path.canonical}`,instanceId:path.segments[0]?.instanceId,instancePath:path,occurrencePath:path.canonical,documentId:path.segments.at(-1)?.referencedDocumentId,versionId:path.segments.at(-1)?.resolvedVersionId,treeNodeId:nodeId};
}
export function staticBodySelection(view:DocumentView,b:StaticInterferenceBody):SelectionItem{
 return {kind:'body',id:`${b.instancePath.canonical}:body:${b.bodyId}`,visualKey:`body:${b.instancePath.canonical}:body:${b.bodyId}`,entityId:b.bodyId,bodyId:b.bodyId,documentId:b.documentId,versionId:b.revisionId,geometryKey:b.geometryKey,occurrencePath:b.instancePath.canonical,instancePath:b.instancePath,instanceId:b.instancePath.segments[0]?.instanceId,rootDocumentId:view.document.id,contextVariantKey:b.variantKey,treeNodeId:view.resolvedInstances?.find(r=>r.occurrencePath===b.instancePath.canonical&&r.bodyId===b.bodyId)?.bodyTreeNodeId};
}
export function staticPairLabel(p:StaticInterferencePair):string{
 if(!p.complete)return '未完成';
 return ({PENETRATION:'干涉',CONTAINMENT:p.containment==='COINCIDENT'?'完全重合':'完全包含',CONTACT:'接触'} as Record<string,string>)[p.classification]??(p.clearanceSatisfied?'间隙满足':'间隙不足');
}
export function staticReportStatus(r:StaticInterferenceReport):string{
 if(!r.pairs.length)return r.issues.length?'检查未完成':'没有可检查的零件对';
 if(!r.complete||r.issues.length||r.checkedPairs!==r.pairs.length||r.pairs.some(p=>!p.complete||p.classification==='INCONCLUSIVE'))return r.pairs.some(p=>p.complete&&p.classification!=='INCONCLUSIVE'&&(p.classification!=='SEPARATED'||!p.clearanceSatisfied))?'已发现问题，检查未完成':'检查未完成';
 return r.pairs.some(p=>p.classification!=='SEPARATED'||!p.clearanceSatisfied)?'发现需要检查的零件对':'检查完成，间隙满足';
}
export function staticResultMatches(view:DocumentView,input:StaticInterferenceInput,result:StaticInterferenceReport):boolean{
 const ids=new Set(result.bodies.map(b=>b.id));
 return result.documentId===view.document.id&&result.revisionId===input.revisionId&&view.document.versionId===input.revisionId&&result.scope===input.scope&&result.clearanceMm===input.clearanceMm&&(result.mechanismId??'')===(input.mechanismId??'')&&
  result.bodies.every(b=>input.scope==='ALL'||input.scope==='AGAINST_ALL'||[...(input.selected??[]),...(input.secondSelection??[])].some(p=>b.instancePath.canonical===p.canonical||b.instancePath.canonical.startsWith(p.canonical+'/')))&&
  result.pairs.every(p=>ids.has(p.firstId)&&ids.has(p.secondId)&&p.firstId!==p.secondId);
}

export function interferenceProblem(p:StaticInterferencePair){return !p.complete||p.classification!=='SEPARATED'||!p.clearanceSatisfied}
export function interferencePairKey(p:StaticInterferencePair){return JSON.stringify([p.firstId,p.secondId])}
export function interferenceNumber(value:number):string {return new Intl.NumberFormat('zh-CN',{maximumSignificantDigits:9}).format(value)}
export function interferenceGroups(r:StaticInterferenceReport){
 const result=new Map<string,{key:string;name:string;pairs:StaticInterferencePair[]}>();
 for(const pair of r.pairs){
  for(const id of [pair.firstId,pair.secondId]){
   const body=r.bodies.find(b=>b.id===id);if(!body)continue;
   const key=body.instancePath.canonical;let group=result.get(key);
   if(!group){group={key,name:body.instancePath.display||body.name,pairs:[]};result.set(key,group)}
   group.pairs.push(pair);
  }
 }
 return [...result.values()];
}
