import {registerDocumentState,type DocumentSessions} from '../../cad/document/document-session';
import type {DocumentStructureNode,DocumentView} from '../../types';

export type TreeRevealIntent={documentId:string;entityId:string;occurrence?:string;kind?:string;expandTarget?:boolean;reason:'CREATE'|'EDIT'|'LOCATE'|'ACTIVATE'};
export const treeRevealState=registerDocumentState<{sequence:number;intent?:TreeRevealIntent}>('workbench.tree.reveal');
export function requestTreeReveal(sessions:DocumentSessions,host:string,intent:TreeRevealIntent){const previous=sessions.read(host,treeRevealState);sessions.write(host,treeRevealState,{sequence:(previous?.sequence??0)+1,intent})}

type RevealNode={key:string;kind?:string;documentId?:string;entityId?:string;instancePath?:{canonical:string};children?:RevealNode[]};
export function treeRevealPath(nodes:RevealNode[],intent:TreeRevealIntent):string[]|undefined {
 for(const node of nodes){
  if(node.entityId===intent.entityId&&node.documentId===intent.documentId&&(!intent.kind||node.kind===intent.kind)&&(intent.occurrence===undefined||(node.instancePath?.canonical??'')===intent.occurrence))return [node.key];
  const nested=node.children&&treeRevealPath(node.children,intent);if(nested)return [node.key,...nested];
 }
}

export function revealExpansion(expanded:string[],automatic:string[],path:string[],expandTarget=false){
 const ancestors=expandTarget?path:path.slice(0,-1),priorAuto=new Set(automatic),manual=new Set(expanded.filter(key=>!priorAuto.has(key)));
 return {expanded:[...new Set([...manual,...ancestors])],automatic:ancestors.filter(key=>!manual.has(key))};
}

const entryIdentity=(node:DocumentStructureNode)=>JSON.stringify([node.subject?.documentId??node.documentId,node.subject?.entityId??node.entityId,node.kind,node.instancePath?.canonical??'']);
const creationPriority:Record<string,number>={INSTANCE:0,MECHANISM:1,MOTION_STUDY:1,MEASUREMENT:1,SPATIAL_ANALYSIS:1,PLANE:2,DATUM_AXIS:2,REFERENCE_POINT:2,GEOMETRICAL_SET:3,BODY:5,PARAMETER:9};
export function createdObjectReveal(before:DocumentView|undefined,after:DocumentView,occurrence?:string):TreeRevealIntent|undefined {
 if(!before?.structureTree||!after.structureTree)return;
 const previous=new Set<string>();const collect=(node:DocumentStructureNode)=>{previous.add(entryIdentity(node));node.children?.forEach(collect)};collect(before.structureTree);
 const candidates:DocumentStructureNode[]=[];const visit=(node:DocumentStructureNode)=>{
  if(node.entityId&&node.documentId&&!previous.has(entryIdentity(node))&&node.presentationRole!=='INPUT_REFERENCE'&&node.kind!=='DATUM_INPUT_REFERENCE'&&node.kind!=='JOINT_SUPPORT'&&node.capabilities?.some(c=>c==='EDIT'||c==='DELETE'))candidates.push(node);
  node.children?.forEach(visit);
 };visit(after.structureTree);
 candidates.sort((a,b)=>(creationPriority[a.kind]??4)-(creationPriority[b.kind]??4));
 const target=candidates.find(n=>occurrence===undefined||(n.instancePath?.canonical??'')===occurrence)??candidates[0];
 if(target)return {documentId:target.documentId!,entityId:target.entityId!,kind:target.kind,occurrence:target.instancePath?.canonical,reason:'CREATE'};
}
