import { referencePropertyRows } from "./reference-properties";
import {formatDisplayNumber} from "../../utils/display-number";
import { ExportOutlined } from "@ant-design/icons";
import { Button, Descriptions, List, Space, Spin, Tag, Typography } from "antd";
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
import { sketchProfileFeedback } from "../../cad/sketch/sketch-profile-analysis";

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
  onEditParameter?: (id:string) => void;
  onEditPublication?: (id:string,kind:"PART"|"PRODUCT") => void;
};

export function Properties({ view, selection, feature, workbench, sketchPlane, activeTool, navigationProfile, diagnostics, topology, topologyLoading,
  onEditParameter, onEditPublication }: PropertiesProps) {
	const parameterFor = (parameterId?: string) => view.part?.parameters?.find((parameter) => parameter.parameterId === parameterId);
	const parameterItems = (parameterId?: string) => {
		const parameter = parameterFor(parameterId);
		if (!parameter) return [];
		return [
			{ key: "parameter-id", label: "ParameterId", children: parameter.parameterId },
			{ key: "parameter-name", label: "参数", children: parameter.qualifiedDisplayPath ?? parameter.displayName ?? parameter.label },
			...(parameter.displayAlias ? [{key:"parameter-alias",label:"别名",children:parameter.displayAlias}] : []),
			{ key: "parameter-source", label: "表达式 / 输入", children: parameterSourceText(parameter) || "—" },
			{ key: "parameter-value", label: "计算值", children: parameterDisplayValue(parameter) },
			...(onEditParameter ? [{key:"parameter-edit",label:"操作",children:<Button size="small" onClick={()=>onEditParameter(parameter.parameterId)}>编辑参数</Button>}] : []),
		];
	};
  if (selection?.publicationId) {
    const publication = selection.publication ?? view.part?.publications?.find((candidate) => candidate.id === selection.publicationId);
    if (publication) return <><div className="property-context-hint">Publication · 稳定公开契约</div>
      {onEditPublication && <Button size="small" onClick={()=>onEditPublication(publication.id, view.document.type === "PRODUCT" ? "PRODUCT" : "PART")}>编辑 Publication</Button>}
      <Descriptions column={1} size="small" bordered className="property-list" items={[
        { key: "id", label: "PublicationId", children: publication.id },
        { key: "name", label: "名称", children: publication.name },
        { key: "type", label: "类型", children: publication.type },
        { key: "purpose", label: "语义用途", children: publication.semanticPurpose || "—" },
        { key: "version", label: "兼容版本", children: publication.compatibilityVersion },
        { key: "target", label: "目标", children: `${publication.target.kind} · ${publication.target.bodyId ?? publication.target.datumId ?? publication.target.featureId ?? publication.target.parameterId ?? publication.target.persistentSelection?.anchor.outputSlot ?? "—"}` },
        { key: "status", label: "解析状态", children: <Tag color={publication.resolution.status === "CONNECTED" ? "success" : "error"}>{publication.resolution.status}</Tag> },
        { key: "resolved", label: "Resolved Revision", children: publication.resolution.resolvedVersionId ?? "—" },
        { key: "source", label: "Source Digest", children: publication.resolution.sourceDigest ?? "—" },
        { key: "provenance", label: "Evaluator / OCCT", children: `${publication.resolution.evaluatorVersion ?? "—"} / ${publication.resolution.occtVersion ?? "—"}` },
        { key: "diagnostic", label: "Diagnostic", children: publication.resolution.diagnosticCode
          ? `${publication.resolution.diagnosticCode}: ${publication.resolution.diagnostic ?? ""}` : "—" },
      ]} /></>;
  }
  if (!selection) {
    const triangleCount = Object.values(view.artifacts ?? {}).reduce((total,a)=>total+a.triangleCount,0);
    const geometryCount = diagnostics?.aggregate.artifactCount
      ?? (view.document.type === "PRODUCT" ? view.resolvedInstances?.length ?? 0 : view.part?.bodies.length ?? 0);
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
    <PartBodies view={view} />
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
        { key: "visual-primitives", label: "Non-solid Geometry", children: detail?.visualization.primitives?.length ?? "按需从 GLB 加载" },
        { key: "rendering", label: "Rendering", children: "Phong Solid + welded feature edges" },
        { key: "features", label: view.document.type === "PART" ? "Features" : "Instances",
          children: view.document.type === "PART" ? view.part?.features.length ?? 0 : view.product?.instances.length ?? 0 },
      ]} /></details></>;
  }
  if (selection.kind === "body") {
    const sameDefinitionSnapshot = selection.documentId === view.document.id &&
      selection.versionId === view.document.versionId;
    const body = sameDefinitionSnapshot ? view.part?.bodies.find((candidate) => candidate.id === selection.bodyId) : undefined;
    const resolved = view.resolvedInstances?.find((candidate) =>
      candidate.documentId === selection.documentId && candidate.bodyId === selection.bodyId &&
      candidate.occurrencePath === (selection.occurrencePath ?? "") &&
      candidate.instancePath.segments.at(-1)?.resolvedVersionId === selection.versionId);
    const geometryKey = resolved?.geometryKey ?? body?.geometryKey ?? selection.geometryKey;
    const artifact = geometryKey ? view.artifacts?.[geometryKey] : undefined;
    const base = (import.meta.env?.VITE_API_BASE_URL ?? "").replace(/\/$/, "");
    return <><Descriptions column={1} size="small" bordered className="property-list" items={[
      { key: "body", label: "Body", children: body?.name ?? resolved?.name ?? selection.bodyId ?? "—" },
      { key: "owner", label: "所属 Part", children: selection.documentId ?? "—" },
      { key: "occurrence", label: "Occurrence", children: selection.instancePath?.display ?? "Part root" },
      { key: "revision", label: "Revision", children: selection.versionId ?? view.document.versionId },
      { key: "output", label: "最终输出", children: body?.consumed ? "已用于布尔" : "参与输出" },
      { key: "visibility", label: "定义显隐", children: body ? body.visible ? "可见" : "隐藏"
        : resolved ? resolved.bodyVisible ? "可见" : "隐藏" : "—" },
      { key: "geometry", label: "Geometry Key", children: geometryKey ?? "—" },
    ]} />
      {artifact && selection.documentId && selection.versionId && selection.bodyId &&
        <section aria-label="Part Files"><strong>实体与文件 / Part Files</strong>
          {Object.entries(artifact.representations ?? {}).map(([role, reference]) =>
            <div key={role}><Space wrap><span>{role === "VISUAL" ? "mesh.glb" : role === "NAMING" ? "naming.pb" : role}</span>
              <small>{reference.size.toLocaleString()} B · v{reference.schemaVersion}</small>
              <a download href={`${base}/api/documents/${encodeURIComponent(selection.documentId!)}/representations/${encodeURIComponent(reference.objectId)}?versionId=${encodeURIComponent(selection.versionId!)}&bodyId=${encodeURIComponent(selection.bodyId!)}&download=1`}>Download</a>
            </Space></div>)}
        </section>}</>;
  }
  if (["face", "edge", "vertex"].includes(selection.kind)) {
    const format = (value: unknown): string => Array.isArray(value)
      ? value.map((entry) => typeof entry === "number" ? formatDisplayNumber(entry) : String(entry)).join(", ")
      : typeof value === "number" ? formatDisplayNumber(value) : String(value);
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
  const referenceRows=referencePropertyRows(view,selection,diagnostics);
  if(referenceRows)return <Descriptions column={1} size="small" bordered className="property-list" items={referenceRows}/>;

	if (selection.kind === "sketch-constraint") {
		const sketch = view.part?.features.find((candidate) => candidate.id === selection.featureId)?.sketch;
		const constraint = sketch?.constraints.find((candidate) => candidate.id === selection.constraintId);
		return <Descriptions column={1} size="small" bordered className="property-list" items={[
			{ key: "type", label: "约束", children: constraint?.kind === "MIRROR" ? "关联镜像" : constraint?.kind ?? selection.constraintType },
			{ key: "status", label: "求解状态", children: sketch?.solve.status ?? "—" },
			...parameterItems(constraint?.parameterId),
		]} />;
	}
  const instance = selection.kind === "instance" ? view.product?.instances.find((item) => item.id === selection.id) : undefined;
  if (selection.kind === "visual") {
    const entity = view.part?.features.find(candidate => candidate.id === selection.featureId)?.sketch?.entities.find(candidate => candidate.id === selection.entityId) ?? view.sketchPatternMembers?.[selection.featureId]?.find(candidate => candidate.id === selection.entityId);
    const external = view.part?.features.find((candidate)=>candidate.id===selection.featureId)?.sketch?.externalGeometry
      ?.find((candidate)=>candidate.id===selection.entityId);
    return <Descriptions column={1} size="small" bordered className="property-list" items={[
    { key: "type", label: "类型", children: selection.visualType },
    { key: "entity", label: "元素", children: selection.entityId },
    { key: "feature", label: "所属特征", children: selection.featureId },
    { key: "role", label: "角色", children: selection.role ?? "—" },
    ...(entity?.sourceEntityId ? [{key:"pattern-source",label:"关联种子",children:entity.sourceEntityId},{key:"pattern-owner",label:"阵列",children:entity.createdByOperationId??"—"}] : []),
    ...(entity?.kind === "SPLINE" ? [
      { key: "spline-mode", label: "样条方式", children: entity.mode === "CONTROL" ? "控制顶点" : "插值拟合点" },
      { key: "spline-points", label: entity.mode === "CONTROL" ? "控制顶点" : "拟合点", children: (entity.mode === "CONTROL" ? entity.poles : entity.controlPoints)?.length ?? 0 },
      { key: "spline-basis", label: "曲线表示", children: entity.poles?.length && entity.knots?.length ? `有理 B-Spline · ${entity.degree} 次` : "待权威曲线计算" },
      { key: "spline-closed", label: "开闭", children: entity.closed ? "闭合（不承诺接缝切向连续）" : "开放" },
    ] : []),
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
      { key: "type", label: "类型", children: constraint?.family ?? ({COINCIDENT:"Coincidence",CONCENTRIC:"Coincidence",DISTANCE:"Offset",ANGLE:"Angle",FIX:"Fix",RIGID:"Fix Together",FIX_TOGETHER:"Fix Together",CONTACT:"Contact"}[constraint?.kind ?? "FIX"] ?? selection.constraintType) },
      { key: "activation", label: "激活状态", children: constraint?.suppressed ? "停用" : "激活" },
      { key: "mode", label: "模式", children: constraint?.mode ?? "DRIVING" },
      ...(constraint?.kind === "DISTANCE" ? [
        {key:"offset-definition",label:"驱动偏移",children:`${formatDisplayNumber(constraint.value ?? 0)} mm`},
        {key:"offset-normal",label:"符号基准",children:constraint.distanceRelation === "SELECTED_PLANE_NORMAL_V1" ? "所选平面法向；双平面取第一元素；第一位置 − 第二位置" : constraint.distanceRelation === "UNSIGNED" || !constraint.distanceRelation ? "无符号无限支撑距离" : "历史第二法向约定"},
        {key:"offset-parameter",label:"长度参数",children:(constraint.quantityParameter) ? `${(constraint.quantityParameter)!.key}: ${(constraint.quantityParameter)!.source.expression?.sourceText ?? "Literal"}` : "Literal"},
      ] : []),
      ...(constraint?.kind === "ANGLE" ? [
        {key:"angle-relation",label:"角度关系",children:constraint.angleRelation ?? "FREE"},
        {key:"angle-parameter",label:"角度参数",children:constraint.quantityParameter ? `${constraint.quantityParameter.key}: ${constraint.quantityParameter.source.expression?.sourceText??"Literal"}` : `${formatDisplayNumber((constraint.value??0)*180/Math.PI)}°`},
      ]:[]),
      ...(constraint?.mode === "MEASURED" ? [{key:"measurement",label:"测量值",children:constraint.measuredValue === undefined ? "不可测" : constraint.kind === "ANGLE" ? `${constraint.measuredValue*180/Math.PI}°` : `${constraint.measuredValue} mm`}] : []),
      { key: "evaluation", label: "Evaluation", children: constraint?.evaluationStatus ?? "NOT_UPDATED" },
      ...(constraint?.kind === "FIX_TOGETHER" ? [
        {key:"group-identity",label:"固联组",children:constraint.name ?? constraint.id},
        {key:"group-members",label:"成员",children:(constraint.groupMembers??[]).map(member=>member.groupId ? `组 ${view.product?.constraints?.find(c=>c.id===member.groupId)?.name??member.groupId}` : view.product?.instances.find(i=>i.id===member.instanceId)?.name??member.instanceId).join(" · ")},
      ] : [
        { key: "first-support", label: "第一支持", children: constraint?.first?.resolution?.result.supportingElementStatus ?? (constraint?.first?.persistentSelection ? "NOT_CONNECTED" : "CONNECTED") },
        ...(constraint?.second ? [{ key: "second-support", label: "第二支持", children: constraint.second.resolution?.result.supportingElementStatus ?? (constraint.second.persistentSelection ? "NOT_CONNECTED" : "CONNECTED") }] : []),
      ]),
      ...(constraint?.kind === "CONTACT" ? [{key:"contact-branch",label:"接触关系",children:`${constraint.contactKind} · ${constraint.contactSide} · branch ${constraint.contactBranch}`}]:[]),
      { key: "diagnostic", label: "Diagnostic", children: constraint?.evaluationSummary ?? "—" },
    ]} />;
  }
  return <Descriptions column={1} size="small" bordered className="property-list" items={[
    { key: "type", label: "类型", children: selection.kind.toUpperCase() },
    { key: "name", label: "名称", children: feature?.name ?? instance?.name ?? selection.id },
    ...(["pad", "import", "feature"].includes(selection.kind) ? [{ key: "contribution", label: "当前结果中的贡献",
      children: "尚无可靠的 Feature 贡献定位；选择保留在设计历史中，不将整个 Body 视为该 Feature 的几何结果。" }] : []),
    ...(selection.kind === "plane" ? [{ key: "plane", label: "基准面", children: selection.plane }] : []),
    ...(feature?.sketch ? [{ key: "entities", label: "草图元素", children: feature.sketch.entities.length },
      { key: "external", label: "外部几何", children: `${feature.sketch.externalGeometry?.length??0} · ${(feature.sketch.externalGeometry??[]).filter((item)=>item.status!=="CONNECTED").length} unresolved` },
      { key: "constraints", label: "约束", children: feature.sketch.constraints.length },
	  { key: "solve", label: "求解", children: `${feature.sketch.solve.status} · ${feature.sketch.solve.degreesOfFreedom} DoF` },
      { key: "profile-analysis", label: "轮廓", children: (() => {
        const feedback = sketchProfileFeedback(view.sketchAnalyses?.[feature.id]);
        return <Space direction="vertical" size={2}><span>{feedback.summary}</span>
          {feedback.issues.map((issue, index) => <Typography.Text key={`${issue.code}:${index}`} type="warning">
            {issue.message}{issue.position ? `（${issue.position.x.toFixed(2)}, ${issue.position.y.toFixed(2)} mm）` : ""}
          </Typography.Text>)}</Space>;
      })() },
	  { key: "support", label: "Sketch Support", children: `${feature.sketch.support.type} · ${feature.sketch.support.status ?? "—"}` },
	  ...(feature.sketch.support.type === "PLANAR_FACE" ? [
		{ key: "support-anchor", label: "Semantic Anchor", children: `${feature.sketch.support.persistentSelection?.anchor.featureId ?? "—"} · ${feature.sketch.support.persistentSelection?.anchor.outputSlot ?? "—"}` },
		{ key: "support-snapshot", label: "Support Snapshot", children: feature.sketch.support.dependencySnapshot?.manifestDigest.slice(0, 20) ?? "—" },
		{ key: "support-diagnostic", label: "Support Diagnostic", children: feature.sketch.support.diagnosticCode ?? "—" },
	  ] : [])] : []),
    ...(feature?.length ? [{ key: "length", label: "长度", children: `${formatDisplayNumber(feature.length)} mm` }] : []),
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

export function PartBodies({view}:{view:DocumentView}) {
  if (!view.part) return null;
  const base=(import.meta.env?.VITE_API_BASE_URL ?? "").replace(/\/$/, "");
  return <section aria-label="Part Files"><strong>实体与文件 / Part Files</strong>
    {view.part.bodies.map(body=><div key={body.id} style={{marginTop:12}}>
      <Typography.Text strong>{body.name}</Typography.Text>
      <Space wrap>{body.id===view.part!.activeBodyId && <Tag>活动实体</Tag>}
        <Tag>{body.visible ? "可见" : "隐藏"}</Tag>{body.displayFallback && <Tag color="error">计算失败 · 显示上次成功结果</Tag>}{body.consumed && <Tag>已用于布尔</Tag>}</Space>
      {body.displayFallback && <div>显示来源版本：{body.displayFallback.sourceVersionId.slice(0,12)}；修复失败特征后恢复。</div>}
      {Object.entries(view.artifacts?.[body.geometryKey??""]?.representations??{}).map(([role,ref])=><div key={role}>
        <Space wrap><span>{role==="VISUAL"?"mesh.glb":role==="NAMING"?"naming.pb":role}</span>
          <small>{ref.size.toLocaleString()} B · v{ref.schemaVersion}</small>
          <a download href={`${base}/api/documents/${encodeURIComponent(view.document.id)}/representations/${encodeURIComponent(ref.objectId)}?versionId=${encodeURIComponent(view.document.versionId)}&bodyId=${encodeURIComponent(body.id)}&download=1`}>Download</a></Space>
        <div title={ref.digest}><Typography.Text type="secondary">{ref.digest.slice(0,20)}…</Typography.Text></div>
      </div>)}
    </div>)}
  </section>;
}
