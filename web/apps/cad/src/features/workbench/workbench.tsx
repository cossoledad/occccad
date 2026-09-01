import {
  AimOutlined, ApartmentOutlined, BuildOutlined, CloudUploadOutlined, DatabaseOutlined, ExportOutlined, GatewayOutlined,
  InsertRowAboveOutlined, MenuFoldOutlined, MenuUnfoldOutlined, NodeIndexOutlined, ScissorOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  App, Button, Descriptions, Empty, Form, Input, InputNumber, List, Segmented,
  Select, Space, Spin, Switch, Tag,
} from "antd";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { api, isMockMode } from "../../api/client";
import { realtime } from "../../api/realtime-client";
import { randomUUID } from "../../utils/random-uuid";
import { queryKeys } from "../../app/query-keys";
import { ShareDialog, type ShareResource } from "../../components/share-dialog";
import { CommandProvider } from "../../cad/command/command-context";
import { CommandRegistry } from "../../cad/command/command-registry";
import { selectionKey, selectionSetToken } from "../../cad/interaction/selection-identity";
import { CommandDialog, FloatingToolbar, ToolbarGroup } from "../../cad/overlay/floating-panel";
import { ToolButton } from "../../cad/overlay/tool-button";
import { CadIcon, type CadIconName } from "../../cad/overlay/cad-icons";
import { CaptureSettingsButton } from "../../cad/overlay/capture-settings-button";
import { CAD_WORKBENCHES, resolveCadWorkbench } from "../../cad/workbench/cad-workbench";
import { useWorkbenchStore, type WorkbenchToolID } from "../../state/workbench-store";
import { useUIPreferences } from "../../state/ui-preferences";
import type { AssemblyConstraint, AssemblyGeometryRef, DatumPlane, DocumentProperties, DocumentStructureNode, DocumentView, Feature, HistoryEntry, Selection, SelectionItem, SketchOperation, SketchPlane, ToolbarCatalogEntry, ToolbarCatalogItem, TopologyElementProperties, Vec3 } from "../../types";
import type { CadViewportHandle } from "../../viewport/cad-viewport";
import { SpecificationTree, type SpecificationTreeNode } from "./specification-tree";
import { closestTreeKey } from "./tree-selection";
import { followedDocumentIDs } from "./product-edit-context";

const CadViewport = lazy(() => import("../../viewport/cad-viewport").then((module) => ({ default: module.CadViewport })));

const sketchToolCommands:WorkbenchToolID[]=["sketch.rectangle","sketch.polygon","sketch.slot","sketch.point","sketch.line","sketch.circle","sketch.arc","sketch.polyline","sketch.spline",
  "sketch.constraint.coincident","sketch.constraint.parallel","sketch.constraint.fixed","sketch.constraint.horizontal","sketch.constraint.vertical",
  "sketch.constraint.perpendicular","sketch.constraint.tangent","sketch.constraint.equal","sketch.dimension.linear",
  "sketch.constraint.radius","sketch.constraint.angle","sketch.constraint.concentric","sketch.constraint.point_on_object","sketch.constraint.midpoint","sketch.constraint.symmetry"];

function sketchPlane(datum: DatumPlane): SketchPlane {
  return { datumPlaneId: datum.id, plane: datum.plane, origin: datum.origin, normal: datum.normal, uDirection: datum.uDirection };
}

function featureSketchPlane(view: DocumentView, feature: Feature): SketchPlane | undefined {
  const datum = view.datumPlanes?.find((candidate) => candidate.id === feature.sketch?.support.datumPlaneId)
    ?? view.part?.datumPlanes.find((candidate) => candidate.id === feature.sketch?.support.datumPlaneId);
  return datum ? sketchPlane(datum) : undefined;
}

function occurrenceSketchPlane(plane: SketchPlane, translation?: Vec3, rotation: [number,number,number,number] = [0,0,0,1]): SketchPlane {
  const rotate = (value: Vec3): Vec3 => {
    const [x,y,z,w]=rotation,[vx,vy,vz]=value;const dot=x*vx+y*vy+z*vz,uu=x*x+y*y+z*z;
    const cross:[number,number,number]=[y*vz-z*vy,z*vx-x*vz,x*vy-y*vx];
    return [2*dot*x+(w*w-uu)*vx+2*w*cross[0],2*dot*y+(w*w-uu)*vy+2*w*cross[1],2*dot*z+(w*w-uu)*vz+2*w*cross[2]];
  };
  const origin=rotate(plane.origin);return {...plane,origin:origin.map((value,index)=>value+(translation?.[index]??0)) as Vec3,
    normal:rotate(plane.normal),uDirection:rotate(plane.uDirection)};
}

function toolbarGroups(items: ToolbarCatalogItem[]): Array<{ key: string; items: ToolbarCatalogItem[] }> {
  const groups = new Map<string, ToolbarCatalogItem[]>();
  for (const item of items) groups.set(item.groupKey, [...(groups.get(item.groupKey) ?? []), item]);
  return [...groups].map(([key, groupItems]) => ({ key, items: groupItems }));
}

function featureIcon(feature: Feature) {
  if (feature.type.toUpperCase().includes("SKETCH")) return <ScissorOutlined />;
  if (["PAD", "LINEAR_EXTRUDE", "REVOLVE"].includes(feature.type.toUpperCase())) return <InsertRowAboveOutlined />;
  return <CloudUploadOutlined />;
}

function isSolidFeature(feature: Feature): boolean {
  return ["PAD", "LINEAR_EXTRUDE", "REVOLVE", "IMPORT_BODY"].includes(feature.type.toUpperCase());
}

function featureNode(feature: Feature): SpecificationTreeNode {
  return {
    key: `${feature.type.toUpperCase().includes("SKETCH") ? "sketch" : isSolidFeature(feature) ? "solid" : "import"}:${feature.id}`,
    title: feature.name ?? feature.type, icon: featureIcon(feature),
  };
}

function structureIcon(kind: DocumentStructureNode["kind"]) {
  if (kind === "PRODUCT") return <ApartmentOutlined />;
  if (kind === "PART" || kind === "INSTANCE") return <BuildOutlined />;
  if (kind === "ORIGIN") return <GatewayOutlined />;
  if (kind === "PLANE") return <NodeIndexOutlined />;
  if (kind === "AXIS_SYSTEM") return <AimOutlined />;
  if (kind === "AXIS") return <NodeIndexOutlined />;
  if (kind === "BODY") return <DatabaseOutlined />;
  if (kind === "SKETCH") return <ScissorOutlined />;
  if (kind === "SKETCH_ENTITY") return <NodeIndexOutlined />;
  if (kind === "SKETCH_CONSTRAINT") return <GatewayOutlined />;
  if (kind === "SKETCH_GEOMETRY_SET" || kind === "SKETCH_CONSTRAINT_SET") return <DatabaseOutlined />;
  if (kind === "PAD" || kind === "REVOLVE") return <InsertRowAboveOutlined />;
  return <CloudUploadOutlined />;
}

