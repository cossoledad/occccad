import * as THREE from "three";
import { CATIA_TRACKBALL_RADIUS_RATIO } from "./navigation-visuals";
import type { CadCamera } from "./camera-rig";

export type TrackballOptions = {
  radiusRatio?: number;
  minRadius?: number;
};

const DEFAULTS: Required<TrackballOptions> = {
  radiusRatio: CATIA_TRACKBALL_RADIUS_RATIO,
  minRadius: 1,
};

/** Incremental virtual trackball. The displayed sphere is a guide, not an
 * input limit: a free rotation must keep moving beyond its visible rim. */
export class VirtualTrackball {
  private previous?: THREE.Vector2;
  private mode:"trackball"|"roll"="trackball";
  private width=0;
  private height=0;
  private center?: {x:number;y:number};
  private readonly options: Required<TrackballOptions>;

  constructor(options: TrackballOptions = {}) {
    this.options = { ...DEFAULTS, ...options };
  }

  begin(x: number, y: number, width: number, height: number, center?:{x:number;y:number}): void {
    const radius=this.radius(width,height);
    this.mode=Math.hypot(x-(center?.x??width/2),y-(center?.y??height/2))>=radius?'roll':'trackball';
    this.rebase(x,y,width,height,center);
  }

  get rotationMode():"trackball"|"roll" {return this.mode;}

  /** Resize changes the coordinate baseline, not the gesture's rotation kind. */
  rebase(x:number,y:number,width:number,height:number,center?:{x:number;y:number}):void {
    this.center=center;
    this.width=width;this.height=height;
    this.previous=new THREE.Vector2(x,y);
  }

  drag(x: number, y: number, width: number, height: number, camera: CadCamera): THREE.Quaternion | undefined {
    if(!this.previous||width!==this.width||height!==this.height){
      this.rebase(x,y,width,height,this.center?{x:width/2,y:height/2}:undefined);
      return undefined;
    }
    const cx=this.center?.x??width/2,cy=this.center?.y??height/2;
    const current=new THREE.Vector2(x,y),radius=this.radius(width,height);
    let viewRotation:THREE.Quaternion;
    if(this.mode==='roll'){
      const a=new THREE.Vector2(this.previous.x-cx,cy-this.previous.y),b=new THREE.Vector2(x-cx,cy-y);
      const angle=a.lengthSq()<1e-12||b.lengthSq()<1e-12?0:Math.atan2(b.x*a.y-b.y*a.x,b.dot(a));
      viewRotation=new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0,0,1),angle);
    }else{
      const dx=x-this.previous.x,dy=y-this.previous.y;
      // Integrate the sphere's local tangent using the *unclamped* pixel delta.
      // Limit the support point to r=.8 (z>=.6): finite gain at/beyond the rim,
      // with no collapsed radial input or accidental switch to roll.
      const steps=Math.max(1,Math.min(256,Math.ceil(Math.hypot(dx,dy)/4)));
      const ux=dx/(radius*steps),uy=-dy/(radius*steps);
      const axis=new THREE.Vector3(),step=new THREE.Quaternion();viewRotation=new THREE.Quaternion();
      for(let i=0;i<steps;i++){
        let px=(this.previous.x+dx*(i+.5)/steps-cx)/radius,py=(cy-this.previous.y-dy*(i+.5)/steps)/radius;
        const distance=Math.hypot(px,py),scale=distance>.8?.8/distance:1;px*=scale;py*=scale;
        const z=Math.sqrt(1-px*px-py*py),uz=-(px*ux+py*uy)/z;
        axis.set(uy*z-uz*py,uz*px-ux*z,ux*py-uy*px);
        const angle=axis.length();if(angle<1e-14)continue;
        step.setFromAxisAngle(axis.multiplyScalar(1/angle),angle);
        viewRotation.multiply(step);
      }
    }
    const basis=camera.getWorldQuaternion(new THREE.Quaternion());
    const delta=basis.clone().multiply(viewRotation).multiply(basis.invert()).normalize();
    this.previous=current;
    return 1-Math.abs(delta.w)<1e-14?undefined:delta;
  }

  reset(): void { this.previous=undefined;this.center=undefined;this.mode='trackball'; }

  private radius(width:number,height:number):number {
    return Math.max(Math.min(width,height)*this.options.radiusRatio,this.options.minRadius);
  }
}

