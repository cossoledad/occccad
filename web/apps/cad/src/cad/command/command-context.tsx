import {CadIconContext} from "../overlay/cad-icons";
import {builtinCatalog,type WorkbenchCatalog} from "./workbench-catalog";
import { useCommandShortcuts } from "./use-command-shortcuts";
import { createContext, useContext, useSyncExternalStore, type PropsWithChildren } from "react";
import { CommandRegistry, type CadCommandState } from "./command-registry";

const CatalogContext=createContext<WorkbenchCatalog>(builtinCatalog);
export function useWorkbenchCatalog():WorkbenchCatalog{return useContext(CatalogContext);}

const Context = createContext<CommandRegistry | null>(null);

export function CommandProvider({ registry, children,catalog }: PropsWithChildren<{ registry: CommandRegistry;catalog?:WorkbenchCatalog }>) {
  useCommandShortcuts(registry);
  return <Context.Provider value={registry}><CatalogContext.Provider value={catalog??builtinCatalog}><CadIconContext.Provider value={catalog?.icons??builtinCatalog.icons}>{children}</CadIconContext.Provider></CatalogContext.Provider></Context.Provider>;
}

export function useOptionalCommandRegistry():CommandRegistry|null{return useContext(Context);}

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