function structureSelection(node: DocumentStructureNode, view: DocumentView): Selection {
  const occurrencePath = node.instancePath?.canonical ?? "";
  const resolved = (view.resolvedInstances ?? []).find((item) => item.instancePath?.canonical === occurrencePath);
  const geometryKey = resolved?.geometryKey ?? view.artifact?.geometryKey;
  const expands = ["PART", "PRODUCT", "INSTANCE", "ORIGIN", "BODY", "SKETCH", "SKETCH_GEOMETRY_SET", "SKETCH_CONSTRAINT_SET",
    "SKETCH_LOGICAL_CONSTRAINT_SET", "SKETCH_DIMENSION_SET"].includes(node.kind);
  const context = { treeNodeId: node.id, expandTreeDescendants: expands || undefined, documentId: node.documentId,
    instancePath: node.instancePath, occurrencePath, geometryKey,
    instanceId: occurrencePath.split("/")[0] || undefined };
  if (node.kind === "INSTANCE" && node.entityId) {
    const path = occurrencePath || node.entityId;
    return { kind: "instance", id: path, visualKey: `occurrence:${path}`, ...context,
      occurrencePath: path, instanceId: path.split("/")[0] };
  }
  if (node.kind === "PLANE" && node.entityId && node.plane) {
    const datumPlane = view.datumPlanes?.find((datum) => datum.id === node.entityId);
    return { kind: "plane", id: `${occurrencePath || "root"}:${node.entityId}`, entityId: node.entityId, plane: node.plane, datumPlane, ...context };
  }
  if (node.kind === "AXIS_SYSTEM" && node.entityId) return {
    kind: "axis-system", id: `${occurrencePath || "root"}:${node.entityId}`, entityId: node.entityId, ...context,
  };
  if (node.kind === "AXIS" && node.entityId && node.axis) return {
    kind: "axis", axis: node.axis, id: `${occurrencePath || "root"}:${node.entityId}:${node.axis}`, entityId: node.entityId, ...context,
  };
  if (node.kind === "DATUM_AXIS" && node.entityId) return {
    kind: "axis", axis: "DATUM", id: `${occurrencePath || "root"}:${node.entityId}`, entityId: node.entityId, ...context,
  };
  const bodyID = `${occurrencePath || "root"}:body`;
  if (node.kind === "BODY" || node.kind === "PART") return { kind: "body", id: bodyID, ...context };
  if (["SKETCH", "PAD", "REVOLVE", "IMPORT"].includes(node.kind) && node.entityId) return {
    kind: (node.kind === "REVOLVE" ? "pad" : node.kind.toLowerCase()) as "sketch" | "pad" | "import", id: node.entityId,
    visualKey: node.kind === "SKETCH" ? undefined : `body:${bodyID}`, ...context,
  };
  if (node.kind === "SKETCH_ENTITY" && node.entityId && node.ownerEntityId) return {
    kind: "visual", id: `${occurrencePath || "root"}:${node.ownerEntityId}:${node.entityId}`,
    visualType: node.entityType === "POINT" ? "POINT" : "CURVE", featureId: node.ownerEntityId,
    entityId: node.entityId, role: node.role, ...context,
  };
  if (node.kind === "SKETCH_CONSTRAINT" && node.entityId && node.ownerEntityId) return {
    kind: "sketch-constraint", id: `${occurrencePath || "root"}:${node.ownerEntityId}:constraint:${node.entityId}`,
    featureId: node.ownerEntityId, constraintId: node.entityId, constraintType: node.entityType ?? "UNKNOWN", ...context,
  };
  if (node.kind === "ASSEMBLY_CONSTRAINT" && node.entityId) return { kind: "assembly-constraint", id: node.entityId,
    constraintId: node.entityId, constraintType: node.entityType ?? "UNKNOWN", ...context };
  return { kind: "tree", id: node.id, ...context };
}

function mapStructureNode(node: DocumentStructureNode, view: DocumentView, editingView?: DocumentView): SpecificationTreeNode {
  const nodeView = node.documentId === editingView?.document.id ? editingView : node.documentId === view.document.id ? view : undefined;
  const canEdit = nodeView?.document.permission === "OWNER" || nodeView?.document.permission === "EDITOR";
  const sketch=nodeView?.part?.features.find((feature)=>feature.id===node.ownerEntityId)?.sketch;
  const conflictConstraints=new Set(sketch?.solve.conflictingConstraintIds??[]), conflictEntities=new Set<string>();
  let changed=true;while(changed){changed=false;for(const constraint of sketch?.constraints??[]){if(constraint.suppressed)continue;
    const ids=constraint.references.flatMap((reference)=>reference.entityId?[reference.entityId]:[]);
    if(conflictConstraints.has(constraint.id)||ids.some((id)=>conflictEntities.has(id))){if(!conflictConstraints.has(constraint.id)){conflictConstraints.add(constraint.id);changed=true;}
      for(const id of ids)if(!conflictEntities.has(id)){conflictEntities.add(id);changed=true;}}
  }}
	const component=node.entityId?sketch?.solve.components?.find((candidate)=>candidate.entityIds.includes(node.entityId!)):undefined;
  return { key: node.id, title: node.name, icon: structureIcon(node.kind), kind: node.kind,
    entityId: node.entityId, documentId: node.documentId, instancePath: node.instancePath,
    plane: node.plane, ownerEntityId: node.ownerEntityId,
	role: node.role, suppressed: node.suppressed, diagnostic: node.diagnostic??(node.kind==="SKETCH_ENTITY"&&node.entityId&&conflictEntities.has(node.entityId)?"CONFLICTING":component?.definitionStatus==="FULLY_CONSTRAINED"||component?.status==="SOLVED"?"FULLY_CONSTRAINED":undefined),
    capabilities: canEdit ? [...new Set([...(node.capabilities??[]), ...(["SKETCH","SKETCH_GEOMETRY_SET","SKETCH_CONSTRAINT_SET","SKETCH_LOGICAL_CONSTRAINT_SET","SKETCH_DIMENSION_SET"].includes(node.kind)?["SUPPRESS" as const]:[])])] : undefined,
    selection: structureSelection(node, view), children: node.children?.map((child) => mapStructureNode(child, view, editingView)) };
}

function treeData(view: DocumentView, editingView?: DocumentView): SpecificationTreeNode[] {
  if (view.structureTree) return [mapStructureNode(view.structureTree, view, editingView)];
  if (view.document.type === "PART") {
    const features = view.part?.features ?? [];
    const sketches = new Map(features.filter((feature) => feature.type.toUpperCase().includes("SKETCH"))
      .map((feature) => [feature.id, feature]));
    const consumedSketches = new Set(features.filter((feature) => isSolidFeature(feature) && feature.profile)
      .map((feature) => feature.profile!));
    const bodyFeatures = features.filter((feature) => !consumedSketches.has(feature.id)).map((feature) => {
      const node = featureNode(feature);
      const profile = isSolidFeature(feature) && feature.profile ? sketches.get(feature.profile) : undefined;
      return profile ? { ...node, children: [featureNode(profile)] } : node;
    });
    return [{ key: "document", title: view.document.name, icon: <BuildOutlined />, children: [
      { key: "origin", title: "Origin", icon: <GatewayOutlined />, children: (view.datumPlanes ?? []).map((plane) => ({
        key: `plane:${plane.id}:${plane.plane}`, title: plane.name, icon: <NodeIndexOutlined />,
      })).concat((view.axisSystems ?? []).map((axis) => ({
        key: `axis:${axis.id}`, title: axis.name, icon: <AimOutlined />,
      }))) },
      { key: "body", title: "PartBody", icon: <DatabaseOutlined />, children: bodyFeatures },
    ] }];
  }
  return [{ key: "document", title: view.document.name, icon: <ApartmentOutlined />, children:
    (view.product?.instances ?? []).map((instance) => ({
      key: `instance:${instance.id}`, title: instance.name, icon: <BuildOutlined />,
    })) }];
}

function selectedFeature(view: DocumentView, selection: Selection): Feature | undefined {
  if (!selection || !["sketch", "pad", "import"].includes(selection.kind)) return undefined;
  return view.part?.features.find((feature) => feature.id === selection.id);
}

function treeKeyForSelection(nodes: SpecificationTreeNode[], selection: Selection): string | undefined {
  if (!selection) return undefined;
  const visit = (items: SpecificationTreeNode[]): string | undefined => {
    for (const node of items) {
      if (selection.treeNodeId === node.key || (selection.documentId && node.documentId === selection.documentId &&
        selection.id === node.entityId)) return node.key;
      const child = node.children ? visit(node.children) : undefined;
      if (child) return child;
    }
    return undefined;
  };
  return visit(nodes) ?? closestTreeKey(nodes, selection.treeNodeId);
}

function treeKeysForSelections(nodes: SpecificationTreeNode[], selections: readonly SelectionItem[]): string[] {
  return [...new Set(selections.map((selection) => treeKeyForSelection(nodes, selection)).filter((key): key is string => Boolean(key)))];
}

