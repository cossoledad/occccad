import { ExportOutlined } from "@ant-design/icons";
import { Button, Descriptions, List, Space, Spin, Tag } from "antd";
import { CAD_WORKBENCHES } from "../../cad/workbench/cad-workbench";
import type {
  DocumentProperties,
  DocumentView,
  Feature,
  HistoryEntry,
  Selection,
  SketchPlane,
  TopologyElementProperties,
} from "../../types";
import { parameterDisplayValue, parameterSourceText } from "./parameter-editor";

type PropertiesProps = {
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
  onEditParameter?: (parameterId: string) => void;
};

export function Properties({ view, selection, feature, workbench, sketchPlane, activeTool, navigationProfile, diagnostics, topology, topologyLoading, onEditParameter }: PropertiesProps) {
	const parameterFor = (parameterId?: string) => view.part?.parameters?.find((parameter) => parameter.parameterId === parameterId);
	const parameterItems = (parameterId?: string) => {
		const parameter = parameterFor(parameterId);
		if (!parameter) return [];
		return [
			{ key: "parameter-id", label: "ParameterId", children: parameter.parameterId },
			{ key: "parameter-key", label: "参数别名", children: <Space>{parameter.key}{onEditParameter && <Button size="small" onClick={() => onEditParameter(parameter.parameterId)}>编辑</Button>}</Space> },
			{ key: "parameter-source", label: "表达式 / 输入", children: parameterSourceText(parameter) || "—" },
			{ key: "parameter-value", label: "计算值", children: parameterDisplayValue(parameter) },
		];
	};
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
        { key: "naming-status", label: "Persistent Naming", children: topology.namingStatus || "UNAVAILABLE" },
        ...(topology.persistentSelection ? [
          { key: "semantic-anchor", label: "Semantic Anchor", children: `${topology.persistentSelection.anchor.featureId} · ${topology.persistentSelection.anchor.outputSlot}` },
          { key: "selection-recipe", label: "Selection Recipe", children: topology.persistentSelection.selector.kind },
          { key: "support-status", label: "Supporting Element", children: topology.namingResolution?.supportingElementStatus ?? "NOT_CONNECTED" },
          { key: "evidence", label: "Evidence Digest", children: topology.namingResolution?.evidenceDigest?.slice(0, 20) ?? "—" },
        ] : [{ key: "naming-note", label: "Naming Diagnostic", children: "当前制品未提供可绑定的完整 semantic topology history" }]),
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
	if (selection.kind === "sketch-constraint") {
		const sketch = view.part?.features.find((candidate) => candidate.id === selection.featureId)?.sketch;
		const constraint = sketch?.constraints.find((candidate) => candidate.id === selection.constraintId);
		return <Descriptions column={1} size="small" bordered className="property-list" items={[
			{ key: "type", label: "约束", children: constraint?.kind ?? selection.constraintType },
			{ key: "status", label: "求解状态", children: sketch?.solve.status ?? "—" },
			...parameterItems(constraint?.parameterId),
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
  if (selection.kind === "assembly-constraint") {
    const constraint = view.product?.constraints?.find((value) => value.id === selection.constraintId);
    return <Descriptions column={1} size="small" bordered className="property-list" items={[
      { key: "type", label: "类型", children: constraint?.kind ?? selection.constraintType },
      { key: "evaluation", label: "Evaluation", children: constraint?.evaluationStatus ?? "NOT_UPDATED" },
      { key: "first-support", label: "First Support", children: constraint?.first.resolution?.result.supportingElementStatus ?? (constraint?.first.persistentSelection ? "NOT_CONNECTED" : "CONNECTED") },
      { key: "second-support", label: "Second Support", children: constraint?.second?.resolution?.result.supportingElementStatus ?? (constraint?.second?.persistentSelection ? "NOT_CONNECTED" : "CONNECTED") },
      { key: "diagnostic", label: "Diagnostic", children: constraint?.evaluationSummary ?? "—" },
    ]} />;
  }
  return <Descriptions column={1} size="small" bordered className="property-list" items={[
    { key: "type", label: "类型", children: selection.kind.toUpperCase() },
    { key: "name", label: "名称", children: feature?.name ?? instance?.name ?? selection.id },
    ...(selection.kind === "plane" ? [{ key: "plane", label: "基准面", children: selection.plane }] : []),
    ...(feature?.sketch ? [{ key: "entities", label: "草图元素", children: feature.sketch.entities.length },
      { key: "constraints", label: "约束", children: feature.sketch.constraints.length },
	  { key: "solve", label: "求解", children: `${feature.sketch.solve.status} · ${feature.sketch.solve.degreesOfFreedom} DoF` },
	  { key: "support", label: "Sketch Support", children: `${feature.sketch.support.type} · ${feature.sketch.support.status ?? "—"}` },
	  ...(feature.sketch.support.type === "PLANAR_FACE" ? [
		{ key: "support-anchor", label: "Semantic Anchor", children: `${feature.sketch.support.persistentSelection?.anchor.featureId ?? "—"} · ${feature.sketch.support.persistentSelection?.anchor.outputSlot ?? "—"}` },
		{ key: "support-snapshot", label: "Support Snapshot", children: feature.sketch.support.dependencySnapshot?.manifestDigest.slice(0, 20) ?? "—" },
		{ key: "support-diagnostic", label: "Support Diagnostic", children: feature.sketch.support.diagnosticCode ?? "—" },
	  ] : [])] : []),
    ...(feature?.length ? [{ key: "length", label: "长度", children: `${feature.length} mm` }] : []),
	...parameterItems(feature ? `parameter:${feature.id}:length` : undefined),
    ...(instance ? [{ key: "transform", label: "位移", children: instance.translation.map((value) => value.toFixed(2)).join(", ") }] : []),
  ]} />;
}

export function History({ entries, onRestore }: { entries: HistoryEntry[]; onRestore: (entry: HistoryEntry) => void }) {
  return <List className="history-list" dataSource={[...entries].reverse()} renderItem={(entry) => <List.Item actions={!entry.isHead ? [<Button key="restore" type="link" icon={<ExportOutlined />} onClick={() => onRestore(entry)}>恢复</Button>] : []}>
    <List.Item.Meta title={<Space><span>#{entry.sequence} {entry.commandType}</span>{entry.isHead && <Tag color="blue">HEAD</Tag>}</Space>}
      description={<span>{entry.versionName ?? entry.versionId.slice(0, 12)}<br />{new Date(entry.createdAt).toLocaleString()}</span>} />
  </List.Item>} />;
}
