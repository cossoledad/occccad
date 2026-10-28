import {Button} from 'antd';
import type {ProductUpdatePlan} from '../../types';
import {engineeringProblem} from '../../cad/command/operation-feedback';

/** Navigation only: each source assembly keeps its own update transaction. */
export function AssociationSourceUpdates({plan,onOpen}:{plan?:ProductUpdatePlan;onOpen:(id:string)=>void}){
 const entries=plan?.entries.filter(e=>e.sourceProductDocumentId&&e.currency==='UPDATE_BLOCKED')??[];
 return <>{entries.map(e=><div key={e.bindingId} className="command-status" role="status">
  <span>{e.sourceDisplayPath}：{engineeringProblem({code:e.diagnosticCode,message:e.diagnostic})}</span>
  <Button size="small" onClick={()=>onOpen(e.sourceProductDocumentId!)}>转到来源装配</Button>
 </div>)}</>;
}
