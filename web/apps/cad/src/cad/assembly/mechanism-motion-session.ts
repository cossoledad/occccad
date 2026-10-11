import { quantityIn, type MechanismSessionBegin, type MechanismSessionFrame, type MechanismSessionOpened, type MechanismSessionUpdate, type MotionKeyframe, type MotionRun, type MotionStudy, type MotionViewpoint } from './motion-study';
export type MechanismMotionIntent = { timeSeconds: number; viewpoint?: MotionViewpoint };
export type MechanismMotionPorts = {
    begin: (input: MechanismSessionBegin, signal: AbortSignal) => Promise<MechanismSessionOpened>;
    update: (id: string, input: MechanismSessionUpdate, signal: AbortSignal) => Promise<MechanismSessionFrame>;
    record: (id: string, sequence: number, key: MotionKeyframe, signal: AbortSignal) => Promise<MotionKeyframe>;
    finalize: (id: string, sequence: number, study: MotionStudy | undefined, signal: AbortSignal) => Promise<Record<string, unknown>>;
    preview: (id: string, study: MotionStudy, signal: AbortSignal) => Promise<MotionRun>;
    cancel: (id: string) => Promise<void>;
    onIntent?: () => MechanismMotionIntent | undefined;
    onFrame: (frame: MechanismSessionFrame, recordable: boolean, intent?: MechanismMotionIntent) => void | Promise<void>;
    onBusy: (busy: boolean) => void;
};
type Input = Omit<MechanismSessionUpdate, 'sequence'>;
type Pending = {
    input: Input;
    recordable: boolean;
    intent?: MechanismMotionIntent;
    resolve: (f: MechanismSessionFrame | undefined) => void;
    reject: (e: unknown) => void;
};
// One owner covers numerical input, viewport gestures and recording. Only the
// server's complete qualified checkpoint is ever published to its consumers.
export class MechanismMotionSession {
    private abort = new AbortController();
    private opened?: MechanismSessionOpened;
    private latest?: MechanismSessionFrame;
    private sequence = 0;
    private pending?: Pending;
    private inFlight?: Promise<void>;
    private disposed = false;
    private finalizing = false;
    private exclusive = false;
    private failedInput?: Input;
    private sideEffects = new Set<Promise<unknown>>();
    constructor(private readonly ports: MechanismMotionPorts) { }
    get checkpoint() { return this.latest; }
    get identity() { return this.opened; }
    get active() { return Boolean(this.opened && !this.disposed && !this.finalizing && !this.exclusive); }
    async begin(input: MechanismSessionBegin) {
        this.ports.onBusy(true);
        try {
            const opened = await this.ports.begin(input, this.abort.signal);
            if (this.disposed) {
                void this.ports.cancel(opened.sessionId);
                return;
            }
            this.opened = opened;
            this.latest = opened;
            await this.ports.onFrame(opened, false);
            return opened;
        }
        finally {
            if (!this.disposed)
                this.ports.onBusy(false);
        }
    }
    request(input: Input, recordable = true): Promise<MechanismSessionFrame | undefined> {
        if (!this.active)
            return Promise.reject(new Error('机构试动会话已结束'));
        // InputNumber can emit its formatted value again on blur. That is already
        // qualified, and must not disable the button receiving that very click.
        if (input.value && !this.inFlight && !this.pending && !this.sideEffects.size && !this.failedInput && this.latest?.accepted && Math.abs(quantityIn(input.value, 'rad') - this.latest.frame.driverValue) < 1e-9)
            return Promise.resolve(this.latest);
        const intent = recordable ? this.ports.onIntent?.() : undefined;
        return new Promise((resolve, reject) => { this.pending?.resolve(undefined); this.pending = { input, recordable, intent, resolve, reject }; this.pump(); });
    }
    private pump() {
        if (this.inFlight || this.sideEffects.size || !this.pending || this.disposed)
            return;
        const pending = this.pending;
        this.pending = undefined;
        this.ports.onBusy(true);
        this.inFlight = (async () => {
            try {
                const f = await this.ports.update(this.opened!.sessionId, { ...pending.input, sequence: ++this.sequence }, this.abort.signal);
                if (this.disposed)
                    return pending.resolve(undefined);
                if (f.sessionId !== this.opened!.sessionId || f.baseRevisionId !== this.opened!.baseRevisionId || (!pending.input.driverId || !f.accepted ? f.inputDigest !== this.opened!.inputDigest : f.activeDriver?.id !== pending.input.driverId) || f.sequence !== this.sequence || !f.frame.kinematicValid)
                    throw new Error('机构试动响应身份不一致');
                if (pending.input.driverId && f.accepted)
                    this.opened = { ...this.opened!, inputDigest: f.inputDigest, driver: f.activeDriver! };
                this.latest = f;
                this.failedInput = undefined;
                await this.ports.onFrame(f, pending.recordable, pending.intent);
                pending.resolve(f);
            }
            catch (e) {
                this.failedInput = pending.input;
                pending.reject(e);
            }
        })().finally(() => { this.inFlight = undefined; if (!this.disposed) {
            this.ports.onBusy(Boolean(this.pending));
            this.pump();
        } });
    }
    async settle() { while (this.inFlight || this.pending || this.sideEffects.size) {
        if (this.inFlight)
            await this.inFlight;
        else if (this.sideEffects.size)
            await Promise.all(this.sideEffects);
        else
            this.pump();
    } if (this.disposed)
        throw new Error('机构试动会话已结束'); }
    // Called from onFrame as well as the recorder. Do not wait for inFlight here:
    // onFrame belongs to that very request, and the next input is held until pinning finishes.
    async record(key: MotionKeyframe) {
        if (!this.opened || !this.latest || this.disposed)
            throw new Error('没有可记录的位置');
        const effect = this.ports.record(this.opened.sessionId, this.latest.sequence, key, this.abort.signal);
        this.sideEffects.add(effect);
        try {
            return await effect;
        }
        finally {
            this.sideEffects.delete(effect);
            this.pump();
        }
    }
    async preview(study: MotionStudy) { await this.settle(); this.exclusive = true; this.ports.onBusy(true); try {
        const run = await this.ports.preview(this.opened!.sessionId, study, this.abort.signal);
        if (this.disposed)
            throw new DOMException('会话已取消', 'AbortError');
        return run;
    }
    finally {
        this.exclusive = false;
        if (!this.disposed)
            this.ports.onBusy(false);
    } }
    async finalize(study?: MotionStudy) {
        await this.settle();
        if (this.failedInput) {
            await this.request(this.failedInput, false);
            await this.settle();
        }
        this.finalizing = true;
        try {
            return await this.ports.finalize(this.opened!.sessionId, this.latest!.sequence, study, this.abort.signal);
        }
        catch (e) {
            this.finalizing = false;
            throw e;
        }
    }
    async cancel() {
        if (this.disposed)
            return;
        this.disposed = true;
        this.abort.abort();
        this.pending?.resolve(undefined);
        this.pending = undefined;
        this.ports.onBusy(false);
        if (this.opened)
            await this.ports.cancel(this.opened.sessionId);
    }
}
