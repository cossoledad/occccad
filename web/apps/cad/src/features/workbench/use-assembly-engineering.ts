import {useEffect,useRef,useState} from "react";
import {useQuery,useQueryClient} from "@tanstack/react-query";
import {api} from "../../api/client";
import type {AssemblyEngineeringEvidence} from "../../cad/assembly/assembly-engineering-state";

type Refresh={key:string;status:"COMPUTING"|"COMPLETE"|"CANCELLED"|"FAILED";error?:string};
export function useAssemblyEngineering(documentId:string|undefined,revisionId:string|undefined,enabled:boolean){
 const client=useQueryClient(),key=JSON.stringify([documentId,revisionId]),queryKey=["assembly-engineering-evidence",documentId,revisionId];
 const query=useQuery({queryKey,queryFn:({signal})=>api.getAssemblyEngineeringEvidence(documentId!,revisionId!,signal),enabled:enabled&&!!documentId&&!!revisionId,retry:false});
 const [refresh,setRefresh]=useState<Refresh>();const sequence=useRef(0),abort=useRef<AbortController|undefined>(undefined),latest=useRef({key,enabled});latest.current={key,enabled};
 useEffect(()=>()=>{sequence.current++;abort.current?.abort();setRefresh(state=>state?.key===key&&state.status==="COMPUTING"?{...state,status:"CANCELLED"}:state);},[key,enabled]);
 const current=refresh?.key===key?refresh:undefined;
 const cancel=()=>{sequence.current++;abort.current?.abort();setRefresh({key,status:"CANCELLED"});};
 const run=async()=>{
  if(!enabled||!documentId||!revisionId)return;
  abort.current?.abort();const controller=new AbortController();abort.current=controller;const epoch=++sequence.current;
  setRefresh({key,status:"COMPUTING"});void client.cancelQueries({queryKey});
  try{
   const result:AssemblyEngineeringEvidence=await api.refreshAssemblyEngineeringEvidence(documentId,revisionId,controller.signal);
   if(controller.signal.aborted||sequence.current!==epoch||latest.current.key!==key||!latest.current.enabled)return;
   if(result.documentId!==documentId||result.revisionId!==revisionId)throw Error("运动证据来源版本不匹配，请重新刷新。");
   client.setQueryData(queryKey,result);setRefresh({key,status:"COMPLETE"});
  }catch(error){if(!controller.signal.aborted&&sequence.current===epoch&&latest.current.key===key&&latest.current.enabled)setRefresh({key,status:"FAILED",error:String(error)});}
 };
 const blocked=current&&current.status!=="COMPLETE";
 return {evidence:enabled&&!blocked?query.data:undefined,status:current?.status??(query.isFetching?"READING":query.error?"FAILED":"IDLE"),error:current?.error??(query.error?String(query.error):undefined),refresh:run,cancel};
}
