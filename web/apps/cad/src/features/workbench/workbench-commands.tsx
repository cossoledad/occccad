import { COMMAND_SHORTCUTS, commandShortcutLabel } from "../../cad/command/command-shortcuts";
import { SearchOutlined, QuestionCircleOutlined } from "@ant-design/icons";
import { App, Button, Empty, Input, Modal, Segmented } from "antd";
import { Fragment, useEffect, useState, useSyncExternalStore } from "react";
import { useUIHelp } from "../../cad/help/ui-help-context";
import { useCommandRegistry } from "../../cad/command/command-context";
import { CadIcon, type CadIconName } from "../../cad/overlay/cad-icons";
import { ToolButton } from "../../cad/overlay/tool-button";
import { CAD_WORKBENCHES, type CadWorkbenchID } from "../../cad/workbench/cad-workbench";
import type { ToolbarCatalogEntry } from "../../types";
import { commandSection, searchCommands, type CommandSection } from "./workbench-command-model";

export function WorkbenchCommands({ toolbars, workbench }: {
  toolbars: ToolbarCatalogEntry[]; workbench: CadWorkbenchID;
}) {
  const registry = useCommandRegistry();
  const uiHelp = useUIHelp();
  useSyncExternalStore(registry.subscribe, registry.getSnapshot, registry.getSnapshot);
  const { message } = App.useApp();
  const [section, setSection] = useState<CommandSection>("model");
  const [searchOpen, setSearchOpen] = useState(false);
  const [shortcutHelpOpen, setShortcutHelpOpen] = useState(false);
  useEffect(() => {
    const removeSearch = registry.register({ id: "ui.command-search", execute: () => { setQuery(""); setSearchOpen(true); } });
    const removeHelp = registry.register({ id: "ui.shortcut-help", execute: () => setShortcutHelpOpen(true) });
    return () => { removeSearch(); removeHelp(); };
  }, [registry]);
  const [query, setQuery] = useState("");
  const results = searchCommands(toolbars, query, (id) => registry.state(id));
  const groups = toolbars.filter((toolbar) => commandSection(toolbar) === section)
    .filter((toolbar) => toolbar.items.some((item) => registry.state(item.commandId).visible));
  return <header className="workbench-command-deck">
    <div className="workbench-command-tabs">
      <span className={`workbench-mode mode-${workbench.toLowerCase()}`}><i />{CAD_WORKBENCHES[workbench].label}</span>
      <Segmented aria-label="命令分类" value={section} onChange={(value) => setSection(value as CommandSection)} options={[
        { label: workbench === "SKETCHER" ? "草图" : workbench === "ASSEMBLY_DESIGN" ? "装配" : "建模", value: "model" },
        { label: "视图", value: "view" }, { label: "文档与协作", value: "document" },
      ]} />
      <div className="workbench-quick-actions">{toolbars.flatMap((toolbar) => toolbar.items
        .filter((item) => item.commandId === "edit.undo" || item.commandId === "edit.redo")
        .map((item) => <ToolButton key={item.commandId} command={item.commandId} icon={<CadIcon name={item.iconKey as CadIconName} />}
          tooltip={item.name} toolbarName={toolbar.name} helpText={item.helpText} />))}</div>
      <Button type="text" size="small" aria-label="快捷键" title="快捷键 · Shift + /" icon={<QuestionCircleOutlined />} onClick={() => setShortcutHelpOpen(true)} />
      <Button aria-keyshortcuts="Control+k Meta+k" aria-label="搜索工具" className="workbench-command-search" icon={<SearchOutlined />} onClick={() => { setQuery(""); setSearchOpen(true); }}>搜索工具</Button>
    </div>
    <div className="workbench-command-groups" aria-label="工作台命令">
      {groups.map((toolbar) => <section key={toolbar.id} className="workbench-command-group" aria-label={toolbar.name}>
        <div className="workbench-command-items">{toolbar.items.map((item) => <ToolButton key={item.commandId}
          command={item.commandId} repeatable={item.repeatable} showLabel icon={<CadIcon name={item.iconKey as CadIconName} />}
          tooltip={item.name} toolbarName={toolbar.name} helpText={item.helpText} />)}</div>
        <span className="workbench-command-group-name">{toolbar.name}</span>
      </section>)}
      {groups.length === 0 && <span className="workbench-command-empty">当前上下文没有可用命令</span>}
    </div>
    <Modal title="搜索工具" open={searchOpen} onCancel={() => setSearchOpen(false)} footer={null} destroyOnHidden
      afterOpenChange={(open) => { if (open) document.getElementById("workbench-tool-search")?.focus(); }}>
      <Input id="workbench-tool-search" aria-label="搜索工具名称或用途" prefix={<SearchOutlined />} allowClear
        placeholder="输入工具名称或用途…" value={query} onChange={(event) => setQuery(event.target.value)} />
      <div className="workbench-command-results">
        {results.map((item) => <button key={item.commandId} className="workbench-command-result"
          disabled={!item.state.enabled && !uiHelp.active} title={item.helpText} onClick={() => {
            setSearchOpen(false);
            if (uiHelp.active) { uiHelp.explain({ toolbarName: item.toolbarName, commandName: item.name, helpText: item.helpText }); return; }
            void registry.execute(item.commandId, { continuous: false }).catch((error) => message.error(String(error)));
          }}>
          <CadIcon name={item.iconKey as CadIconName} /><span><strong>{item.name}</strong><small>{item.toolbarName} · {item.helpText}</small></span>
          {commandShortcutLabel(item.commandId) && <kbd>{commandShortcutLabel(item.commandId)}</kbd>}
          <small>{uiHelp.active ? "帮助" : item.state.enabled ? "执行" : "当前不可用"}</small>
        </button>)}
        {!results.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的工具" />}
      </div>
      <p className="workbench-search-hint">仅搜索当前工作台的工具。不可用的工具需要先满足选择或编辑条件。</p>
    </Modal>
    <Modal title="快捷键" open={shortcutHelpOpen} onCancel={() => setShortcutHelpOpen(false)} footer={null}>
      <div className="shortcut-list">{COMMAND_SHORTCUTS.map((shortcut) => <Fragment key={shortcut.display}>
        <span>{shortcut.label}</span><kbd>{shortcut.display}</kbd></Fragment>)}</div>
      <p className="shortcut-note">Mac 使用 ⌘，Windows / Linux 使用 Ctrl。草图工具仅在草图编辑中可用。输入框保留文字撤销与重做；打开对话框时暂停全局快捷键。Enter 完成当前多阶段操作，Esc 取消当前工具或对话框。</p>
    </Modal>
  </header>;
}

export function WorkbenchViewControls({ toolbars }: { toolbars: ToolbarCatalogEntry[] }) {
  return <div className="workbench-view-controls" role="toolbar" aria-label="视图快捷操作">
    {toolbars.filter((toolbar) => commandSection(toolbar) === "view").flatMap((toolbar) => toolbar.items.map((item) =>
      <ToolButton key={item.commandId} command={item.commandId} icon={<CadIcon name={item.iconKey as CadIconName} />}
        tooltip={item.name} toolbarName={toolbar.name} helpText={item.helpText} />))}
  </div>;
}
