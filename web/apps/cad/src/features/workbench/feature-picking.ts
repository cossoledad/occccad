import type { DocumentView, Feature, SelectionItem } from "../../types";
import { projectSketchFeatureSelection } from "../../cad/interaction/selection-mode";
export function selectedSketch(view: DocumentView, selection: SelectionItem, upstream: Feature[]): Feature | undefined {
    const projected = projectSketchFeatureSelection(selection);
    if (projected?.kind !== "sketch" || (projected.ownerDocumentId ?? projected.documentId) !== view.document.id)
        return;
    if(projected.patternId&&projected.patternMemberSlot!==undefined){
      const pattern=upstream.find(f=>f.id===projected.patternId&&f.type==="SKETCH_PATTERN");
      return pattern?{...pattern,profileMemberSlot:projected.patternMemberSlot}:undefined;
    }
    return upstream.find(f => f.id === projected.id && !!f.sketch);
}
export function selectedAxis(view: DocumentView, selection: SelectionItem, upstream: Feature[]): string | undefined {
    if ((selection.ownerDocumentId ?? selection.documentId) !== view.document.id)
        return;
    if (selection.kind === "visual") {
        const entity = upstream.find(f => f.id === selection.featureId)?.sketch?.entities.find(e => e.id === selection.entityId);
        return entity?.kind === "LINE" ? `SKETCH_LINE:${selection.featureId}:${entity.id}` : undefined;
    }
    if (selection.kind === "axis") {
        if (selection.axis === "DATUM") {
            const axis = view.part?.datumAxes?.find(a => a.id === selection.entityId || selection.id === a.id || selection.id.endsWith(`:${a.id}`));
            return axis ? `DATUM_AXIS:${axis.id}` : undefined;
        }
        const system = view.part?.axisSystems.find(a => a.id === selection.entityId || selection.id === `${a.id}:${selection.axis}` || selection.id.endsWith(`:${a.id}:${selection.axis}`));
        return system ? `AXIS_SYSTEM:${system.id}:${selection.axis}` : undefined;
    }
}
export function bodyStage(selection: SelectionItem, stages: Feature[]): {
    bodyId: string;
    featureId: string;
} | undefined {
    if (!selection.bodyId)
        return;
    // Tree Feature selections explicitly identify a historical stage. A viewport
    // body hit identifies the eligible tip, never the future final Body result.
    const explicit = ["pad", "feature", "import"].includes(selection.kind);
    const stage = explicit ? stages.find(f => f.id === selection.entityId || f.id === selection.id) : selection.displayStageFeatureId ? stages.find(f=>f.id===selection.displayStageFeatureId) : [...stages].reverse().find(f => f.bodyId === selection.bodyId && !f.suppressed);
    return stage?.bodyId === selection.bodyId ? { bodyId: stage.bodyId, featureId: stage.id } : undefined;
}
export function sketchPick(view: DocumentView, id: string, occurrencePath?: string, memberSlot?: number): SelectionItem {
    const memberId = memberSlot === undefined ? undefined : profileSelectionIDs(view, view.part?.features ?? [], id, memberSlot)[0];
    return { kind: "sketch", id: memberId ?? id, ...(memberId ? {patternId:id,patternMemberSlot:memberSlot} : {}), documentId: view.document.id, versionId: view.document.versionId, bodyId: view.part?.features.find(f => f.id === id)?.bodyId, occurrencePath, expandTreeDescendants: true };
}

export function profileSelectionIDs(view:DocumentView,upstream:Feature[],patternId?:string,slot?:number):string[] {
 const ids=patternId?[]:upstream.filter(f=>f.sketch||f.type==="SKETCH_PATTERN").map(f=>f.id);
 const visit=(node:DocumentView["structureTree"])=>{
  if(!node)return;
  if(node.kind==="SKETCH_PATTERN_MEMBER"&&node.entityId&&upstream.some(f=>f.id===node.patternId)&&(!patternId||node.patternId===patternId&&node.patternMemberSlot===slot))ids.push(node.entityId);
  node.children?.forEach(visit);
 };
 if(view.structureTree)visit(view.structureTree);
 return ids;
}

// A generator tool is not the final Body stage. Viewport association may offer
// several actual generators; callers present those candidates explicitly.
export function patternSourceCandidates(selection:SelectionItem,stages:Feature[],kind:"GENERATOR_TOOL"|"BODY_STAGE"|"FEATURE_DELTA"):Feature[] {
 const eligible=stages.filter(f=>!f.suppressed&&f.bodyId===selection.bodyId&&(kind!=="GENERATOR_TOOL"?!f.sketch&&f.type!=="SKETCH_PATTERN":["PAD","LINEAR_EXTRUDE","REVOLVE"].includes(f.type)&&f.extent!=="THROUGH_ALL"));
 if(["pad","feature","import"].includes(selection.kind))return eligible.filter(f=>f.id===(selection.entityId??selection.id));
 if(kind==="GENERATOR_TOOL"||kind==="FEATURE_DELTA")return eligible.filter(f=>(selection.sourceFeatureIds??selection.associatedFeatureIds)?.includes(f.id));
 const stage=selection.displayStageFeatureId?eligible.find(f=>f.id===selection.displayStageFeatureId):eligible.at(-1);
 return stage?[stage]:[];
}

// A material range has an explicit additive seed and inclusive end. Only local
// fillet/chamfer steps are supported; this never promises arbitrary group replay.
export function patternRangeEnds(stages:Feature[],startId?:string):Feature[] {
 const start=stages.find(f=>f.id===startId);
 if(!start||start.suppressed||!["PAD","LINEAR_EXTRUDE","REVOLVE"].includes(start.type)||!["ADD","NEW_BODY"].includes(start.operation??"ADD")||start.extent==="THROUGH_ALL")return [];
 const ends:Feature[]=[];
 for(const f of stages.slice(stages.indexOf(start))){
  if(f.suppressed||f.bodyId!==start.bodyId||f.sketch||f.type==="SKETCH_PATTERN")continue;
  if(f!==start&&!["FILLET","CHAMFER"].includes(f.type))break;
  ends.push(f);
 }
 return ends;
}

export function patternRangeFromSelections(view:DocumentView,selections:SelectionItem[],stages:Feature[],occurrencePath?:string):
 {status:"NONE"}|{status:"INVALID"}|{status:"READY";bodyId:string;startFeatureId:string;endFeatureId:string} {
 const explicit=selections.filter(s=>["pad","feature","import"].includes(s.kind));
 const ids=[...new Set(explicit.map(s=>s.entityId??s.id))];
 if(ids.length<2)return {status:"NONE"};
 if(explicit.some(s=>(s.ownerDocumentId??s.documentId)!==view.document.id||s.versionId!==view.document.versionId||(s.instancePath?.canonical??s.occurrencePath??"")!==(occurrencePath??"")))return {status:"INVALID"};
 const selected=stages.filter(f=>ids.includes(f.id));
 const first=selected[0],last=selected.at(-1);
 if(selected.length!==ids.length||!first?.bodyId||!last||selected.some(f=>f.bodyId!==first.bodyId)||!patternRangeEnds(stages,first.id).some(f=>f.id===last.id))return {status:"INVALID"};
 return {status:"READY",bodyId:first.bodyId,startFeatureId:first.id,endFeatureId:last.id};
}
