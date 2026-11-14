import {SettingOutlined} from "@ant-design/icons";
import {Button} from "antd";
import {createContext,lazy,Suspense,useCallback,useContext,useMemo,useRef,useState,type PropsWithChildren} from "react";
import {CommandRegistry} from "../../cad/command/command-registry";
import type {SceneMetrics} from "../../cad/rendering/scene-settings";
import {useUIPreferences} from "../../state/ui-preferences";
import {APP_OPTIONS_COMMAND,globalOptionsCommand} from "./global-options-command";
import {globalOptionsSnapshot,type GlobalOptionsSnapshot} from "./global-options-model";
const GlobalOptionsDialog=lazy(()=>import("./global-options-dialog").then(module=>({default:module.GlobalOptionsDialog})));
type OptionsContext={draft?:GlobalOptionsSnapshot;open:()=>void;registry:CommandRegistry;setMetrics:(metrics:SceneMetrics|undefined)=>void};
const Context=createContext<OptionsContext|undefined>(undefined);
export const useGlobalOptions=()=>useContext(Context);

export function GlobalOptionsProvider({children,activeDocumentID}:PropsWithChildren<{activeDocumentID?:string}>) {
 const [draft,setDraft]=useState<GlobalOptionsSnapshot>();
 const [metrics,setMetrics]=useState<SceneMetrics>();
 const draftRef=useRef<GlobalOptionsSnapshot|undefined>(undefined);
 const open=useCallback(()=>{const value=globalOptionsSnapshot(useUIPreferences.getState());draftRef.current=value;setDraft(value)},[]);
 const close=()=>{draftRef.current=undefined;setDraft(undefined)};
 const registry=useMemo(()=>{const r=new CommandRegistry();r.register(globalOptionsCommand(open));return r},[open]);
 const patch=(change:Partial<GlobalOptionsSnapshot>)=>{
  const current=draftRef.current;if(!current)return;const next={...current,...change};draftRef.current=next;setDraft(next);
  const preferences=useUIPreferences.getState();
  // Incomplete HEX input stays in the open form; valid settings apply immediately.
  preferences.applyGlobalOptions(/^#[0-9a-fA-F]{6}$/.test(next.creationDefaults.solid.color)?next:{...next,creationDefaults:{solid:{...next.creationDefaults.solid,color:preferences.creationDefaults.solid.color}}});
 };
 const context=useMemo(()=>({draft,open,registry,setMetrics}),[draft,open,registry]);
 return <Context.Provider value={context}>{children}{draft&&<Suspense fallback={null}><GlobalOptionsDialog draft={draft} patch={patch} close={close} activeDocumentID={activeDocumentID} metrics={metrics}/></Suspense>}</Context.Provider>;
}
export function GlobalOptionsButton(){const options=useGlobalOptions();return <Button ghost aria-label="选项" title="全局选项" icon={<SettingOutlined/>} onClick={()=>{void options?.registry.execute(APP_OPTIONS_COMMAND)}}/>}
