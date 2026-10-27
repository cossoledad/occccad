import {Button} from 'antd';
import type {ContextBinding,DocumentView,SelectionItem} from '../../types';
import {useWorkbenchStore} from '../../state/workbench-store';

export function boundContextParameter(view:DocumentView,id?:string){
 const parameter=view.part?.parameters?.find(p=>p.parameterId===id);
 const port=view.part?.contextInputs?.find(p=>p.target.kind==='PARAMETER'&&p.target.targetId===id||p.target.kind==='DATUM'&&!!parameter?.ownerFeatureId&&p.target.targetId===parameter.ownerFeatureId);
 return port?view.contextProduct?.product?.contextBindings?.find(b=>b.contextInputId===port.id&&b.owningInstancePath.canonical===view.contextOccurrencePath):undefined;
}
export function bindingSourceSelection(binding:ContextBinding):SelectionItem {
 const path=binding.sourceInstancePath,leaf=path.segments.at(-1),native=binding.resolution.nativeSketch;
 const identity={documentId:leaf?.referencedDocumentId??'',versionId:binding.accepted.sourceRevisionId,instancePath:path,occurrencePath:path.canonical};
 return {...identity,kind:'publication',id:binding.publication.publicationId,publicationId:binding.publication.publicationId,highlightTarget:native?{...identity,kind:'visual',visualType:native.geometry.kind==='POINT'?'POINT':'CURVE',id:native.entityId,entityId:native.entityId,featureId:native.sketchId}:undefined};
}
export function ContextInputSource({binding}:{binding?:ContextBinding}){
 const locate=useWorkbenchStore(s=>s.setSelections);
 return binding?<Button size="small" type="link" title={binding.sourceInstancePath.display} onClick={()=>locate([bindingSourceSelection(binding)])}>关联输入 · 定位来源</Button>:null;
}
