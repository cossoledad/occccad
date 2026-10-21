import {randomUUID} from "../../utils/random-uuid";
import type {AutomationContext,AutomationTask,TaskInput,TaskStep} from "./automation-api";

export const pendingStep=(task:AutomationTask):TaskStep|undefined=>task.steps.find(step=>!["DISPLAYED","APPLIED","SUPERSEDED","CANCELLED","HANDED_OFF","UI_OPENED","UI_FAILED","UI_UNKNOWN","UI_CANCELLED","UI_COMPLETED"].includes(step.phase));
export function rectangularExtrudePlan(context:AutomationContext,width:number,height:number,length:number):TaskInput{
  if(context.snapshot.documentType!=="PART"||!context.workingBodyId||![width,height,length].every(value=>Number.isFinite(value)&&value>0&&value<=1e6))throw new Error("请选择可编辑的零件并输入正的毫米尺寸");
  const sketch=randomUUID(),rectangle=randomUUID(),extrude=randomUUID();
  return {requestId:randomUUID(),goal:`创建 ${width} × ${height} mm 矩形并拉伸 ${length} mm`,expectedHead:{workspaceId:context.snapshot.workspaceId,revisionId:context.snapshot.revisionId,sequence:context.snapshot.sequence},steps:[
    {id:sketch,operationId:"sketch.create",version:1,arguments:{bodyId:context.workingBodyId,plane:"XY"}},
    {id:rectangle,operationId:"sketch.rectangle",version:1,arguments:{width,height,unit:"mm"},bindings:{sketchId:sketch}},
    {id:extrude,operationId:"part.extrude",version:1,arguments:{bodyId:context.workingBodyId,length,unit:"mm"},bindings:{sketchId:sketch}},
  ]};
}
export function displayAcknowledgment(task:AutomationTask,displayed?:{documentId:string;revisionId:string}):{stepId:string;revisionId:string}|undefined{
  const step=pendingStep(task),revisionId=step?.result?.receipt.revisionId;
  if(step?.phase!=="COMMITTED"||!revisionId||displayed?.documentId!==task.documentId||displayed.revisionId!==revisionId)return undefined;
  return {stepId:step.id,revisionId};
}
export function newerTask(current:AutomationTask|undefined,next:AutomationTask):AutomationTask{return current?.id===next.id&&current.version>next.version?current:next}
export const phaseText:Record<TaskStep["phase"],string>={HANDED_OFF:"工具打开结果待核对",UI_OPENED:"工具已打开，操作待完成",UI_FAILED:"工具未打开",UI_UNKNOWN:"工具结果未知",UI_CANCELLED:"用户已取消工具",UI_COMPLETED:"工具操作回执已核对",PLANNED:"已规划",PREPARING:"正在预览",PREVIEW_READY:"预览就绪",COMMITTING:"正在提交",COMMITTED:"模型已应用，未单独呈现",APPLIED:"模型已应用，未单独呈现",DISPLAYED:"已实际呈现",SUPERSEDED:"已应用，被后续版本替代",CANCELLED:"修订后取消",FAILED:"执行失败"};
