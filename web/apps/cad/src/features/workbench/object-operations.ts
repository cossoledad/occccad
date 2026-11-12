
/** Stable domain target kinds shared by menus, keyboard deletion and dialogs. */
export function objectTargetKind(kind:string):string {
 const upper=kind.replaceAll('-','_').toUpperCase();
 if(['PAD','REVOLVE','IMPORT','SKETCH'].includes(upper))return 'FEATURE';
 if(upper==='PLANE')return 'DATUM_PLANE';
 if(upper==='AXIS')return 'DATUM_AXIS';
 return upper;
}
export type ObjectDeleteTarget={targetKind:string;targetId:string;ownerEntityId?:string};
export const managedPartKinds=new Set(['BODY','GEOMETRICAL_SET','FEATURE','DATUM_PLANE','DATUM_AXIS','REFERENCE_POINT']);
export function organizationalNode(node:{kind?:string;bodyId?:string}):boolean{return node.kind!=='BODY'&&(['GEOMETRICAL_SET','REFERENCE_POINT','PLANE','DATUM_AXIS','SKETCH'].includes(node.kind??'')||node.kind==='FEATURE'&&!node.bodyId)}
export function objectDeleteTarget(node:{kind?:string;entityId?:string;ownerEntityId?:string}):ObjectDeleteTarget {
 return {targetKind:objectTargetKind(node.kind??''),targetId:node.entityId!,ownerEntityId:node.ownerEntityId};
}
