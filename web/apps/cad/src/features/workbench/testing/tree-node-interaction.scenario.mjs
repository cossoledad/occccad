import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const {defaultTreeCommand}=await server.ssrLoadModule("/src/features/workbench/tree-node-interaction.ts");
 const {structureCommands}=await server.ssrLoadModule("/src/features/workbench/commands/structure-commands.ts");
 const {CommandRegistry}=await server.ssrLoadModule("/src/cad/command/command-registry.ts");
 const {resolveCommandShortcut}=await server.ssrLoadModule("/src/cad/command/command-shortcuts.ts");
 const nodes=["PARAMETER","SKETCH","FEATURE","ASSEMBLY_CONSTRAINT","PUBLICATION","CONTEXT_INPUT","FUTURE_EDITABLE_NODE"].map((kind,index)=>({key:`node-${index}`,kind,documentId:"owner",entityId:`entity-${index}`,definitionDigest:"definition",capabilities:["EDIT"],selection:{versionId:"revision"}}));
 const opened=[];const registry=new CommandRegistry();registry.registerMany(structureCommands({onEdit:(node,operation)=>{assert(operation.current);opened.push(node.entityId);}},nodes));
 for(const node of nodes){assert.equal(defaultTreeCommand(node),"tree.edit");assert(await registry.execute(defaultTreeCommand(node),{payload:{node}}));}
 assert.equal(opened.length,nodes.length,"future editable kinds use the same command path");
 assert(await registry.execute("tree.edit",{payload:{nodeKey:nodes[0].key}}));assert.equal(opened.at(-1),nodes[0].entityId,"keyboard target is the focused node");
 assert.equal(registry.state("tree.edit").enabled,false,"no implicit target without a unique selection");
 assert.equal(registry.state("tree.edit",{payload:{node:{...nodes[0],definitionDigest:"stale"}}}).enabled,false);
 const readOnly={...nodes[0],key:"readonly",capabilities:[]};
 const viewer=new CommandRegistry();viewer.registerMany(structureCommands({onEdit:()=>assert.fail("read-only edit")},[readOnly]));
 assert.equal(defaultTreeCommand(readOnly),undefined);assert.equal(await viewer.execute("tree.edit",{payload:{node:readOnly}}),false);
 assert.equal(defaultTreeCommand({...nodes[0],presentationRole:"INPUT_REFERENCE"}),undefined);
 assert.equal(defaultTreeCommand({kind:"MECHANISM_REFERENCE",presentationRole:"INPUT_REFERENCE",capabilities:["ACTIVATE"]}),"tree.activate","read-only references may explicitly enter their owning application without editing the reference");
 assert.equal(defaultTreeCommand({kind:"BODY"}),"tree.activate");assert.equal(defaultTreeCommand({kind:"INSTANCE"}),"tree.activate");
 const shortcut=resolveCommandShortcut({key:"Enter",ctrlKey:false,metaKey:false,altKey:false,shiftKey:false,repeat:false,isComposing:false});assert.equal(shortcut.command,"tree.edit");assert.equal(shortcut.scope,"tree");
} finally {await server.close();}
