/** One exact calculation at a time, with only the newest pending snapshot.
 * Completed samples may be displayed during a gesture; only the caller's
 * current snapshot is eligible for saving. Cancellation fences late results. */
export class SectionPreviewQueue<I,O> {
 private pending?:I;private running=false;private generation=0;private controller?:AbortController;
 constructor(private execute:(input:I,signal:AbortSignal)=>Promise<O>,private result:(output:O,input:I)=>void,private failure:(error:unknown,input:I)=>void,private busy:(value:boolean)=>void){}
 submit(input:I){this.pending=input;void this.pump()}
 cancel(){this.generation++;this.pending=undefined;this.controller?.abort();this.busy(false)}
 private async pump(){
  if(this.running||this.pending===undefined)return;
  this.running=true;this.busy(true);
  const input=this.pending,generation=this.generation,controller=new AbortController();this.pending=undefined;this.controller=controller;
  try{const output=await this.execute(input,controller.signal);if(!controller.signal.aborted&&generation===this.generation)this.result(output,input)}
  catch(error){if(!controller.signal.aborted&&generation===this.generation)this.failure(error,input)}
  finally{this.running=false;if(this.pending!==undefined)void this.pump();else this.busy(false)}
 }
}
