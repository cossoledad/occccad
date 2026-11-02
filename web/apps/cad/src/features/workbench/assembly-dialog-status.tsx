import {Button,Spin} from 'antd';
import type {ReactNode} from 'react';

/** One dialog-level feedback area. Support errors stay beside their pick field. */
export function AssemblyDialogStatus({pending,error,warning,summary,details,onRetry}:{pending?:boolean;error?:string;warning?:string;summary?:string;details?:ReactNode;onRetry?:()=>void}){
 if(!pending&&!error&&!warning&&!summary)return null;
 return <div className={`assembly-dialog-status${error?' has-error':''}`} aria-live="polite">
  <div role={error?'alert':'status'}>{error??warning??(pending?<><Spin size="small"/> 正在预览…</>:summary)}</div>
  {error&&onRetry&&<Button size="small" onClick={onRetry}>重试</Button>}
  {details&&!error&&!pending&&<details><summary>求解详情</summary>{details}</details>}
 </div>;
}