export function Workbench() {
  const { documentID = "" } = useParams();
  const client = useQueryClient();
  const { message } = App.useApp();
  const commandRegistry = useMemo(() => new CommandRegistry(), []);
  const viewport = useRef<CadViewportHandle>(null);
  const [padOpen, setPadOpen] = useState(false);
  const [padGenerator, setPadGenerator] = useState<"LINEAR_EXTRUDE" | "REVOLVE">("LINEAR_EXTRUDE");
  const [padSketchID, setPadSketchID] = useState<string>();
  const [padPreviewPending, setPadPreviewPending] = useState(false);
  const padPreviewAbort = useRef<AbortController | undefined>(undefined);
  const padPreviewSequence = useRef(0);
  const padIntentRequestID = useRef<string | undefined>(undefined);
  const latestDocumentVersion = useRef<string | undefined>(undefined);
  const [activeDocumentID, setActiveDocumentID] = useState(documentID);
  const [activeInstancePath, setActiveInstancePath] = useState<string>();
  const [insertOpen, setInsertOpen] = useState(false);
  const [versionOpen, setVersionOpen] = useState(false);
  const [datumPlaneOpen, setDatumPlaneOpen] = useState(false);
  const [datumAxisOpen, setDatumAxisOpen] = useState(false);
  const [pendingAssemblyConstraint, setPendingAssemblyConstraint] = useState<{ kind: "angle" | "distance"; references: AssemblyGeometryRef[] }>();
  const [editingAssemblyConstraint, setEditingAssemblyConstraint] = useState<AssemblyConstraint>();
  const inspectorOpen = useUIPreferences((state) => state.inspectorOpen);
  const setInspectorOpen = useUIPreferences((state) => state.setInspectorOpen);
  const hiddenTreeKeys = useUIPreferences((state) => state.hiddenTreeKeys);
  const toggleTreeVisibility = useUIPreferences((state) => state.toggleTreeVisibility);
  const [shareResource, setShareResource] = useState<ShareResource>();
  const [padForm] = Form.useForm<{ generator: "LINEAR_EXTRUDE" | "REVOLVE"; operation: "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT";
    length: number; angle: number; axisEntityId?: string; reversed: boolean }>();
  const [insertForm] = Form.useForm<{ referencedDocumentID: string }>();
  const [versionForm] = Form.useForm<{ name: string; description: string }>();
  const [datumPlaneForm] = Form.useForm<{ name: string; offset: number }>();
  const [datumAxisForm] = Form.useForm<{ name: string; ox: number; oy: number; oz: number; dx: number; dy: number; dz: number }>();
  const [assemblyConstraintForm] = Form.useForm<{ value: number }>();
  const store = useWorkbenchStore();
  const document = useQuery({ queryKey: queryKeys.document(documentID), queryFn: () => api.getDocument(documentID), enabled: Boolean(documentID) });
	useEffect(() => { setActiveDocumentID(documentID); setActiveInstancePath(undefined); store.endSketch(); store.setSelection(null); }, [documentID]);
  const activeDocument = useQuery({ queryKey: queryKeys.document(activeDocumentID), queryFn: () => api.getDocument(activeDocumentID),
    enabled: Boolean(activeDocumentID && activeDocumentID !== documentID) });
	const activeID = activeDocumentID || documentID;
	const toolbarCatalog = useQuery({ queryKey: ["ui", "toolbars"], queryFn: api.toolbarCatalog, staleTime: 5 * 60_000 });
  const properties = useQuery({ queryKey: queryKeys.documentProperties(activeID), queryFn: () => api.getDocumentProperties(activeID),
    enabled: Boolean(activeID && inspectorOpen && store.inspectorTab === "properties"), staleTime: 30_000 });
  const history = useQuery({ queryKey: queryKeys.history(activeID), queryFn: () => api.getHistory(activeID),
    enabled: Boolean(activeID && inspectorOpen && store.inspectorTab === "history"), staleTime: 10_000 });
  const catalog = useQuery({ queryKey: queryKeys.documents({ workbench: true }), queryFn: () => api.listDocuments({ limit: 100, allFolders: true }) });
  const topologySelection = store.selection && ["face", "edge", "vertex"].includes(store.selection.kind)
    ? store.selection as Extract<Exclude<Selection, null>, { kind: "face" | "edge" | "vertex" }> : undefined;
  const topology = useQuery({
    queryKey: topologySelection ? queryKeys.topologyProperties(activeID, topologySelection.geometryKey ?? "",
      topologySelection.kind, topologySelection.topologyId) : ["topology-properties", "none"],
    queryFn: () => api.getTopologyProperties(activeID, topologySelection!.geometryKey!,
      topologySelection!.kind.toUpperCase() as "FACE" | "EDGE" | "VERTEX", topologySelection!.topologyId),
    enabled: Boolean(activeID && inspectorOpen && store.inspectorTab === "properties" && topologySelection?.geometryKey), staleTime: 5 * 60_000,
  });

  useEffect(() => {
    if (document.data) void client.invalidateQueries({ queryKey: queryKeys.openDocuments });
  }, [client, document.data]);
  const refresh = useCallback(async (view?: DocumentView) => {
    if (view) client.setQueryData(queryKeys.document(view.document.id), view);
    const changedID = view?.document.id ?? activeID;
    await Promise.all([view ? (changedID === documentID ? Promise.resolve() : client.invalidateQueries({ queryKey: queryKeys.document(documentID) }))
      : client.invalidateQueries({ queryKey: queryKeys.document(changedID) }),
    client.invalidateQueries({ queryKey: queryKeys.history(changedID), refetchType: "active" }),
    client.invalidateQueries({ queryKey: queryKeys.documentProperties(changedID), refetchType: "active" }),
    client.invalidateQueries({ queryKey: ["documents"] }),
    client.invalidateQueries({ queryKey: queryKeys.openDocuments })]);
  }, [activeID, client, documentID]);
  useEffect(() => {
    if (!documentID || isMockMode) return;
    let disposed = false;
    let unsubscribe: (() => void) | undefined;
    void realtime.subscribe(documentID, (event) => {
      if (event.type === "document.snapshot.v1") {
        const snapshot = event.payload as { view: DocumentView };
        client.setQueryData(queryKeys.document(documentID), snapshot.view);
      } else {
        useWorkbenchStore.getState().setSelection(null);
      }
      void Promise.all([
        event.type === "document.snapshot.v1" ? Promise.resolve() : client.invalidateQueries({ queryKey: queryKeys.document(documentID) }),
        client.invalidateQueries({ queryKey: queryKeys.history(documentID), refetchType: "active" }),
        client.invalidateQueries({ queryKey: queryKeys.documentProperties(documentID), refetchType: "active" }),
        client.invalidateQueries({ queryKey: ["documents"] }),
        client.invalidateQueries({ queryKey: queryKeys.openDocuments }),
      ]);
    }).then((dispose) => {
      if (disposed) dispose(); else unsubscribe = dispose;
    }).catch((error: Error) => {
      if (!disposed) message.error(`实时连接失败：${error.message}`);
    });
    return () => { disposed = true; unsubscribe?.(); };
  }, [client, documentID, message]);
  const command = useMutation({
    mutationFn: (operation: () => Promise<DocumentView>) => operation(),
    onSuccess: (view) => { store.setSelection(null); void refresh(view); }, onError: (error) => message.error(error.message)
  });
  const moveCommand = useMutation({mutationFn:(operation:()=>Promise<DocumentView>)=>operation(),
    onSuccess:(updated)=>{void refresh(updated);},onError:(error)=>{message.error(error.message);void refresh();}});
  const view = document.data;
  const editingView = activeDocumentID === documentID ? view : activeDocument.data;
  const activeResolvedInstance = activeInstancePath
    ? view?.resolvedInstances?.find((instance) => instance.instancePath?.canonical === activeInstancePath) : undefined;
  latestDocumentVersion.current = editingView?.document.versionId;
  const followedIDs = useMemo(() => [...new Set([
    ...followedDocumentIDs(view?.structureTree), ...(activeID !== documentID ? [activeID] : []),
  ])].filter((id) => id !== documentID), [activeID, documentID, view?.structureTree]);
  useEffect(() => {
    if (isMockMode || !view || followedIDs.length === 0) return;
    let disposed = false; const unsubscribers: Array<() => void> = [];
    for (const dependencyID of followedIDs) void realtime.subscribe(dependencyID, (event) => {
      if (event.type === "document.snapshot.v1") {
        const snapshot = event.payload as { view: DocumentView };
        client.setQueryData(queryKeys.document(dependencyID), snapshot.view);
        return;
      }
      // The Product Revision is unchanged, but its FOLLOW_HEAD projection is not.
      store.setSelection(null);
      void client.invalidateQueries({ queryKey: queryKeys.document(dependencyID) });
      void client.invalidateQueries({ queryKey: queryKeys.document(documentID) });
      void client.invalidateQueries({ queryKey: queryKeys.documentProperties(documentID), refetchType: "active" });
    }).then((unsubscribe) => { if (disposed) unsubscribe(); else unsubscribers.push(unsubscribe); })
      .catch((error: Error) => { if (!disposed) message.error(`引用文档实时连接失败：${error.message}`); });
    return () => { disposed = true; unsubscribers.forEach((unsubscribe) => unsubscribe()); };
  }, [client, documentID, followedIDs.join("|"), message, view]);
  const treeNodes = useMemo(() => {
    const decorate = (node: SpecificationTreeNode): SpecificationTreeNode => ({ ...node,
      hidden: hiddenTreeKeys.includes(node.key), children: node.children?.map(decorate) });
    return view ? treeData(view, editingView).map(decorate) : [];
  }, [view, editingView, hiddenTreeKeys]);
  const canEdit = editingView?.document.permission === "OWNER" || editingView?.document.permission === "EDITOR";
  const activeWorkbench = resolveCadWorkbench(editingView?.document.type ?? "PART", Boolean(store.sketchPlane));

  const editSketch = (featureID: string, operations: SketchOperation[]) => {
    if (!editingView) return;
    command.mutate(() => api.editSketch(editingView.document.id, featureID, operations));
  };
  const moveInstance = (instanceID: string, translation: Vec3, rotation:[number,number,number,number]) => {
    if (editingView?.document.type === "PRODUCT" && canEdit) moveCommand.mutate(() => api.move(editingView.document.id, instanceID, translation,rotation));
  };
  const executeHistory = (direction: "undo" | "redo") => {
    if (!editingView) return; command.mutate(() => direction === "undo" ? api.undo(editingView.document.id) : api.redo(editingView.document.id));
  };
  const startSketch = () => {
    if (!editingView || !store.selection) return;
    if (store.selection.kind === "sketch") {
      const feature = editingView.part?.features.find((candidate) => candidate.id === store.selection!.id);
      const localPlane = feature ? featureSketchPlane(editingView, feature) : undefined;
      const plane = localPlane ? occurrenceSketchPlane(localPlane, activeResolvedInstance?.translation, activeResolvedInstance?.rotation) : undefined;
      if (feature && plane) store.beginSketch(feature.id, plane);
      return;
    }
    if (store.selection.kind !== "plane") return;
    const datum = store.selection.datumPlane ?? editingView.datumPlanes?.find((candidate) => store.selection?.id.endsWith(candidate.id));
    if (!datum) return;
    const plane = occurrenceSketchPlane(sketchPlane(datum), activeResolvedInstance?.translation, activeResolvedInstance?.rotation);
    command.mutate(() => api.createSketch(editingView.document.id, datum.plane, datum.id), { onSuccess: (updated) => {
      const sketch = [...(updated.part?.features ?? [])].reverse().find((feature) => feature.type.toUpperCase() === "SKETCH");
      if (sketch) store.beginSketch(sketch.id, plane);
    }});
  };
  const padSketch = (values: { generator: "LINEAR_EXTRUDE" | "REVOLVE"; operation: "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT";
    length: number; angle: number; axisEntityId?: string; reversed: boolean }) => {
    if (!editingView || !padSketchID) return;
    padPreviewAbort.current?.abort();
    viewport.current?.clearCommandPreview();
    const generator = values.generator ?? padGenerator;
    command.mutate(() => api.createSolidFeature(editingView.document.id, { sketchId: padSketchID, generator,
      operation: values.operation, length: generator === "LINEAR_EXTRUDE" ? values.length : undefined,
      angle: generator === "REVOLVE" ? values.angle : undefined,
      axisEntityId: generator === "REVOLVE" ? values.axisEntityId : undefined, reversed: values.reversed }, padIntentRequestID.current));
    setPadOpen(false); setPadSketchID(undefined); padIntentRequestID.current = undefined;
  };
  const closePad = () => {
    padPreviewAbort.current?.abort(); padPreviewSequence.current += 1; setPadPreviewPending(false);
    viewport.current?.clearCommandPreview(); setPadOpen(false); setPadSketchID(undefined); padIntentRequestID.current = undefined;
  };
  const requestPadPreview = async (sketchID: string, generatorOverride?: "LINEAR_EXTRUDE" | "REVOLVE") => {
    if (!editingView) return;
    const values = padForm.getFieldsValue(); values.generator = generatorOverride ?? values.generator ?? padGenerator;
    if (values.generator === "LINEAR_EXTRUDE" && (!Number.isFinite(values.length) || values.length <= 0)) return;
    if (values.generator === "REVOLVE" && (!Number.isFinite(values.angle) || values.angle <= 0 || !values.axisEntityId)) return;
    padPreviewAbort.current?.abort();
    const abort = new AbortController(); padPreviewAbort.current = abort;
    const sequence = ++padPreviewSequence.current; const baseVersionID = editingView.document.versionId;
    setPadPreviewPending(true);
    try {
      const preview = await api.previewCommand(editingView.document.id, { type: "CREATE_SOLID_FEATURE", sketchId: sketchID,
        generator: values.generator, operation: values.operation, length: values.length, angle: values.angle,
        axisEntityId: values.axisEntityId, reversed: values.reversed,
        ...(padIntentRequestID.current ? { requestId: padIntentRequestID.current } : {}) }, abort.signal);
      if (sequence !== padPreviewSequence.current || preview.baseVersionId !== baseVersionID ||
        preview.baseVersionId !== latestDocumentVersion.current || !preview.artifact) return;
      viewport.current?.previewArtifact(preview.artifact);
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) message.error(`预览失败：${(error as Error).message}`);
    } finally {
      if (sequence === padPreviewSequence.current) setPadPreviewPending(false);
    }
  };
  const previewPad = () => {
    if (padSketchID) void requestPadPreview(padSketchID);
  };
  const openSolidFeature = (generator: "LINEAR_EXTRUDE" | "REVOLVE", operation?: "NEW_BODY" | "ADD" | "REMOVE") => {
    if (store.selection?.kind !== "sketch") return;
    const hasBody = Boolean(editingView?.part?.features.some((feature) => feature.type === "IMPORT_BODY" ||
      ["PAD", "LINEAR_EXTRUDE", "REVOLVE"].includes(feature.type.toUpperCase())));
    const selectedOperation = operation ?? (hasBody ? "ADD" : "NEW_BODY");
    const sketchID = store.selection.id;
    padIntentRequestID.current = randomUUID(); setPadSketchID(sketchID); setPadGenerator(generator);
    padForm.setFieldsValue({ generator, operation: selectedOperation, length: 40, angle: 360,
      axisEntityId: undefined, reversed: false });
    setPadOpen(true);
  };
  useEffect(() => {
    if (!padOpen || !padSketchID) return;
    void requestPadPreview(padSketchID, padGenerator);
  }, [padOpen, padSketchID, padGenerator]);
  useEffect(() => {
    if (!padOpen || padGenerator !== "REVOLVE" || !padSketchID || !store.selection) return;
    const selection = store.selection;
    let reference: string | undefined;
    if (selection.kind === "axis") {
      const parts = selection.id.split(":");
      reference = selection.axis === "DATUM"
        ? `DATUM_AXIS:${parts.at(-1)}` : `AXIS_SYSTEM:${parts.at(-2)}:${selection.axis}`;
    } else if (selection.kind === "visual") {
      const selectedSketch = editingView?.part?.features.find((feature) => feature.id === selection.featureId)?.sketch;
      const entity = selectedSketch?.entities.find((candidate) => candidate.id === selection.entityId);
      if (entity?.kind === "LINE") reference = `SKETCH_LINE:${selection.featureId}:${entity.id}`;
    }
    if (!reference) return;
    padForm.setFieldValue("axisEntityId", reference);
    void requestPadPreview(padSketchID, "REVOLVE");
  }, [padOpen, padGenerator, padSketchID, store.selection, editingView]);
  const insertDocument = (values: { referencedDocumentID: string }) => {
    if (editingView?.document.type !== "PRODUCT") return;
    command.mutate(() => api.insert(editingView.document.id, values.referencedDocumentID)); setInsertOpen(false);
  };
  const deleteTreeNodes = (nodes: SpecificationTreeNode[]) => {
    if (!editingView || !canEdit || command.isPending) return;
    const candidates = nodes.filter((node) => node.entityId && node.kind && node.capabilities?.includes("DELETE"));
    const selectedFeatures = new Set(candidates.filter((node) => !["SKETCH_ENTITY", "SKETCH_CONSTRAINT", "ASSEMBLY_CONSTRAINT", "INSTANCE"].includes(node.kind!))
      .map((node) => node.entityId));
    const featureOrder = new Map((editingView.part?.features ?? []).map((feature, index) => [feature.id, index]));
    const targets = candidates.filter((node) => !node.ownerEntityId || !selectedFeatures.has(node.ownerEntityId)).sort((left, right) => {
      const rank = (node: SpecificationTreeNode) => node.kind === "SKETCH_CONSTRAINT" ? 0 : node.kind === "SKETCH_ENTITY" ? 1 : 2;
      const difference = rank(left) - rank(right); if (difference) return difference;
      return (featureOrder.get(right.entityId!) ?? 0) - (featureOrder.get(left.entityId!) ?? 0);
    });
    if (!targets.length) return;
    command.mutate(() => api.deleteNodes(editingView.document.id, targets.map((node) => ({
      targetKind: ["SKETCH_ENTITY", "SKETCH_CONSTRAINT", "ASSEMBLY_CONSTRAINT", "INSTANCE"].includes(node.kind!) ? node.kind! : "FEATURE",
      targetId: node.entityId!, ownerEntityId: node.ownerEntityId,
    }))));
  };
  const createVersion = async (values: { name: string; description: string }) => {
    if (!editingView) return; await api.createVersion(editingView.document.id, values.name, values.description); setVersionOpen(false);
    await history.refetch(); message.success("版本已创建");
  };
  useEffect(() => {
    const selectedInstance = () => {
      const selection = useWorkbenchStore.getState().selection;
      return selection?.kind === "instance" ? editingView?.product?.instances.find((instance) => instance.id === selection.id) : undefined;
    };
    const disposers = [
      commandRegistry.register({ id: "tool.select", execute: () => store.setActiveTool("select", "once"),
        isActive: () => store.activeToolID === "select" }),
      commandRegistry.register({ id: "assembly.move", execute: () => store.setActiveTool("assembly.move", "continuous"),
        isVisible: () => editingView?.document.type === "PRODUCT", isEnabled: () => Boolean(canEdit), isActive: () => store.activeToolID === "assembly.move" }),
      commandRegistry.register({ id: "sketch.start", execute: startSketch,
        isVisible: () => editingView?.document.type === "PART", isEnabled: () => Boolean(canEdit && (store.selection?.kind === "plane" || store.selection?.kind === "sketch")) }),
      commandRegistry.register({ id: "sketch.finish", execute: store.endSketch,
        isVisible: () => Boolean(store.sketchPlane), isEnabled: () => Boolean(canEdit) }),
      ...sketchToolCommands.map((toolID)=>commandRegistry.register({id:toolID,execute:(invocation)=>store.setActiveTool(toolID,invocation?.continuous?"continuous":"once"),
        isVisible:()=>Boolean(store.sketchPlane),isEnabled:()=>Boolean(canEdit&&store.sketchPlane),isActive:()=>store.activeToolID===toolID})),
      commandRegistry.register({ id: "part.pad", execute: () => openSolidFeature("LINEAR_EXTRUDE"), isVisible: () => editingView?.document.type === "PART",
        isEnabled: () => Boolean(canEdit && store.selection?.kind === "sketch") }),
      commandRegistry.register({ id: "part.pocket", execute: () => openSolidFeature("LINEAR_EXTRUDE", "REMOVE"), isVisible: () => editingView?.document.type === "PART",
        isEnabled: () => Boolean(canEdit && store.selection?.kind === "sketch" && editingView?.part?.features.some((feature) => isSolidFeature(feature))) }),
      commandRegistry.register({ id: "part.revolve", execute: () => openSolidFeature("REVOLVE"), isVisible: () => editingView?.document.type === "PART",
        isEnabled: () => Boolean(canEdit && store.selection?.kind === "sketch") }),
      commandRegistry.register({ id: "part.datum-plane", execute: () => { datumPlaneForm.setFieldsValue({ name: "Plane", offset: 10 }); setDatumPlaneOpen(true); },
        isVisible: () => editingView?.document.type === "PART", isEnabled: () => Boolean(canEdit && store.selection?.kind === "plane") }),
      commandRegistry.register({ id: "part.datum-axis", execute: () => { datumAxisForm.setFieldsValue({ name: "Axis", ox: 0, oy: 0, oz: 0, dx: 0, dy: 0, dz: 1 }); setDatumAxisOpen(true); },
        isVisible: () => editingView?.document.type === "PART", isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "product.insert", execute: () => setInsertOpen(true), isVisible: () => editingView?.document.type === "PRODUCT",
        isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "product.reference.toggle", execute: () => {
        const instance = selectedInstance();
        if (editingView && instance) command.mutate(() => api.setReferenceMode(editingView.document.id, instance.id,
          instance.referenceMode === "PINNED" ? "FOLLOW_HEAD" : "PINNED"));
      }, isVisible: () => editingView?.document.type === "PRODUCT", isEnabled: () => Boolean(canEdit && selectedInstance()) }),
      ...(["fix", "rigid", "coincident", "concentric", "angle", "distance"] as const).map((constraint) => commandRegistry.register({
        id: `assembly.${constraint}`,
        execute: (invocation) => store.setActiveTool(`assembly.${constraint}`, invocation?.continuous ? "continuous" : "once"),
        isVisible: () => editingView?.document.type === "PRODUCT",
        isEnabled: () => Boolean(canEdit),
        isActive: () => store.activeToolID === `assembly.${constraint}`,
      })),
      commandRegistry.register({ id: "history.version", execute: () => setVersionOpen(true), isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "document.share", execute: () => editingView && setShareResource({ type: "documents", id: editingView.document.id, name: editingView.document.name }),
        isEnabled: () => editingView?.document.permission === "OWNER" }),
      commandRegistry.register({ id: "edit.undo", execute: () => executeHistory("undo"),
        isEnabled: () => Boolean(canEdit && editingView?.document.canUndo && !command.isPending) }),
      commandRegistry.register({ id: "edit.redo", execute: () => executeHistory("redo"),
        isEnabled: () => Boolean(canEdit && editingView?.document.canRedo && !command.isPending) }),
      commandRegistry.register({ id: "view.fit", execute: () => viewport.current?.fit() }),
      commandRegistry.register({ id: "view.top", execute: () => viewport.current?.setStandardView("TOP") }),
      commandRegistry.register({ id: "view.front", execute: () => viewport.current?.setStandardView("FRONT") }),
      commandRegistry.register({ id: "view.right", execute: () => viewport.current?.setStandardView("RIGHT") }),
      commandRegistry.register({ id: "view.iso", execute: () => viewport.current?.setStandardView("ISO") }),
      commandRegistry.register({ id: "navigation.profile.toggle", execute: () => store.setNavigationProfile(
        store.navigationProfile === "default" ? "catia" : "default"), isActive: () => store.navigationProfile === "catia" }),
	  commandRegistry.register({ id: "debug.download", execute: () => editingView && api.downloadDiagnosticBundle(editingView.document.id),
		isVisible: () => !isMockMode, isEnabled: () => Boolean(editingView) }),
    ];
    return () => { for (const dispose of disposers.reverse()) dispose(); };
  }, [commandRegistry, editingView, canEdit, store.selection, store.sketchPlane, store.activeToolID, store.navigationProfile, command.isPending, assemblyConstraintForm]);

  useEffect(() => { commandRegistry.notifyStateChanged(); }, [commandRegistry, editingView, store.selection, store.sketchPlane,
    store.activeToolID, store.navigationProfile, command.isPending]);

  const selected = selectedFeature(editingView ?? {} as DocumentView, store.selection);
  if (document.isLoading) return <div className="workbench-loading"><Spin size="large" /></div>;
  if (!view) return <Empty description="无法打开文档" />;

  return <CommandProvider registry={commandRegistry}><section className="cad-workbench">
    <main className="workbench-stage"><section className={`viewport-frame ${inspectorOpen ? "inspector-open" : ""}`}>
        <Suspense fallback={<div className="viewport-loading"><Spin size="large" /></div>}><CadViewport ref={viewport} view={view}
          editingView={editingView} activeInstancePath={activeInstancePath} activeInstanceTranslation={activeResolvedInstance?.translation}
          activeInstanceRotation={activeResolvedInstance?.rotation}
          activeBodyTreeNodeId={activeResolvedInstance?.bodyTreeNodeId}
          selections={store.selections}
          preselection={store.preselection}
          hiddenTreeKeys={hiddenTreeKeys}
          sketchPlane={store.sketchPlane} activeSketchID={store.activeSketchID} activeToolID={store.activeToolID} navigationProfile={store.navigationProfile}
          captureSettings={store.captureSettings} onSelectionsChange={store.setSelections} onPreselectionChange={store.setPreselection} onSketchOperations={editSketch}
          onToolUseComplete={store.completeToolUse} onActiveToolChange={store.setActiveTool}
          onAssemblyConstraint={(kind, references) => {
            if (!editingView) return;
            if (kind === "angle" || kind === "distance") {
              assemblyConstraintForm.setFieldsValue({ value: 0 });
              setPendingAssemblyConstraint({ kind, references });
              return;
            }
            command.mutate(() => api.addAssemblyConstraint(editingView.document.id, {
              constraintKind: kind.toUpperCase(), firstAssemblyRef: references[0], secondAssemblyRef: references[1],
              value: 0, directionRelation: "UNORIENTED", distanceRelation: "UNSIGNED",
            }));
          }}
          onInstanceMovePreview={async(instanceId,translation,rotation)=>{
            if(editingView?.document.type!=="PRODUCT")return[];const preview=await api.previewCommand(editingView.document.id,{type:"MOVE_INSTANCE",requestId:randomUUID(),instanceId,translation,rotation});return preview.instancePoses??[];
          }}
          onInstanceMoved={moveInstance} /></Suspense>
		{toolbarCatalog.data?.toolbars.filter((toolbar) => toolbar.workbench === "ALL" || toolbar.workbench === activeWorkbench)
		  .map((toolbar: ToolbarCatalogEntry) => <FloatingToolbar key={toolbar.id} id={toolbar.id} label={toolbar.name}
			position={toolbar.position} orientation={toolbar.orientation}
			className={`${toolbar.styleKey === "part" ? "part-design-toolbar" : toolbar.styleKey === "sketch" ? "sketcher-toolbar" : toolbar.styleKey === "assembly" ? "assembly-design-toolbar" : toolbar.styleKey === "debug" ? "debug-toolbar" : "common-toolbar"} ${toolbar.id}-toolbar`}>
			{toolbarGroups(toolbar.items).map((group) => <ToolbarGroup key={group.key}>{group.items.map((item) => item.commandId === "capture.settings"
			  ? <CaptureSettingsButton key={item.commandId} settings={store.captureSettings} onEnabledChange={store.setCaptureEnabled}
				  onSelectionToggle={store.toggleSelectionCapture} onSketchToggle={store.toggleSketchSnap}
				  onAll={store.captureAll} onPointsOnly={store.capturePointsOnly} />
			  : <ToolButton key={item.commandId} command={item.commandId} repeatable={item.repeatable}
				  icon={<CadIcon name={item.iconKey as CadIconName} />} tooltip={item.name}
				  toolbarName={toolbar.name} helpText={item.helpText} />)}</ToolbarGroup>)}
		  </FloatingToolbar>)}
        <aside className="floating-structure-tree">
          <SpecificationTree nodes={treeNodes} selectedKeys={treeKeysForSelections(treeNodes, store.selections)}
            selectedIdentityKeys={store.selections.map(selectionKey)}
            selectionToken={selectionSetToken(store.selections)}
            highlightedKey={treeKeyForSelection(treeNodes, store.preselection)}
            activeDocumentId={activeID}
            activeInstancePath={activeInstancePath}
            onSelect={(nodes) => store.setSelections(nodes.flatMap((node) => node.selection ? [node.selection] : []))}
            onActivate={(node) => {
              if (node.kind === "ASSEMBLY_CONSTRAINT" && node.entityId) {
                const constraint = editingView?.product?.constraints?.find((candidate) => candidate.id === node.entityId);
                if (constraint) { setEditingAssemblyConstraint(constraint); assemblyConstraintForm.setFieldsValue({ value: constraint.kind === "ANGLE" ? (constraint.value ?? 0) * 180 / Math.PI : constraint.value ?? 0 }); }
                return;
              }
              if (node.documentId && ["PART", "PRODUCT", "INSTANCE"].includes(node.kind ?? "")) {
                setActiveDocumentID(node.documentId); setActiveInstancePath(node.instancePath?.canonical);
                store.endSketch(); store.setSelection(null); return;
              }
              if (!canEdit || !node.selection || !editingView) return;
              if (node.selection.kind === "sketch") {
                const feature=editingView.part?.features.find((candidate)=>candidate.id===node.selection!.id);
                const localPlane=feature?featureSketchPlane(editingView,feature):undefined;
                const plane=localPlane?occurrenceSketchPlane(localPlane,activeResolvedInstance?.translation,activeResolvedInstance?.rotation):undefined;
                if(feature&&plane)store.beginSketch(feature.id,plane);
              } else if (node.selection.kind === "sketch-constraint") viewport.current?.editDimension(node.selection);
            }}
            onHover={(node) => store.setPreselection(node?.selection ?? null)} onDelete={deleteTreeNodes}
            onToggleVisibility={(node)=>toggleTreeVisibility(node.key)}
            onToggleSuppression={(node)=>{
              const leaves:SpecificationTreeNode[]=[];const visit=(item:SpecificationTreeNode)=>{if(item.kind==="SKETCH_ENTITY"||item.kind==="SKETCH_CONSTRAINT")leaves.push(item);else item.children?.forEach(visit);};visit(node);
              const targetState=!leaves.every((item)=>item.suppressed);const bySketch=new Map<string,SketchOperation[]>();
              for(const item of leaves){if(!item.ownerEntityId||!item.entityId)continue;const operations=bySketch.get(item.ownerEntityId)??[];
                operations.push(item.kind==="SKETCH_ENTITY"?{type:"UPDATE_ENTITY_SUPPRESSION",entityId:item.entityId,suppressed:targetState}
                  :{type:"UPDATE_CONSTRAINT_SUPPRESSION",constraintId:item.entityId,suppressed:targetState});bySketch.set(item.ownerEntityId,operations);}
              for(const [sketchID,operations] of bySketch)editSketch(sketchID,operations);
            }}
            onToggleConstruction={(node)=>{
              if(!node.ownerEntityId||!node.entityId)return;
              editSketch(node.ownerEntityId,[{type:"UPDATE_ENTITY_ROLE",entityId:node.entityId,
                role:node.role==="CONSTRUCTION"?"PROFILE":"CONSTRUCTION"}]);
            }} />
        </aside>
        <button className={`inspector-toggle ${inspectorOpen ? "open" : ""}`} onClick={() => setInspectorOpen(!inspectorOpen)}
          title={inspectorOpen ? "收起属性面板" : "展开属性面板"}>
          {inspectorOpen ? <MenuFoldOutlined /> : <MenuUnfoldOutlined />}
        </button>
        <aside className={`inspector-overlay ${inspectorOpen ? "open" : ""}`}>
          <Segmented block value={store.inspectorTab} onChange={(value) => store.setInspectorTab(value as "properties" | "history")}
            options={[{ label: "属性", value: "properties" }, { label: "历史", value: "history" }]} />
          <div className="inspector-overlay-content">{store.inspectorTab === "properties"
            ? <Properties view={editingView ?? view} selection={store.selection} feature={selected}
              workbench={activeWorkbench} sketchPlane={store.sketchPlane} activeTool={store.activeToolID}
              navigationProfile={store.navigationProfile} diagnostics={properties.data}
              topology={topology.data} topologyLoading={topology.isLoading} />
            : <History entries={history.data ?? []} onRestore={(entry) => command.mutate(() => api.restore(activeID, entry.versionId))} />}</div>
        </aside>
      </section></main>
    <CommandDialog id="assembly-constraint-edit" open={Boolean(editingAssemblyConstraint)} title="编辑装配约束"
      onClose={() => setEditingAssemblyConstraint(undefined)} confirmLoading={command.isPending} onConfirm={async()=>{
        if(!editingView||!editingAssemblyConstraint)return;const {value}=await assemblyConstraintForm.validateFields();const constraint=editingAssemblyConstraint;
        command.mutate(()=>api.editAssemblyConstraint(editingView.document.id,constraint.id,{value:constraint.kind==="ANGLE"?value*Math.PI/180:value,
          directionRelation:constraint.directionRelation??"UNORIENTED",distanceRelation:constraint.distanceRelation??"UNSIGNED"}),{onSuccess:()=>setEditingAssemblyConstraint(undefined)});
      }}>
      <Form form={assemblyConstraintForm} layout="vertical"><Form.Item name="value" label={editingAssemblyConstraint?.kind==="ANGLE"?"角度（deg）":"距离（mm）"}
        rules={[{required:true}]}><InputNumber disabled={!editingAssemblyConstraint||!["ANGLE","DISTANCE"].includes(editingAssemblyConstraint.kind)} style={{width:"100%"}} /></Form.Item></Form>
    </CommandDialog>
    <CommandDialog id="assembly-constraint-value" open={Boolean(pendingAssemblyConstraint)}
      title={pendingAssemblyConstraint?.kind === "angle" ? "装配角度" : "装配距离"}
      onClose={() => setPendingAssemblyConstraint(undefined)} confirmLoading={command.isPending}
      onConfirm={async () => {
        if (!editingView || !pendingAssemblyConstraint) return;
        const { value } = await assemblyConstraintForm.validateFields();
        const pending = pendingAssemblyConstraint;
        command.mutate(() => api.addAssemblyConstraint(editingView.document.id, {
          constraintKind: pending.kind.toUpperCase(), firstAssemblyRef: pending.references[0], secondAssemblyRef: pending.references[1],
          value: pending.kind === "angle" ? value * Math.PI / 180 : value,
          directionRelation: "UNORIENTED", distanceRelation: "UNSIGNED",
        }), { onSuccess: () => setPendingAssemblyConstraint(undefined) });
      }}>
      <Form form={assemblyConstraintForm} layout="vertical"><Form.Item name="value"
        label={pendingAssemblyConstraint?.kind === "angle" ? "角度（deg）" : "距离（mm）"}
        rules={[{ required: true }, { type: "number", min: 0, max: pendingAssemblyConstraint?.kind === "angle" ? 180 : undefined }]}>
        <InputNumber min={0} max={pendingAssemblyConstraint?.kind === "angle" ? 180 : undefined} precision={2} style={{ width: "100%" }} />
      </Form.Item></Form>
    </CommandDialog>
    <CommandDialog id="solid-generator" open={padOpen} title="实体特征" onClose={closePad} confirmLoading={command.isPending}
      onConfirm={async () => padSketch(await padForm.validateFields())}>
      <Form form={padForm} layout="vertical"><Form.Item name="generator" hidden><Input /></Form.Item>
        <Form.Item name="operation" label="Body 操作" rules={[{ required: true }]}>
        <Select onChange={previewPad} options={[{ value: "NEW_BODY", label: "新建实体" }, { value: "ADD", label: "添加材料" },
          { value: "REMOVE", label: "移除材料" }, { value: "INTERSECT", label: "保留交集" }]} /></Form.Item>
        <Form.Item noStyle shouldUpdate={(before, after) => before.generator !== after.generator}>{({ getFieldValue }) => getFieldValue("generator") === "REVOLVE" ? <>
          <Form.Item name="axisEntityId" label="旋转轴" rules={[{ required: true }]}><Select onChange={previewPad}
            placeholder="在视图区或结构树选择直线/轴"
            options={[...(editingView?.part?.features ?? []).flatMap((feature) => (feature.sketch?.entities ?? [])
              .filter((entity) => entity.kind === "LINE")
              .map((entity, index) => ({ value: `SKETCH_LINE:${feature.id}:${entity.id}`, label: `${feature.name ?? feature.id} · 直线 ${index + 1}` }))),
              ...(editingView?.axisSystems ?? []).flatMap((axis) => (["X","Y","Z"] as const).map((direction) => ({
                value: `AXIS_SYSTEM:${axis.id}:${direction}`, label: `${axis.name} · ${direction}` }))),
              ...(editingView?.datumAxes ?? []).map((axis) => ({ value: `DATUM_AXIS:${axis.id}`, label: axis.name }))]} /></Form.Item>
          <Form.Item name="angle" label="旋转角度（deg）" rules={[{ required: true }, { type: "number", min: 0.1, max: 360 }]}>
            <InputNumber min={0.1} max={360} precision={2} style={{ width: "100%" }} onBlur={previewPad} onPressEnter={previewPad} /></Form.Item>
        </> : <Form.Item name="length" label="拉伸长度（mm）" rules={[{ required: true }, { type: "number", min: 0.1 }]}>
          <InputNumber min={0.1} precision={2} style={{ width: "100%" }} onBlur={previewPad} onPressEnter={previewPad} /></Form.Item>}</Form.Item>
        <Form.Item name="reversed" label="反向" valuePropName="checked"><Switch onChange={previewPad} /></Form.Item>
        <small className="cad-command-hint">{padPreviewPending ? "后端正在求值预览…" : "输入后按 Enter 或点击视口可刷新后端瞬态预览；预览不会创建 Revision。"}</small></Form>
    </CommandDialog>
    <CommandDialog id="insert" open={insertOpen} title="插入 Part / Product" onClose={() => setInsertOpen(false)}
      confirmLoading={command.isPending} onConfirm={async () => insertDocument(await insertForm.validateFields())}>
      <Form form={insertForm} layout="vertical"><Form.Item name="referencedDocumentID" label="引用文档" rules={[{ required: true }]}><Select showSearch optionFilterProp="label" options={(catalog.data?.documents ?? []).filter((item) => item.id !== activeID).map((item) => ({ value: item.id, label: `${item.name} (${item.type})` }))} /></Form.Item>
      </Form>
    </CommandDialog>
    <CommandDialog id="datum-plane" open={datumPlaneOpen} title="创建基准面" onClose={() => setDatumPlaneOpen(false)}
      confirmLoading={command.isPending} onConfirm={async () => {
        const values = await datumPlaneForm.validateFields(); const selectedPlane = store.selection?.kind === "plane" ? store.selection.datumPlane : undefined;
        if (!selectedPlane) return; const origin = selectedPlane.origin.map((value, index) => value + selectedPlane.normal[index] * values.offset) as Vec3;
        command.mutate(() => api.createDatumPlane(activeID, { name: values.name, origin, normal: selectedPlane.normal, uDirection: selectedPlane.uDirection }),
          { onSuccess: () => setDatumPlaneOpen(false) });
      }}><Form form={datumPlaneForm} layout="vertical"><Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="offset" label="偏置（mm）" rules={[{ required: true }, { type: "number" }]}><InputNumber style={{ width: "100%" }} /></Form.Item></Form>
    </CommandDialog>
    <CommandDialog id="datum-axis" open={datumAxisOpen} title="创建基准轴" onClose={() => setDatumAxisOpen(false)}
      confirmLoading={command.isPending} onConfirm={async () => { const v = await datumAxisForm.validateFields();
        command.mutate(() => api.createDatumAxis(activeID, { name: v.name, origin: [v.ox,v.oy,v.oz], direction: [v.dx,v.dy,v.dz] }),
          { onSuccess: () => setDatumAxisOpen(false) }); }}><Form form={datumAxisForm} layout="vertical"><Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
        <Space><Form.Item name="ox" label="原点 X"><InputNumber /></Form.Item><Form.Item name="oy" label="Y"><InputNumber /></Form.Item><Form.Item name="oz" label="Z"><InputNumber /></Form.Item></Space>
        <Space><Form.Item name="dx" label="方向 X"><InputNumber /></Form.Item><Form.Item name="dy" label="Y"><InputNumber /></Form.Item><Form.Item name="dz" label="Z"><InputNumber /></Form.Item></Space></Form>
    </CommandDialog>
    <CommandDialog id="version" open={versionOpen} title="创建命名版本" onClose={() => setVersionOpen(false)}
      onConfirm={async () => createVersion(await versionForm.validateFields())}>
      <Form form={versionForm} layout="vertical"><Form.Item name="name" label="版本名称" rules={[{ required: true }]}><Input placeholder="V1 - Initial concept" /></Form.Item>
        <Form.Item name="description" label="说明"><Input.TextArea rows={3} /></Form.Item></Form>
    </CommandDialog>
    <ShareDialog resource={shareResource} onClose={() => setShareResource(undefined)} />
  </section></CommandProvider>;
}

