import type {CommandRegistry,CadCommandInvocation} from "../../cad/command/command-registry";
import type {SpecificationTreeNode} from "../workbench/specification-tree";

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
