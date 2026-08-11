import { createContext, useContext, useSyncExternalStore, type PropsWithChildren } from "react";
import { CommandRegistry, type CadCommandState } from "./command-registry";

const Context = createContext<CommandRegistry | null>(null);

export function CommandProvider({ registry, children }: PropsWithChildren<{ registry: CommandRegistry }>) {
  return <Context.Provider value={registry}>{children}</Context.Provider>;
}

export function useCommandRegistry(): CommandRegistry {
  const registry = useContext(Context);
  if (!registry) throw new Error("ToolButton must be rendered inside CommandProvider");
  return registry;
}

export function useCommandState(commandID: string): CadCommandState {
  const registry = useCommandRegistry();
  useSyncExternalStore(registry.subscribe, registry.getSnapshot, registry.getSnapshot);
  return registry.state(commandID);
}

