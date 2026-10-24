import type {CadCommand} from "../../../cad/command/command-registry";
export function selectionSummaryCommand(input:{getContext:()=>{documentId:string;documentName?:string;selectionCount:number};show:(text:string)=>void}):CadCommand {
 return {id:"view.selection-summary",execute:()=>{const context=input.getContext();input.show(`${context.documentName??"当前文档"} · 已选择 ${context.selectionCount} 个对象`);}};
}
