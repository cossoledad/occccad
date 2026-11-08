import type {SketchEntity,SketchOperation} from '../../types';
import {LatestPreviewQueue} from '../../features/workbench/latest-preview-queue';

type Result={entities:SketchEntity[]};
type Candidate={key:string;status:'pending'|'ready'|'failed';operations?:SketchOperation[];entities?:SketchEntity[];error?:unknown;controller:AbortController;timer?:ReturnType<typeof setTimeout>;start:()=>Promise<void>;promise?:Promise<void>};

/** One immutable drawing candidate, one running solve and one replaceable
 * queued solve. Confirmation flushes the debounce and consumes these exact IDs. */
export class SketchCreationPreview {
 private queue=new LatestPreviewQueue<Result>();
 current?:Candidate;
 select(key:string,operations:()=>SketchOperation[],run:(operations:SketchOperation[],signal:AbortSignal)=>Promise<Result>,valid:()=>boolean,changed:()=>void):Candidate {
  if(this.current?.key===key)return this.current;
  this.cancel();
  const controller=new AbortController();
  const candidate:Candidate={key,status:'pending',controller,start:()=>{
   if(candidate.promise)return candidate.promise;
   clearTimeout(candidate.timer);
   const accepted=()=>this.current===candidate&&!controller.signal.aborted&&valid();
   candidate.promise=this.queue.submit(signal=>{
    if(!accepted())throw new DOMException('Drawing context changed','AbortError');
    candidate.operations=operations();return run(candidate.operations,signal);
   },controller.signal).then(result=>{
    if(!accepted())throw new DOMException('Drawing preview superseded','AbortError');
    candidate.entities=result.entities;candidate.status='ready';changed();
   },error=>{if(accepted()){candidate.status='failed';candidate.error=error;changed();}throw error;});
   // The UI may only inspect the preview. A later confirm awaits the same promise.
   void candidate.promise.catch(()=>{});return candidate.promise;
  }};
  this.current=candidate;candidate.timer=setTimeout(()=>void candidate.start().catch(()=>{}),150);return candidate;
 }
 async resolve(candidate:Candidate):Promise<SketchOperation[]>{
  if(this.current!==candidate)throw new DOMException('Drawing preview superseded','AbortError');
  await candidate.start();
  if(this.current!==candidate||candidate.status!=='ready')throw new DOMException('Drawing preview superseded','AbortError');
  return candidate.operations!;
 }
 cancel(){const candidate=this.current;this.current=undefined;if(candidate){clearTimeout(candidate.timer);candidate.controller.abort();}}
}
