import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import type { CaptureSettings } from "../cad/interaction/capture-settings";
import { InputDebugOverlay, type InputDebugSnapshot } from "../cad/overlay/input-debug-overlay";
import type { NavigationProfileID } from "../cad/navigation/navigation-profile";
import type { WorkbenchToolID } from "../state/workbench-store";
import type { Artifact, DocumentView, PlaneName, Selection, SelectionItem, SketchOperation, Vec3 } from "../types";
import { CadViewportEngine } from "./cad-viewport-engine";

export type CadViewportHandle = {
  fit: () => void;
  setStandardView: (view: "TOP" | "FRONT" | "RIGHT" | "ISO") => void;
  previewArtifact: (artifact: Artifact) => void;
  clearCommandPreview: () => void;
};

type Props = {
  view: DocumentView;
  selections: SelectionItem[];
  preselection: Selection;
  sketchPlane?: PlaneName;
  activeSketchID?: string;
  activeToolID: WorkbenchToolID;
  navigationProfile: NavigationProfileID;
  captureSettings: CaptureSettings;
  onSelectionsChange: (selections: SelectionItem[]) => void;
  onPreselectionChange: (selection: Selection) => void;
  onSketchOperations: (operations: SketchOperation[]) => void;
  onToolUseComplete: () => void;
  onActiveToolChange: (toolID: WorkbenchToolID) => void;
  onInstanceMoved: (instanceID: string, translation: Vec3) => void;
};

export const CadViewport = forwardRef<CadViewportHandle, Props>(function CadViewport(props, ref) {
  const host = useRef<HTMLDivElement>(null);
  const engine = useRef<CadViewportEngine | undefined>(undefined);
  const callbacks = useRef(props);
  const [debug, setDebug] = useState<InputDebugSnapshot>();
  const [toolPrompt, setToolPrompt] = useState("");
  callbacks.current = props;
  const activeSketch = props.view.part?.features.find((feature) => feature.id === props.activeSketchID)?.sketch;

  useEffect(() => {
    if (!host.current) return;
    const instance = new CadViewportEngine(host.current, {
      selectionsChanged: (selections) => callbacks.current.onSelectionsChange(selections),
      preselectionChanged: (selection) => callbacks.current.onPreselectionChange(selection),
      sketchOperations: (operations) => callbacks.current.onSketchOperations(operations),
      toolUseCompleted: () => callbacks.current.onToolUseComplete(),
      activeToolChanged: (toolID) => callbacks.current.onActiveToolChange(toolID),
      toolPromptChanged: setToolPrompt,
      instanceMoved: (instanceID, translation) => callbacks.current.onInstanceMoved(instanceID, translation),
      debugStateChanged: import.meta.env.DEV && import.meta.env.VITE_INPUT_DEBUG === "true" ? setDebug : undefined,
    });
    engine.current = instance;
    instance.render(callbacks.current.view);
    instance.selectMany(callbacks.current.selections, false);
    instance.preselect(callbacks.current.preselection, false);
    if (callbacks.current.sketchPlane && callbacks.current.activeSketchID) {
      instance.beginSketch(callbacks.current.activeSketchID, callbacks.current.sketchPlane);
    }
    instance.setActiveTool(callbacks.current.activeToolID);
    instance.setNavigationProfile(callbacks.current.navigationProfile);
    instance.setCaptureSettings(callbacks.current.captureSettings);
    return () => { instance.dispose(); engine.current = undefined; };
  }, [CadViewportEngine]);

  useEffect(() => {
    const instance = engine.current; if (!instance) return;
    instance.render(props.view);
    instance.selectMany(callbacks.current.selections, false);
    instance.preselect(callbacks.current.preselection, false);
  }, [props.view]);
  useEffect(() => { engine.current?.selectMany(props.selections, false); }, [props.selections]);
  useEffect(() => { engine.current?.preselect(props.preselection, false); }, [props.preselection]);
  useEffect(() => {
    if (props.sketchPlane && props.activeSketchID) engine.current?.beginSketch(props.activeSketchID, props.sketchPlane);
    else engine.current?.endSketch();
  }, [props.sketchPlane, props.activeSketchID]);
  useEffect(() => { engine.current?.setActiveTool(props.activeToolID); }, [props.activeToolID]);
  useEffect(() => { engine.current?.setNavigationProfile(props.navigationProfile); }, [props.navigationProfile]);
  useEffect(() => { engine.current?.setCaptureSettings(props.captureSettings); }, [props.captureSettings]);

  useImperativeHandle(ref, () => ({
    fit: () => engine.current?.fit(),
    setStandardView: (view) => engine.current?.setStandardView(view),
    previewArtifact: (artifact) => engine.current?.previewArtifact(artifact),
    clearCommandPreview: () => engine.current?.clearCommandPreview(),
  }), []);

  return <><div ref={host} className="cad-viewport-canvas" />
    {props.sketchPlane && <div className="sketch-tool-prompt" role="status" aria-live="polite">
      <strong>Sketcher · {props.sketchPlane}</strong><span>{toolPrompt}</span>
      {activeSketch && <small>{activeSketch.solve.status} · {activeSketch.solve.degreesOfFreedom} DoF</small>}
    </div>}
    {debug && <InputDebugOverlay snapshot={debug} />}</>;
});
