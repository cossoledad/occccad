// Only the provider is scripted. UI, API, persistence, evaluator and Worker are
// real; this fixture is not evidence of a remote model's planning quality.
import {createServer,type Server} from "node:http";
import type {AddressInfo} from "node:net";
export const fixturePort=19091;
type ToolCall={id:string;type:string;function:{name:string;arguments:string}};
type Message={role:string;content?:string|null;reasoning_content?:string;tool_calls?:ToolCall[]};
type Scope={snapshot:{revisionId:string};workingBodyId:string};
type Envelope={current:Scope;frozenScope?:{selections:unknown[]};goal?:{text:string;version:number};run?:{id:string;epoch:number;stageCount:number};recentFacts?:unknown[];recentStages?:unknown[]};
const findSet=(value:unknown):{selectionSet:string;selectionSetCount:number}|undefined=>{
 if(!value||typeof value!=="object")return;
 const record=value as Record<string,unknown>;
 if(typeof record.selectionSet==="string")return record as {selectionSet:string;selectionSetCount:number};
 for(const child of Object.values(record)){const result=findSet(child);if(result)return result}
};
export async function startAssistantProvider(){
 let calls=0,holdNext=false,release:undefined|(()=>void);
 const generations:{model:string;effort:string;thinking:string;maxTokens:number;runId?:string;stageCount:number}[]=[],protocolErrors:string[]=[],scopes:unknown[]=[];
 const server:Server=createServer(async(req,res)=>{
  res.setHeader("Content-Type","application/json");
  if(req.headers.authorization!=="Bearer synthetic-browser-assistant-key"){res.writeHead(401);res.end('{"error":"unauthorized"}');return}
  if(req.url==="/models"){res.end(JSON.stringify({data:[{id:"deepseek-flash",effort:{supported_levels:["low","high","max"]}},{id:"deepseek-v4-pro",effort:{supported_levels:["low","high","max"]}}]}));return}
  if(req.url!=="/chat/completions"){res.writeHead(404);res.end("{}");return}
  calls++;
  const chunks:Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.from(chunk));
  const input=JSON.parse(Buffer.concat(chunks).toString()) as {model:string;thinking:{type:string};reasoning_effort?:string;max_tokens:number;messages:Message[];tools:{function:{name:string}}[]};
  const envelope=JSON.parse(input.messages[1].content!) as Envelope,scope=envelope.current,stageCount=envelope.run?.stageCount??0;
  generations.push({model:input.model,effort:input.reasoning_effort??"off",thinking:input.thinking.type,maxTokens:input.max_tokens,runId:envelope.run?.id,stageCount});scopes.push(envelope.frozenScope);
  if(input.thinking.type==="enabled"&&input.messages.some(m=>m.role==="assistant"&&!("reasoning_content" in m))){protocolErrors.push("missing thinking tool transcript field");res.writeHead(400);res.end("{}");return}
  const latestUser=input.messages.findLastIndex(m=>m.role==="user"),current=input.messages.slice(latestUser+1),text=input.messages[latestUser]?.content??envelope.goal?.text??"",goal=envelope.goal?.text??text;
  if(holdNext&&!text.includes("进展")&&!text.includes("参数")){holdNext=false;await new Promise<void>(resolve=>release=resolve);release=undefined;if(res.destroyed)return}
  const tool=(name:string,args:unknown):ToolCall=>({id:crypto.randomUUID(),type:"function",function:{name,arguments:JSON.stringify(args)}});
  const tools=current.filter(m=>m.role==="tool").map(m=>{try{return JSON.parse(m.content!)}catch{return {}}});
  const hasDescribe=current.some(m=>m.tool_calls?.some(call=>call.function.name==="cad_describe_operation"));
  const hasVerification=current.some(m=>m.tool_calls?.some(call=>call.function.name==="cad_verify_goal"));
  const hasContext=current.some(m=>m.tool_calls?.some(call=>call.function.name==="cad_context"));
  let message:Message;
  if(text.includes("进展")||text.includes("参数")){
   message={role:"assistant",content:"可以在运行中查询进展；模型状态以当前快照和已提交回执为准。"};
  }else if(goal.includes("预算测试")){
   message=hasContext?{role:"assistant",content:null,tool_calls:[tool("cad_finish",{summary:"预算测试已完成，累计预算可以调整后继续原目标。"})]}:{role:"assistant",content:null,tool_calls:[tool("cad_context",{})]};
  }else if(goal.includes("打开")&&goal.includes("圆角")){
   message=stageCount>0?{role:"assistant",content:null,tool_calls:[tool("cad_finish",{summary:"原界面操作已按精确领域回执核对。"})]}:hasDescribe?{role:"assistant",content:null,tool_calls:[tool("cad_create_task",{steps:[{id:"open-fillet",operationId:"ui.part.fillet",version:1,arguments:{}}]})]}:{role:"assistant",content:null,tool_calls:[tool("cad_describe_operation",{operationId:"ui.part.fillet"})]};
  }else if(goal.includes("选中的边")){
   const topology=findSet(tools);
   message=stageCount>0?{role:"assistant",content:null,tool_calls:[tool("cad_finish",{summary:"发送时选中的边已按 R1 加工。"})]}:topology?{role:"assistant",content:null,tool_calls:[tool("cad_create_task",{steps:[{id:"selected-fillet",operationId:"part.fillet",version:1,arguments:{bodyId:scope.workingBodyId,selectionSet:topology.selectionSet,size:1,unit:"mm"}}]})]}:{role:"assistant",content:null,tool_calls:[tool("cad_describe_operation",{operationId:"part.fillet"}),tool("cad_query_topology",{revisionId:scope.snapshot.revisionId,bodyId:scope.workingBodyId,kind:"EDGE",limit:2})]};
  }else if(stageCount===0){
   message=hasDescribe?{role:"assistant",content:null,tool_calls:[tool("cad_create_task",{steps:[
    {id:"sketch",operationId:"sketch.create",version:1,arguments:{bodyId:scope.workingBodyId,plane:"XY"}},
    {id:"rectangle",operationId:"sketch.rectangle",version:1,arguments:{width:80,height:50,unit:"mm"},bindings:{sketchId:"sketch"}},
    {id:"pad",operationId:"part.extrude",version:1,arguments:{bodyId:scope.workingBodyId,length:goal.includes("20 mm")?20:12,unit:"mm"},bindings:{sketchId:"sketch"}},
   ]})]}:{role:"assistant",content:null,tool_calls:["sketch.create","sketch.rectangle","part.extrude"].map(operationId=>tool("cad_describe_operation",{operationId}))};
  }else if(stageCount===1&&!goal.includes("不做圆角")){
   const topology=findSet(tools);
   if(!topology)message={role:"assistant",content:null,tool_calls:[tool("cad_describe_operation",{operationId:"part.fillet"}),tool("cad_query_topology",{revisionId:scope.snapshot.revisionId,bodyId:scope.workingBodyId,kind:"EDGE",limit:2})]};
   else{
    if(topology.selectionSetCount!==12){protocolErrors.push("all-edge selection incomplete");res.writeHead(500);res.end("{}");return}
    message={role:"assistant",content:null,tool_calls:[tool("cad_create_task",{steps:[{id:"fillet",operationId:"part.fillet",version:1,arguments:{bodyId:scope.workingBodyId,selectionSet:topology.selectionSet,size:2,unit:"mm"}}]})]};
   }
  }else{
   message=hasVerification?{role:"assistant",content:null,tool_calls:[tool("cad_finish",{summary:"建模阶段已应用，目标检查已完成。"})]}:{role:"assistant",content:null,tool_calls:[tool("cad_verify_goal",{})]};
  }
  if(input.thinking.type==="enabled")message.reasoning_content=`private-fixture-reasoning-${calls}`;
  const usage=goal.includes("预算测试")?{prompt_tokens:40000,completion_tokens:10000,total_tokens:50000}:{prompt_tokens:300,completion_tokens:120,total_tokens:420};
  res.end(JSON.stringify({id:`fixture-${calls}`,choices:[{message,finish_reason:message.tool_calls?"tool_calls":"stop"}],usage}));
 });
 await new Promise<void>((resolve,reject)=>{server.once("error",reject);server.listen(fixturePort,"127.0.0.1",resolve)});
 return {url:`http://127.0.0.1:${(server.address() as AddressInfo).port}`,calls:()=>calls,generations:()=>generations,scopes:()=>scopes,protocolErrors:()=>protocolErrors,
  holdNext:()=>{holdNext=true},blocked:()=>Boolean(release),release:()=>release?.(),close:()=>{release?.();return new Promise<void>((resolve,reject)=>server.close(error=>error?reject(error):resolve()))}};
}
