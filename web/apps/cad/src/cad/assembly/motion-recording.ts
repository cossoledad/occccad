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
    if (initial?.id === key.id && (key.timeSeconds !== 0 || Math.abs(quantityIn(initial.value, 'rad') - quantityIn(key.value, 'rad')) > 1e-9))
        throw new Error('起点位置不能修改');
    return [...keys.filter(k => k.id !== key.id), key].sort((a, b) => a.timeSeconds - b.timeSeconds);
}
export function motionTargetAt(study: MotionStudy, time: number): number {
    const keys = study.keyframes ?? [];
    if (!keys.length)
        throw new Error('模拟没有关键帧');
    if (time <= keys[0].timeSeconds)
        return quantityIn(keys[0].value, 'rad');
    const index = keys.findIndex(k => k.timeSeconds >= time);
    if (index < 0)
        return quantityIn(keys.at(-1)!.value, 'rad');
    const a = keys[index - 1], b = keys[index], u = (time - a.timeSeconds) / (b.timeSeconds - a.timeSeconds);
    return quantityIn(a.value, 'rad') * (1 - u) + quantityIn(b.value, 'rad') * u;
}
export function sameMotionKeyValue(a: MotionKeyframe, b: MotionKeyframe): boolean {
    return Math.abs(quantityIn(a.value, 'rad') - quantityIn(b.value, 'rad')) < 1e-9 && JSON.stringify(a.viewpoint) === JSON.stringify(b.viewpoint);
}
