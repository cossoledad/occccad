import {request} from "../../api";
export type Thinking="off"|"low"|"high"|"max";
export type BudgetLevel="low"|"medium"|"high";
export type BudgetPreset={id:BudgetLevel;label:string;totalTokens:number;maxModelCalls:number;maxToolCalls:number;maxOutputTokens:number;maxThinkingTokens:number;maxContextBytes:number;timeoutSeconds:number;requestTimeoutSeconds:number};
export type AssistantModel={id:string;name?:string;effort?:{supported_levels:string[]}};
export type AssistantSettings={configured:boolean;provider:string;model:string;thinking:Thinking;budget:BudgetLevel;budgetPresets:BudgetPreset[]};
export type TurnInput={requestId:string;text:string;workingBodyId?:string};
export type AssistantTurn={id:string;requestId:string;documentId:string;text:string;reply:string;status:"THINKING"|"COMPLETED"|"FAILED"|"CANCELLED"|"INTERRUPTED";taskId?:string;failure?:{code:string;message:string;retryable:boolean};createdAt:string;generation?:{model:string;thinking:Thinking;budget:BudgetLevel};budget?:BudgetPreset;modelCalls?:number;usage?:{prompt_tokens:number;completion_tokens:number;total_tokens:number}};
export type Conversation={turns:AssistantTurn[];hasEarlier:boolean};
const path=(id:string)=>`/api/documents/${encodeURIComponent(id)}/assistant/turns`;
export const assistantApi={
  settings:()=>request<AssistantSettings>("/api/assistant/settings"),
  saveKey:(apiKey:string)=>request<AssistantSettings>("/api/assistant/settings",{method:"PUT",body:JSON.stringify({apiKey})}),
  models:()=>request<{models:AssistantModel[]}>("/api/assistant/models"),
  savePreferences:(preferences:{model?:string;thinking?:Thinking;budget?:BudgetLevel})=>request<AssistantSettings>("/api/assistant/settings",{method:"PUT",body:JSON.stringify(preferences)}),
  deleteKey:()=>request<{configured:boolean}>("/api/assistant/settings",{method:"DELETE"}),
  conversation:(id:string)=>request<Conversation>(path(id)),
  send:(id:string,input:TurnInput,signal:AbortSignal)=>request<AssistantTurn>(path(id),{method:"POST",body:JSON.stringify(input),signal}),
  cancel:(id:string,turnId:string)=>request<AssistantTurn>(`${path(id)}/${encodeURIComponent(turnId)}/cancel`,{method:"POST",body:"{}"}),
};
