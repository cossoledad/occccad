import * as THREE from 'three';
import type {SpatialAnalysis} from '../assembly/spatial-analysis';
import {AssemblyManipulator} from './assembly-manipulator';
import type {CadShaderLibrary} from '../rendering/shader/cad-shader-library';
import {worldUnitsPerCssPixel,solveScreenConstrainedParameter,type ViewportMetrics} from '../rendering/viewport-metrics';
import {disposeSectionObjects,sectionRepresentation} from '../rendering/section-presentation';
import {CATIA_VISUAL_THEME as theme} from '../rendering/cad-visual-theme';
import {InputResult,type CadInputSink,type CadPointerEvent,type CadKeyboardEvent} from '../input/input-types';

export type SectionGesturePhase='start'|'change'|'finish'|'cancel';
export type SectionEditSession={documentId:string;revisionId:string;definition:SpatialAnalysis;enabled:boolean;picking?:boolean;onChange:(definition:SpatialAnalysis,phase:SectionGesturePhase)=>void};
type DirectHit={axis?:number;side?:number;point:THREE.Vector3};
type DirectDrag={id:number;original:SpatialAnalysis;base:SpatialAnalysis;hit:DirectHit;parameter:number;offset:THREE.Vector2;uv:boolean;uvAnchor?:THREE.Vector3;anchor:THREE.Vector3};
const axis=(i:number)=>new THREE.Vector3(i===0?1:0,i===1?1:0,i===2?1:0);
const clone=(d:SpatialAnalysis):SpatialAnalysis=>({...d,frame:{translation:[...d.frame.translation],rotation:[...d.frame.rotation]},size:[...d.size]});
export function resizeSection(d:SpatialAnalysis,index:number,side:number,delta:number):SpatialAnalysis{
 const next=clone(d),size=Math.max(1e-4,d.size[index]+side*delta),shift=axis(index).applyQuaternion(new THREE.Quaternion().fromArray(d.frame.rotation)).multiplyScalar(side*(size-d.size[index])/2);
 next.size[index]=size;next.frame.translation=new THREE.Vector3().fromArray(d.frame.translation).add(shift).toArray();return next;
}
/** A command-owned draft manipulator. InputManager/InteractionRouter own all DOM
 * events and capture; this object never writes models or installs listeners. */
