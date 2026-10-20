// A test-owned OpenAI-format endpoint. Browser/UI and CAD API/Worker remain real;
// only the remote model response is scripted, so this does not measure LLM quality.
import {createServer,type Server} from "node:http";
import type {AddressInfo} from "node:net";
export const fixturePort=19091;
export async function startAssistantProvider(){
  let calls=0;
  const server:Server=createServer(async(req,res)=>{
    res.setHeader("Content-Type","application/json");
    if(req.headers.authorization!=="Bearer synthetic-browser-assistant-key"){res.writeHead(401);res.end('{"error":"unauthorized"}');return}
    if(req.url==="/models"){res.end(JSON.stringify({data:[{id:"deepseek-flash"}]}));return}
    if(req.url!=="/chat/completions"){res.writeHead(404);res.end("{}");return}
    calls++;
    const chunks:Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.from(chunk));
    const input=JSON.parse(Buffer.concat(chunks).toString()) as {messages:{role:string;content:string;tool_call_id?:string;tool_calls?:{function:{name:string}}[]}[]};
    const latestUser=input.messages.findLastIndex(m=>m.role==="user"),current=input.messages.slice(latestUser+1);
    const scope=JSON.parse(input.messages[1].content).current;
    const text=input.messages[latestUser].content;
    if(text.includes("等待")){
      await new Promise(resolve=>setTimeout(resolve,1500));
      if(res.destroyed)return;
    }
    const tool=(name:string,args:unknown)=>({id:crypto.randomUUID(),type:"function",function:{name,arguments:JSON.stringify(args)}});
    let message:unknown;
    if(text.includes("参数")){
      const values=current.filter(m=>m.role==="tool");
      message=values.length===0?{role:"assistant",content:null,tool_calls:[tool("cad_query_model",{kind:"PARAMETER",fields:["name","value","displayValue","unit"],limit:20})]}:{role:"assistant",content:"当前拉伸长度为 9 mm。你可以告诉我希望修改成多少。"};
    }else if(text.includes("圆")){
      message={role:"assistant",content:"当前还不支持圆形草图。我可以创建矩形草图并拉伸，或者修改已有参数。"};
    }else if(current.filter(m=>m.role==="tool").length===0){
      message={role:"assistant",content:null,tool_calls:["sketch.create","sketch.rectangle","part.extrude"].map(operationId=>tool("cad_describe_operation",{operationId}))};
    }else{
      message={role:"assistant",content:null,tool_calls:[tool("cad_create_task",{steps:[
        {id:"sketch",operationId:"sketch.create",version:1,arguments:{bodyId:scope.workingBodyId,plane:"XY"}},
        {id:"rectangle",operationId:"sketch.rectangle",version:1,arguments:{width:64,height:36,unit:"mm"},bindings:{sketchId:"sketch"}},
        {id:"pad",operationId:"part.extrude",version:1,arguments:{bodyId:scope.workingBodyId,length:9,unit:"mm"},bindings:{sketchId:"sketch"}},
      ]})]};
    }
    res.end(JSON.stringify({choices:[{message,finish_reason:(message as {tool_calls?:unknown[]}).tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:300,completion_tokens:120,total_tokens:420}}));
  });
  await new Promise<void>((resolve,reject)=>{server.once("error",reject);server.listen(fixturePort,"127.0.0.1",resolve)});
  return {url:`http://127.0.0.1:${(server.address() as AddressInfo).port}`,calls:()=>calls,close:()=>new Promise<void>((resolve,reject)=>server.close(error=>error?reject(error):resolve()))};
}
