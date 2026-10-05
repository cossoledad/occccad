import {App,Button} from "antd";
import {DiagnosticCopy,diagnosticReference} from "./diagnostic-copy";
import {useCallback} from "react";

export type OperationFailure={message:string;details:string;code:string;requestId?:string;diagnosticId?:string;phase?:string;retryable:boolean;severity:"error"|"warning"};
export function normalizeOperationFailure(error:unknown):OperationFailure {
  const value=error as {message?:string;code?:string;requestId?:string;correlationId?:string;diagnosticId?:string;phase?:string;retryable?:boolean;terminationStage?:string;terminationReason?:string};
  const raw=value?.message??String(error),code=value?.code??raw.match(/\[([A-Z_]+)\]/)?.[1]??"OPERATION_FAILED";
  const details=value?.terminationStage||value?.terminationReason?JSON.stringify({message:raw,terminationStage:value.terminationStage,terminationReason:value.terminationReason}):raw;
  const retryable=value?.retryable??["ASSEMBLY_INTERACTION_UNCONFIRMED","TIMEOUT","CONNECTION_CLOSED","REALTIME_UNAVAILABLE","DATABASE_BUSY","REALTIME_BUSY"].includes(code);
  const message=code==="ASSEMBLY_INTERACTION_UNCONFIRMED"?"移动尚未保存；可重试确认当前目标，或取消恢复。":retryable?"操作未完成，请检查连接后重试。":
    /STALE|MISMATCH|VERSION|CONFLICT/.test(code)?"模型状态已变化，请使用当前状态重新操作。":
    code==="VALIDATION_FAILED"?"当前输入无法执行，请检查参数和支持元素。":
    code==="FORBIDDEN"?"当前没有执行此操作的权限。":"操作未完成，请查看诊断详情后重试。";
  return {message,details,code,requestId:value?.requestId??value?.correlationId,diagnosticId:value?.diagnosticId,phase:value?.phase,retryable,severity:retryable?"warning":"error"};
}
export class OperationFailureGate {
  private seen=new Map<string,number>();private objects=new WeakSet<object>();
  accept(error:unknown,scope:string,now=Date.now()):boolean {
    if(error&&typeof error==="object"){if(this.objects.has(error))return false;this.objects.add(error);}
    const failure=normalizeOperationFailure(error),key=`${scope}:${failure.requestId??failure.code+failure.details}`;
    if(now-(this.seen.get(key)??-Infinity)<5000)return false;
    this.seen.set(key,now);if(this.seen.size>128)this.seen.delete(this.seen.keys().next().value!);return true;
  }
}
const gate=new OperationFailureGate();
/** One Antd App-owned outlet for command/dialog/interaction failures. Raw
 * protocol messages live in details, never in the primary notification. */
export function useOperationFeedback(){
  const {notification,modal}=App.useApp();
  return useCallback((error:unknown,scope="操作")=>{
    if(!gate.accept(error,scope))return;
    const failure=normalizeOperationFailure(error);
    const recovery=error as {retry?:()=>void;cancel?:()=>void};
    notification[failure.severity]({className:"cad-operation-notification",placement:"bottomRight",duration:recovery?.retry||recovery?.cancel?0:6,key:`failure:${scope}`,title:`${scope}未完成`,description:<>
      <div>{failure.message}</div>{recovery?.retry&&<Button size="small" onClick={()=>{notification.destroy(`failure:${scope}`);recovery.retry?.();}}>重试确认</Button>}{recovery?.cancel&&<Button size="small" onClick={()=>{notification.destroy(`failure:${scope}`);recovery.cancel?.();}}>取消未保存移动</Button>}<Button size="small" onClick={()=>modal.info({title:"技术诊断",width:640,content:<>
        <pre className="cad-technical-log" style={{whiteSpace:"pre-wrap",maxHeight:320,overflow:"auto"}}>{JSON.stringify(failure,null,2)}</pre>
        <DiagnosticCopy text={diagnosticReference(failure)}/>
      </>})}>查看诊断</Button></>});
  },[notification,modal]);
}
