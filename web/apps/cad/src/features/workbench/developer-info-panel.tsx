import {useState,type ComponentProps,useSyncExternalStore} from 'react';
import {useQuery} from '@tanstack/react-query';
import {Alert,Button,Segmented} from 'antd';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {api} from '../../api/client';
import {queryKeys} from '../../app/query-keys';
import {useDeveloperEvidence} from '../../cad/command/developer-evidence';
import {useOperationFeedback} from '../../cad/command/operation-feedback';
import {inspectorObjectData,inspectorOwnerDocumentId} from './inspector-object-data';
import {topologyPropertyContext} from './topology-property-context';
import {Properties} from './workbench-inspector';
import {AssistantDeveloperTrace} from '../assistant/assistant-trace';
import {AutomationDebugPanel} from '../assistant/automation-debug-panel';
import {lastClientPerformanceSample,subscribeClientPerformance} from "../../utils/performance";
function LastRequestTime(){
 const sample=useSyncExternalStore(subscribeClientPerformance,lastClientPerformanceSample,()=>undefined);
 return <p title="最后完成的一次请求，包含网络等待和响应处理" aria-label="最后一次后端请求耗时">后端请求：{sample?`${Math.round(sample.durationMs)} ms`:"—"}</p>;
}

export function DeveloperInfoPanel(props:Pick<ComponentProps<typeof Properties>,'view'|'selection'|'feature'>&{onClose:()=>void}){
 const [developerTab,setDeveloperTab]=useState('object'),evidence=useDeveloperEvidence(),feedback=useOperationFeedback(),documentID=props.view.document.id;
 const ownerID=inspectorOwnerDocumentId(props.view,props.selection);
 const owner=useQuery({queryKey:queryKeys.document(ownerID),queryFn:()=>api.getDocument(ownerID),enabled:developerTab==='object'&&ownerID!==documentID,staleTime:30_000});
 const properties=useQuery({queryKey:queryKeys.documentProperties(documentID),queryFn:()=>api.getDocumentProperties(documentID),enabled:developerTab==='object',staleTime:30_000});
 const selection=props.selection&&['face','edge','vertex'].includes(props.selection.kind)?props.selection as Extract<NonNullable<typeof props.selection>,{kind:'face'|'edge'|'vertex'}>:undefined;
 const context=topologyPropertyContext(selection??null,documentID);
 const topology=useQuery({queryKey:selection?queryKeys.topologyProperties(context.documentId,selection.geometryKey??'',selection.kind,selection.topologyId,selection.versionId):['topology-properties','none'],queryFn:()=>api.getTopologyProperties(context.documentId,selection!.geometryKey!,selection!.kind.toUpperCase() as 'FACE'|'EDGE'|'VERTEX',selection!.topologyId,context.versionId),enabled:developerTab==='object'&&!!selection?.geometryKey,staleTime:5*60_000});
 return <CommandDialog id="developer-info" open title="开发信息" size="L" footer={false} onClose={props.onClose} onConfirm={()=>{}}>
 {owner.error||properties.error||topology.error?<Alert type="error" title="开发数据读取失败" action={<Button onClick={()=>{void owner.refetch();void properties.refetch();if(selection?.geometryKey)void topology.refetch();}}>重试</Button>}/>:null}
<LastRequestTime/><Button size="small" onClick={()=>void (props.view.product?api.downloadAssemblyDiagnostic(props.view.document.id):api.downloadDiagnosticBundle(props.view.document.id)).catch(error=>feedback(error,"开发诊断"))}>下载诊断数据</Button><Segmented size="small" aria-label="开发信息内容" value={developerTab} onChange={value=>setDeveloperTab(String(value))} options={[{value:"object",label:"当前对象"},{value:"failure",label:"最近结果与失败"},{value:"query",label:"查询调试"},{value:"assistant",label:"助手调用"}]}/>
          {developerTab==="assistant"?<AssistantDeveloperTrace documentId={documentID}/>:developerTab==="query"?<AutomationDebugPanel view={props.view}/>:<pre className="inspector-object-data" aria-label="开发数据">{JSON.stringify(developerTab==="failure"?evidence??{message:"暂无记录"}:inspectorObjectData(props.view,props.selection,props.feature,topology.data,properties.data,owner.data),(key,value)=>value instanceof Error?{...value,message:value.message,stack:value.stack}:value,2)}</pre>}
        
 </CommandDialog>;
}
