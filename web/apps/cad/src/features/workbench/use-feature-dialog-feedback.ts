import {useEffect,useMemo,useState,type FocusEventHandler} from 'react';
import {engineeringProblem,useOperationFailureNotification} from '../../cad/command/operation-feedback';
import {CommandInputError} from '../../cad/command/operation-failure';

/** Command failures share the application outlet. Incomplete quantity text is
 * validated on blur/confirmation, so typing does not produce notifications. */
export function useFeatureDialogFeedback(failure:unknown,scope:string,validation?:unknown,repair?:{name:string}[],retry?:()=>void){
 const local=(value:unknown)=>typeof value==='string'?new CommandInputError(value):value;
 const reportedFailure=useMemo(()=>local(failure),[failure]);
 const reportedValidation=useMemo(()=>local(validation),[validation]);
 const key=validation?engineeringProblem(reportedValidation):undefined;
 const message=repair?.length?`下游需要修复：${repair.map(item=>item.name||'引用特征').join('、')}`:undefined;
 const warning=useMemo(()=>message?Object.assign(new CommandInputError(message),{code:'FEATURE_DOWNSTREAM_REPAIR_REQUIRED'}):undefined,[message]);
 const [reported,setReported]=useState<string>();
 useEffect(()=>{if(!key)setReported(undefined)},[key]);
 useOperationFailureNotification(reportedFailure??(reported===key?reportedValidation:undefined)??warning,scope,reportedFailure&&retry?{retry,retryLabel:'重试'}:undefined);
 const reportValidation=()=>{if(key)setReported(key)};
 const onBlurCapture:FocusEventHandler<HTMLElement>=event=>{
  if(event.target instanceof HTMLInputElement||event.target instanceof HTMLTextAreaElement)reportValidation();
 };
 return {onBlurCapture,reportValidation};
}
