import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
const originalNavigator=Object.getOwnPropertyDescriptor(globalThis,"navigator"), originalDocument=Object.getOwnPropertyDescriptor(globalThis,"document");
try{
 const {copyTextToClipboard}=await server.ssrLoadModule("/src/utils/clipboard.ts");
 let copied,removed=false,focused=false;
 Object.defineProperty(globalThis,"navigator",{configurable:true,value:{clipboard:{writeText:async value=>{copied=value}}}});
 assert.equal(await copyTextToClipboard("distance_1"),true);assert.equal(copied,"distance_1");
 Object.defineProperty(globalThis,"navigator",{configurable:true,value:{}});
 const input={style:{},select(){copied=this.value},remove(){removed=true}};
 Object.defineProperty(globalThis,"document",{configurable:true,value:{activeElement:{focus(){focused=true}},createElement(){return input},body:{appendChild(){}},execCommand(command){return command==="copy"}}});
 assert.equal(await copyTextToClipboard("angle_1"),true);assert.equal(copied,"angle_1");assert.equal(removed,true);assert.equal(focused,true);
 document.execCommand=()=>false;assert.equal(await copyTextToClipboard("length_1"),false);
 console.log("Clipboard API and HTTP fallback passed.");
}finally{
 if(originalNavigator)Object.defineProperty(globalThis,"navigator",originalNavigator);else delete globalThis.navigator;
 if(originalDocument)Object.defineProperty(globalThis,"document",originalDocument);else delete globalThis.document;
 await server.close();
}
