import type { InstancePath, Vec3 } from "../../types";

export type AssemblyRotation = [number, number, number, number];
export type AssemblyPose = { translation: Vec3; rotation: AssemblyRotation };
export type AssemblyInteractionPose = AssemblyPose & { instanceId: string };
export type AssemblyDragTarget = {
  bodyId: string;
  localGrabPoint: Vec3;
  targetPose: AssemblyPose;
  frameRotation: AssemblyRotation;
  translationComponents: [boolean, boolean, boolean];
  rotationComponents: [boolean, boolean, boolean];
  targetSequence: number;
};
export type AssemblyInteractionBegin = {
  baseRevisionId: string;
  instanceId: string;
  occurrencePath?: InstancePath;
  localGrabPoint: Vec3;
  frameRotation: AssemblyRotation;
  editContext?: { rootDocumentId: string; rootRevisionId: string;activeDocumentId:string;activeRevisionId:string; instancePath?: InstancePath };
};
export type AssemblyInteractionSession = {
  sessionId: string;
  bodyId: string;
  baseRevisionId: string;
  inputDigest: string;
  nominalPoses: AssemblyInteractionPose[];
};
export type AssemblyInteractionEvidence = {
  status: "REACHED" | "CONSTRAINED" | "BUDGET" | "CANCELLED" | "FAILED";
  hardFeasible: boolean;
  targetConverged: boolean;
  eligibleForCommit: boolean;
  targetError: number;
  targetOptimality: number;
  targetSequence: number;
  diagnostic?: string;
};
export type AssemblyInteractionUpdate = { sessionId: string; sequence: number; final: boolean; target: AssemblyDragTarget };
export type AssemblyInteractionFrame = {
  previewId: string;
  requestId: string;
  sessionId: string;
  inputDigest: string;
  sequence: number;
  unchanged?: boolean;
  commitCommand?: Record<string, unknown>;
  instancePoses: AssemblyInteractionPose[];
  interaction: AssemblyInteractionEvidence;
};
export type AssemblyInteractionCommit = AssemblyInteractionUpdate & AssemblyInteractionFrame & {
  baseRevisionId: string;
  inputDigest: string;
};
export type AssemblyInteractionState = "idle" | "waiting" | "allowed" | "constrained" | "blocked" | "invalidated" | "failed" | "committing" | "committed";
export function assemblyInteractionFailureState(error:unknown):"invalidated"|"failed"{
  const message=error instanceof Error?error.message:String(error);
  return ["ASSEMBLY_SESSION_BASE_CHANGED","ASSEMBLY_SESSION_EDIT_CONTEXT_CHANGED","ASSEMBLY_SESSION_EXPIRED_OR_SCOPE_MISMATCH","ASSEMBLY_SESSION_CANCELLED","ASSEMBLY_SESSION_POLICY_OR_TARGET_MISMATCH","ASSEMBLY_SESSION_FINAL_CANDIDATE_STALE_OR_MISMATCHED"].some(code=>message.includes(code))?"invalidated":"failed";
}
export type AssemblyInteractionPort = {
  begin(input: AssemblyInteractionBegin, signal: AbortSignal): Promise<AssemblyInteractionSession>;
  update(input: AssemblyInteractionUpdate, signal: AbortSignal): Promise<AssemblyInteractionFrame>;
  cancel(sessionId: string): Promise<unknown>;
  frame(value: AssemblyInteractionFrame): void;
  state(value: AssemblyInteractionState, reason?: string): void;
};

/** A transient solver Session, never a TREE-03 document EditSession.
 * One in-flight request finishes normally while pointer movement only replaces
 * the pending target. Completed feasible frames remain displayable even when a
 * newer target is pending; dropping all such frames would create starvation.
 * Final confirmation is a separately identified final=true solve, not whichever
 * earlier frame happened to finish last. */
export class AssemblyInteractionController {
  private epoch = 0;
  private abort?: AbortController;
  private session?: AssemblyInteractionSession;
  private pending?: AssemblyInteractionUpdate;
  private latest?: AssemblyDragTarget;
  private sequence = 0;
  private inFlight = false;
  private finalRequested = false;
  private finishResolve?: (candidate?: AssemblyInteractionCommit) => void;
  private finishPromise?: Promise<AssemblyInteractionCommit | undefined>;
  private context?: AssemblyInteractionBegin;
  private phase: AssemblyInteractionState = "idle";
  private phaseReason?:string;

  private readonly port: AssemblyInteractionPort;
  constructor(port: AssemblyInteractionPort) { this.port = port; }
  get state(): AssemblyInteractionState { return this.phase; }
  get reason():string|undefined{return this.phaseReason;}
  get nominalPoses(): readonly AssemblyInteractionPose[] { return this.session?.nominalPoses ?? []; }
  get sessionId(): string | undefined { return this.session?.sessionId; }

  begin(input: AssemblyInteractionBegin): void {
    this.cancel("new gesture", "idle");
    const epoch = this.epoch;
    const abort = this.abort = new AbortController();
    this.context = structuredClone(input);
    this.sequence = 0;
    this.finalRequested = false;
    this.finishPromise = undefined;
    this.setState("waiting");
    void this.port.begin(structuredClone(input), abort.signal).then(session => {
      if (epoch !== this.epoch || abort.signal.aborted) {
        void this.port.cancel(session.sessionId).catch(() => undefined);
        return;
      }
      if (!session.sessionId || !session.bodyId || !session.inputDigest || session.baseRevisionId !== input.baseRevisionId) throw new Error("Invalid assembly Session identity");
      this.session = structuredClone(session);
      // Nominal and target context never update from an accepted frame.
      for (const pose of this.session.nominalPoses) { Object.freeze(pose.translation); Object.freeze(pose.rotation); Object.freeze(pose); }
      Object.freeze(this.session.nominalPoses);
      if (this.pending) { this.pending.sessionId = session.sessionId; this.pending.target.bodyId = session.bodyId; }
      this.drain();
    }).catch(error => {
      if (epoch === this.epoch && !abort.signal.aborted) this.cancel(String(error),assemblyInteractionFailureState(error));
    });
  }

