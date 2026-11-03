import type {AssemblyGeometryRef,DocumentView} from '../../types';
import {assemblyPublicationNode} from '../../cad/assembly/assembly-publication-owner';
import {structureSelection} from './structure-selection';
export function assemblyPublicationSelection(view:DocumentView,reference:AssemblyGeometryRef,ownerOccurrence=''){
 const node=assemblyPublicationNode(reference,view,ownerOccurrence);return node?structureSelection(node,view):undefined;
}

import {selectionKey} from '../../cad/interaction/selection-identity';
import type {SelectionItem,DocumentStructureNode} from '../../types';
/** Prefer the nearest public boundary in the active Product, never a name or
 * mesh-number correspondence. Multiple aliases remain an explicit choice. */
export function assemblyPublicationCandidates(view:DocumentView,pick:SelectionItem,ownerOccurrence=''):SelectionItem[]{
 if(pick.kind==='publication')return [pick];
 const raw=pick.highlightTarget??pick,matches:Array<{depth:number;selection:SelectionItem}>=[];
 const visit=(n:DocumentStructureNode)=>{
  const owner=n.instancePath?.canonical??'';
  if((n.kind==='PUBLICATION'||n.kind==='PRODUCT_PUBLICATION')&&(!ownerOccurrence||owner===ownerOccurrence||owner.startsWith(ownerOccurrence+'/'))){
   const selected=structureSelection(n,view),target=selected?.highlightTarget;
   if(target&&selectionKey(target)===selectionKey(raw))matches.push({depth:n.instancePath?.segments.length??0,selection:selected!});
  }
  n.children?.forEach(visit);
 };if(view.structureTree)visit(view.structureTree);
 const depth=Math.min(...matches.map(m=>m.depth));return matches.filter(m=>m.depth===depth).map(m=>m.selection);
}
export type AssemblyPublicationChoice={choices:SelectionItem[];choose:(selection:SelectionItem)=>void;cancel:()=>void};
