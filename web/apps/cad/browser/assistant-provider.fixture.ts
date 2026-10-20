// A test-owned OpenAI-format endpoint. Browser/UI and CAD API/Worker remain real;
// only the remote model response is scripted, so this does not measure LLM quality.
import {createServer,type Server} from "node:http";
import type {AddressInfo} from "node:net";
export const fixturePort=19091;
export async function startAssistantProvider(){
  let calls=0;
  const generations:{model:string;effort:string;thinking:string;maxTokens:number}[]=[],protocolErrors:string[]=[];
  const server:Server=createServer(async(req,res)=>{
    res.setHeader("Content-Type","application/json");
    if(req.headers.authorization!=="Bearer synthetic-browser-assistant-key"){res.writeHead(401);res.end('{"error":"unauthorized"}');return}
    if(req.url==="/models"){res.end(JSON.stringify({data:[{id:"deepseek-flash",effort:{supported_levels:["low","high","max"]}},{id:"deepseek-v4-pro",effort:{supported_levels:["low","high","max"]}}]}));return}
    if(req.url!=="/chat/completions"){res.writeHead(404);res.end("{}");return}
    calls++;
    const chunks:Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.from(chunk));
    const input=JSON.parse(Buffer.concat(chunks).toString()) as {model:string;thinking:{type:string};reasoning_effort?:string;max_tokens:number;messages:{role:string;content:string;reasoning_content?:string;tool_call_id?:string;tool_calls?:{function:{name:string}}[]}[]};
    generations.push({model:input.model,effort:input.reasoning_effort??"off",thinking:input.thinking.type,maxTokens:input.max_tokens});
    if(input.thinking.type==="enabled"&&input.messages.some(m=>m.role==="assistant"&&!m.reasoning_content)){protocolErrors.push("missing thinking tool transcript");res.writeHead(400);res.end("{}");return}
    const latestUser=input.messages.findLastIndex(m=>m.role==="user"),current=input.messages.slice(latestUser+1);
    const scope=JSON.parse(input.messages[1].content).current;
    const text=input.messages[latestUser].content;
    if(text.includes("等待")){
      await new Promise(resolve=>setTimeout(resolve,1500));
      if(res.destroyed)return;
    }
    const tool=(name:string,args:unknown)=>({id:crypto.randomUUID(),type:"function",function:{name,arguments:JSON.stringify(args)}});
    let message:unknown;
    if(text.includes("预算测试")){
      message=current.some(m=>m.role==="tool")?{role:"assistant",content:"预算测试已完成，可以继续建模。"}:{role:"assistant",content:null,tool_calls:[tool("cad_context",{})]};
    }else if(text.includes("参数")){
      const values=current.filter(m=>m.role==="tool");
      message=values.length===0?{role:"assistant",content:null,tool_calls:[tool("cad_query_model",{kind:"PARAMETER",fields:["name","value","displayValue","unit"],limit:20})]}:{role:"assistant",content:"当前拉伸长度为 9 mm。你可以告诉我希望修改成多少。"};
    }else if(text.includes("圆角")){
      const results=current.filter(m=>m.role==="tool");
      if(results.length===0)message={role:"assistant",content:null,tool_calls:[tool("cad_describe_operation",{operationId:"part.fillet"}),tool("cad_query_topology",{revisionId:scope.snapshot.revisionId,bodyId:scope.workingBodyId,kind:"EDGE",limit:2})]};
      else{
        const topology=results.map(m=>JSON.parse(m.content)).find(value=>value.selectionSet);
        if(!topology||topology.selectionSetCount!==12){protocolErrors.push("all-edge selection incomplete");res.writeHead(500);res.end("{}");return}
        message={role:"assistant",content:null,tool_calls:[tool("cad_create_task",{steps:[{id:"fillet",operationId:"part.fillet",version:1,arguments:{bodyId:scope.workingBodyId,selectionSet:topology.selectionSet,size:2,unit:"mm"}}]})]};
      }
    }else if(text.includes("适合窗口")){
      message=current.filter(m=>m.role==="tool").length===0?{role:"assistant",content:null,tool_calls:[tool("cad_describe_operation",{operationId:"ui.view.fit"})]}:{role:"assistant",content:null,tool_calls:[tool("cad_create_task",{steps:[{id:"fit",operationId:"ui.view.fit",version:1,arguments:{}}]})]};
    }else if(current.filter(m=>m.role==="tool").length===0){
      message={role:"assistant",content:null,tool_calls:["sketch.create","sketch.rectangle","part.extrude"].map(operationId=>tool("cad_describe_operation",{operationId}))};
    }else{
      message={role:"assistant",content:null,tool_calls:[tool("cad_create_task",{steps:[
        {id:"sketch",operationId:"sketch.create",version:1,arguments:{bodyId:scope.workingBodyId,plane:"XY"}},
        {id:"rectangle",operationId:"sketch.rectangle",version:1,arguments:{width:64,height:36,unit:"mm"},bindings:{sketchId:"sketch"}},
        {id:"pad",operationId:"part.extrude",version:1,arguments:{bodyId:scope.workingBodyId,length:9,unit:"mm"},bindings:{sketchId:"sketch"}},
      ]})]};
    }
    if(input.thinking.type==="enabled")(message as {reasoning_content:string}).reasoning_content=`private-fixture-reasoning-${calls}`;
    const usage=text.includes("预算测试")?{prompt_tokens:40000,completion_tokens:10000,total_tokens:50000}:{prompt_tokens:300,completion_tokens:120,total_tokens:420};
    res.end(JSON.stringify({choices:[{message,finish_reason:(message as {tool_calls?:unknown[]}).tool_calls?"tool_calls":"stop"}],usage}));
  });
  await new Promise<void>((resolve,reject)=>{server.once("error",reject);server.listen(fixturePort,"127.0.0.1",resolve)});
  return {url:`http://127.0.0.1:${(server.address() as AddressInfo).port}`,calls:()=>calls,generations:()=>generations,protocolErrors:()=>protocolErrors,close:()=>new Promise<void>((resolve,reject)=>server.close(error=>error?reject(error):resolve()))};
}
