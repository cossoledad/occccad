import { createContext, useContext, useMemo, useState, useSyncExternalStore, type ReactNode, type SetStateAction } from "react";
import { DocumentSessions, type DocumentStateSlot } from "./document-session";

const Context = createContext<DocumentSessions | undefined>(undefined);
export function DocumentSessionProvider({ children }: { children: ReactNode }) {
  const sessions = useMemo(() => new DocumentSessions(), []);
  return <Context.Provider value={sessions}>{children}</Context.Provider>;
}
export function useDocumentSessions(): DocumentSessions {
  const sessions = useContext(Context);
  if (!sessions) throw new Error("Missing document session provider");
  return sessions;
}

/** Safe view state restores synchronously; domain edit contexts validate separately. */
export function useDocumentState<T>(sessions: DocumentSessions | undefined, documentId: string | undefined,
  slot: DocumentStateSlot<T>, initial: () => T): [T, (next: SetStateAction<T>) => void] {
  const fallback = useMemo(initial, [documentId]);
  const [local, setLocal] = useState(fallback);
  const snapshot = () => sessions?.read(documentId ?? "", slot) ?? fallback;
  const value = useSyncExternalStore(sessions?.subscribe ?? (() => () => {}), snapshot, snapshot);
  return [sessions && documentId ? value : local, next => {
    if (!sessions || !documentId) { setLocal(next); return; }
    const current = snapshot();
    sessions.write(documentId, slot, typeof next === "function" ? (next as (value: T) => T)(current) : next);
  }];
}
