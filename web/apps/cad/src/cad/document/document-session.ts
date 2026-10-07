export type DocumentStateSlot<T> = { readonly id: string; readonly stateType?: T };

const definitions = new Set<string>();
/** Register plain, cloneable UI snapshots; never register commands or scene objects. */
export function registerDocumentState<T>(id: string): DocumentStateSlot<T> {
  if (!id.trim() || definitions.has(id)) throw new Error(`Invalid or duplicate document state ID: ${id}`);
  definitions.add(id);
  return Object.freeze({ id });
}

/** Window/user scoped, transient state. Reading/writing never opens a document. */
export class DocumentSessions {
  private readonly opened = new Set<string>();
  private readonly values = new Map<string, Map<string, unknown>>();
  private readonly listeners = new Set<() => void>();

  open(documentId: string): void { if (documentId) this.opened.add(documentId); }
  read<T>(documentId: string, slot: DocumentStateSlot<T>): T | undefined {
    return this.values.get(documentId)?.get(slot.id) as T | undefined;
  }
  write<T>(documentId: string, slot: DocumentStateSlot<T>, value: T): boolean {
    if (!this.opened.has(documentId)) return false;
    const copy = structuredClone(value);
    const state = this.values.get(documentId) ?? new Map<string, unknown>();
    state.set(slot.id, copy); this.values.set(documentId, state); this.notify();
    return true;
  }
  close(documentId: string): void {
    this.opened.delete(documentId); this.values.delete(documentId); this.notify();
  }
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener); return () => { this.listeners.delete(listener); };
  };
  private notify(): void { for (const listener of this.listeners) listener(); }
}
