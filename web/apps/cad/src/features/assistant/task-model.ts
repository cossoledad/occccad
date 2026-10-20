import {randomUUID} from "../../utils/random-uuid";
import type {AutomationContext,AutomationTask,TaskInput,TaskStep} from "./automation-api";

export const pendingStep=(task:AutomationTask):TaskStep|undefined=>task.steps.find(step=>step.phase!=="DISPLAYED"&&step.phase!=="SUPERSEDED"&&step.phase!=="HANDED_OFF"&&step.phase!=="UI_OPENED"&&step.phase!=="UI_FAILED");
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
export const phaseText:Record<TaskStep["phase"],string>={HANDED_OFF:"工具状态待确认",UI_OPENED:"已打开界面工具",UI_FAILED:"工具未打开",PLANNED:"已规划",PREPARING:"正在预览",PREVIEW_READY:"预览就绪",COMMITTING:"正在提交",COMMITTED:"已提交，等待显示",DISPLAYED:"已显示",SUPERSEDED:"已提交，当前版本已变化",FAILED:"执行失败"};