function Properties({ view, selection, feature, workbench, sketchPlane, activeTool, navigationProfile, diagnostics, topology, topologyLoading }: {
  view: DocumentView;
  selection: Selection;
  feature?: Feature;
  workbench: keyof typeof CAD_WORKBENCHES;
  sketchPlane?: SketchPlane;
  activeTool: string;
  navigationProfile: string;
  diagnostics?: DocumentProperties;
  topology?: TopologyElementProperties;
  topologyLoading?: boolean;
}) {
  if (!selection) {
    const triangleCount = view.artifact?.mesh.triangles.length
      ?? (view.resolvedInstances ?? []).reduce((total, instance) =>
        total + (view.artifacts?.[instance.geometryKey]?.mesh.triangles.length ?? 0), 0);
    const geometryCount = diagnostics?.aggregate.artifactCount
      ?? (view.document.type === "PRODUCT" ? view.resolvedInstances?.length ?? 0 : view.artifact ? 1 : 0);
    const detail = diagnostics?.artifacts[0];
    const bytes = (value = 0) => value < 1024 ? `${value} B` : `${(value / 1024).toFixed(1)} KiB`;
    return <><div className="property-context-hint">未选择对象 · 当前工作环境</div><Descriptions column={1} size="small"
      bordered className="property-list" items={[
        { key: "workbench", label: "Workbench", children: `${CAD_WORKBENCHES[workbench].label} · ${CAD_WORKBENCHES[workbench].domain}` },
        { key: "document", label: "文档", children: `${view.document.name} (${view.document.type})` },
        { key: "workspace", label: "工作区", children: view.document.workspaceName ?? "Main" },
        { key: "permission", label: "权限", children: view.document.permission },
        { key: "version", label: "Head Version", children: view.document.versionId.slice(0, 16) },
        { key: "tool", label: "Active Tool", children: activeTool },
        { key: "navigation", label: "Navigation", children: navigationProfile.toUpperCase() },
        ...(sketchPlane ? [{ key: "plane", label: "Sketch Plane", children: sketchPlane.plane }] : []),
        { key: "history", label: "History", children: `Undo ${view.document.canUndo ? "Yes" : "No"} · Redo ${view.document.canRedo ? "Yes" : "No"}` },
        { key: "geometry", label: "Display Geometry", children: `${geometryCount} object(s) · ${triangleCount} triangles` },
        { key: "topology", label: "Topology", children: diagnostics
          ? `${diagnostics.aggregate.solidCount} solid(s) · ${diagnostics.aggregate.vertexCount} vertices` : "Loading…" },
        { key: "artifact", label: "Geometry Key", children: detail?.geometryKey ?? "—" },
        { key: "geometry-id", label: "Geometry ID", children: detail?.geometryId ?? "—" },
        { key: "evaluator", label: "Evaluator", children: detail?.evaluatorVersion ?? "—" },
        { key: "artifacts", label: "Artifacts", children: diagnostics
          ? `GLB ${bytes(diagnostics.aggregate.glbBytes)} · B-Rep ${bytes(diagnostics.aggregate.brepBytes)}` : "Loading…" },
        { key: "storage", label: "Storage", children: detail?.storageState ?? "—" },
        { key: "artifact-worker", label: "Artifact Worker", children: detail?.workerId ?? "—" },
        { key: "worker", label: "Worker Route", children: diagnostics?.worker.available
          ? `${diagnostics.worker.workerId} · ${diagnostics.worker.residentGeometryCount} resident` : diagnostics?.worker.error ?? "Loading…" },
        { key: "occt", label: "OCCT", children: diagnostics?.worker.occtVersion ?? detail?.occtVersion ?? "—" },
        { key: "reference", label: "Reference Geometry", children: detail
          ? `${detail.visualization.referenceGeometry.datumPlanes.length} planes · ${detail.visualization.referenceGeometry.axisSystems.length} axis system(s)` : "—" },
        { key: "visual-primitives", label: "Non-solid Geometry", children: detail?.visualization.primitives.length ?? 0 },
        { key: "rendering", label: "Rendering", children: "Phong Solid + welded feature edges" },
        { key: "features", label: view.document.type === "PART" ? "Features" : "Instances",
          children: view.document.type === "PART" ? view.part?.features.length ?? 0 : view.product?.instances.length ?? 0 },
      ]} /></>;
  }
  if (["face", "edge", "vertex"].includes(selection.kind)) {
    const format = (value: unknown): string => Array.isArray(value)
      ? value.map((entry) => typeof entry === "number" ? Number(entry).toPrecision(7) : String(entry)).join(", ")
      : typeof value === "number" ? Number(value).toPrecision(9) : String(value);
    if (topologyLoading || !topology) return <Spin size="small" tip="从 Geometry Worker 读取 B-Rep…" />;
    return <><div className="property-context-hint">OCCT B-Rep 拓扑属性</div><Descriptions column={1} size="small"
      bordered className="property-list" items={[
        { key: "kind", label: "Topology", children: `${topology.kind} #${topology.localId}` },
        { key: "geometry-type", label: "Geometry", children: topology.geometryType },
        { key: "geometry-id", label: "Geometry ID", children: topology.geometryId },
        { key: "worker", label: "Worker", children: topology.workerId },
        { key: "occt", label: "OCCT", children: topology.occtVersion },
        ...(topology.point ? [{ key: "point", label: "Point", children: format(topology.point) }] : []),
        ...Object.entries(topology.properties).map(([key, value]) => ({ key: `brep-${key}`, label: key, children: format(value) })),
      ]} /></>;
  }
  if (selection.kind === "axis" || selection.kind === "axis-system") {
    const systems = view.axisSystems ?? diagnostics?.artifacts.flatMap((item) => item.visualization.referenceGeometry.axisSystems) ?? [];
    const axisSystem = systems.find((item) => selection.id.includes(item.id));
    const direction = selection.kind === "axis" && axisSystem
      ? selection.axis === "X" ? axisSystem.xDirection : selection.axis === "Y" ? axisSystem.yDirection : axisSystem.zDirection
      : undefined;
    return <Descriptions column={1} size="small" bordered className="property-list" items={[
      { key: "type", label: "类型", children: selection.kind === "axis" ? `${selection.axis} Axis` : "Axis System" },
      { key: "name", label: "名称", children: axisSystem?.name ?? selection.id },
      { key: "origin", label: "原点", children: axisSystem?.origin.join(", ") ?? "—" },
      ...(direction ? [{ key: "direction", label: "方向", children: direction.join(", ") }] : []),
      { key: "reference", label: "引用路径", children: selection.occurrencePath || "Part root" },
    ]} />;
  }
  const instance = selection.kind === "instance" ? view.product?.instances.find((item) => item.id === selection.id) : undefined;
  if (selection.kind === "visual") return <Descriptions column={1} size="small" bordered className="property-list" items={[
    { key: "type", label: "类型", children: selection.visualType },
    { key: "entity", label: "元素", children: selection.entityId },
    { key: "feature", label: "所属特征", children: selection.featureId },
    { key: "role", label: "角色", children: selection.role ?? "—" },
    { key: "reference", label: "Occurrence", children: selection.occurrencePath || "Part root" },
  ]} />;
  return <Descriptions column={1} size="small" bordered className="property-list" items={[
    { key: "type", label: "类型", children: selection.kind.toUpperCase() },
    { key: "name", label: "名称", children: feature?.name ?? instance?.name ?? selection.id },
    ...(selection.kind === "plane" ? [{ key: "plane", label: "基准面", children: selection.plane }] : []),
    ...(feature?.sketch ? [{ key: "entities", label: "草图元素", children: feature.sketch.entities.length },
      { key: "constraints", label: "约束", children: feature.sketch.constraints.length },
      { key: "solve", label: "求解", children: `${feature.sketch.solve.status} · ${feature.sketch.solve.degreesOfFreedom} DoF` }] : []),
    ...(feature?.length ? [{ key: "length", label: "长度", children: `${feature.length} mm` }] : []),
    ...(instance ? [{ key: "transform", label: "位移", children: instance.translation.map((value) => value.toFixed(2)).join(", ") },
    { key: "reference", label: "引用", children: instance.referenceMode ?? "FOLLOW_HEAD" }] : []),
  ]} />;
}

function History({ entries, onRestore }: { entries: HistoryEntry[]; onRestore: (entry: HistoryEntry) => void }) {
  return <List className="history-list" dataSource={[...entries].reverse()} renderItem={(entry) => <List.Item actions={!entry.isHead ? [<Button key="restore" type="link" icon={<ExportOutlined />} onClick={() => onRestore(entry)}>恢复</Button>] : []}>
    <List.Item.Meta title={<Space><span>#{entry.sequence} {entry.commandType}</span>{entry.isHead && <Tag color="blue">HEAD</Tag>}</Space>}
      description={<span>{entry.versionName ?? entry.versionId.slice(0, 12)}<br />{new Date(entry.createdAt).toLocaleString()}</span>} />
  </List.Item>} />;
}
