# 持久拓扑命名与引用恢复

本文拥有当前拓扑历史、PersistentSelection、Product 支持元素与 Reconnect 合同。先读身份链和源码路由，再按需求展开后面的具体实现。

## 身份链

```text
Feature / Body / profile entity stable IDs
  -> evaluator naming policy + digest
  -> OCCT Generated/Modified/Deleted evidence
  -> semantic topology outputs + TopologyHistory
  -> immutable topology manifest artifact + digest
  -> revision-local raw pick as creation evidence
  -> server-side PersistentSelection binding
  -> target Revision resolution
  -> Connected / NotConnected + ambiguity/tombstone evidence
```

- Persistent identity never uses OCCT pointer, traversal index, mesh primitive, coordinates alone or display name.
- A raw Face/Edge/Vertex local ID identifies only one geometry artifact and is accepted only as verified creation/reconnect evidence.
- Semantic outputs derive from stable Feature/Profile identities and actual OCCT history. Boolean/unify first preserves Generated/Modified/Unchanged/Split/Merged/Deleted evidence; a policy-versioned closure may name only otherwise-uncovered final intersection topology from already named semantic adjacency, without rewriting primary ancestry.
- Naming policy contains explicit length/angle tolerance and version digest. It participates in request, manifest and cache identity.
- Complete history covers every live final topology element exactly once and rejects conflicting live/tombstone or duplicate semantic/local identities.
- Ambiguity is a result, not permission to select the first candidate. Deleted topology remains a tombstone; repair is explicit Reconnect.

## 当前边界

Linear Extrude/Boolean、旋转、导入根及局部修改已有命名路径；各自依据真实历史或冻结身份提供完整性证据，详细范围见下文与[实体 Feature](solid-features.md)。`topology_history_complete=false` 的结果不可作为完整持久拓扑输入，不能仅按特征类别推断安全性。

## 源码路由

| Concern | Start here |
|---|---|
| OCCT-free topology contract | `kernel/api/include/occccad/kernel/topology_naming.hpp` |
| OCCT history generation/gates | search operation or semantic output in `kernel/occt/src/occt_kernel.cpp` |
| worker Proto manifest | `proto/occccad/worker/v1/geometry_worker.proto` |
| artifact adoption/digest checks | `services/internal/workspace/service.go` and geometry adapter symbols |
| binding and cross-Revision resolution | `services/internal/workspace/topology_selection.go` |
| Product supporting-element state | workspace assembly solve/structure projection and Web constraint UX |
| conformance | `kernel/occt/tests/geometry_exchange_scenarios.cpp`, workspace topology tests, control integration tests |

Do not start from generated protobuf or STEP corpus bytes. Begin with the stable semantic output or failing selection and expand to the exact OCCT/test range.

局部修改的隔离副本不算语义修改。倒圆角合成历史前，以原生拓扑身份核对实际结果成员：OCCT `IsDeleted` 对仍存活的 Edge/Vertex 返回删除时，不把该结果转为 tombstone；真实 Modified/Generated/Deleted 继续保留。经过副本映射且未改变的成员维持上游语义引用及 UNCHANGED，不因副本 TShape 不同而计入圆角贡献。邻接闭包只处理尚未覆盖的真实新增拓扑，不能重新接管这些存活成员。

回归入口：`GeometryExchange.FilletPreservesUntouchedExtrudeEdges`、`services/internal/control/fillet_naming_integration_test.go`；检查三条未选竖边的来源/UNCHANGED、显示贡献、半径编辑、历史与冷重建，以及下游倒角拾取。

## 失败门禁

- Geometry may be numerically identical while Feature identity differs; cache/naming digest must keep them distinct.
- Adopting an external manifest requires agreement between inline response, referenced immutable object and adopted digest.
- Resolver input freezes source/target Revision and policy evidence; resolving against “latest” silently is invalid.
- Reconnect stores a new stable binding through one Domain Command; it does not persist the raw local ID as long-term identity.
- Product/UI presents supporting-element connectivity separately from constraint solve status.

