import type {DocumentStructureNode,DocumentView,ElementPresentation,SelectionItem} from '../../types';

// Human-readable paths are projections, never topology identities or repair keys.
export function elementPresentation(view:DocumentView,selection:SelectionItem):ElementPresentation|undefined{
 const publication=selection.publication??selection.productPublication;
 if(selection.kind==='publication'){
  const e=publication?.resolution.element;if(!e)return;
  const path=selection.productPublication?.target.instancePath;
  const occurrence=[selection.instancePath?.display,path?.display].filter(Boolean).join('/');
  return {...e,path:[occurrence||view.document.name,e.path].filter(Boolean).join(' / ')};
 }
 if(selection.kind!=='face'&&selection.kind!=='edge'&&selection.kind!=='vertex')return;
 const bodyNodes:DocumentStructureNode[]=[];
 const visit=(n:DocumentStructureNode)=>{if(n.kind==='BODY'&&n.bodyId===selection.bodyId&&n.geometryKey===selection.geometryKey&&n.documentId===selection.documentId&&n.versionId===selection.versionId&&(n.instancePath?.canonical??'')===(selection.occurrencePath??''))bodyNodes.push(n);n.children?.forEach(visit)};
 if(view.structureTree)visit(view.structureTree);
 if(bodyNodes.length!==1)return;
 const body=bodyNodes[0],names=new Map<string,string>();
 const features=(n:DocumentStructureNode)=>{if(n.entityId&&['PAD','REVOLVE','IMPORT','FEATURE'].includes(n.kind))names.set(n.entityId,n.name);n.children?.forEach(features)};features(body);
 const a=selection.geometryKey?view.artifacts?.[selection.geometryKey]?.visualization?.featureAssociations?.elements.find(e=>e.kind===selection.kind.toUpperCase()&&e.localId===selection.topologyId):undefined;
 const featureIds=a?.primary??[];
 const owners=featureIds.map(id=>names.get(id)).filter((s):s is string=>!!s);
 const name=({face:'面',edge:'边线',vertex:'顶点'} as Record<string,string>)[selection.kind];
 return {name,bodyId:selection.bodyId,featureId:featureIds.length===1?featureIds[0]:undefined,path:[selection.instancePath?.display||view.document.name,body.name,...owners,name].filter(Boolean).join(' / ')};
}
