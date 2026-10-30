import type {HistoricalCreationType} from "../historical-creation";
import {copyTextToClipboard} from "../../../utils/clipboard";
import type {ComponentProps} from 'react';
import type {CadCommand,CadCommandInvocation,CommandOperation} from '../../../cad/command/command-registry';
import type {SpecificationTree,SpecificationTreeNode} from '../specification-tree';
type TreeActions=Pick<ComponentProps<typeof SpecificationTree>,
 'onActivate'|'onOpenDocumentTab'|'onViewResult'|'onMove'|'onInsertAfter'|'onEdit'|'onProperties'|'onCreatePart'|'onReferenceMode'|'onDetach'|'onReconnect'|'onRefresh'|'onDelete'|'onToggleConstruction'|'onToggleVisibility'|'onToggleSuppression'>;
export type StructureActions={[Action in keyof TreeActions]?: (...args:[...Parameters<NonNullable<TreeActions[Action]>>,operation?:CommandOperation])=>void|Promise<void>};
export const structureCommandIDs={onActivate:'tree.activate',onOpenDocumentTab:'tree.open-document',onViewResult:'tree.view-result',onEdit:'tree.edit',onMove:'tree.move',onInsertAfter:'tree.insert-after',onProperties:'tree.properties',onCreatePart:'tree.create-part',onReferenceMode:'tree.reference-mode',onDetach:'tree.detach',onReconnect:'tree.reconnect',onRefresh:'tree.refresh',onDelete:'edit.delete',onToggleConstruction:'tree.construction',onToggleVisibility:'tree.visibility',onToggleSuppression:'tree.suppression'} as const;
export type StructureInvocation={node?:SpecificationTreeNode;nodeKey?:string;nodes?:SpecificationTreeNode[];referenceMode?:'PINNED'|'FOLLOW_HEAD';scope?:'DEFINITION'|'OCCURRENCE';visibilityMode?:'SHOW'|'HIDE'|'INHERIT';modifierType?:HistoricalCreationType};
export function structureInvocation(invocation?:CadCommandInvocation):StructureInvocation {
 if(!invocation?.payload||typeof invocation.payload!=='object')throw new Error('Missing structure command target');
 return invocation.payload as StructureInvocation;
}
export function resolveStructureNode(nodes:readonly SpecificationTreeNode[],target:SpecificationTreeNode):SpecificationTreeNode {
 const find=(nodes:readonly SpecificationTreeNode[]):SpecificationTreeNode|undefined=>{
  for(const node of nodes){if(node.key===target.key)return node;const found=node.children&&find(node.children);if(found)return found;}
 };
 const node=find(nodes);
 if(!node||node.documentId!==target.documentId||node.entityId!==target.entityId||node.ownerDocumentId!==target.ownerDocumentId||node.ownerEntityId!==target.ownerEntityId||node.bodyId!==target.bodyId||node.definitionDigest!==target.definitionDigest||node.sourceRevisionId!==target.sourceRevisionId||node.instancePath?.canonical!==target.instancePath?.canonical||node.selection?.versionId!==target.selection?.versionId)throw new Error('Structure command context changed');
 return node;
}
export function structureCommands(actions:StructureActions={},currentNodes:readonly SpecificationTreeNode[]=[],show:(text:string)=>void=()=>{},selectedNodes:readonly SpecificationTreeNode[]=[]):CadCommand[]{
 const current=(invocation?:CadCommandInvocation,multiple=false)=>{
  const target:StructureInvocation=invocation?.payload?structureInvocation(invocation):{node:selectedNodes.length===1||multiple?selectedNodes[0]:undefined,nodes:multiple?[...selectedNodes]:undefined};
  if(target.nodeKey){
   const find=(nodes:readonly SpecificationTreeNode[]):SpecificationTreeNode|undefined=>{for(const node of nodes){if(node.key===target.nodeKey)return node;const found=node.children&&find(node.children);if(found)return found;}};
   target.node=find(currentNodes);
  }
  if(!target.node)throw new Error('Missing structure node');
  return {...target,node:resolveStructureNode(currentNodes,target.node),nodes:target.nodes?.map(n=>resolveStructureNode(currentNodes,n))};
 };
 const command=(action:keyof StructureActions,execute:(target:StructureInvocation&{node:SpecificationTreeNode},operation?:CommandOperation)=>void|Promise<void>):CadCommand=>({id:structureCommandIDs[action],isEnabled:invocation=>{
   if(!actions[action])return false;
   if(action!=="onEdit"&&action!=="onMove"&&action!=="onInsertAfter")return true;
   try{const {node,nodes}=current(invocation,action==="onMove");return (action!=="onMove"||!nodes?.length||nodes.every(n=>n.presentationRole!=="INPUT_REFERENCE"&&n.capabilities?.includes("MOVE")&&n.documentId===node.documentId&&n.bodyId===node.bodyId&&(n.selection?.occurrencePath??"")===(node.selection?.occurrencePath??"")))&&node.presentationRole!=="INPUT_REFERENCE"&&Boolean(node.capabilities?.includes(action==="onMove"?"MOVE":action==="onInsertAfter"?"INSERT_AFTER":"EDIT"));}catch{return false;}
  },execute:invocation=>{
   const target=current(invocation,action==="onMove");
   const capability:Partial<Record<keyof StructureActions,string>>={onInsertAfter:'INSERT_AFTER',onMove:'MOVE',onEdit:'EDIT',onCreatePart:'CREATE_PART',onDetach:'DETACH',onReconnect:'RECONNECT',onRefresh:'REFRESH',onToggleConstruction:'DELETE',onToggleSuppression:'SUPPRESS'};
   const required=action==='onReferenceMode'?(target.referenceMode==='PINNED'?'PIN_VERSION':'FOLLOW_HEAD'):capability[action];if(required&&!target.node.capabilities?.some(capability=>capability===required))throw new Error('Structure operation unavailable');
   return execute(target,invocation?.operation);
  }});
 return [
  {id:'tree.copy-alias',execute:async invocation=>{
   const {node}=current(invocation);if(!node.parameterAlias)throw new Error('Missing parameter alias');
   const copied=await copyTextToClipboard(node.parameterAlias);
   if(invocation?.operation&&!invocation.operation.current)return;
   if(!copied)throw new Error('剪贴板不可用，请从参数列表选择别名复制');show('已复制参数别名');
  }},
  command('onActivate',({node},operation)=>actions.onActivate?.(node,operation)),
  command('onOpenDocumentTab',({node},operation)=>actions.onOpenDocumentTab?.(node,operation)),
  command('onViewResult',({node},operation)=>actions.onViewResult?.(node,operation)),
  command('onInsertAfter',({node,modifierType},operation)=>actions.onInsertAfter?.(node,modifierType??(node.kind==='SKETCH'?'EXTRUDE_ADD':'FILLET'),operation)),
  command('onMove',({node,nodes},operation)=>{if(nodes?.some(n=>!n.capabilities?.includes('MOVE')||n.presentationRole==='INPUT_REFERENCE'))throw new Error('Structure operation unavailable');return actions.onMove?.(node,nodes,operation)}),
  command('onEdit',({node},operation)=>actions.onEdit?.(node,operation)),
  command('onProperties',({node})=>actions.onProperties?.(node)),
  command('onCreatePart',({node})=>actions.onCreatePart?.(node)),
  command('onReferenceMode',({node,referenceMode})=>{if(!referenceMode)throw new Error('Missing reference mode');actions.onReferenceMode?.(node,referenceMode);}),
  command('onDetach',({node})=>actions.onDetach?.(node)),
  command('onReconnect',({node})=>actions.onReconnect?.(node)),
  command('onRefresh',({node})=>actions.onRefresh?.(node)),
  command('onToggleConstruction',({node})=>actions.onToggleConstruction?.(node)),
  command('onToggleVisibility',({node,scope,visibilityMode})=>{if(!scope)throw new Error('Missing visibility scope');actions.onToggleVisibility?.(node,scope,visibilityMode);}),
  command('onToggleSuppression',({node})=>actions.onToggleSuppression?.(node)),
 ];
}
