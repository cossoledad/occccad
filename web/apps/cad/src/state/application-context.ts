import { create } from "zustand";

// Cross-shell routing context only. This is deliberately not persisted and is
// separate from both revisioned document state and durable UI preferences.
type ApplicationContextState = {
  activeDocumentID?: string;
  setActiveDocumentID: (documentID?: string) => void;
};

export const useApplicationContext = create<ApplicationContextState>()((set) => ({
  activeDocumentID: undefined,
  setActiveDocumentID: (activeDocumentID) => set({ activeDocumentID }),
}));
