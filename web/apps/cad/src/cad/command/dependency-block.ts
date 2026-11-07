/** Read-only domain identities for explaining a rejected removal. */
export type DependencyObject={kind:string;id:string;name:string};
export type DependencyBlock={subject:DependencyObject;consumers:DependencyObject[];total:number};
const kinds=new Set(['MEASUREMENT','PARAMETER','FEATURE','ASSEMBLY_CONSTRAINT','PUBLICATION','PRODUCT_PUBLICATION','CONTEXT_INPUT','PLANE','DATUM_AXIS','DATUM_POINT']);
export function dependencyBlock(value:unknown):DependencyBlock|undefined {
 const object=(v:unknown):v is DependencyObject=>!!v&&typeof v==='object'&&kinds.has((v as DependencyObject).kind)&&typeof (v as DependencyObject).id==='string'&&(v as DependencyObject).id.length>0&&(v as DependencyObject).id.length<=256&&typeof (v as DependencyObject).name==='string'&&(v as DependencyObject).name.length>0&&(v as DependencyObject).name.length<=512;
 const b=value as DependencyBlock;if(!b||!object(b.subject)||!Array.isArray(b.consumers)||!b.consumers.length||b.consumers.length>32||!b.consumers.every(object)||!Number.isSafeInteger(b.total)||b.total<b.consumers.length)return;
 return b;
}
export const dependencyKind=(kind:string)=>({MEASUREMENT:'测量',PARAMETER:'参数',FEATURE:'特征',ASSEMBLY_CONSTRAINT:'装配约束',PUBLICATION:'发布',PRODUCT_PUBLICATION:'发布',CONTEXT_INPUT:'关联输入',PLANE:'参考面',DATUM_AXIS:'参考线',DATUM_POINT:'参考点'}[kind]??'对象');
