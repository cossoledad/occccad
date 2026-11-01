import type {DocumentView,SelectionItem} from '../../types';
import {assemblyGeometryRef} from './assembly-reference';
import {staticPartLeaves} from './static-interference';
/** A positioning pick is a one-shot exact measurement, not a new association. */
export function sectionTargetPick(view:DocumentView,pick:SelectionItem):boolean {
 if(!['plane','axis','face','edge'].includes(pick.kind))return false;
 const reference=assemblyGeometryRef(pick),path=pick.instancePath;if(!reference||!path||path.rootDocumentId!==view.document.id)return false;
 const accepted=staticPartLeaves(view).get(path.canonical)?.path;
 if(!accepted||accepted.segments.length!==path.segments.length)return false;
 if(path.segments.some((p,i)=>{const a=accepted.segments[i];return p.instanceId!==a.instanceId||p.ownerDocumentId!==a.ownerDocumentId||p.ownerVersionId!==a.ownerVersionId||p.resolvedVersionId!==a.resolvedVersionId||p.referencedDocumentId!==a.referencedDocumentId}))return false;
 const leaf=accepted.segments.at(-1)!;if((pick.ownerDocumentId??pick.documentId)!==leaf.referencedDocumentId||pick.versionId!==leaf.resolvedVersionId)return false;
 if(pick.geometryKey&&!view.resolvedInstances?.some(r=>r.occurrencePath===path.canonical&&r.geometryKey===pick.geometryKey&&!r.displayFallback))return false;
 return true;
}
