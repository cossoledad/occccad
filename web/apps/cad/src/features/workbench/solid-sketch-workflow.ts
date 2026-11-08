import type {DocumentView,Feature} from '../../types';
import type {DisplayLengthUnit} from '../../state/ui-preferences';
import type {FeatureInsertionContext} from './historical-creation';

export type SolidQuantitySlot='length'|'length2'|'startOffset'|'endOffset'|'thin:thickness'|'thin:thickness2'|'angle'|'diameter'|'depth'|'recessDiameter'|'recessDepth'|'countersinkAngle'|'drillTipAngle'|'thread:majorDiameter'|'thread:pitch'|'thread:depth'|'loft:startMagnitude'|'loft:endMagnitude';
export type SolidFeatureDraft={inputRole?:"PROFILE"|"SWEEP_PATH";feature:Feature;quantities:Record<SolidQuantitySlot,string>;unit:DisplayLengthUnit;intentId:string};
export type SolidSketchWorkflow={documentId:string;rootDocumentId:string;occurrencePath?:string;draft:SolidFeatureDraft;insertion?:FeatureInsertionContext;inputInsertion?:FeatureInsertionContext;sketchId?:string;created?:boolean};

/** The owning occurrence survives its own revisions; a different occurrence
 * must never inherit another command's draft or selections. */
export function solidSketchWorkflowMatches(workflow:SolidSketchWorkflow,view:DocumentView,rootDocumentId:string,occurrencePath?:string){
 return workflow.documentId===view.document.id&&workflow.rootDocumentId===rootDocumentId&&(workflow.occurrencePath??'')===(occurrencePath??'');
}
export function resumeSolidSketchDraft(workflow:SolidSketchWorkflow,view:DocumentView):SolidFeatureDraft|undefined {
 const saved=workflow.draft,latest=saved.feature.id?view.part?.features.find(f=>f.id===saved.feature.id):undefined;
 if(!view.part||saved.feature.id&&(!latest||latest.type!==saved.feature.type||latest.bodyId!==saved.feature.bodyId))return;
 let feature={...saved.feature};
 if(workflow.created&&workflow.sketchId){
  if(!view.part.features.some(f=>f.id===workflow.sketchId&&f.sketch))return;
  if(saved.inputRole==="SWEEP_PATH"&&feature.sweep) {feature={...feature,sweep:{...feature.sweep,path:{sketchId:workflow.sketchId,entityIds:[]}}};}
  else feature={...feature,profile:workflow.sketchId,profileMemberSlot:undefined,profileSelection:{mode:'REGIONS',regionIds:[...(view.sketchAnalyses?.[workflow.sketchId]?.regionIds??[])]}};
 }
 // Existing explicit region identities are retained, including missing ones.
 // Repair remains an explicit choice in the resumed editor.
 return {...saved,feature};
}
