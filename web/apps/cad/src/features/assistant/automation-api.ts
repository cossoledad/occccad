import {request} from "../../api";
import {realtime} from "../../api/realtime-client";
import type {CommandPreview} from "../../types";

export type Baseline={workspaceId:string;revisionId:string;sequence:number};
export type EntityRef={documentId:string;revisionId:string;kind:string;id:string};
export type AutomationContext={snapshot:Baseline&{documentId:string;documentType:string;modelHash:string};permission:string;units:string;workingBodyId?:string;counts:Record<string,number>;catalogDigest:string;scope:string};
export type StepSpec={id:string;operationId:string;version:number;arguments:Record<string,unknown>;bindings?:Record<string,string>};
export type TaskInput={requestId:string;goal:string;expectedHead:Baseline;steps:StepSpec[]};
export type TaskStep=StepSpec&{phase:"PLANNED"|"PREPARING"|"PREVIEW_READY"|"COMMITTING"|"COMMITTED"|"DISPLAYED"|"SUPERSEDED"|"FAILED";action?:{arguments:Record<string,unknown>;requestId:string};preview?:CommandPreview;result?:{receipt:Baseline&{transactionId:string;requestId:string};output?:EntityRef};failure?:{code:string;message:string;retryable:boolean}};
export type AutomationTask={id:string;documentId:string;version:number;goal:string;status:"ACTIVE"|"PAUSED"|"STOPPED"|"COMPLETED";baseline:Baseline;steps:TaskStep[];eventSequence:number};
export type OperationSummary={id:string;version:number;name:string;description:string;documentType:string};
const path=(documentId:string)=>`/api/documents/${encodeURIComponent(documentId)}/automation`;
export const automationApi={
  context:(id:string)=>request<AutomationContext>(`${path(id)}/context`),
  operations:()=>request<{operations:OperationSummary[];catalogDigest:string}>("/api/automation/operations?documentType=PART"),
  tasks:(id:string)=>request<{tasks:AutomationTask[]}>(`${path(id)}/tasks`),
  task:(id:string,taskId:string)=>request<AutomationTask>(`${path(id)}/tasks/${encodeURIComponent(taskId)}`),
  create:(id:string,input:TaskInput)=>request<AutomationTask>(`${path(id)}/tasks`,{method:"POST",body:JSON.stringify(input)}),
  advance:(id:string,task:AutomationTask)=>realtime.automation<AutomationTask>({documentId:id,method:"task.advance",taskId:task.id,expectedVersion:task.version}),
  control:(id:string,task:AutomationTask,action:"pause"|"resume"|"stop"|"displayed",stepId?:string,revisionId?:string,expectedHead?:Baseline)=>realtime.automation<AutomationTask>({documentId:id,method:"task.control",taskId:task.id,control:{action,expectedVersion:task.version,stepId,revisionId,expectedHead}}),
};
