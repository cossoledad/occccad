import {request} from "../../api";
import type {CommandPreview} from "../../types";

export type Baseline={workspaceId:string;revisionId:string;sequence:number};
export type EntityRef={documentId:string;revisionId:string;kind:string;id:string};
export type AutomationContext={snapshot:Baseline&{documentId:string;documentType:string;modelHash:string};permission:string;units:string;workingBodyId?:string;counts:Record<string,number>;catalogDigest:string;scope:string};
export type StepSpec={id:string;operationId:string;version:number;arguments:Record<string,unknown>;bindings?:Record<string,string>};
export type TaskInput={requestId:string;goal:string;expectedHead:Baseline;steps:StepSpec[]};
export type TaskStep=StepSpec&{phase:"UI_OPENED"|"UI_FAILED"|"UI_UNKNOWN"|"UI_CANCELLED"|"UI_COMPLETED"|"HANDED_OFF"|"PLANNED"|"PREPARING"|"PREVIEW_READY"|"COMMITTING"|"COMMITTED"|"APPLIED"|"DISPLAYED"|"SUPERSEDED"|"CANCELLED"|"FAILED";action?:{arguments:Record<string,unknown>;requestId:string};handoff?:{id:string;runId?:string;planEpoch:number;uiCommand:string;baseline:Baseline;domainRequestId:string};preview?:CommandPreview;result?:{receipt:Baseline&{transactionId:string;requestId:string};output?:EntityRef};failure?:{code:string;message:string;retryable:boolean}};
export type AutomationTask={id:string;documentId:string;version:number;goal:string;status:"ACTIVE"|"WAIT_UI"|"PAUSED"|"STOPPED"|"COMPLETED";baseline:Baseline;steps:TaskStep[];eventSequence:number;runId?:string;planEpoch?:number;executionMode?:string};
export type OperationSummary={id:string;version:number;name:string;description:string;documentType:string;effect:string};
const path=(documentId:string)=>`/api/documents/${encodeURIComponent(documentId)}/automation`;
export const automationApi={
  operations:()=>request<{operations:OperationSummary[];catalogDigest:string}>("/api/automation/operations?all=true"),
  task:(id:string,taskId:string)=>request<AutomationTask>(`${path(id)}/tasks/${encodeURIComponent(taskId)}`),
};
