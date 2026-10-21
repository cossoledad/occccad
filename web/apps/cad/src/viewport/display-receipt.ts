import type {DocumentView as DocumentDescriptor} from "../types";

export type DisplayReceipt={documentId:string;revisionId:string};
// Called only after a successful hydrated frame. An editing definition is not
// evidence by itself: the occurrence rendered from the root must use that exact
// source revision, including the requested occurrence among shared instances.
export function frameDocuments(view:DocumentDescriptor,editing?:{view:DocumentDescriptor;occurrencePath?:string}):DisplayReceipt[]{
 const receipts=[{documentId:view.document.id,revisionId:view.document.versionId}];
 const target=editing?.view.document;
 if(!target||target.id===view.document.id||!editing?.occurrencePath)return receipts;
 const path=editing.occurrencePath;
 const rendered=(view.resolvedInstances??[]).some(instance=>{
  if(instance.occurrencePath!==path&&!instance.occurrencePath.startsWith(`${path}/`))return false;
  const segments=instance.instancePath.segments;
  const index=path.split("/").length-1,segment=segments[index];
  return segment?.referencedDocumentId===target.id&&segment.resolvedVersionId===target.versionId;
 });
 if(rendered)receipts.push({documentId:target.id,revisionId:target.versionId});
 return receipts;
}
