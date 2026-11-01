import {Input,Select} from 'antd';
import {useMemo,useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {api} from '../../api/client';
import type {DocumentView,ParameterDefinition} from '../../types';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus} from '../../cad/overlay/command-form';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import {useFeaturePreview} from './use-feature-preview';

export function ParameterExternalEditor({view,parameter,onClose,onApply}:{view:DocumentView;parameter:ParameterDefinition;onClose:()=>void;onApply:(view:DocumentView)=>Promise<unknown>}){
 const [search,setSearch]=useState(''),[sourceID,setSourceID]=useState(parameter.source.external?.sourceDocumentId),[publicationID,setPublicationID]=useState(parameter.source.external?.publicationId),[mode,setMode]=useState(parameter.source.external?.revision.mode??'FOLLOW_WORKSPACE_WITH_ACCEPT');
 const [error,setError]=useState<string>(),[busy,setBusy]=useState(false);
 const documents=useQuery({queryKey:['parameter-sources',search],queryFn:({signal})=>api.listDocuments({query:search,allFolders:true,limit:100},signal)});
 const source=useQuery({queryKey:['parameter-source',sourceID],queryFn:()=>api.getDocument(sourceID!),enabled:!!sourceID});
 const publications=(source.data?.part?.publications??source.data?.product?.publications??[]).filter(p=>p.type==='PARAMETER'&&p.contract.valueType===parameter.valueType&&JSON.stringify(p.contract.dimension)===JSON.stringify(parameter.dimension));
 const selected=publications.find(p=>p.id===publicationID);
 const command=useMemo(()=>selected&&source.data?{type:'SET_PARAMETER_EXTERNAL',versionId:view.document.versionId,parameterId:parameter.parameterId,sourceDocumentId:sourceID,sourceVersionId:source.data.document.versionId,publicationId:publicationID,referenceMode:mode}:undefined,[selected,source.data,view.document.versionId,parameter.parameterId,sourceID,publicationID,mode]);
 const preview=useFeaturePreview(view.document.id,view.document.versionId,command,undefined,()=>{});
 return <CommandDialog id="parameter-external" open title="引用发布参数" size="M" onClose={onClose} confirmLoading={busy} confirmDisabled={!command||!preview.previewId||preview.pending||!!preview.error} commandStatus={<CommandStatus pending={source.isFetching||preview.pending} error={error??preview.error??(source.error?engineeringProblem(source.error):undefined)} onRetry={preview.retry}/>} onConfirm={async()=>{if(!command||!preview.previewId||busy)return;setBusy(true);try{await onApply(await api.command(view.document.id,{...command,previewId:preview.previewId}));onClose()}catch(cause){setError(engineeringProblem(cause));setBusy(false)}}}>
  <CommandField label="接收参数"><span>{view.document.name} / {parameter.qualifiedDisplayPath??parameter.label}</span></CommandField>
  <CommandField label="搜索文档"><Input value={search} onChange={e=>setSearch(e.target.value)} allowClear/></CommandField>
  <CommandField label="来源文档"><Select showSearch optionFilterProp="label" value={sourceID} loading={documents.isFetching} options={(documents.data?.documents??[]).filter(d=>d.id!==view.document.id&&['PART','PRODUCT'].includes(d.type)).map(d=>({value:d.id,label:d.name}))} onChange={id=>{setSourceID(id);setPublicationID(undefined)}}/></CommandField>
  <CommandField label="发布参数"><Select value={publicationID} placeholder="选择类型和量纲匹配的接口" options={publications.map(p=>({value:p.id,label:p.name,disabled:p.resolution.status!=='CONNECTED'||!!p.resolution.currencyStatus&&p.resolution.currencyStatus!=='CURRENT'}))} onChange={setPublicationID}/></CommandField>
  <CommandField label="更新方式"><Select value={mode} onChange={setMode} options={[{value:'FOLLOW_WORKSPACE_WITH_ACCEPT',label:'接受更新后采用'},{value:'PINNED',label:'固定当前版本'}]}/></CommandField>
 </CommandDialog>
}
