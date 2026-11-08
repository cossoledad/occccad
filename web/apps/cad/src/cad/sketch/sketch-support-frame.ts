import type {DocumentView,Feature,SketchPlane} from "../../types";

export function featureSketchPlane(view: DocumentView, feature: Feature): SketchPlane | undefined {
	const support = feature.sketch?.support;
 if(support?.status==="FAILED_SUPPORT"||support?.status==="BLOCKED")return;
	if (support?.type === "PLANAR_FACE" && support.origin && support.normal && support.xDirection) {
		return { datumPlaneId: `face:${support.persistentSelection?.anchor.featureId ?? feature.id}`,
			plane: "CUSTOM", origin: support.origin, normal: support.normal, uDirection: support.xDirection };
	}
  const datum = view.datumPlanes?.find((candidate) => candidate.id === feature.sketch?.support.datumPlaneId)
    ?? view.part?.datumPlanes.find((candidate) => candidate.id === feature.sketch?.support.datumPlaneId);
  return datum?{datumPlaneId:datum.id,plane:datum.plane,origin:datum.origin,normal:datum.normal,uDirection:datum.uDirection}:undefined;
}

