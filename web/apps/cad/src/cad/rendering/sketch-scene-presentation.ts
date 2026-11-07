import * as THREE from 'three';
import type {SketchColors} from './sketch-colors';

export type SketchEditScope = {featureId:string;documentId?:string;versionId?:string;occurrencePath:string};
type Style = {
  depthTest:boolean;depthWrite:boolean;transparent:boolean;opacity:number;linewidth?:number;pointSize?:number;
  color?:number;uniformColors:Record<string,number>;uniformOpacity?:number;
};

/** Presentation only: keep the same geometry, visibility, and stable pick bindings. */
export class SketchScenePresentation {
  private readonly styles = new WeakMap<THREE.Material,Style>();
  private readonly orders = new WeakMap<THREE.Object3D,number>();
  private readonly applied = new WeakMap<THREE.Material,{colors:SketchColors;inactive:boolean;interactionRevision:number}>();

  apply(object:THREE.Object3D,colors:SketchColors,inactive:boolean,convert:(color:number)=>number):void {
    if(!this.orders.has(object))this.orders.set(object,object.renderOrder);
    object.renderOrder=inactive?2:this.orders.get(object)!;
    const material=(object as THREE.Mesh).material;
    for(const m of Array.isArray(material)?material:material?[material]:[]){
      if(m instanceof THREE.MeshStandardMaterial)continue;
      const interactionRevision=m.userData.cadInteractionRevision??0,previous=this.applied.get(m);
      if(previous?.colors===colors&&previous.inactive===inactive&&previous.interactionRevision===interactionRevision)continue;
      const shader=m instanceof THREE.ShaderMaterial?m:undefined;
      const line=m as THREE.Material&{linewidth?:number;color?:THREE.Color};
      let style=this.styles.get(m);
      if(!style){
        style={depthTest:m.depthTest,depthWrite:m.depthWrite,transparent:m.transparent,opacity:m.opacity,
          linewidth:line.linewidth,pointSize:m.userData.cssPointSize,
          color:m.userData.baseColor??line.color?.getHex(),uniformColors:{},uniformOpacity:shader?.uniforms.uOpacity?.value};
        for(const key of ['uColor','uCenterColor']){const value=shader?.uniforms[key]?.value;if(value instanceof THREE.Color)style.uniformColors[key]=value.getHex();}
        m.userData.baseOpacity??=m.opacity;if(line.linewidth!==undefined)m.userData.baseLinewidth??=line.linewidth;
        this.styles.set(m,style);
      }
      const state=m.userData.cadInteractionState,emphasized=state==='selected'||state==='hover';
      const amount=inactive?(emphasized?1:.55):1;
      m.depthTest=inactive?true:style.depthTest;m.depthWrite=inactive?false:style.depthWrite;
      const transparent=inactive||style.transparent;
      if(m.transparent!==transparent){m.transparent=transparent;m.needsUpdate=true;}
      m.opacity=style.opacity*amount;
      if(style.linewidth!==undefined)line.linewidth=inactive?(emphasized?2:1.15):style.linewidth;
      if(style.pointSize!==undefined)m.userData.cssPointSize=inactive?(style.pointSize>10?6:4):style.pointSize;
      if(line.color&&style.color!==undefined)line.color.setHex(state==='selected'?colors.selected:state==='hover'?colors.hover:inactive?colors.inactive:convert(style.color));
      for(const [key,value] of Object.entries(style.uniformColors))shader!.uniforms[key].value.setHex(inactive?colors.inactive:convert(value));
      if(shader?.uniforms.uOpacity&&style.uniformOpacity!==undefined)shader.uniforms.uOpacity.value=style.uniformOpacity*amount;
      this.applied.set(m,{colors,inactive,interactionRevision});
    }
  }
}
