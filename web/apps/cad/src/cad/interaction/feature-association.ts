import type { FeatureAssociationIndex, FeatureTopologyAssociation, SelectionItem } from "../../types";
const indices=new WeakMap<FeatureAssociationIndex,{byLocator:Map<string,FeatureTopologyAssociation>;byFeature:Map<string,FeatureTopologyAssociation[]>}>();
function index(source:FeatureAssociationIndex) {
 let value=indices.get(source);if(value)return value;
 value={byLocator:new Map(),byFeature:new Map()};
 for(const element of source.elements){value.byLocator.set(`${element.kind}:${element.localId}`,element);for(const id of element.origins??[]){const elements=value.byFeature.get(id)??[];elements.push(element);value.byFeature.set(id,elements);}}
 indices.set(source,value);return value;
}
export function topologyFeatureAssociation(source:FeatureAssociationIndex|undefined,kind:string,id:number) {return source?index(source).byLocator.get(`${kind.toUpperCase()}:${id}`):undefined;}
export function featureContribution(source:FeatureAssociationIndex|undefined,id:string) {
 if(!source||!source.features.includes(id))return {status:"MAPPING_MISSING" as const,elements:[]};
 const elements=index(source).byFeature.get(id)??[];
 return {status:elements.length?"CURRENT" as const:source.elements.some(e=>e.status==="MAPPING_MISSING")?"MAPPING_MISSING" as const:"NO_CURRENT_CONTRIBUTION" as const,elements};
}
// Feature/tree position never substitutes for snapshot/occurrence identity.
export function sameDisplayContext(selection:SelectionItem,context:{documentId:string;versionId?:string;bodyId?:string;geometryKey:string;occurrencePath:string;contextVariantKey?:string;displayStageFeatureId?:string;instancePath?:SelectionItem["instancePath"];rootDocumentId?:string}) {
 return (selection.ownerDocumentId??selection.documentId)===context.documentId&&selection.versionId===context.versionId&&selection.bodyId===context.bodyId&&selection.geometryKey===context.geometryKey&&(selection.instancePath?.canonical??selection.occurrencePath??"")===context.occurrencePath&&selection.contextVariantKey===context.contextVariantKey&&selection.displayStageFeatureId===context.displayStageFeatureId&&(selection.instancePath?.rootDocumentId??selection.rootDocumentId??"")===(context.instancePath?.rootDocumentId??context.rootDocumentId??"");
}
