import type {CadCommand} from "../../../cad/command/command-registry";
export function selectionSummaryCommand(input:{getContext:()=>{documentId:string;selectionCount:number};show:(text:string)=>void}):CadCommand {
 return {id:"view.selection-summary",execute:()=>{const context=input.getContext();input.show(`编辑目标 ${context.documentId} · 已选择 ${context.selectionCount} 个对象`);}};
}
