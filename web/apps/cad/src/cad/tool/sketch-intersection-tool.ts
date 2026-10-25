import type {CadTool,ToolContext} from './cad-tool';
import {InputResult,SelectionInputResult,type CadPointerEvent,type CadKeyboardEvent} from '../input/input-types';
import type {SketchOperation,SelectionItem,SketchGeometryRef} from '../../types';
import type {SketchCommandAction} from './sketch-command-session';
import {SketchDimensionCommitSession} from '../sketch/sketch-dimension-editor';
import {localSketchSelection} from './sketch-edit-tool';
import {engineeringProblem} from '../command/operation-feedback';
import {randomUUID} from '../../utils/random-uuid';
export class SketchIntersectionTool implements CadTool{
 readonly id:string;private selected:string[]=[];private mode='BOTH';private first?:[number,number];private error?:string;private pointer?:number;private busy=false;private generation=0;private opID=randomUUID();private session=new SketchDimensionCommitSession();private pending?:SketchOperation[];
 constructor(private split=false){this.id=split?'sketch.edit.split_intersections':'sketch.intersection'}
 private refs():SketchGeometryRef[]{return this.selected.map(entityId=>({target:'ENTITY',entityId,subElement:'WHOLE'}))}
 private publish(c:ToolContext){c.viewport.setToolPrompt("");c.viewport.setSketchCommandState?.({toolId:this.id,operation:this.split?'交点分割':'创建交点',phase:this.busy?'committing':this.session.unknown?'unknown':this.error?'failed':'selection',presentation:'inline',role:this.selected.length<2?'选择两条曲线':this.first?'选择第二个交点':'选择交点',selectedIds:this.selected,references:this.refs(),fields:[],options:this.split?[{name:'curves',label:'分割对象',value:this.mode,choices:[{value:'BOTH',label:'两条曲线'},{value:'FIRST',label:'第一条曲线'}]}]:[],recovery:this.error&&!this.busy?{label:'重试',action:'retry'}:undefined,canConfirm:false,next:'选择交点',error:this.error});}
 activate(c:ToolContext){this.publish(c)}
 selectionInput(selections:readonly SelectionItem[],c:ToolContext){if(this.busy)return SelectionInputResult.Rejected;const entities=localSketchSelection(selections,c);if(entities.some(e=>e.kind==='POINT')||entities.length>2)return SelectionInputResult.Rejected;this.selected=[...new Set(entities.map(e=>e.id))];this.first=undefined;this.publish(c);return SelectionInputResult.Accepted}
 commandAction(a:SketchCommandAction,c:ToolContext){if(a.type==='cancel'){this.cancel(c);c.viewport.finishToolUse(true);return}if(this.busy)return;if(a.type==='option'&&a.name==='curves'){this.mode=a.value;this.first=undefined;this.publish(c)}if(a.type==='back'){this.first=undefined;this.selected.pop();this.error=undefined;this.publish(c)}if(a.type==='retry'){if(this.pending)void this.commit(c);else {c.viewport.retrySketchIntersections?.();this.error=undefined;this.publish(c)}}}
 pointerDown(e:CadPointerEvent,c:ToolContext){if(e.button!==0||this.busy||!c.viewport.hasActiveSketch())return InputResult.Ignored;this.pointer=e.pointerId;
  if(this.session.unknown)return InputResult.Consumed;
  if(this.selected.length<2){const r=c.viewport.sketchReferenceAt(e.x,e.y,'CURVE',undefined,r=>r.target==='ENTITY'&&!this.selected.includes(r.entityId!));if(r?.entityId){this.selected.push(r.entityId);this.error=undefined;c.viewport.showReferencePreview(r,this.refs());this.publish(c)}return InputResult.Capture}
  const hit=c.viewport.sketchIntersectionAt?.(e.x,e.y,this.selected);if(!hit){this.error='此位置尚无可用的精确交点';this.publish(c);return InputResult.Capture}
  // The cache uses canonical curve order; command roles keep the user's order.
  const params=this.selected.map(id=>hit.parameters[hit.references.findIndex(r=>r.entityId===id)]) as [number,number];
  const sources=c.viewport.currentSketchEntities?.()??[];const closed=this.selected.some((id,i)=>!(this.mode==='FIRST'&&i===1)&&sources.some(s=>s.id===id&&(s.kind==='CIRCLE'||s.kind==='ELLIPSE'||s.closed)));
  if(this.split&&closed&&!this.first){this.first=params;c.viewport.showPointPreview(hit.point);this.error=undefined;this.publish(c);return InputResult.Capture}
  if(!this.split&&!hit.constraintSupported){this.error='此交点的支撑曲线尚不支持点在对象上约束';this.publish(c);return InputResult.Capture}
  this.pending=[{type:this.split?'SPLIT_INTERSECTION':'CREATE_INTERSECTION',operationId:this.opID,entityIds:[...this.selected],parameters:this.first?[...this.first,...params]:params,mode:this.mode}];void this.commit(c);return InputResult.Capture;
 }
 private async commit(c:ToolContext){if(!this.pending||this.busy)return;const scope=c.viewport.currentSketchIdentity?.();if(!scope)return;const generation=++this.generation;this.busy=true;this.error=undefined;this.publish(c);try{await this.session.submit(this.pending,scope.versionId,(ops,intent)=>Promise.resolve(c.viewport.commitSketchOperations(ops,intent)));if(generation!==this.generation)return;this.busy=false;this.cancel(c);c.viewport.finishToolUse()}catch(err){if(generation!==this.generation)return;this.busy=false;this.error=engineeringProblem(err);this.publish(c)}}
 pointerMove(e:CadPointerEvent,c:ToolContext){if(this.busy||e.state.buttons.middle||e.state.buttons.right)return InputResult.Ignored;if(this.selected.length<2){const r=c.viewport.sketchReferenceAt(e.x,e.y,'CURVE',undefined,r=>r.target==='ENTITY'&&!this.selected.includes(r.entityId!));if(r)c.viewport.showReferencePreview(r,this.refs());else c.viewport.clearReferenceHover?.()}else{c.viewport.clearToolPreview();const hit=c.viewport.sketchIntersectionAt?.(e.x,e.y,this.selected);if(hit)c.viewport.showPointPreview(hit.point)}return InputResult.Consumed}
 pointerUp(e:CadPointerEvent){if(e.button!==0||e.pointerId!==this.pointer)return InputResult.Ignored;this.pointer=undefined;return InputResult.ReleaseCapture}
 pointerCancel(_e:CadPointerEvent,c:ToolContext){this.cancel(c);return InputResult.Consumed}
 keyDown(e:CadKeyboardEvent,c:ToolContext){if(e.editableTarget||e.isComposing)return InputResult.Ignored;if(e.key==='Escape'){this.cancel(c);return InputResult.Ignored}if(e.key==='Backspace'){this.commandAction({type:'back'},c);return InputResult.Consumed}return InputResult.Ignored}
 cancel(c:ToolContext){this.generation++;this.busy=false;this.pointer=undefined;this.first=undefined;this.pending=undefined;this.session=new SketchDimensionCommitSession();this.selected=[];this.error=undefined;this.opID=randomUUID();c.viewport.clearReferencePreview();c.viewport.clearToolPreview();this.publish(c)}
 deactivate(c:ToolContext){this.cancel(c);c.viewport.setSketchCommandState?.(undefined)}
}
