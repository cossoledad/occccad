import type {CommandRegistry,CadCommandInvocation} from "../../cad/command/command-registry";
import type {SpecificationTreeNode} from "../workbench/specification-tree";

export type AssistantToolHandoff={runId:string;taskId:string;stepId:string;epoch:number;handoffId:string;domainRequestId:string;documentId:string;baselineRevisionId:string;uiCommand:string};
export type AssistantHandoffSession=AssistantToolHandoff&{state:"OPENED"|"COMMITTED"|"UNKNOWN"};
export function handoffDispatchAllowed(run:{id:string;epoch:number;status:string;activeTaskId?:string},handoff:AssistantToolHandoff):boolean{
 return run.id===handoff.runId&&run.epoch===handoff.epoch&&run.status==="WAITING_UI"&&run.activeTaskId===handoff.taskId;
}
// A handoff belongs to one particular editor. It is never a blanket override
// for the next unrelated Domain Command issued by the workbench.
export function handoffEditorIntent(handoff:AssistantHandoffSession|undefined,documentId:string,revisionId:string,featureType:string):string|undefined{
 if(!handoff||handoff.state!=="OPENED"||handoff.documentId!==documentId||handoff.baselineRevisionId!==revisionId)return;
 const type=handoff.uiCommand==="part.fillet"?"FILLET":handoff.uiCommand==="part.chamfer"?"CHAMFER":undefined;
 return type&&type===featureType?handoff.domainRequestId:undefined;
}
export function handoffMatchesCommit(handoff:AssistantHandoffSession|undefined,documentId:string,revisionId:string,command:Record<string,unknown>):boolean{
 const feature=command.feature as {type?:string}|undefined;
 const intent=typeof feature?.type==="string"?handoffEditorIntent(handoff,documentId,revisionId,feature.type):undefined;
 return Boolean(intent&&command.type==="CREATE_MODIFY_FEATURE"&&command.requestId===intent);
}

export function assistantTarget(nodes:readonly SpecificationTreeNode[],documentId:string,id:string):SpecificationTreeNode|undefined{
 const pending=[...nodes];
 while(pending.length){const node=pending.pop()!;if(node.entityId===id&&(node.ownerDocumentId??node.documentId)===documentId)return node;if(node.children)pending.push(...node.children)}
 return undefined;
}

// The model receives catalog IDs, never executable browser code. Runtime
// availability remains owned by the same registry as buttons and shortcuts.
export async function dispatchAssistantCommand(registry:CommandRegistry,id:string,invocation?:CadCommandInvocation):Promise<void>{
 const state=registry.state(id,invocation);
 if(!registry.declaration(id)||!state.visible||!state.enabled)throw new Error("这个工具在当前编辑状态下不可用，请检查选择对象或结束当前操作后重试");
 if(!await registry.execute(id,invocation))throw new Error("工具未打开或已取消，请从工具栏继续操作");
}
