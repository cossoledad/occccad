import {cloneElement,isValidElement,createContext,useContext,useId,type PropsWithChildren,type ReactElement,type ReactNode} from 'react';
import {Button,Spin,Form,type FormItemProps} from 'antd';
import type {SelectionItem} from '../../types';

export const CommandFormContext=createContext<{cancelPick?:()=>boolean;locate?:(items:SelectionItem[])=>void}>({});
export const useCommandForm=()=>useContext(CommandFormContext);
export type CommandFieldWidth='number'|'enum'|'expression'|'name'|'option'|'selection'|'content'|'coordinate';
export function CommandField({label,width='expression',error,children}:PropsWithChildren<{label:ReactNode;width?:CommandFieldWidth;error?:ReactNode}>){
 const generated=useId(),child=isValidElement(children)?children as ReactElement<{id?:string}>:undefined,id=child?.props.id??generated;
 return <div className={`command-field command-field-${width}`}><label className="command-field-label" htmlFor={id}>{label}</label><div className="command-field-control" role="group" aria-label={typeof label==='string'?label:undefined}>{child?cloneElement(child,{id}):children}{error&&<div className="command-field-error" role="alert">{error}</div>}</div></div>;
}
/** Ant Form owns registration and validation; layout uses the same field tokens. */
export function CommandFormItem({width='expression',...props}:FormItemProps&{width?:CommandFieldWidth}){
 return <Form.Item {...props} colon={width==='coordinate'?false:props.colon} layout="horizontal" labelCol={{flex:width==='coordinate'?'0 0 14px':'0 0 var(--ui-form-label-width)'}} wrapperCol={{flex:'1 1 0'}} className={`command-form-item command-form-item-${width} ${props.className??''}`}/>;
}
/** Three named components share available width without full field-label columns. */
export function CommandVector({label,children}:PropsWithChildren<{label:ReactNode}>){return <fieldset className="command-vector"><legend>{label}</legend><div className="command-vector-fields">{children}</div></fieldset>}
export function CommandGroup({label,children}:PropsWithChildren<{label:ReactNode}>){return <fieldset className="command-group"><legend>{label}</legend>{children}</fieldset>}
export function CommandStatus({pending,error,onRetry}:{pending?:boolean;error?:ReactNode;onRetry?:()=>void}){
 if(!pending&&!error)return null;
 return <div className="command-status" role="status">{pending?<><Spin size="small"/>计算中…</>:<><span className="command-field-error" role="alert">{error}</span>{onRetry&&<Button size="small" onClick={onRetry}>重新预览</Button>}</>}</div>;
}
/** Local validation is authored by the owning form; it is not a service error.
 * Preserve its exact reason without passing it through the protocol mapper. */
export const fieldErrorText=(error:unknown)=>error instanceof Error?error.message:String(error);
