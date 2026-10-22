import type {AssistantRun,AssistantTurn,RunEvent,SequencePage} from "./assistant-api";

export function newerRun(current:AssistantRun|undefined,next:AssistantRun):AssistantRun{
 return current?.id===next.id&&(current.epoch>next.epoch||current.version>next.version)?current:next;
}
// Pages remain separate; the caller chooses a bounded history window.
export function mergeMessagePage(current:SequencePage<AssistantTurn>|undefined,next:SequencePage<AssistantTurn>,limit=30):SequencePage<AssistantTurn>{
 const items=new Map((current?.items??[]).map(item=>[item.sequence,item]));
 const order:Record<AssistantTurn["status"],number>={RECEIVED:0,WAITING_BOUNDARY:1,WAITING_INPUT:2,WAITING_BUDGET:2,APPLIED:2,ANSWERED:3,FAILED:3,CANCELLED:3};
 for(const item of next.items){const previous=items.get(item.sequence);if(!previous||(order[item.status]??2)>=(order[previous.status]??2))items.set(item.sequence,item)}
 const all=[...items.values()].sort((a,b)=>a.sequence-b.sequence);
 return {...next,items:all.slice(-limit),hasEarlier:Boolean(current?.hasEarlier||next.hasEarlier||all.length>limit),nextAfter:Math.max(current?.nextAfter??0,next.nextAfter??0,...all.map(item=>item.sequence),0)};
}
export const runActive=(run:AssistantRun)=>!["COMPLETED","STOPPED","BLOCKED"].includes(run.status);
export function presentedAcknowledgment(run:AssistantRun,displayed?:{documentId:string;revisionId:string}):string|undefined{
 return displayed?.documentId===run.documentId&&displayed.revisionId===run.latestRevisionId&&run.presentedRevisionId!==displayed.revisionId?displayed.revisionId:undefined;
}
export const runStatusText:Record<AssistantRun["status"],string>={QUEUED:"等待调度",PLANNING:"正在规划",EXECUTING:"正在建模",WAITING_INPUT:"等待补充信息",WAITING_CONFIRMATION:"等待确认预览",WAITING_UI:"等待界面操作",WAITING_BUDGET:"等待调整预算",WAITING_RETRY:"等待重试",PAUSED:"已暂停",STOPPED:"已停止",COMPLETED:"本次已结束",BLOCKED:"需要处理问题"};
export const turnStatusText:Record<AssistantTurn["status"],string>={RECEIVED:"已收到",WAITING_BOUNDARY:"等待当前操作结束后调整",APPLIED:"已收到",ANSWERED:"已回复",WAITING_INPUT:"等待回复",WAITING_BUDGET:"等待调整预算",CANCELLED:"已取消",FAILED:"未能处理"};

export function assistantFailure(failure:{code?:string;message?:string}|undefined):string|undefined{
 if(!failure)return undefined;
 const messages:Record<string,string>={FEATURE_EDIT_STALE:"特征已变化，需要重新读取后再编辑。",AI_NO_PROGRESS:"暂时无法继续，请调整要求或接管操作。"};
 return messages[failure.code??""]??failure.message;
}
// Keep a bounded recent journal window. Public explanations and operation
// receipts are readable progress; provider accounting remains in details.
export function mergeProgressPage(current:SequencePage<RunEvent>|undefined,next:SequencePage<RunEvent>,limit=200):SequencePage<RunEvent>{
 const merged=new Map((current?.items??[]).map(event=>[event.sequence,event]));
 for(const event of next.items)merged.set(event.sequence,event);
 const all=[...merged.values()].sort((a,b)=>a.sequence-b.sequence);
 return {...next,items:all.slice(-limit),hasEarlier:Boolean(current?.hasEarlier||next.hasEarlier||all.length>limit),nextAfter:Math.max(current?.nextAfter??0,next.nextAfter??0)};
}
export function conversationProgress(events:RunEvent[],run:AssistantRun):RunEvent[]{
 const visible=events.filter(event=>event.epoch===run.epoch&&event.data?.goalVersion===run.goalVersion&&(event.data?.sourceEpoch===undefined||event.data.sourceEpoch===run.epoch)&&!event.data?.stale&&!event.data?.inputId&&Boolean(event.data?.message)&&["ASSISTANT_MESSAGE","CAD_APPLIED","TOOL_RESULT","STAGE_DIAGNOSTIC"].includes(event.type));
 const result:RunEvent[]=[];
 for(const event of visible){
  const previous=result.at(-1);
  if(event.type==="TOOL_RESULT"&&previous?.type==="TOOL_RESULT")result[result.length-1]=event;
  else if(event.data?.message!==previous?.data?.message)result.push(event);
 }
 return result.slice(-24);
}

const cadCheckNames:Record<string,string>={RECTANGLE_DIMENSIONS:"矩形尺寸",EXTRUSION_LENGTH:"基体拉伸",ALL_EDGES_FILLET:"全部边圆角",SELECTED_EDGES_FILLET:"选中边圆角",HOLE_RIM_FILLET:"孔口圆角",CENTERED_THROUGH_HOLE:"中心贯穿孔",THROUGH_HOLE:"贯穿孔",ENTITY_EXISTS:"目标实体",BODY_GEOMETRY_VALID:"实体有效性"};
export const cadCheckLabel=(kind:string)=>cadCheckNames[kind]??kind;
