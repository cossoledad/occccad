import {decodeMeshGLB, type DecodedVisual} from './mesh-glb';
import {packedMesh} from './packed-mesh';
import type {MeshData} from '../../types';
import type {MeshBuffers} from '../rendering/mesh-topology-index';
import {buildContourIncidence,type ContourIncidence} from '../rendering/mesh-contour-incidence';
type Result = {decoded: DecodedVisual; buffer: ArrayBuffer};
type Request = {buffer:ArrayBuffer;contours:boolean}|{incidence:MeshBuffers};
type Reply = {id:number;error?:string;contourIncidence?:ContourIncidence;buffer:ArrayBuffer;buffers:MeshBuffers;edges:MeshData['edges'];topologyVertices:MeshData['topologyVertices'];edgeProfile:MeshData['edgeProfile'];association:DecodedVisual['association'];visualization:DecodedVisual['visualization']};
type Job = {id:number;request:Request;transfer:ArrayBuffer[];signal:AbortSignal;complete:(data:Reply)=>void;reject:(e:unknown)=>void;off:()=>void};
const canceled=()=>new DOMException('Display canceled','AbortError');

/** One bounded lane per viewport, for decoding and promotion-only indices.
 * Canceling active work terminates its worker; queued buffers stay with their owner. */
export class VisualDecoder {
  private queue:Job[]=[];
  private nextID=0;
  private active?:Job;
  private worker?:Worker;
  decode(buffer:ArrayBuffer,contours:boolean,signal:AbortSignal):Promise<Result> {
    if(signal.aborted)return Promise.reject(canceled());
    if(typeof Worker==='undefined'||buffer.byteLength<512*1024) {
      const decoded=decodeMeshGLB(buffer);
      if(contours&&decoded.mesh.buffers!.indices.length)Object.defineProperty(decoded.mesh,'contourIncidence',{value:buildContourIncidence(decoded.mesh.buffers!)});
      return Promise.resolve({decoded,buffer});
    }
    return this.submit({buffer,contours},[buffer],signal,data=>{
      const mesh=packedMesh(data.buffers,data.edges,data.topologyVertices);mesh.edgeProfile=data.edgeProfile;
      if(data.contourIncidence)Object.defineProperty(mesh,'contourIncidence',{value:data.contourIncidence});
      return {buffer:data.buffer,decoded:{mesh,association:data.association,visualization:data.visualization}};
    });
  }
  prepareContours(mesh:MeshData,signal:AbortSignal):Promise<ContourIncidence|undefined> {
    if(signal.aborted)return Promise.reject(canceled());
    if(mesh.contourIncidence||!mesh.buffers?.indices.length)return Promise.resolve(mesh.contourIncidence);
    if(typeof Worker==='undefined'||mesh.buffers.indices.length<32768)return Promise.resolve(buildContourIncidence(mesh.buffers));
    // Transfer only private copies required by incidence. The live preview mesh,
    // its BVH, normals, raw GLB and GPU buffers retain their existing ownership.
    const buffers:MeshBuffers={positions:mesh.buffers.positions.slice(),indices:mesh.buffers.indices.slice(),faceIds:mesh.buffers.faceIds.slice()};
    return this.submit({incidence:buffers},[buffers.positions.buffer,buffers.indices.buffer,buffers.faceIds.buffer] as ArrayBuffer[],signal,data=>data.contourIncidence);
  }
  private submit<T>(request:Request,transfer:ArrayBuffer[],signal:AbortSignal,finish:(data:Reply)=>T):Promise<T> {
    return new Promise((resolve,reject)=>{
      const job:Job={id:++this.nextID,request,transfer,signal,complete:data=>resolve(finish(data)),reject,off:()=>signal.removeEventListener('abort',abort)};
      const abort=()=>{
        if(this.active===job){this.worker?.terminate();this.worker=undefined;this.active=undefined;}
        else this.queue=this.queue.filter(j=>j!==job);
        job.off();reject(canceled());this.start();
      };
      signal.addEventListener('abort',abort,{once:true});
      if(signal.aborted){abort();return;}
      this.queue.push(job);this.start();
    });
  }
  private start():void {
    if(this.active)return;
    const aborted=this.queue.filter(job=>job.signal.aborted);this.queue=this.queue.filter(job=>!job.signal.aborted);
    for(const job of aborted){job.off();job.reject(canceled());}
    if(!this.queue.length)return;
    const job=this.active=this.queue.shift()!;
    try {
      this.worker??=new Worker(new URL('./visual-decode.worker.ts',import.meta.url),{type:'module'});
      this.worker.onmessage=({data}:MessageEvent<Reply>)=>{
        if(this.active!==job||data.id!==job.id)return;
        this.active=undefined;job.off();
        if(this.worker){this.worker.onmessage=null;this.worker.onerror=null;}
        if(data.error)job.reject(new Error(data.error));else {try{job.complete(data);}catch(error){job.reject(error);}}
        this.start();
      };
      this.worker.onerror=()=>{
        if(this.active!==job)return;
        this.worker?.terminate();this.worker=undefined;this.active=undefined;
        job.off();job.reject(new Error('Display decode worker failed'));this.start();
      };
      this.worker.postMessage({id:job.id,...job.request},job.transfer);
    }catch(error){this.active=undefined;job.off();if(this.worker){this.worker.onmessage=null;this.worker.onerror=null;}job.reject(error);this.start();}
  }
  dispose():void {
    this.worker?.terminate();this.worker=undefined;
    for(const job of [...(this.active?[this.active]:[]),...this.queue]){job.off();job.reject(canceled());}
    this.active=undefined;this.queue=[];
  }
}
