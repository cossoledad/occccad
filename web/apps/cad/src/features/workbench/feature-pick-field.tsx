import {Button} from 'antd';
import type {ReactNode} from 'react';
import type {SelectionItem} from '../../types';
import {CommandField,useCommandForm} from '../../cad/overlay/command-form';

/** One presentation for a scalar reference, a collection or one ordered item.
 * The owning command continues to validate and persist its formal references. */
export function FeaturePickField({label,value,active,disabled,onActivate,onCancel,onClear,onLocate,selected,mode="single",selections,detail,error,actions,children}: {
 label:string;value?:ReactNode;active:boolean;disabled?:boolean;onActivate?:()=>void;onCancel?:()=>void;onClear?:()=>void;onLocate?:()=>void;
 mode?:"single"|"multiple"|"ordered";selected?:boolean;selections?:SelectionItem[];detail?:ReactNode;error?:ReactNode;actions?:ReactNode;children?:ReactNode;
}){
 const form=useCommandForm(),hasValue=selected??Boolean(onClear||selections?.length);
 const cancel=()=>onCancel?onCancel():form.cancelPick?.();
 const locateAction=onLocate??(selections?.length&&form.locate?()=>form.locate?.(selections):undefined);
 const locate=locateAction?()=>{cancel();locateAction()}:undefined;
 return <div className={`feature-pick-field ${active?'is-active':''}`} data-pick-active={active||undefined}>
  <CommandField label={label} width="selection" error={error}>
   <div className="feature-pick-content"><span className="feature-pick-value" title={typeof value==='string'?value:undefined}>{value||'未选择'}</span>
    {detail&&<small className="feature-pick-detail">{detail}</small>}
    <div className="feature-pick-actions">
     {locate&&<Button size="small" disabled={disabled||!hasValue} aria-label={`定位${label}`} onClick={locate}>定位</Button>}
     {onActivate&&<Button size="small" disabled={disabled} aria-label={`选择${label}`} aria-pressed={active} type={active?'primary':'default'} onClick={active?cancel:onActivate}>{active?'取消选择':hasValue?mode==='single'?'更换':'增减':'选择'}</Button>}
     {onClear&&<Button size="small" disabled={disabled} aria-label={`移除${label}`} onClick={()=>{cancel();onClear()}}>移除</Button>}{actions}
    </div>
   </div>{children}
  </CommandField>
 </div>;
}
