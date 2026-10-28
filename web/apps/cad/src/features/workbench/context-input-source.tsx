import {Button} from 'antd';
import type {ContextBinding,DocumentView,SelectionItem} from '../../types';
import {useWorkbenchStore} from '../../state/workbench-store';
import {structureSelection} from './structure-selection';

export function boundContextParameter(view:DocumentView,id?:string){
 const parameter=view.part?.parameters?.find(p=>p.parameterId===id);
 const port=view.part?.contextInputs?.find(p=>p.target.kind==='PARAMETER'&&p.target.targetId===id||p.target.kind==='DATUM'&&!!parameter?.ownerFeatureId&&p.target.targetId===parameter.ownerFeatureId);
 return port?view.contextProduct?.product?.contextBindings?.find(b=>b.contextInputId===port.id&&b.owningInstancePath.canonical===view.contextOccurrencePath):undefined;
}
export function bindingSourceSelection(binding:ContextBinding,root?:DocumentView):SelectionItem {
 const path=binding.sourceInstancePath,leaf=path.segments.at(-1),native=binding.resolution.nativeSketch;
 const identity={documentId:leaf?.referencedDocumentId??'',versionId:binding.accepted.sourceRevisionId,instancePath:path,occurrencePath:path.canonical};
 const find=(n:NonNullable<DocumentView['structureTree']>):SelectionItem|undefined=>{
  if((n.kind==='PUBLICATION'||n.kind==='PRODUCT_PUBLICATION')&&n.entityId===binding.publication.publicationId&&n.instancePath?.canonical===path.canonical&&n.versionId===binding.accepted.sourceRevisionId)return structureSelection(n,root!)??undefined;
  for(const child of n.children??[]){const found=find(child);if(found)return found}
 };
 const accepted=root?.structureTree?find(root.structureTree):undefined;
 // A module's native descriptor is in module coordinates, but its sketch ID
 // belongs to a descendant. Only the authoritative tree can locate that source.
 return accepted??{...identity,kind:'publication',id:binding.publication.publicationId,publicationId:binding.publication.publicationId,highlightTarget:native&&binding.resolution.resolvedVersionId===binding.accepted.sourceRevisionId?{...identity,kind:'visual',visualType:native.geometry.kind==='POINT'?'POINT':'CURVE',id:native.entityId,entityId:native.entityId,featureId:native.sketchId}:undefined};
}
export function ContextInputSource({binding,view}:{binding?:ContextBinding;view?:DocumentView}){
 const locate=useWorkbenchStore(s=>s.setSelections);
 return binding?<Button size="small" type="link" title={binding.sourceInstancePath.display} onClick={()=>locate([bindingSourceSelection(binding,view?.contextProduct??view)])}>关联输入 · 定位来源</Button>:null;
}
