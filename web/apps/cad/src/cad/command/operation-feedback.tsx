import {App,Button} from "antd";
import {recordDeveloperEvidence} from "./developer-evidence";
import {useCallback} from "react";

import {normalizeOperationFailure} from "./operation-failure";
export {normalizeOperationFailure,engineeringProblem} from "./operation-failure";
export type {OperationFailure} from "./operation-failure";
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
  const {notification}=App.useApp();
  return useCallback((error:unknown,scope="操作")=>{
    if(!gate.accept(error,scope))return;
    const failure=normalizeOperationFailure(error);
    recordDeveloperEvidence(scope,{error,failure});
    const recovery=error as {retry?:()=>void;cancel?:()=>void};
    notification[failure.severity]({className:"cad-operation-notification",placement:"bottomRight",duration:recovery?.retry||recovery?.cancel?0:6,key:`failure:${scope}`,title:`${scope}未完成`,description:<>
      <div>{failure.message}</div>{recovery?.retry&&<Button size="small" onClick={()=>{notification.destroy(`failure:${scope}`);recovery.retry?.();}}>重试确认</Button>}{recovery?.cancel&&<Button size="small" onClick={()=>{notification.destroy(`failure:${scope}`);recovery.cancel?.();}}>取消并恢复</Button>}</>});
  },[notification]);
}
