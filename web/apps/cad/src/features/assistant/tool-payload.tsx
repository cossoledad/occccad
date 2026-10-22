// Shared by the actual call trace and the model-free query inspector.
export function ToolPayload({value,className="assistant-trace-payload"}:{value:unknown;className?:string}){
 let text:string;
 if(typeof value==="string"){
  try{text=JSON.stringify(JSON.parse(value),null,2)}catch{text=value}
 }else text=JSON.stringify(value,null,2)??"无";
 return <pre className={className}>{text}</pre>;
}
