import {request} from '../api';
import type {AnnotationLayoutAPI,AnnotationLayouts} from '../cad/measurement/annotation-layout';

const mockLayouts=new Map<string,AnnotationLayouts>();
export const annotationLayoutAPI:AnnotationLayoutAPI=import.meta.env.VITE_API_MODE==='mock'?{
 load:async id=>structuredClone(mockLayouts.get(id)??{}),
 save:async(doc,id,input)=>{const values=mockLayouts.get(doc)??{};if((values[id]?.version??0)!==input.version)throw new Error('presentation conflict');const saved={version:input.version+1,offset:input.offset};values[id]=saved;mockLayouts.set(doc,values);return structuredClone(saved)},
}:{
 load:id=>request(`/api/documents/${encodeURIComponent(id)}/annotation-layouts`),
 save:(doc,id,input)=>request(`/api/documents/${encodeURIComponent(doc)}/annotation-layouts/${encodeURIComponent(id)}`,{method:'PUT',body:JSON.stringify(input)}),
};
