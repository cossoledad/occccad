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
  if (selection?.publicationId) {
    const publication = selection.publication ?? view.part?.publications?.find((candidate) => candidate.id === selection.publicationId);
    if (publication) return <><div className="property-context-hint">Publication · 稳定公开契约</div>
      <Descriptions column={1} size="small" bordered className="property-list" items={[
        { key: "id", label: "PublicationId", children: publication.id },
        { key: "name", label: "名称", children: publication.name },
        { key: "type", label: "类型", children: publication.type },
        { key: "purpose", label: "语义用途", children: publication.semanticPurpose || "—" },
        { key: "version", label: "兼容版本", children: publication.compatibilityVersion },
        { key: "target", label: "目标", children: `${publication.target.kind} · ${publication.target.datumId ?? publication.target.featureId ?? publication.target.parameterId ?? publication.target.persistentSelection?.anchor.outputSlot ?? "—"}` },
        { key: "status", label: "解析状态", children: <Tag color={publication.resolution.status === "CONNECTED" ? "success" : "error"}>{publication.resolution.status}</Tag> },
        { key: "resolved", label: "Resolved Revision", children: publication.resolution.resolvedVersionId ?? "—" },
        { key: "source", label: "Source Digest", children: publication.resolution.sourceDigest ?? "—" },
        { key: "provenance", label: "Evaluator / OCCT", children: `${publication.resolution.evaluatorVersion ?? "—"} / ${publication.resolution.occtVersion ?? "—"}` },
        { key: "diagnostic", label: "Diagnostic", children: publication.resolution.diagnosticCode
          ? `${publication.resolution.diagnosticCode}: ${publication.resolution.diagnostic ?? ""}` : "—" },
      ]} /></>;
  }
  if (!selection) {
    const triangleCount = view.artifact?.mesh.triangles.length
      ?? (view.resolvedInstances ?? []).reduce((total, instance) =>
        total + (view.artifacts?.[instance.geometryKey]?.mesh.triangles.length ?? 0), 0);
    const geometryCount = diagnostics?.aggregate.artifactCount
      ?? (view.document.type === "PRODUCT" ? view.resolvedInstances?.length ?? 0 : view.artifact ? 1 : 0);
    const detail = diagnostics?.artifacts[0];
    const bytes = (value = 0) => value < 1024 ? `${value} B` : `${(value / 1024).toFixed(1)} KiB`;
    return <><div className="inspector-document-overview">
      <span className="inspector-eyebrow">{view.document.type === "PART" ? "零件文档" : "装配文档"}</span>
      <h3>{view.document.name}</h3>
      <p>选择模型或结构树中的对象，查看几何、特征和参数。</p>
    </div>
    <Descriptions column={1} size="small" className="property-list" items={[
      { key: "workspace", label: "工作区", children: view.document.workspaceName ?? "Main" },
      { key: "workbench", label: "工作台", children: CAD_WORKBENCHES[workbench].label },
      { key: "permission", label: "访问权限", children: view.document.permission === "OWNER" ? "所有者"
        : view.document.permission === "EDITOR" ? "可编辑" : "只读" },
      { key: "features", label: view.document.type === "PART" ? "特征数量" : "组件数量",
        children: view.document.type === "PART" ? view.part?.features.length ?? 0 : view.product?.instances.length ?? 0 },
      ...(sketchPlane ? [{ key: "plane", label: "草图平面", children: sketchPlane.plane }] : []),
    ]} />
    <details className="inspector-diagnostics"><summary>技术详情与诊断</summary><Descriptions column={1} size="small"
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
      ]} /></details></>;
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
        ] : [{ key: "naming-note", label: "Naming Diagnostic", children: topology.namingDiagnostic?.diagnostic ?? topology.namingResolution?.diagnostic ?? "当前制品未提供可绑定的完整 semantic topology history" }]),
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
  if (selection.kind === "visual") {
    const external = view.part?.features.find((candidate)=>candidate.id===selection.featureId)?.sketch?.externalGeometry
      ?.find((candidate)=>candidate.id===selection.entityId);
    return <Descriptions column={1} size="small" bordered className="property-list" items={[
    { key: "type", label: "类型", children: selection.visualType },
    { key: "entity", label: "元素", children: selection.entityId },
    { key: "feature", label: "所属特征", children: selection.featureId },
    { key: "role", label: "角色", children: selection.role ?? "—" },
    ...(external ? [
      {key:"external-status",label:"External Status",children:external.status},
      {key:"external-anchor",label:"Semantic Anchor",children:`${external.persistentSelection.anchor.featureId} · ${external.persistentSelection.anchor.outputSlot}`},
      {key:"external-kind",label:"Projection",children:`${external.projectionKind} · ${external.snapshot?.kind??"—"}`},
      {key:"external-digest",label:"Source Digest",children:external.resolvedSourceDigest?.slice(0,20)??"—"},
      {key:"external-diagnostic",label:"External Diagnostic",children:external.diagnosticCode??"—"},
      {key:"external-affected",label:"Affected",children:[...(external.affectedConstraintIds??[]),...(external.affectedProfileRegionIds??[]),...(external.downstreamFeatureIds??[])].join(", ")||"—"},
    ] : []),
    { key: "reference", label: "Occurrence", children: selection.occurrencePath || "Part root" },
  ]} />;
  }
  if (selection.kind === "assembly-constraint") {
    const constraint = view.product?.constraints?.find((value) => value.id === selection.constraintId);
    return <Descriptions column={1} size="small" bordered className="property-list" items={[
      { key: "type", label: "类型", children: constraint?.kind ?? selection.constraintType },
      { key: "activation", label: "激活状态", children: constraint?.suppressed ? "停用" : "激活" },
      { key: "mode", label: "模式", children: constraint?.mode ?? "DRIVING" },
      ...(constraint?.mode === "MEASURED" ? [{key:"measurement",label:"测量值",children:constraint.measuredValue === undefined ? "不可测" : constraint.kind === "ANGLE" ? `${constraint.measuredValue*180/Math.PI}°` : `${constraint.measuredValue} mm`}] : []),
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
      { key: "external", label: "外部几何", children: `${feature.sketch.externalGeometry?.length??0} · ${(feature.sketch.externalGeometry??[]).filter((item)=>item.status!=="CONNECTED").length} unresolved` },
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

export function History({ entries, onRestore, canRestore = true }: { entries: HistoryEntry[]; onRestore: (entry: HistoryEntry) => void; canRestore?: boolean }) {
  return <List className="history-list" dataSource={[...entries].reverse()} renderItem={(entry) => <List.Item actions={!entry.isHead && canRestore ? [<Button key="restore" type="link" icon={<ExportOutlined />} onClick={() => onRestore(entry)}>恢复</Button>] : []}>
    <List.Item.Meta title={<Space><span>#{entry.sequence} {entry.commandType}</span>{entry.isHead && <Tag color="blue">HEAD</Tag>}</Space>}
      description={<span>{entry.versionName ?? entry.versionId.slice(0, 12)}<br />{new Date(entry.createdAt).toLocaleString()}</span>} />
  </List.Item>} />;
}
