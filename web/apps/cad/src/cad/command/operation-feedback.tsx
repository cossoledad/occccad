import {App,Button} from "antd";
import {useCallback} from "react";

export type OperationFailure={message:string;details:string;code:string;requestId?:string;retryable:boolean;severity:"error"|"warning"};
export function normalizeOperationFailure(error:unknown):OperationFailure {
  const value=error as {message?:string;code?:string;requestId?:string;correlationId?:string};
  const details=value?.message??String(error),code=value?.code??details.match(/\[([A-Z_]+)\]/)?.[1]??"OPERATION_FAILED";
  const retryable=["TIMEOUT","CONNECTION_CLOSED","REALTIME_UNAVAILABLE","DATABASE_BUSY","REALTIME_BUSY"].includes(code);
  const message=retryable?"操作未完成，请检查连接后重试。":
    /STALE|MISMATCH|VERSION|CONFLICT/.test(code)?"装配状态已变化，请使用当前状态重新操作。":
    code==="VALIDATION_FAILED"?"当前输入无法执行，请检查参数和支持元素。":
    code==="FORBIDDEN"?"当前没有执行此操作的权限。":"操作未完成，请查看诊断详情后重试。";
  return {message,details,code,requestId:value?.requestId??value?.correlationId,retryable,severity:retryable?"warning":"error"};
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
  const {notification,modal,message}=App.useApp();
  return useCallback((error:unknown,scope="操作")=>{
    if(!gate.accept(error,scope))return;
    const failure=normalizeOperationFailure(error);
    notification[failure.severity]({key:`failure:${scope}`,message:`${scope}未完成`,description:<>
      <div>{failure.message}</div><Button size="small" onClick={()=>modal.info({title:"技术诊断",width:640,content:<>
        <pre style={{whiteSpace:"pre-wrap",maxHeight:300,overflow:"auto"}}>{JSON.stringify(failure,null,2)}</pre>
        <Button onClick={()=>void navigator.clipboard.writeText(JSON.stringify(failure,null,2)).then(()=>message.success("已复制诊断"))}>复制诊断</Button>
      </>})}>查看诊断</Button></>});
  },[notification,modal,message]);
}
