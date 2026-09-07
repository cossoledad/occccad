import type { AssemblyComponentDof } from "../../types";
import { assign, createActor, setup } from "xstate";

export type AssemblyPreviewState = "idle" | "drafting" | "pending" | "succeeded" | "committing" | "committed" | "cancelled" | "failed";

type PreviewContext = {
  sequence: number;
  components?: AssemblyComponentDof[];
  error?: string;
  errorCode?: string;
  phase?: string;
  retryable: boolean;
};

type PreviewEvent =
	| { type: "START" }
	| { type: "CHANGE" }
  | { type: "REQUEST"; sequence: number }
  | { type: "RESOLVE"; sequence: number; components?: AssemblyComponentDof[] }
  | { type: "REJECT"; sequence: number; error: string; errorCode?: string; phase?: string; retryable?: boolean }
  | { type: "CANCEL"; sequence: number }
	| { type: "CONFIRM" }
	| { type: "COMMIT_SUCCESS" }
	| { type: "COMMIT_FAILURE"; error: string; errorCode?: string; phase?: string; retryable?: boolean }
  | { type: "RESET" };

export const assemblyPreviewMachine = setup({
  types: {} as { context: PreviewContext; events: PreviewEvent },
  guards: {
    isCurrent: ({ context, event }) => "sequence" in event && event.sequence === context.sequence,
  },
  actions: {
    begin: assign(({ event }) => {
      if (event.type !== "REQUEST") return {};
      return { components: undefined, sequence: event.sequence, error: undefined, errorCode: undefined, phase: undefined, retryable: false };
    }),
	fail: assign(({ event }) => event.type === "REJECT" || event.type === "COMMIT_FAILURE" ? {
      error: event.error, errorCode: event.errorCode, phase: event.phase,
      retryable: Boolean(event.retryable),
    } : {}),
    resolve: assign(({ event }) => event.type === "RESOLVE" ? { components: event.components } : {}),
    clear: assign({ components: undefined, error: undefined, errorCode: undefined, phase: undefined, retryable: false }),
  },
}).createMachine({
  id: "assemblyConstraintPreview",
  initial: "idle",
  context: { sequence: 0, retryable: false },
  states: {
	idle: { on: { START: { target: "drafting", actions: "clear" }, REQUEST: { target: "pending", actions: "begin" } } },
	drafting: { on: {
	  REQUEST: { target: "pending", actions: "begin" },
	  CANCEL: { target: "cancelled", actions: "clear" },
	  RESET: { target: "idle", actions: "clear" },
	} },
    pending: { on: {
      REQUEST: { target: "pending", reenter: true, actions: "begin" },
      RESOLVE: { target: "succeeded", guard: "isCurrent", actions: "resolve" },
      REJECT: { target: "failed", guard: "isCurrent", actions: "fail" },
      CANCEL: { target: "idle", guard: "isCurrent", actions: "clear" },
      RESET: { target: "idle", actions: "clear" },
    } },
    succeeded: { on: {
      REQUEST: { target: "pending", actions: "begin" },
	  CHANGE: { target: "drafting", actions: "clear" },
	  CONFIRM: { target: "committing" },
	  CANCEL: { target: "cancelled", actions: "clear" },
      RESET: { target: "idle", actions: "clear" },
    } },
	committing: { on: {
	  COMMIT_SUCCESS: { target: "committed", actions: "clear" },
	  COMMIT_FAILURE: { target: "failed", actions: "fail" },
	} },
	committed: { on: { RESET: { target: "idle", actions: "clear" } } },
	cancelled: { on: { RESET: { target: "idle", actions: "clear" }, REQUEST: { target: "pending", actions: "begin" } } },
    failed: { on: {
      REQUEST: { target: "pending", actions: "begin" },
	  CHANGE: { target: "drafting", actions: "clear" },
      RESET: { target: "idle", actions: "clear" },
    } },
  },
});

export function createAssemblyPreviewActor() {
  return createActor(assemblyPreviewMachine);
}
