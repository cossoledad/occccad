import type {AssemblyGeometryRef,DocumentStructureNode,DocumentView} from '../../types';

/** The publication's owner is independent of the geometry's moving body. */
export function assemblyPublicationNode(reference:AssemblyGeometryRef,view:DocumentView|undefined,ownerOccurrence=''):DocumentStructureNode|undefined{
 const port=reference.publicationRef;if(!port||!view?.structureTree)return;
 const owner=port.ownerPath??reference.instancePath;
 const path=port.scope==='LOCAL'?ownerOccurrence:[ownerOccurrence,owner?.canonical??reference.instanceId].filter(Boolean).join('/');
 const leaf=port.scope==='LOCAL'?undefined:owner?.segments.at(-1);
 const visit=(n:DocumentStructureNode):DocumentStructureNode|undefined=>{
  if((n.kind==='PUBLICATION'||n.kind==='PRODUCT_PUBLICATION')&&n.entityId===port.publicationId&&(n.instancePath?.canonical??'')===path&&(!leaf||n.documentId===leaf.referencedDocumentId&&n.versionId===leaf.resolvedVersionId))return n;
  for(const c of n.children??[]){const found=visit(c);if(found)return found}
 };return visit(view.structureTree);
}
