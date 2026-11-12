import {decodeMeshGLB, type DecodedVisual} from './mesh-glb';
import {packedMesh} from './packed-mesh';
import {buildContourIncidence} from '../rendering/mesh-contour-incidence';
type Result = {decoded: DecodedVisual; buffer: ArrayBuffer};
type Job = {id: number; buffer: ArrayBuffer; contours: boolean; signal: AbortSignal; resolve: (r: Result) => void; reject: (e: unknown) => void; off: () => void};
const canceled = () => new DOMException('Display canceled', 'AbortError');

/** One bounded decode lane per viewport. Canceling an active job terminates its
 * worker; queued jobs keep their original buffers and resume in a fresh lane. */
export class VisualDecoder {
  private queue: Job[] = [];
  private nextID = 0;
  private active?: Job;
  private worker?: Worker;
  decode(buffer: ArrayBuffer, contours: boolean, signal: AbortSignal): Promise<Result> {
    if (signal.aborted) return Promise.reject(canceled());
    if (typeof Worker === 'undefined' || buffer.byteLength < 512 * 1024) {
      const decoded=decodeMeshGLB(buffer);
      if(contours&&decoded.mesh.buffers!.indices.length)Object.defineProperty(decoded.mesh,'contourIncidence',{value:buildContourIncidence(decoded.mesh.buffers!)});
      return Promise.resolve({decoded,buffer});
    }
    return new Promise((resolve, reject) => {
      const job: Job = {id: ++this.nextID, buffer, contours, signal, resolve, reject, off: () => signal.removeEventListener('abort', abort)};
      const abort = () => {
        if (this.active === job) { this.worker?.terminate(); this.worker = undefined; this.active = undefined; }
        else this.queue = this.queue.filter(j => j !== job);
        job.off(); reject(canceled()); this.start();
      };
      signal.addEventListener('abort', abort, {once: true});
      this.queue.push(job); this.start();
    });
  }
  private start(): void {
    if (this.active) return;
    const aborted=this.queue.filter(job=>job.signal.aborted);
    this.queue=this.queue.filter(job=>!job.signal.aborted);
    for(const job of aborted){job.off();job.reject(canceled());}
    if (!this.queue.length) return;
    const job = this.active = this.queue.shift()!;
    try {
      this.worker ??= new Worker(new URL('./visual-decode.worker.ts', import.meta.url), {type: 'module'});
      this.worker.onmessage = ({data}) => {
        if (this.active !== job || data.id !== job.id) return;
        this.active = undefined; job.off();
        if(this.worker){this.worker.onmessage=null;this.worker.onerror=null;}
        if (data.error) job.reject(new Error(data.error));
        else {
          const mesh = packedMesh(data.buffers, data.edges, data.topologyVertices);
          if (data.contourIncidence) Object.defineProperty(mesh,'contourIncidence',{value:data.contourIncidence});
          job.resolve({buffer: data.buffer, decoded: {mesh, association: data.association, visualization: data.visualization}});
        }
        this.start();
      };
      this.worker.onerror = () => {
        if (this.active !== job) return;
        this.worker?.terminate(); this.worker = undefined; this.active = undefined;
        job.off(); job.reject(new Error('Display decode worker failed')); this.start();
      };
      this.worker.postMessage({id: job.id, buffer: job.buffer, contours: job.contours}, [job.buffer]);
    } catch (error) { this.active = undefined; job.off(); if(this.worker){this.worker.onmessage=null;this.worker.onerror=null;} job.reject(error); this.start(); }
  }
  dispose(): void {
    this.worker?.terminate(); this.worker = undefined;
    for (const job of [...(this.active ? [this.active] : []), ...this.queue]) {job.off(); job.reject(canceled());}
    this.active = undefined; this.queue = [];
  }
}