## 验证入口

Use an exact geometry/workspace test first, then:

```bash
invoke check --scope geometry
invoke check --scope workspace
```

Changes spanning Proto/Worker/Router/Product require `invoke check --scope all` and a test through the formal Router. Only read `tests/test.data/` when the regression explicitly requires real STEP/BREP input.

## 当前实现

> 返回[当前架构目录](README.md)。代码和测试定义当前事实；验证范围见[验证说明](validation.md)。

服务端已实现 PersistentSelection bind/resolver。bind 只接受 source Revision 指定 sourceBodyId 的 Body Tip 的 local pick，并固化 semantic anchor 与 creation evidence；resolver 固定校验 source/target Revision、document/body、expected type、creation evidence、manifest digest 和 policy digest，沿 lineage 返回唯一解析、缺失、歧义、类型不符或当前 tip 外等状态，绝不以相同 local ID 或最近几何自动选面。解析缓存以 selection、target Revision、manifest/evidence/policy digest 为身份，并可从不可变 topology artifact 冷重建。右侧属性面板现在展示 semantic anchor、selection recipe、supporting-element 状态和 evidence digest。

拓扑 endpoint 持久化 occurrence、source Part Revision 和 PersistentSelection，并保存固定 target Revision、topology manifest/policy digest 与 resolution result；`geometryKey + localId` 只作为创建或 Reconnect 时的瞬时 pick evidence。默认实例使用 `FOLLOW_HEAD`；Part Head 更新后 Product Update Plan 投影 NotUpdated/UpdateAvailable，Web 自动批量推进 Revision、重新解析 endpoint 并求解，用户可用 `PINNED` 显式截断自动跟随。Supporting Element 的 Connected/NotConnected 与 Constraint 的 NotUpdated/Broken/Impossible/Verified 是两个独立状态域：解析失败的约束不会进入 Solver，其余 connected component 仍经正式 Router 的静态层级求解路径求解。结构树和属性面板显示状态与 provenance，持久 ChangeSet 支持刷新及补偿式历史。

约束恢复复用这些状态。创建和编辑使用同一非模态约束定义面板，服务端成功 preview 除 occurrence poses 和 component 诊断外，还返回候选 Constraint 状态与两个 Supporting Element 状态；已连接支持元素在 `SOLVING` 阶段的不可重试结构化失败明确投影为 Impossible，基础设施或可重试失败保持 NotUpdated，前端不从颜色或普通异常文本猜测领域状态。结构树双击/右键 Edit 打开同一编辑器，Broken 节点提供 Reconnect，非 Verified 节点提供 typed `UPDATE_REFERENCES` Refresh。Reconnect 复用一次性 Selection Tool 和现有 preview actor，替换端点后立即权威预览，确认以一个 `EDIT_ASSEMBLY_CONSTRAINT` Transaction 提交。结构树状态包含图标、文字和可访问标签；视口的 NotUpdated、Impossible、Broken 各用不同的屏幕稳定 SDF glyph，精确拓扑锚点丢失时回退到 occurrence 中心，确保 Broken 约束仍可被选择并修复。