  target(value: Omit<AssemblyDragTarget, "bodyId" | "targetSequence">): void {
    if (!this.abort || this.abort.signal.aborted || this.finalRequested) return;
    const target: AssemblyDragTarget = { ...structuredClone(value), bodyId: this.session?.bodyId ?? "", targetSequence: ++this.sequence };
    this.latest = target;
    this.pending = { sessionId: this.session?.sessionId ?? "", sequence: target.targetSequence, final: false, target };
    this.drain();
  }

  finish(): Promise<AssemblyInteractionCommit | undefined> {
    if (this.finishPromise) return this.finishPromise;
    if (!this.latest || !this.abort || this.abort.signal.aborted) { this.cancel("gesture had no target", "idle"); return Promise.resolve(undefined); }
    this.finalRequested = true;
    this.finishPromise = new Promise(resolve => { this.finishResolve = resolve; });
    const target = { ...structuredClone(this.latest), targetSequence: ++this.sequence };
    this.pending = { sessionId: this.session?.sessionId ?? "", sequence: target.targetSequence, final: true, target };
    this.setState("waiting", "waiting for final authoritative frame");
    this.drain();
    return this.finishPromise;
  }

  /** Called before the normal command path, so its own Head notification is
   * distinguished from an external revision invalidating an uncommitted drag. */
  committing(): void { this.setState("committing"); }
  committed(): void {
    const sessionId=this.session?.sessionId;
    this.setState("committed");this.clear(false);
    // No-change has no Domain Command to consume its Session; successful
    // commands may already consume it. Closing is transport-idempotent cleanup.
    if(sessionId)void this.port.cancel(sessionId).catch(()=>undefined);
  }
  invalidate(baseRevisionId: string): void {
    if (this.context && baseRevisionId !== this.context.baseRevisionId && this.phase !== "committing" && this.phase !== "committed") this.cancel("assembly input changed", "invalidated");
  }
  cancel(reason = "cancelled", state: AssemblyInteractionState = "idle"): void {
    const sessionId = this.session?.sessionId;
    this.clear(true);
    this.setState(state, reason);
    if (sessionId) void this.port.cancel(sessionId).catch(() => undefined);
  }
  private clear(abort: boolean): void {
    this.epoch++;
    if (abort) this.abort?.abort();
    this.abort = undefined;
    this.session = undefined;
    this.context = undefined;
    this.pending = undefined;
    this.latest = undefined;
    this.inFlight = false;
    this.finalRequested = false;
    this.finishResolve?.(undefined);
    this.finishResolve = undefined;
  }
  private setState(state: AssemblyInteractionState, reason?: string): void { this.phase = state;this.phaseReason=reason; this.port.state(state, reason); }
  private drain(): void {
    if (this.inFlight || !this.pending || !this.session || !this.abort) return;
    const request = this.pending;
    this.pending = undefined;
    request.sessionId = this.session.sessionId;
    request.target.bodyId = this.session.bodyId;
    const epoch = this.epoch, abort = this.abort, session = this.session;
    this.inFlight = true;
    void this.port.update(request, abort.signal).then(frame => {
      if (epoch !== this.epoch || abort.signal.aborted) return;
      if (frame.interaction.targetSequence !== request.sequence || frame.sequence !== request.sequence || frame.sessionId !== session.sessionId || frame.inputDigest !== session.inputDigest) throw new Error("Assembly target response identity mismatch");
      const evidence = frame.interaction;
      if (evidence.hardFeasible) this.port.frame(frame);
      const eligible = evidence.hardFeasible && evidence.targetConverged && evidence.eligibleForCommit && (Boolean(frame.previewId && frame.requestId && frame.commitCommand) || Boolean(frame.unchanged))
        && (evidence.status === "REACHED" || evidence.status === "CONSTRAINED");
      const axes=["X","Y","Z"],translation=request.target.translationComponents.flatMap((v,i)=>v?[axes[i]]:[]),rotation=request.target.rotationComponents.flatMap((v,i)=>v?[axes[i]]:[]);
      const reason=`${evidence.diagnostic??evidence.status} · 冻结坐标架平移 [${translation.join(",")}] / 旋转 [${rotation.join(",")}] · 目标残差 ${evidence.targetError.toPrecision(4)} / 最优性 ${evidence.targetOptimality.toPrecision(4)}${evidence.status==="CONSTRAINED"?"（指定目标未到达，已确认约束受限最优；不是冲突证明）":""}`;
      this.setState(evidence.status === "REACHED" && evidence.hardFeasible && evidence.targetConverged ? "allowed" : evidence.status === "CONSTRAINED" && evidence.hardFeasible && evidence.targetConverged ? "constrained" : evidence.status === "FAILED" || evidence.status === "CANCELLED" ? "failed" : "blocked",reason);
      if (request.final) {
        const candidate = eligible ? { ...structuredClone(frame), ...structuredClone(request), baseRevisionId: session.baseRevisionId, inputDigest: session.inputDigest } : undefined;
        this.finishResolve?.(candidate);
        this.finishResolve = undefined;
      }
    }).catch(error => {
      if (epoch === this.epoch && !abort.signal.aborted) this.cancel(String(error),assemblyInteractionFailureState(error));
    }).finally(() => {
      if (epoch !== this.epoch) return;
      this.inFlight = false;
      this.drain();
    });
  }
}
