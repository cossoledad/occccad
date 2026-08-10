import { forwardRef, useEffect, useImperativeHandle, useRef } from "react";
import type { DocumentView, PlaneName, RectangleDraft, Selection, Vec3 } from "../types";
import { CadViewportEngine } from "./cad-viewport-engine";

export type CadViewportHandle = {
  fit: () => void;
  setStandardView: (view: "TOP" | "FRONT" | "RIGHT" | "ISO") => void;
};

type Props = {
  view: DocumentView;
  selection: Selection;
  sketchPlane?: PlaneName;
  sketchTool: "SELECT" | "RECTANGLE";
  onSelectionChange: (selection: Selection) => void;
  onRectangleCreated: (rectangle: RectangleDraft) => void;
  onInstanceMoved: (instanceID: string, translation: Vec3) => void;
};

export const CadViewport = forwardRef<CadViewportHandle, Props>(function CadViewport(props, ref) {
  const host = useRef<HTMLDivElement>(null);
  const engine = useRef<CadViewportEngine | undefined>(undefined);
  const callbacks = useRef(props);
  callbacks.current = props;

  useEffect(() => {
    if (!host.current) return;
    const instance = new CadViewportEngine(host.current, {
      selectionChanged: (selection) => callbacks.current.onSelectionChange(selection),
      rectangleCreated: (rectangle) => callbacks.current.onRectangleCreated(rectangle),
      instanceMoved: (instanceID, translation) => callbacks.current.onInstanceMoved(instanceID, translation),
    });
    engine.current = instance;
    return () => { instance.dispose(); engine.current = undefined; };
  }, []);

  useEffect(() => { engine.current?.render(props.view); }, [props.view]);
  useEffect(() => { engine.current?.select(props.selection, false); }, [props.selection]);
  useEffect(() => {
    if (props.sketchPlane) engine.current?.beginSketch(props.sketchPlane);
    else engine.current?.endSketch();
  }, [props.sketchPlane]);
  useEffect(() => { engine.current?.setSketchTool(props.sketchTool); }, [props.sketchTool]);

  useImperativeHandle(ref, () => ({
    fit: () => engine.current?.fit(),
    setStandardView: (view) => engine.current?.setStandardView(view),
  }), []);

  return <div ref={host} className="cad-viewport-canvas" />;
});
