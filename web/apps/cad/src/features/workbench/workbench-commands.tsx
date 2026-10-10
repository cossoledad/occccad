import {type WorkbenchCatalog} from "../../cad/command/workbench-catalog";
import { toolbarPages } from "./toolbar-pages";
import {LeftOutlined, RightOutlined} from "@ant-design/icons";
import {CadSplitToolButton} from "../../cad/overlay/cad-split-tool-button";
import {toolbarVariantGroups,toolbarVariantDefault,rememberToolbarVariant,type ToolbarVariantGroup} from "./toolbar-variants";
import { COMMAND_SHORTCUTS, commandShortcutLabel } from "../../cad/command/command-shortcuts";
import { SearchOutlined } from "@ant-design/icons";
import { Button, Empty, Input, Modal, Tabs, Tooltip } from "antd";
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
  const [availableWidth,setAvailableWidth]=useState(0),[page,setPage]=useState(0);
  const pageGroups=groups.map(toolbar=>({id:toolbar.id,name:toolbar.name,variants:toolbarVariantGroups(toolbar.items.filter(item=>registry.state(item.commandId).visible))}));
  const fullPages=toolbarPages(pageGroups,availableWidth-8);
  const pages=fullPages.length>1?toolbarPages(pageGroups,Math.max(1,availableWidth-56)):fullPages;
  const currentPage=Math.min(page,pages.length-1);
  useEffect(()=>{const node=ribbon.current;if(!node)return;const measure=()=>setAvailableWidth(node.clientWidth);const observer=new ResizeObserver(measure);observer.observe(node);measure();return ()=>observer.disconnect();},[]);
  useEffect(()=>setPage(0),[workbench,tabID]);
  useEffect(()=>{if(page!==currentPage)setPage(currentPage);},[page,currentPage]);
  return <header className="workbench-command-deck">
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
    <div ref={ribbon} className="workbench-ribbon">
     {pages.length>1&&<Button className="workbench-page-control" aria-label="上一页命令" disabled={currentPage===0} icon={<LeftOutlined/>} onClick={()=>setPage(currentPage-1)}/>}
     <div className="workbench-command-page" aria-label={`工作台命令 · 第${currentPage+1}/${pages.length}页`}><div className="workbench-command-groups">
      {pages.flatMap((groups,pageIndex)=>groups.map((toolbar,index)=><section data-page-last={index===groups.length-1} key={`${toolbar.id}:${toolbar.start}`} hidden={pageIndex!==currentPage} className="workbench-command-group" aria-label={toolbar.name}>
        <div className="workbench-command-items">{toolbar.variants.map(group=><ToolbarVariantButton key={group.key} group={group} toolbarName={toolbar.name}/>)}</div>
        <span className="workbench-command-group-name">{toolbar.name}</span>
      </section>))}
      {groups.length===0&&<span className="workbench-command-empty">当前上下文没有可用命令</span>}
     </div></div>
     {pages.length>1&&<Button className="workbench-page-control" aria-label="下一页命令" disabled={currentPage===pages.length-1} onClick={()=>setPage(currentPage+1)}><RightOutlined/><span className="workbench-page-count" aria-label={`第${currentPage+1}/${pages.length}页`}>{currentPage+1}/{pages.length}</span></Button>}
    </div>
    <Modal width={640} title={declaration("ui.command-search")?.name} open={searchOpen} onCancel={() => setSearchOpen(false)} footer={null} destroyOnHidden
      afterOpenChange={(open) => { if (open) document.getElementById("workbench-tool-search")?.focus(); }}>
      <Input id="workbench-tool-search" aria-label="搜索工具名称或用途" prefix={<SearchOutlined />} allowClear
        placeholder="输入工具名称或用途…" value={query} onChange={(event) => setQuery(event.target.value)} />
      <div className="workbench-command-results">
        {results.map((item) => <button key={item.commandId} className="workbench-command-result"
          disabled={!item.state.enabled && !uiHelp.active} title={item.helpText} onClick={() => {
            setSearchOpen(false);
            if (uiHelp.active) { uiHelp.explain({ toolbarName: item.toolbarName, commandName: item.name, helpText: item.helpText }); return; }
            void registry.execute(item.commandId, { continuous: false }).catch((error) => feedback(error,"命令"));
          }}>
          <CadIcon name={item.iconKey as CadIconName} /><span><strong>{item.name}</strong><small>{item.toolbarName} · {item.helpText}</small></span>
          {commandShortcutLabel(item.commandId) && <kbd>{commandShortcutLabel(item.commandId)}</kbd>}
          <small>{uiHelp.active ? "帮助" : item.state.enabled ? "执行" : "当前不可用"}</small>
        </button>)}
        {!results.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的工具" />}
      </div>
      <p className="workbench-search-hint">仅搜索当前工作台的工具。不可用的工具需要先满足选择或编辑条件。</p>
    </Modal>
    <Modal title={declaration("ui.shortcut-help")?.name} open={shortcutHelpOpen} onCancel={() => setShortcutHelpOpen(false)} footer={null}>
      <div className="shortcut-list">{COMMAND_SHORTCUTS.map((shortcut) => <Fragment key={shortcut.display}>
        <span>{declaration(shortcut.command)?.name??shortcut.command}</span><kbd>{shortcut.display}</kbd></Fragment>)}</div>
      <p className="shortcut-note">Mac 使用 ⌘，Windows / Linux 使用 Ctrl。草图工具仅在草图编辑中可用。输入框保留文字撤销与重做；打开对话框时暂停全局快捷键。Enter 完成当前多阶段操作，Esc 取消当前工具或对话框。</p>
    </Modal>
  </header>;
}

function ToolbarVariantButton({group,toolbarName}:{group:ToolbarVariantGroup;toolbarName:string}){
 const [,refresh]=useState(0),item=toolbarVariantDefault(group);
 if(group.items.length===1)return <ToolButton command={item.commandId} repeatable={item.repeatable} showLabel icon={<CadIcon name={item.iconKey as CadIconName}/>} tooltip={item.name} toolbarName={toolbarName} helpText={item.helpText}/>;
 return <CadSplitToolButton items={group.items} defaultId={item.commandId} familyName={group.label} toolbarName={toolbarName}
  onDefaultChange={id=>{rememberToolbarVariant(group,id);refresh(n=>n+1);}}/>;
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
