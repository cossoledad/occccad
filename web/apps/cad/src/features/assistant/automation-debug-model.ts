import type {DocumentView,SelectionItem} from "../../types";
import type {FrozenScope} from "./assistant-api";
import type {ScopeSource} from "./frozen-scope";

export type DebugTool="cad_query_model"|"cad_query_topology"|"cad_context";
export type DebugQueryInput={scope:FrozenScope;tool:DebugTool;arguments:Record<string,unknown>;selectedOnly:boolean};
type ToolResult={callId:string;content:unknown;isError:boolean};
export type DebugQueryResult={input:DebugQueryInput;toolCall:{id:string;name:string;arguments:Record<string,unknown>};serviceResult?:ToolResult;serviceBytes:number;serviceLimitBytes:number;serviceOmitted:boolean;toolResult:ToolResult;toolBytes:number;toolLimitBytes:number;outputReplaced:boolean;supplement:{snapshot:unknown;page?:Record<string,unknown>;selectionResolutions?:unknown[]}};

export function debugSelectionSource(view:DocumentView,selections:readonly SelectionItem[]):ScopeSource {
 const copied=structuredClone(selections);
 const first=copied[0];
 const documentId=first?.entityRef?.documentId??first?.ownerDocumentId??first?.documentId??view.document.id;
 const revisionId=first?.snapshotScope?.revisionId??first?.versionId??(documentId===view.document.id?view.document.versionId:undefined);
 if(!revisionId)throw new Error("所选来源缺少明确 Revision；不会改查最新版本。");
 let bodyId=first?.bodyId;
 for(const item of copied){
  const owner=item.entityRef?.documentId??item.ownerDocumentId??item.documentId??view.document.id;
  const revision=item.snapshotScope?.revisionId??item.versionId??(owner===view.document.id?view.document.versionId:undefined);
  if(owner!==documentId||revision!==revisionId)throw new Error("请固定同一文档和 Revision 的选择。");
  if(item.contextVariantKey)throw new Error("当前定义查询不支持上下文变体，不能用普通 Body 数据替代。");
  if(bodyId&&item.bodyId&&bodyId!==item.bodyId)throw new Error("请固定同一个 Body 的选择。");
  bodyId??=item.bodyId;
 }
 if(!bodyId&&!copied.length&&documentId===view.document.id)bodyId=view.part?.activeBodyId;
 return {rootDocumentId:view.document.id,rootRevisionId:view.document.versionId,documentId,revisionId,workingBodyId:bodyId,selections:copied};
}

// Explicit execution freezes the draft. Viewing a result never calls this or
// reads the live selection again; pagination derives only from this snapshot.
export function freezeDebugQuery(input:DebugQueryInput):DebugQueryInput {
 if(!input.scope.documentId.trim()||!input.scope.revisionId.trim())throw new Error("请固定文档和 Revision。");
 if(!input.arguments||typeof input.arguments!=="object"||Array.isArray(input.arguments))throw new Error("查询参数必须是 JSON 对象。");
 return structuredClone(input);
}
export function nextDebugQuery(input:DebugQueryInput,cursor:string):DebugQueryInput {
 const next=freezeDebugQuery(input);
 next.arguments.cursor=cursor;
 return next;
}

export const debugRecord=(value:unknown):Record<string,unknown>|undefined=>value!==null&&typeof value==="object"&&!Array.isArray(value)?value as Record<string,unknown>:undefined;
