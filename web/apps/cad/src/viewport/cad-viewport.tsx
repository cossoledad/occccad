import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import type { CommandRegistry } from "../cad/command/command-registry";
import { InputDebugOverlay, type InputDebugSnapshot } from "../cad/overlay/input-debug-overlay";
import type { NavigationProfileID } from "../cad/navigation/navigation-profile";
import type { WorkbenchToolID } from "../state/workbench-store";
import type { DocumentView, PlaneName, RectangleDraft, Selection, Vec3 } from "../types";
import { CadViewportEngine } from "./cad-viewport-engine";

export type CadViewportHandle = {
  fit: () => void;
  setStandardView: (view: "TOP" | "FRONT" | "RIGHT" | "ISO") => void;
};

type Props = {
  view: DocumentView;
  selection: Selection;
  preselection: Selection;
  sketchPlane?: PlaneName;
  activeToolID: WorkbenchToolID;
  navigationProfile: NavigationProfileID;
  commandRegistry: CommandRegistry;
  onSelectionChange: (selection: Selection) => void;
  onPreselectionChange: (selection: Selection) => void;
  onRectangleCreated: (rectangle: RectangleDraft) => void;
  onInstanceMoved: (instanceID: string, translation: Vec3) => void;
};

export const CadViewport = forwardRef<CadViewportHandle, Props>(function CadViewport(props, ref) {
  const host = useRef<HTMLDivElement>(null);
  const engine = useRef<CadViewportEngine | undefined>(undefined);
  const callbacks = useRef(props);
  const [debug, setDebug] = useState<InputDebugSnapshot>();
  callbacks.current = props;

  useEffect(() => {
    if (!host.current) return;
    const instance = new CadViewportEngine(host.current, {
      selectionChanged: (selection) => callbacks.current.onSelectionChange(selection),
      preselectionChanged: (selection) => callbacks.current.onPreselectionChange(selection),
      rectangleCreated: (rectangle) => callbacks.current.onRectangleCreated(rectangle),
      instanceMoved: (instanceID, translation) => callbacks.current.onInstanceMoved(instanceID, translation),
      debugStateChanged: import.meta.env.DEV && import.meta.env.VITE_INPUT_DEBUG === "true" ? setDebug : undefined,
    }, props.commandRegistry);
    engine.current = instance;
    instance.render(callbacks.current.view);
    instance.select(callbacks.current.selection, false);
    instance.preselect(callbacks.current.preselection, false);
    if (callbacks.current.sketchPlane) instance.beginSketch(callbacks.current.sketchPlane);
    instance.setActiveTool(callbacks.current.activeToolID);
    instance.setNavigationProfile(callbacks.current.navigationProfile);
    return () => { instance.dispose(); engine.current = undefined; };
  }, [props.commandRegistry, CadViewportEngine]);

  useEffect(() => { engine.current?.render(props.view); }, [props.view]);
  useEffect(() => { engine.current?.select(props.selection, false); }, [props.selection]);
  useEffect(() => { engine.current?.preselect(props.preselection, false); }, [props.preselection]);
  useEffect(() => {
    if (props.sketchPlane) engine.current?.beginSketch(props.sketchPlane);
    else engine.current?.endSketch();
  }, [props.sketchPlane]);
  useEffect(() => { engine.current?.setActiveTool(props.activeToolID); }, [props.activeToolID]);
  useEffect(() => { engine.current?.setNavigationProfile(props.navigationProfile); }, [props.navigationProfile]);

  useImperativeHandle(ref, () => ({
    fit: () => engine.current?.fit(),
    setStandardView: (view) => engine.current?.setStandardView(view),
  }), []);

  return <><div ref={host} className="cad-viewport-canvas" />{debug && <InputDebugOverlay snapshot={debug} />}</>;
});
