export type AnnotationLayout={version:number;offset:[number,number]|null};
export type AnnotationLayouts=Record<string,AnnotationLayout>;
export type AnnotationLayoutAPI={load:(documentId:string)=>Promise<AnnotationLayouts>;save:(documentId:string,id:string,layout:AnnotationLayout)=>Promise<AnnotationLayout>};

/** Coalesces gestures per object, preserving CAS versions and document lifetime. */
export class AnnotationLayoutSession {
 private layouts:AnnotationLayouts={};
 private pending=new Map<string,[number,number]|null>();
 private running=new Set<string>();
 private active=true;
 constructor(private documentId:string,private api:AnnotationLayoutAPI,private changed:(layouts:AnnotationLayouts)=>void,private failed:(retry:()=>void)=>void){}
 async load(){try{const values=await this.api.load(this.documentId);if(!this.active)return;this.layouts=values;this.changed(this.snapshot())}catch{if(this.active)this.failed(()=>void this.load())}}
 dispose(){this.active=false;this.pending.clear()}
 private snapshot(){return structuredClone(this.layouts)}
 set=(id:string,offset:[number,number]|undefined)=>{
  if(!this.active)return;
  this.layouts[id]={version:this.layouts[id]?.version??0,offset:offset??null};this.pending.set(id,offset??null);this.changed(this.snapshot());void this.flush(id);
 };
 private async flush(id:string){
  if(this.running.has(id)||!this.active)return;this.running.add(id);
  try{while(this.active&&this.pending.has(id)){
   const offset=this.pending.get(id)!;this.pending.delete(id);
   try{const saved=await this.api.save(this.documentId,id,{version:this.layouts[id]?.version??0,offset});if(!this.active)return;this.layouts[id]={...this.layouts[id],version:saved.version};this.changed(this.snapshot())}
   catch{if(!this.active)return;if(!this.pending.has(id))this.pending.set(id,offset);this.failed(()=>void this.reconcile(id));return}
  }}finally{this.running.delete(id)}
 }
 private async reconcile(id:string){try{const values=await this.api.load(this.documentId);if(!this.active)return;this.layouts[id]={...this.layouts[id],version:values[id]?.version??0};void this.flush(id)}catch{if(this.active)this.failed(()=>void this.reconcile(id))}}
}