export class SectionManipulator implements CadInputSink {
 readonly root=new THREE.Group();readonly gizmo:AssemblyManipulator;
 private session?:SectionEditSession;private value?:SpatialAnalysis;private base?:SpatialAnalysis;
 private direct?:DirectDrag;private representation?:THREE.Group;private gesture=false;private silent=false;private directHovered=false;
 constructor(shaders:CadShaderLibrary,private readonly camera:()=>THREE.Camera,private readonly surface:HTMLElement,private readonly changed:(d?:SpatialAnalysis)=>void,private readonly invalidate:()=>void){
  this.root.name='section-edit';this.gizmo=new AssemblyManipulator(shaders,{visualChanged:invalidate,pivotChanged:()=>{},snapPivot:()=>undefined,dragStarted:()=>{this.gesture=true;this.emit('start')},poseChanged:()=>{if(!this.base)return;const pose=this.gizmo.candidatePose();this.value={...this.base,frame:{translation:pose.position.toArray(),rotation:pose.rotation.multiply(new THREE.Quaternion().fromArray(this.base.frame.rotation)).normalize().toArray()}};this.gizmo.setPreviewPose(pose.position,new THREE.Quaternion().fromArray(this.value.frame.rotation));this.render();this.emit('change')},dragFinished:commit=>{if(!commit&&this.base)this.value=clone(this.base);this.gesture=false;this.attach();this.render();this.emit(commit?'finish':'cancel')}});
  this.gizmo.setPivotEnabled(false);this.root.add(this.gizmo.root);this.root.visible=false;
 }
 setSession(s?:SectionEditSession){
  const previous=this.session,identity=(v?:SectionEditSession)=>v?`${v.documentId}/${v.revisionId}/${v.definition.id}`:'';
  if(identity(previous)!==identity(s)){this.silent=true;this.cancel();this.silent=false;}
  this.session=s;
  if(!s){this.value=undefined;this.directHovered=false;this.root.visible=false;this.gizmo.detach();disposeSectionObjects(this.representation);this.representation=undefined;this.changed();this.invalidate();return}
  if(this.isDragging()){if(!s.enabled)this.cancel();else return}
  this.value=clone(s.definition);this.root.visible=true;this.attach();this.render();
 }
 definition(){return this.value?clone(this.value):undefined}
 isDragging(){return this.gesture||!!this.direct}
 updateScale(metrics:ViewportMetrics){this.gizmo.updateScale(this.camera(),metrics)}
 cancelGesture():boolean{if(!this.isDragging())return false;this.cancel();return true}
 cancel(){
  if(this.direct){const d=this.direct;this.direct=undefined;this.value=clone(d.original);this.attach();this.render();this.emit('cancel')}
  this.gizmo.pointerUp(-1,false);
 }
 pointerDown(e:CadPointerEvent):InputResult{
  if(this.isDragging())return InputResult.Consumed;
  if(!this.session?.enabled||!this.value||e.button!==0||e.state.buttons.middle||e.state.buttons.right)return InputResult.Ignored;
  this.base=clone(this.value);this.updateScale(this.metrics());
  if(this.gizmo.pointerDown(e.pointerId,e.x,e.y,this.camera(),this.surface))return InputResult.Capture;
  const hit=this.directHit(e.x,e.y);if(!hit)return InputResult.Ignored;
  const uv=e.state.modifiers.shift,screen=this.screen(hit.point),anchor=new THREE.Vector3().fromArray(this.value.frame.translation);
  this.direct={id:e.pointerId,original:clone(this.value),base:clone(this.value),hit,parameter:0,offset:new THREE.Vector2(e.x,e.y).sub(screen),uv,uvAnchor:this.onPlane(e.x,e.y,anchor),anchor};this.emit('start');return InputResult.Capture;
 }
 pointerMove(e:CadPointerEvent):InputResult{
  if(!this.session?.enabled)return InputResult.Ignored;
  const drag=this.direct;
  if(!drag){const gizmo=this.gizmo.pointerMove(e.pointerId,e.x,e.y,this.camera(),this.surface),hit=!gizmo&&!!this.directHit(e.x,e.y);this.setDirectHover(hit);return gizmo||hit?InputResult.Consumed:InputResult.Ignored;}
  if(e.pointerId!==drag.id)return InputResult.Consumed;
  const uv=!Number.isFinite(drag.hit.axis)&&(e.state.buttons.middle||e.state.modifiers.shift);
  if(uv!==drag.uv){drag.uv=uv;drag.base=clone(this.value!);drag.anchor.fromArray(drag.base.frame.translation);drag.uvAnchor=this.onPlane(e.x,e.y,drag.anchor);drag.parameter=0;drag.hit.point=this.onPlane(e.x,e.y,drag.anchor)??drag.anchor.clone();drag.offset.set(e.x,e.y).sub(this.screen(drag.hit.point));}
  if(uv&&drag.uvAnchor){const point=this.onPlane(e.x,e.y,drag.anchor);if(point){this.value=clone(drag.base);this.value.frame.translation=drag.anchor.clone().add(point.sub(drag.uvAnchor)).toArray()}}
  else{
   const direction=axis(drag.hit.axis??2).applyQuaternion(new THREE.Quaternion().fromArray(drag.base.frame.rotation));
   drag.parameter=solveScreenConstrainedParameter(new THREE.Vector2(e.x,e.y).sub(drag.offset),drag.parameter,t=>this.screen(drag.hit.point.clone().addScaledVector(direction,t)),worldUnitsPerCssPixel(this.camera(),drag.hit.point,this.metrics()));
   if(drag.hit.axis!==undefined)this.value=resizeSection(drag.base,drag.hit.axis,drag.hit.side!,drag.parameter);
   else{this.value=clone(drag.base);this.value.frame.translation=new THREE.Vector3().fromArray(drag.base.frame.translation).addScaledVector(direction,drag.parameter).toArray()}
  }
  this.attach();this.render();this.emit('change');return InputResult.Consumed;
 }
 pointerUp(e:CadPointerEvent):InputResult{
  if(!this.isDragging())return InputResult.Ignored;
  if(e.button!==0&&e.state.buttons.left)return InputResult.Consumed;
  this.pointerMove(e);
  if(this.direct){if(this.direct.id!==e.pointerId)return InputResult.Consumed;this.direct=undefined;this.emit('finish');return InputResult.ReleaseCapture}
  return this.gizmo.pointerUp(e.pointerId,true)?InputResult.ReleaseCapture:InputResult.Consumed;
 }
 pointerCancel(_e:CadPointerEvent){return this.cancelGesture()?InputResult.Consumed:InputResult.Ignored}
 keyDown(e:CadKeyboardEvent){return e.key==='Escape'&&this.cancelGesture()?InputResult.Consumed:InputResult.Ignored}
 dispose(){this.setSession();this.gizmo.dispose();this.root.removeFromParent()}
 private emit(phase:SectionGesturePhase){if(!this.silent&&this.value)this.session?.onChange(clone(this.value),phase)}
 private attach(){if(!this.value)return;if(!this.gizmo.isDragging())this.gizmo.attach(new THREE.Vector3().fromArray(this.value.frame.translation),new THREE.Quaternion().fromArray(this.value.frame.rotation));if(!this.session?.enabled)this.gizmo.detach()}
 private render(){
  disposeSectionObjects(this.representation);if(!this.value)return;
  this.representation=sectionRepresentation(this.value,true);this.root.add(this.representation);
  const points:THREE.Vector3[]=[];for(let i=0;i<(this.value.sectionType==='PLANE'?2:3);i++)for(const sign of [-1,1])points.push(axis(i).multiplyScalar(sign*this.value.size[i]/2));
  const handles=new THREE.Points(new THREE.BufferGeometry().setFromPoints(points),new THREE.PointsMaterial({color:theme.selected,size:7,sizeAttenuation:false,depthTest:false,depthWrite:false}));this.representation.add(handles);
  this.applyDirectHover();this.changed(this.value);this.invalidate();
 }
 private setDirectHover(value:boolean){if(value===this.directHovered)return;this.directHovered=value;this.applyDirectHover();this.invalidate()}
 private applyDirectHover(){this.representation?.traverse(o=>{if(o instanceof THREE.Mesh&&(o.material as THREE.Material).transparent)(o.material as THREE.Material).opacity=this.directHovered ? .085 : .045})}
 private metrics(){return {cssWidth:Math.max(1,this.surface.clientWidth),cssHeight:Math.max(1,this.surface.clientHeight),devicePixelRatio:1}}
 private screen(p:THREE.Vector3){const v=p.clone().project(this.camera()),m=this.metrics();return new THREE.Vector2((v.x+1)*m.cssWidth/2,(1-v.y)*m.cssHeight/2)}
 private ray(x:number,y:number){const m=this.metrics(),r=new THREE.Raycaster();r.setFromCamera(new THREE.Vector2(x/m.cssWidth*2-1,1-y/m.cssHeight*2),this.camera());return r.ray}
 private onPlane(x:number,y:number,origin:THREE.Vector3){const n=new THREE.Vector3(0,0,1).applyQuaternion(new THREE.Quaternion().fromArray(this.value!.frame.rotation));return this.ray(x,y).intersectPlane(new THREE.Plane().setFromNormalAndCoplanarPoint(n,origin),new THREE.Vector3())??undefined}
 private directHit(x:number,y:number):DirectHit|undefined{
  if(!this.value)return;const d=this.value,q=new THREE.Quaternion().fromArray(d.frame.rotation),origin=new THREE.Vector3().fromArray(d.frame.translation),pointer=new THREE.Vector2(x,y);
  const world=(v:THREE.Vector3)=>v.applyQuaternion(q).add(origin),near=(a:THREE.Vector3,b:THREE.Vector3)=>{const from=this.screen(world(a)),to=this.screen(world(b)),delta=to.clone().sub(from),t=Math.max(0,Math.min(1,pointer.clone().sub(from).dot(delta)/Math.max(1e-12,delta.lengthSq())));return pointer.distanceTo(from.addScaledVector(delta,t))};
  for(let i=0;i<2;i++)for(const side of [-1,1]){const center=axis(i).multiplyScalar(side*d.size[i]/2),span=axis(1-i).multiplyScalar(d.size[1-i]/2);if(near(center.clone().sub(span),center.clone().add(span))<8)return {axis:i,side,point:world(center)}}
  if(d.sectionType!=='PLANE')for(const side of [-1,1]){const p=world(new THREE.Vector3(0,0,side*d.size[2]/2));if(pointer.distanceTo(this.screen(p))<9)return {axis:2,side,point:p}}
  const point=this.onPlane(x,y,origin);if(!point)return;const local=point.clone().sub(origin).applyQuaternion(q.clone().invert());if(Math.abs(local.x)<=d.size[0]/2&&Math.abs(local.y)<=d.size[1]/2)return {point};
 }
}
