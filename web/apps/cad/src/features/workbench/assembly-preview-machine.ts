import { assign, createActor, setup } from "xstate";

export type AssemblyPreviewState = "idle" | "pending" | "succeeded" | "failed";

type PreviewContext = {
  sequence: number;
  error?: string;
  errorCode?: string;
  phase?: string;
  retryable: boolean;
};

type PreviewEvent =
  | { type: "REQUEST"; sequence: number }
  | { type: "RESOLVE"; sequence: number }
  | { type: "REJECT"; sequence: number; error: string; errorCode?: string; phase?: string; retryable?: boolean }
  | { type: "CANCEL"; sequence: number }
  | { type: "RESET" };

export const assemblyPreviewMachine = setup({
  types: {} as { context: PreviewContext; events: PreviewEvent },
  guards: {
    isCurrent: ({ context, event }) => "sequence" in event && event.sequence === context.sequence,
  },
  actions: {
    begin: assign(({ event }) => {
      if (event.type !== "REQUEST") return {};
      return { sequence: event.sequence, error: undefined, errorCode: undefined, phase: undefined, retryable: false };
    }),
    fail: assign(({ event }) => event.type === "REJECT" ? {
      error: event.error, errorCode: event.errorCode, phase: event.phase,
      retryable: Boolean(event.retryable),
    } : {}),
    clear: assign({ error: undefined, errorCode: undefined, phase: undefined, retryable: false }),
  },
}).createMachine({
  id: "assemblyConstraintPreview",
  initial: "idle",
  context: { sequence: 0, retryable: false },
  states: {
    idle: { on: { REQUEST: { target: "pending", actions: "begin" } } },
    pending: { on: {
      REQUEST: { target: "pending", reenter: true, actions: "begin" },
      RESOLVE: { target: "succeeded", guard: "isCurrent" },
      REJECT: { target: "failed", guard: "isCurrent", actions: "fail" },
      CANCEL: { target: "idle", guard: "isCurrent", actions: "clear" },
      RESET: { target: "idle", actions: "clear" },
    } },
    succeeded: { on: {
      REQUEST: { target: "pending", actions: "begin" },
      RESET: { target: "idle", actions: "clear" },
    } },
    failed: { on: {
      REQUEST: { target: "pending", actions: "begin" },
      RESET: { target: "idle", actions: "clear" },
    } },
  },
});

export function createAssemblyPreviewActor() {
  return createActor(assemblyPreviewMachine);
}
