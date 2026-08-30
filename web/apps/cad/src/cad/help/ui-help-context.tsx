import { Modal } from "antd";
import { createContext, useContext, useEffect, useMemo, useState, type PropsWithChildren } from "react";

type HelpTarget = { toolbarName: string; commandName: string; helpText: string };
type UIHelpContextValue = { active: boolean; toggle(): void; explain(target: HelpTarget): void };
const UIHelpContext = createContext<UIHelpContextValue | null>(null);

export function UIHelpProvider({ children }: PropsWithChildren) {
  const [active, setActive] = useState(false);
  const [target, setTarget] = useState<HelpTarget>();
  const value = useMemo<UIHelpContextValue>(() => ({
    active,
    toggle: () => setActive((current) => !current),
    explain: (next) => { setTarget(next); setActive(false); },
  }), [active]);
  useEffect(() => {
    document.body.classList.toggle("ui-help-active", active);
    return () => document.body.classList.remove("ui-help-active");
  }, [active]);
  return <UIHelpContext.Provider value={value}>{children}
    <Modal open={Boolean(target)} title={target?.commandName} footer={null} width={420} onCancel={() => setTarget(undefined)}>
      <div className="context-help-toolbar-name">{target?.toolbarName}</div>
      <p className="context-help-text">{target?.helpText}</p>
    </Modal>
  </UIHelpContext.Provider>;
}

export function useUIHelp(): UIHelpContextValue {
  const value = useContext(UIHelpContext);
  if (!value) throw new Error("UI help must be used inside UIHelpProvider");
  return value;
}
