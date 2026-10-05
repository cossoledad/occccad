import {Button,Input} from "antd";
import {useState} from "react";

export async function copyDiagnosticText(text:string,clipboard?:Pick<Clipboard,"writeText">):Promise<boolean>{
 try{const target=clipboard??(typeof navigator!=="undefined"?navigator.clipboard:undefined);if(!target)return false;await target.writeText(text);return true;}catch{return false;}
}
// An explicit user click owns clipboard access. HTTP/denied clipboard contexts
// retain selectable text instead of silently failing or claiming success.
export function DiagnosticCopy({text}:{text:string}){
 const [state,setState]=useState<"idle"|"copied"|"manual">("idle");
 return <><Button size="small" onClick={()=>void copyDiagnosticText(text).then(ok=>setState(ok?"copied":"manual"))}>{state==="copied"?"已复制":"复制诊断"}</Button>
 {state==="manual"&&<label>剪贴板不可用，请选择并复制<Input.TextArea aria-label="手动复制诊断" readOnly value={text} autoSize={{minRows:2,maxRows:6}} onFocus={event=>event.currentTarget.select()}/></label>}</>;
}
export function diagnosticReference(error:unknown,context?:{documentId:string;versionId:string}):string{
 const value=error as {diagnosticId?:string;code?:string;phase?:string;requestId?:string;message?:string};
 return value?.diagnosticId?`CAD_DIAGNOSTIC ${value.diagnosticId} ${value.code??"OPERATION_FAILED"}`:JSON.stringify({stage:value?.phase??"CLIENT",...context,code:value?.code??"OPERATION_FAILED",requestId:value?.requestId,message:value?.message??String(error)},null,2);
}