既有圆形切除测试使用 `REMOVE` Linear Extrude；独立 Hole 的范围与验证入口见[实体 Feature](solid-features.md#独立-hole)，两者复用同一命名链。XZ 矩形贯穿 Cut 的 OCCT corpus 固定验证六个基础面语义引用继续存在并新增四个孔壁引用；同一 corpus 还覆盖保留面、删除面与侧开口 split 歧义。Go 端可执行 fixture 经真实 Workspace、当前配置的 SQLite/PostgreSQL、ArtifactStore、GeometryPool、正式 Router 和 C++ Worker 创建 Part/Product 面约束，覆盖通孔更新、Part Undo/Redo、真实删除、Broken 隔离、Reconnect 后继续编辑以及 ambiguous 不自动选择，并重新执行保存的 `.3dreplay`。编辑任何 Broken 约束会先把 evaluation 重置为 NotUpdated，使当前命令的权威解析与求解能够恢复到 Verified；最终 Revision 才保存求值结果。fixture 可通过 `OCCCCAD_P6_EVIDENCE_DIR` 输出逐阶段 ResolutionSnapshot 摘要和 solver replay。

Boolean semantic adjacency closure 受版本化 evaluator 与 policy digest 约束，旧制品不能冒充新 manifest；准确版本由[命名策略源码](../../../services/internal/modelcore/topology_naming.go)拥有。Linear Extrude 除 cap/side Face 外，还从 profile curve entity 与共享 endpoint identity 生成 start/end cap boundary Edge、vertical Edge 和 start/end Vertex；Boolean 与 same-domain history 优先按原拓扑类型传播，并只为 OCCT 未提供同类型 history 的最终交线/端点执行可审计 closure。Edge evidence 包含解析曲线类型、SI 长度、质心、原点/方向、参数区间与端点角色，Vertex evidence 包含稳定点坐标与端点角色，三类 output 均带跨类型邻接语义引用。声明完整的 Shape gate 要求最终 Face、Edge、Vertex 各自被唯一 semantic output 和 lineage 覆盖；矩形拉伸基线为 6/12/8，长度编辑、面上 boss/pocket、ADD/REMOVE、贯穿孔、多区域/圆环/Arc/Spline profile、edge merge/split、vertex delete、圆环 seam 和容差以下短边均有确定性 corpus。Worker Proto 保留真实 topology type。服务端把 Vertex 解析为精确 Point、线性 Edge 解析为 Axis，并经正式 Router 验证 Vertex-Vertex、Vertex-Plane、Edge-Edge、Edge-Plane 的创建、更新、Broken 隔离和 Reconnect；缺少 manifest 与不完整 history 使用稳定诊断，并在没有可求解 component 时不产生 replay。

## History 生成与制品门

Persistent topology naming 已具备 Linear Extrude/Boolean history 生成与服务端 PersistentSelection bind/resolver。Profile Pad 求值必须携带稳定的 Feature、Body、输入 Feature 和 Profile Feature identity，以及固定版本 naming policy 的 digest 和以米、弧度表达的匹配容差；Part `geometry_key` 同时包含这些 identity、完整 Feature 输入和 policy digest，不能让几何相同但业务身份不同的 Feature 复用 topology manifest。Linear Extrude 从 profile region/entity ID 生成 start cap、end cap 和 side semantic outputs；OCCT adapter 保留 Prism、Fuse/Cut/Common 与 `ShapeUpgrade_UnifySameDomain` 的历史对象，将 Generated/Modified/Unchanged/Split/Merged/Deleted 折叠为逐 Feature `TopologyHistory`。Boolean/同域 history 后还会对未覆盖的合法最终拓扑执行 semantic adjacency closure：Face 优先读取已命名边界 Edge，Edge 读取 incident Face，Vertex 读取 incident Edge；派生 identity 绑定当前 Feature 与排序后的稳定 source ref，数值 evidence 只在相同 source set 内确定次序，不能以 local ID 或坐标单独补名。每个 live Face/Edge/Vertex 输出包含最终 GeometryId 内的 typed local ID、geometry evidence 与排序后的相邻语义引用；完整 history 的 shape gate 要求最终每个元素恰有一个唯一 semantic output 和 lineage result，并拒绝 live/tombstone 冲突、无稳定来源或无法消歧的派生候选。history digest 覆盖 source/result、lineage kind、歧义、删除、证据、邻接和 policy。Worker 在 `PartEvaluationManifest` 只返回轻量 Feature identity、策略摘要和 ArtifactReference；完整命名写入带 SHA-256 的 `naming.pb` v2。控制面核对 Worker reference、已采用对象摘要和冻结 Geometry 身份。`topology_history_complete=false` 是显式契约：未命名的导入 B-Rep 会返回诊断，不能被下游当作完整 history。服务端只允许从 source Revision 指定 sourceBodyId 的 Body Tip 的 local pick 创建带 creation evidence 的 `PersistentSelection`，并按固定 source/target Revision、manifest/policy digest 与 semantic lineage 解析；歧义不会自动选取。Product topology endpoint 已升级为 occurrence + source Revision + PersistentSelection；revision-local local ID 仅作为瞬时 pick evidence。

## 实现与验证入口

- [命名 policy](../../../services/internal/modelcore/topology_naming.go)
- [bind/resolver](../../../services/internal/workspace/topology_selection.go)
- [resolver corpus](../../../services/internal/workspace/topology_selection_test.go)
- [正式 Router Edge/Vertex 集成](../../../services/internal/control/edge_vertex_naming_integration_test.go)

## 导入根命名

有效 Solid/Compound 的 STEP/BREP Definition 生成完整 Face/Edge/Vertex 根命名。XDE Definition 保持一个 Part；多 Solid 不再触发业务拆分或伪造装配。导入 Definition 先检查有效性，必要时在拷贝上运行 ShapeFix（mm：precision 1e-6、min 1e-7、max 1e-3），针对相邻圆柱面拆分造成的周期参数边界未闭合，仅在参数坐标系一致、两面各只有一条三维闭合边界且仅共享一条边时合并面片；不拼接近似同轴曲面。修复后再次检查有效 Solid 集、数量保持且无游离拓扑，再冻结实际 BREP SHA-256；修复失败返回组件诊断，不丢弃坏实体。准备与修复发生在身份分配之前，已冻结的历史快照不重写。ImportExchange 负责规范化精确快照，提交 ImportBody 前控制面分配独立随机 topology ID，写入不可变 `identity.pb` Artifact，并由 `import_definitions` 索引；Feature 的 `importDefinitionId` 引用该重放输入。定义记录绑定 Feature/Body、原始源对象和源 Definition ID（新 Jobs 导入）、精确 BREP digest、OCCT 版本、导入策略及毫米单位。旧文档修复可仅依据其已冻结的精确 BREP，不虚构已丢失的原始源文件。

身份分配先按文档/Feature/策略冻结；并发或失败重试读取同一获胜记录，不能重新分配已发布身份。随机 ID 不由 local ID、数组位置或几何坐标派生。定位映射只在冻结 BREP 摘要内有效，Worker 同时校验 OCCT 版本/策略，内核校验摘要、有效 Solid 集与无散落拓扑、覆盖、重复身份与 locator。更换输入快照不按编号或最近几何继承身份。定义表及其引用的源对象、精确几何是重放输入，不能作为显示缓存回收；几何/邻接 evidence 在由该冻结映射生成的 topology manifest 中物化。导入根的邻接以 Face→Edge、Face→Vertex、Edge→Vertex 的包含索引，以及共享边/顶点的反向索引生成，避免对全部拓扑做两两扫描；身份覆盖/重复检查使用集合，输出仍按原 canonical 顺序排列，不改变引用与历史语义。

EvaluatePart 的 `import_seed` 初始化导入根 FeatureResult 和已有 named topology，再复用原生 Add/Remove/Intersect、OCCT history 与同域合并传播。初始输出有完整邻接、类型、几何 evidence 和有向法向；后续 split/merge/deletion 使用同一 resolver，分裂多候选保留 Ambiguous。缓存身份包含完整 seed，依赖图将 ImportBody 作为 Body Tip，后续草图/实体读取其命名和几何。

已有无定义的导入由 `REPAIR_IMPORT_NAMING`（typed URI `occccad://part/exchange/repair-naming`）显式建立命名。工作台告警提供“建立导入命名”入口。修复保留原 Feature 和 BREP，通过正常 Transaction、CAS、entity ChangeSet 和新 Revision 提交；Undo/Redo 使用同一冻结定义，旧 Revision 不改写。已命名 Feature 不能以修复操作重新分配身份；缺失定义的历史快照仍使用 [IMPORT-DIAGNOSTICS](jobs-artifacts.md#持久任务与制品) 的明确诊断。

当前回归入口包括 kernel/occt/tests/geometry_exchange_scenarios.cpp、services/internal/control/import_naming_integration_test.go、import_naming_diagnostics_test.go 和 workspace/import_naming_test.go。开放壳和混合散落拓扑不属于此 Solid/Compound 导入能力，源文件替换/新版内核迁移也未实现自动身份对照。

完整 Naming 的唯一持久载荷为 `naming.pb`；RPC 和数据库不重复保存 FeatureResults。导入 identity 也通过 ArtifactReference 重放。数据边界见[几何制品](geometry-representations.md)。

当前持久格式见 [Visual/Naming Artifact v2](visual-naming-artifacts.md)。SemanticRef 与 typed Evidence 通过共享表引用，Body 显式拥有 transition 链和 tip locator 索引。resolver 的历史存在性来自 transition 生成/删除记录，不依赖每个 Feature 的完整快照；领域 FeatureResult 仅作为消费者内存对象，绝不重复持久保存 v1。

每个 Body 独立 Naming Artifact；bind 先验证 GeometryKey 属于指定 Revision/Body，再解释 localId。resolver 在 source/target Revision 中分别寻找同一个稳定 Body，不会扫描兄弟 Body 的 Naming 作为回退。Loader 同时验证 GeometryId、BREP SHA-256 及单 Body transition 归属；Body 删除后引用返回不可用，不按同名 Body 或相同 FACE 1 自动重连。

实体 evaluator 支持旋转扫掠历史、跨 Body 工具快照和局部修改/放样历史。工具的 Naming 与 BREP 摘要、Body 和 Feature 阶段必须一致。抽壳区分原边界延续与新增偏置边界，见[抽壳历史归属](shell-algorithm.md#拓扑历史与归属)；真正的分裂无法消歧仍失败。通用合同见[实体 Feature](solid-features.md)。

## 阵列复制与选择延续

`LINEAGE_DESCENDANT` 跟踪所选拓扑的实际延续，沿 Modified/Unchanged/Split/Merged 解析；`GENERATED` 表达构造来源，不是原拓扑身份的延续。因此抽壳内壁、圆角生成面和 `MEMBER/slot/seed-digest` 刚体副本不会成为原面／边选择的候选，原拓扑消失时也不能跳到新生成拓扑。显式选中的内壁或 MEMBER 以自身语义引用为锚点，继续跟踪后续延续；真实分裂仍保留 Ambiguous。引用解析缓存包含解析策略版本，不修改既有 Naming/BREP 制品或持久引用格式。

Feature 的当前贡献与树／视图联动使用 `featureAssociations.primary`，只突出其实际拥有的结果；`origins` 保留复制来源链用于溯源，不能把祖先链当作当前归属而连带选中其他副本。成员槽位及持久拓扑选择合同保持不变，不按坐标挑选候选。回归见 `TestResolveManifestGeneratedBoundaryIsNotSourceContinuation` 和 `TestSketchOnShellOuterFaceThroughRouter`：外面与内面独立绑定、外面支撑的草图／拉伸、壁厚更新、历史及冷重建；视图区流程见 `solid-feature-live.spec.ts`。

解析直线样条的 evidence 与类型查询共用完整裁剪区间控制点证明；origin/direction 和 parameterStart/End 表达毫米直线区间，底层 BREP 原生参数仍在属性查询中报告。该识别不产生新拓扑身份或几何替换，详见[实体 Feature](solid-features.md)。

拔模的实际角度面使用 `DRAFTED_FROM` 输出归属拔模，来源仍为原面的 `Modified` 延续；显示主归属采用该正式角色，溯源保留原生成步骤。相邻未变拓扑和一般裁剪面的主归属不随之改变。范围复制只消费[精确净新增材料](solid-features.md#参数化阵列)，不把拔模声明为独立工具。
