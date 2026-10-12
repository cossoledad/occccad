import { quantityIn, type MotionKeyframe, type MotionStudy } from './motion-study';
// Only active input advances this clock. Idle time and solver wait do not
// lengthen the animation; coalesced inputs retain their original intent time.
export class MotionRecordingClock {
    private cursor = 0;
    private previousInput?: number;
    anchor(time: number) {
        this.cursor = Math.max(this.cursor, time);
        this.previousInput = undefined;
    }
    stamp(now: number): number {
        const elapsed = this.previousInput === undefined ? .2 : Math.max(.001, Math.min(.2, (now - this.previousInput) / 1000));
        this.previousInput = now;
        this.cursor += elapsed;
        return this.cursor;
    }
}
// Times are persistent exact inputs. Display formatting never rounds them.
export function replaceMotionKey(keys: readonly MotionKeyframe[], key: MotionKeyframe): MotionKeyframe[] {
    if (!Number.isFinite(key.timeSeconds) || key.timeSeconds < 0 || key.timeSeconds > 3600)
        throw new Error('时间须在 0–3600 s 之间');
    if (keys.some(k => k.id !== key.id && Math.abs(k.timeSeconds - key.timeSeconds) < 1e-9))
        throw new Error('该时间已有关键帧');
    const initial = keys[0];
    if (initial?.id === key.id && (key.timeSeconds !== 0 || !sameMotionVector(initial.values,key.values)))
        throw new Error('起点位置不能修改');
    return [...keys.filter(k => k.id !== key.id), key].sort((a, b) => a.timeSeconds - b.timeSeconds);
}
export function motionTargetAt(study:MotionStudy,time:number):Record<string,import('./motion-study').MotionQuantity>{
 const keys=study.keyframes??[];if(!keys.length)throw new Error('模拟没有关键帧');
 const index=keys.findIndex(key=>key.timeSeconds>=time);
 const a=time<=keys[0].timeSeconds?keys[0]:index<0?keys.at(-1)!:keys[index-1],b=index<=0?a:keys[index];
 const fraction=a===b?0:(time-a.timeSeconds)/(b.timeSeconds-a.timeSeconds);
 return Object.fromEntries(study.activeDriverIds.map(id=>{const value=a.values[id],end=b.values[id];if(!value||!end)throw new Error('模拟参数不完整');const unit=value.unit==='rad'||value.unit==='deg'?'rad':'mm';return [id,{value:quantityIn(value,unit)*(1-fraction)+quantityIn(end,unit)*fraction,unit}]}));
}
export function sameMotionVector(a:Record<string,import('./motion-study').MotionQuantity>,b:Record<string,import('./motion-study').MotionQuantity>):boolean{
 return Object.keys(a).length===Object.keys(b).length&&Object.entries(a).every(([id,q])=>{const unit=q.unit==='rad'||q.unit==='deg'?'rad':'mm';return b[id]&&Math.abs(quantityIn(q,unit)-quantityIn(b[id],unit))<1e-9});
}
export function sameMotionKeyValue(a:MotionKeyframe,b:MotionKeyframe):boolean{return sameMotionVector(a.values,b.values)&&JSON.stringify(a.viewpoint)===JSON.stringify(b.viewpoint)}
