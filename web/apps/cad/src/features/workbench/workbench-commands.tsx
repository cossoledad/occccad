import {type WorkbenchCatalog} from "../../cad/command/workbench-catalog";
import { approximateToolbarTextWidth, layoutToolbar, toolbarMetrics, type ToolbarLayoutInput } from "./toolbar-layout";
import {CadSplitToolButton} from "../../cad/overlay/cad-split-tool-button";
import {toolbarVariantGroups,toolbarVariantDefault,rememberToolbarVariant,type ToolbarVariantGroup} from "./toolbar-variants";
import { COMMAND_SHORTCUTS, commandShortcutLabel } from "../../cad/command/command-shortcuts";
import { MoreOutlined, RightOutlined, SearchOutlined } from "@ant-design/icons";
import { Button, Dropdown, Empty, Input, Modal, Tabs, Tooltip } from "antd";
import { Fragment, useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import { useUIHelp } from "../../cad/help/ui-help-context";
import { useCommandRegistry,useCommandState } from "../../cad/command/command-context";
import { CadIcon, type CadIconName } from "../../cad/overlay/cad-icons";
import { ToolButton } from "../../cad/overlay/tool-button";
import { type CadWorkbenchID } from "../../cad/command/workbench-catalog";
import type { ToolbarCatalogEntry } from "../../types";
import { commandSection, searchCommands, type CommandSection } from "./workbench-command-model";
import {useOperationFeedback} from "../../cad/command/operation-feedback";
import { registerDocumentState, type DocumentSessions } from "../../cad/document/document-session";
import { useDocumentState } from "../../cad/document/document-session-context";

const toolbarState = registerDocumentState<Record<string, CommandSection>>("workbench.toolbar-tabs");

/** The configured tab is the sole application/category selection for a document. */
export function useWorkbenchTab(documentSessions:DocumentSessions|undefined,hostDocumentId:string|undefined,workbench:CadWorkbenchID,tabs:WorkbenchCatalog["tabs"]){
 const [sections,setSections]=useDocumentState<Record<string,CommandSection>>(documentSessions,hostDocumentId,toolbarState,()=>({}));
 const tab=tabs.find(value=>value.id===sections[workbench])??tabs[0];
 return [tab,(id:string)=>setSections(previous=>({...previous,[workbench]:id}))] as const;
}

export function WorkbenchCommands({ documentSessions, hostDocumentId, toolbars, workbench, catalog,tabs:contextTabs,onTabChange }: {
  documentSessions?: DocumentSessions; hostDocumentId?: string;
  toolbars: ToolbarCatalogEntry[]; workbench: CadWorkbenchID; catalog?:WorkbenchCatalog;tabs:WorkbenchCatalog["tabs"];onTabChange?:(tab:WorkbenchCatalog["tabs"][number])=>void;
}) {
  const registry = useCommandRegistry();
  const uiHelp = useUIHelp();
  const declaration=(id:string)=>registry.declaration(id)??catalog?.commands.find(command=>command.id===id);
  const execute=(id:string)=>{void registry.execute(id).catch(error=>feedback(error,"命令"));};
  useSyncExternalStore(registry.subscribe, registry.getSnapshot, registry.getSnapshot);
  const feedback=useOperationFeedback();
  const tabs=[...contextTabs].sort((a,b)=>a.order-b.order);
  const [activeTab,setTab]=useWorkbenchTab(documentSessions,hostDocumentId,workbench,tabs);
  const tabID=activeTab?.id;
  const [searchOpen, setSearchOpen] = useState(false);
  const focusToolAfterSearch=useRef(false);
  const [shortcutHelpOpen, setShortcutHelpOpen] = useState(false);
  useLayoutEffect(() => {
    const removeSearch = registry.bindUI("ui.command-search", () => { setQuery(""); setSearchOpen(true); });
    const removeHelp = registry.bindUI("ui.shortcut-help", () => setShortcutHelpOpen(true));
    return () => { removeSearch(); removeHelp(); };
  }, [registry]);
  const [query, setQuery] = useState("");
  const results = searchCommands(toolbars, query, (id) => registry.state(id));
  const groups = toolbars.filter((toolbar) => toolbar.tabIds?.includes(tabID??""))
    .filter((toolbar) => toolbar.items.some((item) => registry.state(item.commandId).visible));
  const ribbon=useRef<HTMLDivElement>(null);
  const [availableWidth,setAvailableWidth]=useState(0);
  const [,refreshLayout]=useState(0);
  const [measureLabel,setMeasureLabel]=useState<((label:string)=>number)>(()=>approximateToolbarTextWidth);
  const layoutGroups=groups.map(toolbar=>({id:toolbar.id,name:toolbar.name,variants:toolbarVariantGroups(toolbar.items.filter(item=>registry.state(item.commandId).visible)).map(family=>({...family,
    displayCommandId:family.items.find(item=>registry.state(item.commandId).active)?.commandId??toolbarVariantDefault(family).commandId}))}));
  const layout=layoutToolbar(layoutGroups,availableWidth,measureLabel);
  useLayoutEffect(()=>{
    const node=ribbon.current;if(!node)return;
    const resize=()=>setAvailableWidth(node.clientWidth);
    const observer=new ResizeObserver(resize);observer.observe(node);resize();
    const context=document.createElement("canvas").getContext("2d");
    let alive=true;
    const fonts=()=>{if(!alive||!context)return;context.font=`400 12px ${getComputedStyle(node).fontFamily}`;setMeasureLabel(()=>(label:string)=>context.measureText(label).width);};
    fonts();void document.fonts?.ready.then(fonts);document.fonts?.addEventListener("loadingdone",fonts);
    return ()=>{alive=false;observer.disconnect();document.fonts?.removeEventListener("loadingdone",fonts);};
  },[]);
  return <header className="workbench-command-deck" data-toolbar-rows={layout.rows}>
    <div className="workbench-command-tabs">
      <span className={`workbench-mode mode-${workbench.toLowerCase()}`}><i />{activeTab?.modeLabel??workbench}</span>
      <Tabs className="workbench-section-tabs" aria-label="命令分类" type="line" size="small" activeKey={tabID} onChange={id=>{const tab=tabs.find(t=>t.id===id);if(tab)onTabChange?.(tab);setTab(id)}} items={tabs.map(tab=>({label:tab.name,key:tab.id}))} />
      <div className="workbench-quick-actions">{toolbars.flatMap((toolbar) => toolbar.items
        .filter((item) => item.commandId === "edit.undo" || item.commandId === "edit.redo")
        .map((item) => <ToolButton key={`${toolbar.id}:${toolbar.tabIds?.join(",")}:${item.commandId}`} command={item.commandId} icon={<CadIcon name={item.iconKey as CadIconName} />}
          tooltip={item.name} toolbarName={toolbar.name} helpText={item.helpText} />))}</div>
      <Tooltip title={<>快捷键<kbd>{commandShortcutLabel("ui.shortcut-help")}</kbd></>} open={shortcutHelpOpen?false:undefined} placement="bottom" mouseEnterDelay={.45} mouseLeaveDelay={.1} arrow={false} classNames={{root:"cad-tool-tooltip"}}>
        <Button className="workbench-header-action" type="text" size="small" aria-label={declaration("ui.shortcut-help")?.name} icon={<CadIcon name={declaration("ui.shortcut-help")?.iconKey??""} />} onClick={() => execute("ui.shortcut-help")} />
      </Tooltip>
      <Tooltip title={<>搜索工具<kbd>{commandShortcutLabel("ui.command-search")}</kbd></>} open={searchOpen?false:undefined} placement="bottom" mouseEnterDelay={.45} mouseLeaveDelay={.1} arrow={false} classNames={{root:"cad-tool-tooltip"}}>
        <Button aria-label="搜索工具" aria-keyshortcuts="Control+k Meta+k" className="workbench-command-search" icon={<CadIcon name={declaration("ui.command-search")?.iconKey??""} />} onClick={() => execute("ui.command-search")}>{declaration("ui.command-search")?.name}</Button>
      </Tooltip>
    </div>
    <div className="workbench-ribbon">
     <div ref={ribbon} className="workbench-toolbar-scroll" role="toolbar" aria-label="工作台命令"><div className="workbench-command-groups">
      {layout.groups.map(toolbar=><section key={toolbar.id} className="workbench-command-group" aria-label={toolbar.label} style={{width:toolbar.width}}>
        <div className="workbench-command-items" style={{gridTemplateRows:`repeat(2, ${toolbarMetrics.rowHeight}px)`,gridTemplateColumns:toolbar.columns.map(width=>`${width}px`).join(" ")}}>
          {toolbar.controls.map(control=><div className={`workbench-tool-slot${control.iconOnly?" icon-only":""}`} key={control.key} style={{width:control.width}}>
            <ToolbarVariantButton group={{...control.family,items:control.items}} toolbarName={control.toolbarName} onDefaultChange={()=>refreshLayout(value=>value+1)}/>
          </div>)}
        </div>
        <span className="workbench-command-group-name">{toolbar.name}</span>
      </section>)}
      {layout.overflowGroups.length>0&&<ToolbarOverflow groups={layout.overflowGroups} onDefaultChange={()=>refreshLayout(value=>value+1)}/>}
      {groups.length===0&&<span className="workbench-command-empty">当前上下文没有可用命令</span>}
     </div></div>
    </div>
    <Modal width={640} title={declaration("ui.command-search")?.name} open={searchOpen} onCancel={() => setSearchOpen(false)} footer={null} destroyOnHidden
      focusable={{focusTriggerAfterClose:false}} afterOpenChange={(open) => { if (open) document.getElementById("workbench-tool-search")?.focus(); else if(focusToolAfterSearch.current){focusToolAfterSearch.current=false;if(!document.querySelector('[role="dialog"]')&&!document.activeElement?.closest('[data-quantity-input="true"]'))document.querySelector<HTMLCanvasElement>('canvas')?.focus({preventScroll:true});} }}>
      <Input id="workbench-tool-search" aria-label="搜索工具名称或用途" prefix={<SearchOutlined />} allowClear
        placeholder="输入工具名称或用途…" value={query} onChange={(event) => setQuery(event.target.value)} />
      <div className="workbench-command-results">
        {results.map((item) => <button key={item.commandId} className="workbench-command-result"
          disabled={!item.state.enabled && !uiHelp.active} title={item.helpText} onClick={() => {
            focusToolAfterSearch.current=!uiHelp.active&&declaration(item.commandId)?.implementation==="tool";
            setSearchOpen(false);
            if (uiHelp.active) { uiHelp.explain({ toolbarName: item.toolbarName, commandName: item.name, helpText: item.helpText }); return; }
            void registry.execute(item.commandId, { continuous: false }).catch((error) => feedback(error,"命令"));
          }}>
          <CadIcon name={item.iconKey as CadIconName} /><span><strong>{item.name}</strong><small>{item.toolbarName}{uiHelp.active && ` · ${item.helpText}`}</small></span>
          {commandShortcutLabel(item.commandId) && <kbd>{commandShortcutLabel(item.commandId)}</kbd>}
          <small>{uiHelp.active ? "帮助" : item.state.enabled ? "" : "当前不可用"}</small>
        </button>)}
        {!results.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的工具" />}
      </div>
    </Modal>
    <Modal title={declaration("ui.shortcut-help")?.name} open={shortcutHelpOpen} onCancel={() => setShortcutHelpOpen(false)} footer={null}>
      <div className="shortcut-list">{COMMAND_SHORTCUTS.map((shortcut) => <Fragment key={shortcut.display}>
        <span>{declaration(shortcut.command)?.name??shortcut.command}</span><kbd>{shortcut.display}</kbd></Fragment>)}</div>
    </Modal>
  </header>;
}

function ToolbarOverflow({groups,onDefaultChange}:{groups:ToolbarLayoutInput[];onDefaultChange:()=>void}){
 const registry=useCommandRegistry(),help=useUIHelp(),feedback=useOperationFeedback();
 const [open,setOpen]=useState(false),trigger=useRef<HTMLButtonElement>(null),popup=useRef<HTMLDivElement>(null);
 const entries=groups.flatMap(group=>group.variants.flatMap(family=>family.items.map(item=>({key:`${group.id}:${item.commandId}`,group,family,item}))));
 useEffect(()=>{
  if(!open)return;
  // Canvas pointer handlers stop bubbling, so dismiss on the capture phase.
  const dismiss=(event:PointerEvent)=>{if(event.target instanceof Element&&!trigger.current?.contains(event.target)&&!popup.current?.contains(event.target)&&!event.target.closest(".workbench-overflow-submenu"))setOpen(false);};
  document.addEventListener("pointerdown",dismiss,true);return()=>document.removeEventListener("pointerdown",dismiss,true);
 },[open]);
 return <Dropdown open={open} onOpenChange={setOpen} trigger={["click"]} placement="bottomRight" autoAdjustOverflow classNames={{root:"cad-command-menu"}}
  popupRender={menu=><div ref={popup} onPointerDown={event=>event.stopPropagation()} onWheel={event=>event.stopPropagation()} onKeyDown={event=>{if(event.key==="Escape"&&!event.nativeEvent.isComposing){event.preventDefault();event.stopPropagation();setOpen(false);trigger.current?.focus();}}}>{menu}</div>}
  menu={{expandIcon:<RightOutlined aria-hidden/>,items:groups.map(group=>({key:group.id,label:group.name,popupClassName:"workbench-overflow-submenu",children:entries.filter(entry=>entry.group.id===group.id).map(({key,item})=>({key,label:item.name,icon:<CadIcon name={item.iconKey as CadIconName}/>,disabled:!help.active&&!registry.state(item.commandId).enabled}))})),onClick:({key})=>{
   const entry=entries.find(entry=>entry.key===key);if(!entry)return;setOpen(false);
   if(help.active){help.explain({toolbarName:entry.group.name,commandName:entry.item.name,helpText:entry.item.helpText});return;}
   if(!registry.state(entry.item.commandId).enabled)return;
   if(entry.family.items.length>1){rememberToolbarVariant(entry.family,entry.item.commandId);onDefaultChange();}
   void registry.execute(entry.item.commandId,{continuous:false}).catch(error=>feedback(error,"命令"));
  }}}>
  <Button ref={trigger} className="workbench-toolbar-more" type="text" icon={<MoreOutlined/>} aria-label="更多工具" aria-haspopup="menu" aria-expanded={open}
   onKeyDown={event=>{if(event.key==="Enter"||event.key===" "||event.altKey&&event.key==="ArrowDown"){event.preventDefault();event.stopPropagation();setOpen(true);}else if(event.key==="Escape"&&open){event.preventDefault();event.stopPropagation();setOpen(false);}}}>更多</Button>
 </Dropdown>;
}

function ToolbarVariantButton({group,toolbarName,onDefaultChange}:{group:ToolbarVariantGroup;toolbarName:string;onDefaultChange:()=>void}){
 const [,refresh]=useState(0),item=toolbarVariantDefault(group);
 if(group.items.length===1)return <ToolButton command={item.commandId} repeatable={item.repeatable} showLabel icon={<CadIcon name={item.iconKey as CadIconName}/>} tooltip={item.name} toolbarName={toolbarName} helpText={item.helpText}/>;
 return <CadSplitToolButton items={group.items} defaultId={item.commandId} familyName={group.label} toolbarName={toolbarName}
  onDefaultChange={id=>{rememberToolbarVariant(group,id);refresh(n=>n+1);onDefaultChange();}}/>;
}

export function WorkbenchViewControls({ toolbars }: { toolbars: ToolbarCatalogEntry[] }) {
  const sketching=useCommandState("sketch.finish").visible;
  const registry=useCommandRegistry();
  return <div className="workbench-view-controls" role="toolbar" aria-label="视图快捷操作">
    {toolbars.filter((toolbar) => commandSection(toolbar) === "view").flatMap((toolbar) => toolbar.items.map((item) =>
      <ToolButton key={`${toolbar.id}:${toolbar.tabIds?.join(",")}:${item.commandId}`} command={item.commandId} icon={<CadIcon name={item.iconKey as CadIconName} />}
        tooltip={sketching&&item.commandId==="view.normal"?registry.declaration(item.commandId)?.labels?.sketch??item.name:item.name} toolbarName={toolbar.name} helpText={sketching&&item.commandId==="view.normal"?registry.declaration(item.commandId)?.labels?.sketchHelp??item.helpText:item.helpText} />))}
  </div>;
}
