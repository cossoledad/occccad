import type {DocumentView,Feature,SelectionItem} from "../../types";
import type {FeatureSelectionSession} from "../../cad/interaction/feature-selection";

// Classification and hole ownership arrive from the exact backend Builder.
// These samples are only a pick surface; persisted identity is always region.id.
export function profileRegionCandidates(view:DocumentView,upstream:Feature[],profile?:string):NonNullable<FeatureSelectionSession["profileRegions"]> {
 return upstream.flatMap(feature=>{
  if(profile&&feature.id!==profile)return [];
  const analysis=view.sketchAnalyses?.[feature.id], support=feature.sketch?.support;
  if(!analysis?.geometryVerified||analysis.status!=="CLOSED"||!analysis.regionViews||!support||support.status==="FAILED_SUPPORT"||feature.suppressed)return [];
  const datum=(view.datumPlanes??view.part?.datumPlanes)?.find(d=>d.id===support.datumPlaneId);
  const plane=support.type==="PLANAR_FACE"&&support.origin&&support.normal&&support.xDirection
   ? {datumPlaneId:`face:${feature.id}`,plane:"CUSTOM" as const,origin:support.origin,normal:support.normal,uDirection:support.xDirection}
   : datum?{datumPlaneId:datum.id,plane:datum.plane,origin:datum.origin,normal:datum.normal,uDirection:datum.uDirection}:undefined;
  return plane?[{sketchId:feature.id,bodyId:feature.bodyId,plane,regions:analysis.regionViews}]:[];
 });
}
export function profileRegionPick(view:DocumentView,profile:string,regionId:string,occurrencePath?:string):SelectionItem {
 return {kind:"profile-region",id:`${profile}/${regionId}`,featureId:profile,regionId,documentId:view.document.id,versionId:view.document.versionId,bodyId:view.part?.features.find(f=>f.id===profile)?.bodyId,occurrencePath};
}
