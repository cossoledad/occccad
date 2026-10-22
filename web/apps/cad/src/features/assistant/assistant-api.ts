import {request} from "../../api";
import type {PersistentSelection} from "../../types";

export type Thinking="off"|"low"|"high"|"max";
export type AssistantModel={id:string;name?:string;effort?:{supported_levels:string[]}};
export type AssistantSettings={configured:boolean;provider:string;model:string;thinking:Thinking;tokenBudget:number|null;version:number};
export type FrozenSelection={kind:string;id?:string;bodyId?:string;featureId?:string;publicationId?:string;datumId?:string;axis?:string;instancePath?:string[];selection?:PersistentSelection;sourceDocumentId?:string;sourceVersionId?:string};
export type FrozenScope={rootDocumentId:string;documentId:string;revisionId:string;rootRevisionId?:string;workingBodyId?:string;editContextId?:string;instancePath?:string[];selections:FrozenSelection[];selectionCount:number;complete:boolean};
export type RunMode="AUTOMATIC"|"CONFIRM";
export type RunStatus="QUEUED"|"PLANNING"|"EXECUTING"|"WAITING_INPUT"|"WAITING_CONFIRMATION"|"WAITING_UI"|"WAITING_BUDGET"|"WAITING_RETRY"|"PAUSED"|"STOPPED"|"COMPLETED"|"BLOCKED";
export type RunUsage={prompt_tokens:number;completion_tokens:number;total_tokens:number;unknownAttempts:number;reportedCalls:number};
export type RunFailure={code:string;message:string;retryable:boolean};
export type AssistantRun={id:string;documentId:string;version:number;epoch:number;goalVersion:number;goal:string;status:RunStatus;mode:RunMode;scope:FrozenScope;activeTaskId?:string;stageCount:number;stepCount:number;modelCalls:number;toolCalls:number;messageSequence:number;eventSequence:number;usage:RunUsage;tokenBudget:number|null;appliedCount:number;requestAppliedCount:number;latestAppliedRevisionId?:string;latestRevisionId?:string;presentedRevisionId?:string;reply?:string;failure?:RunFailure;waitingReason?:string;createdAt?:string;updatedAt?:string};
export type RunInput={requestId:string;text:string;workingBodyId?:string;scope:FrozenScope;mode:RunMode;tokenBudget?:number|null};
export type MessageInput={requestId:string;text:string;scope:FrozenScope};
export type AssistantTurn={id:string;runId:string;sequence:number;requestId:string;text:string;kind:"GOAL"|"STEER"|"QUERY";status:"RECEIVED"|"APPLIED"|"ANSWERED"|"WAITING_BOUNDARY"|"WAITING_INPUT"|"WAITING_BUDGET"|"CANCELLED"|"FAILED";reply?:string;scope:FrozenScope;goalVersion:number;failure?:RunFailure};
export type RunEvent={sequence:number;epoch:number;type:string;at:string;data?:{message?:string;summary?:string;[key:string]:unknown}};
export type RunStage={sequence:number;epoch:number;goalVersion:number;taskId:string;status:string};
export type RuntimeStamp={engine:string;revision?:string;modified?:boolean;catalogDigest:string;promptDigest:string};
export type PublicModelMessage={role:string;content:string;tool_calls?:{id:string;function:{name:string;arguments:string}}[];tool_call_id?:string};
export type ModelCallSummary={id:string;sequence?:number;epoch:number;inputId?:string;status:string;startedAt:string;finishedAt?:string;model:string;reply?:string;toolNames:string[];requestAvailable:boolean;failure?:RunFailure};
export type ModelTracePage={items:ModelCallSummary[];hasEarlier:boolean;nextCursor?:string;runtime:RuntimeStamp};
export type ModelCallTrace={call:{id:string;status:string;epoch:number;sequence?:number;generation:{model:string;thinking:string;maxTokens:number};startedAt:string;finishedAt?:string;requestDigest:string;response?:PublicModelMessage;failure?:RunFailure;runtime?:RuntimeStamp};request?:{callId:string;messages:PublicModelMessage[];tools:unknown[];generation:unknown;runtime:RuntimeStamp};tools:{id:string;name:string;arguments:unknown;feedback?:unknown;isError:boolean;executed:boolean;execution?:{taskId:string;status:string;steps:{id:string;operationId:string;phase:string;arguments:unknown;failure?:RunFailure;revisionId?:string}[]}}[]};
export type RunPage={items:AssistantRun[];nextCursor?:string;hasMore:boolean};
export type SequencePage<T>={items:T[];nextAfter:number;hasMore:boolean;hasEarlier?:boolean};
export type RunControl={action:"pause"|"resume"|"stop"|"budget"|"mode"|"confirm"|"presented"|"handoff_start"|"handoff_finish";expectedVersion:number;epoch:number;tokenBudget?:number|null;mode?:RunMode;taskId?:string;stepId?:string;taskVersion?:number;previewId?:string;revisionId?:string;handoffId?:string;outcome?:"UI_OPENED"|"FAILED"|"UNKNOWN"|"CANCELLED"|"COMPLETED";requestId?:string};
const path=(id:string)=>`/api/documents/${encodeURIComponent(id)}/assistant/runs`;
const runPath=(id:string,runId:string)=>`${path(id)}/${encodeURIComponent(runId)}`;
const sequenceQuery=(page?:{after?:number;before?:number;limit?:number})=>{
 const query=new URLSearchParams();for(const [key,value] of Object.entries(page??{}))if(value!==undefined)query.set(key,String(value));return query.size?`?${query}`:"";
};
export const assistantApi={
 settings:()=>request<AssistantSettings>("/api/assistant/settings"),
 saveKey:(apiKey:string)=>request<AssistantSettings>("/api/assistant/settings",{method:"PUT",body:JSON.stringify({apiKey})}),
 models:()=>request<{models:AssistantModel[]}>("/api/assistant/models"),
 savePreferences:(preferences:{model?:string;thinking?:Thinking;tokenBudget?:number|null})=>request<AssistantSettings>("/api/assistant/settings",{method:"PUT",body:JSON.stringify(preferences)}),
 deleteKey:()=>request<{configured:boolean}>("/api/assistant/settings",{method:"DELETE"}),
 runs:(id:string,cursor?:string)=>request<RunPage>(`${path(id)}${cursor?`?cursor=${encodeURIComponent(cursor)}`:""}`),
 run:(id:string,runId:string)=>request<AssistantRun>(runPath(id,runId)),
 create:(id:string,input:RunInput)=>request<AssistantRun>(path(id),{method:"POST",body:JSON.stringify(input)}),
 send:(id:string,runId:string,input:MessageInput)=>request<{run:AssistantRun;turn:AssistantTurn}>(`${runPath(id,runId)}/messages`,{method:"POST",body:JSON.stringify(input)}),
 messages:(id:string,runId:string,page?:{after?:number;before?:number;limit?:number})=>request<SequencePage<AssistantTurn>>(`${runPath(id,runId)}/messages${sequenceQuery(page)}`),
 events:(id:string,runId:string,page?:{after?:number;before?:number;limit?:number})=>request<SequencePage<RunEvent>>(`${runPath(id,runId)}/events${sequenceQuery(page)}`),
 trace:(id:string,runId:string,cursor?:string)=>request<ModelTracePage>(`${runPath(id,runId)}/trace${cursor?`?cursor=${encodeURIComponent(cursor)}`:""}`),
 callTrace:(id:string,runId:string,callId:string)=>request<ModelCallTrace>(`${runPath(id,runId)}/calls/${encodeURIComponent(callId)}`),
 stages:(id:string,runId:string,page?:{after?:number;before?:number;limit?:number})=>request<SequencePage<RunStage>>(`${runPath(id,runId)}/stages${sequenceQuery(page)}`),
 control:(id:string,runId:string,input:RunControl)=>request<AssistantRun>(`${runPath(id,runId)}/control`,{method:"POST",body:JSON.stringify(input)}),
};
