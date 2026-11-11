# STEP/BREP 导入与导出

> 返回[当前架构](README.md)。任务、Outbox 与制品归属见 [Router/Jobs](jobs-artifacts.md)；外观见[颜色材质](surface-appearance.md)，独立结果见[线框与曲面](wireframe-surfaces.md#开放几何交换)。

入口：`api/jobs.go` → `occccad-jobs/import.go` / `main.go` → `workspace/exchange_graph.go` → `geometry_worker.proto` → `workers/geometry/src/main.cpp` → `kernel/occt/src/exchange_graph.cpp`。原始文件、BREP、GLB、Naming 均使用统一 ArtifactStore；参数模型和不可变 Revision 是业务真相。

## 导入合同

HTTP 完整保存输入 Artifact 后入队 EXCHANGE_IMPORT。`InspectExchange` 用 STEPCAF/XDE 一次解析源图，每个唯一 Part 输出本地坐标的规范 BREP；Jobs 有界并行求值并创建 Part，再按依赖顺序用正式实例命令创建 Product。Definition-local BREP 不混入 occurrence placement。纯实体进入 IMPORT_BODY；点、线、面和混合几何进入独立 GEOMETRY/IMPORTED 结果，不制造材料 Body。

同一源 Definition 的多个 occurrence 引用同一文档。相同 BREP 的不同源 Definition 仍是不同文档与独立 Naming 根。临时 XDE label 只在该文件的准备图中标识 Definition；持久引用使用分配的 stable ID、PINNED Revision、InstancePath 和冻结 import identity。多 Solid 的一个源 Definition 保持一个 Part，不按 Solid 数虚构装配。名称、平移、单位四元数、嵌套层级、整体/分面 RGB 和透明度进入已有领域模型。同级源名冲突时分配确定性后缀并保留 importedName；用户未重命名时导出恢复源名。

**可读取但不支持的内容不导致整个文件失败。** PMI、图层分组和离散网格不创建对应领域数据。OCCT 能读、但无法进入当前有效 BREP/Naming 模型的 Definition 保留诊断并建立空 Part，其他 Definition 继续。全文件没有可创建几何时，也通过幂等 CreateDocument 创建空 Part，不伪造 Feature、GeometryKey 或拓扑身份。原始输入仍由 Job 的 Artifact 引用保留。无法解析、转换异常、存储失败或取消继续返回失败/取消，不能转成成功警告。

源 reader 关闭 GDT/view/material 和离散几何转换，避免构造无领域承接的数据。底层 STEP 实体及引用仍由 OCCT 解析。离散网格未转换为精确 BREP：例如 NIST FTC-08 AP242-E1-TG 会产生空 Part 和明确警告，这不代表其几何已完整建模。开放/混合结果的材料体积仍遵循原领域边界，不把 MIXED_SET 的零存储值解释为真实材料体积。

外部引用不读取源文件指定的路径或 URL。standalone STEP reader 保留文件内嵌几何为一个 Part，并警告外部内容、装配层级和样式没有导入；无内嵌几何时建立空 Part。图验证继续拒绝悬空/重复身份、环、非法 pose、不可达定义及资源超限；深度最多 128、Definition 最多 100000、总引用最多 1000000，现有 Product 展开预算独立生效。

修复在分配持久身份之前执行。无法证明分面颜色与修复后拓扑一一对应时，保留几何和整体外观，跳过分面赋值并警告；不按 ordinal 绑定修复后的面。已冻结 identity 与 BREP 不一致仍是错误。根可直接由 Part 内容表达时返回 Part；多个根、非单位根 placement 或 Product 根状态使用容器 Product。

## 完成报告和通知

`payload.exchange` 记录 schemaVersion、源 SHA-256、FILE_SCHEMA、实际 OCCT 版本、Definition/空 Part 数、结构化 warnings 和 prepare/evaluate/createParts/assemble/total 的毫秒耗时。warning 有 code、message、definitionId、count 和原生 detail。成功状态、报告和 JOB Outbox 在同一事务提交；取消或丢失 lease owner 不能发布成功。

实时 Job 携带文件名和按 code/message 归并的 warnings；详细 Definition 诊断保存在报告中。成功警告在通知中显示内容，消息中心从 GET /api/jobs 恢复，刷新或重连后仍可查看、下载或打开文档。Outbox 批次只为当前在线用户选择待发布 Job，离线用户的旧事件不会耗尽在线用户的批次配额；离线事件保留到重连。空 Part 与普通成功文档使用相同打开入口，不变成可重试失败。导出也可携带成功警告。

## 导出合同

任务捕获根 Revision，递归读取 Product 已接受的 Part/Product 引用和 READY ContextVariant 的权威 Bodies；不以扁平显示列表代替上下文真相。Head 已变或所需上下文未 READY 时明确失败，避免混合快照。API 的用户请求 ID 重复绑定不同输入、Revision 或选项返回冲突；自动导出键包含用户、Revision、格式及选项。

`excludeHidden` 默认 true。后端按保存的语义树解析 Body、独立 FeatureResult、几何图形集祖先和 Product occurrence 的 SHOW/HIDE/INHERIT，子对象 SHOW 不能越过隐藏祖先。过滤发生在 BREP/外观加载前。已排除的隐藏失败对象不阻止有效选择，所选对象失败则明确拒绝，不导出旧显示回退。编辑时的临时显示和完全透明不等同隐藏。false 包含隐藏对象；suppressed、consumed 和无有效结果的对象始终排除，无可导出内容返回 EXCHANGE_NO_EXPORTABLE_CONTENT。

当前 OCCT 不能完整往返隐藏状态，尤其是 Compound 的子形体与装配实例。包含隐藏对象的 STEP 会携带 STEP_VISIBILITY_LIMITATION，完成通知说明重新打开后可能显示这些对象。导出选择已在后端执行，不依赖接收端支持隐藏状态。

STEP 默认 AP242，也可选 AP214IS。writer 使用请求局部 DESTEP_Parameters，明确毫米、AsIs、subshape names 和精确 BREP 输出，不依赖全局 Interface_Static。OCCT 7.9 的 AP242DIS 输出属于几何/样式交换，不承诺完整 Edition 2/3 或 PMI。STEP 保留 Part/Product 图；原生 BREP 仅按 placement 合成所选 Compound，不携带名称层级和外观。

导出 Definition 按 document + revision + 实际上下文几何、外观、显隐和子定义结果复用；不同有效外观需要临时变体时不改写业务模型。XDE 使用独立 label 和直接拓扑副本隔离不同业务定义，避免相同 BREP 被合并。不再把 BREP 序列化后读回 OCCT。

## 时间性能与边界

短期交换 Kernel 保留整个操作图，不受长期 Shape 缓存的 16 项淘汰影响，也不在每次插入时重新序列化全部驻留形体。一次请求内复用已解析的不可变 Part/Product。重试和重复导入可以复用已发布的源求值；键包含源、Definition、规范 BREP 摘要、实际 OCCT 版本和求值策略，文档仍分配独立命名身份。准备图仍会在每次尝试解析，未交付跨进程整图检查点。

体积积分在私有拓扑/几何副本上拆分样条曲面的区间，再使用原有 OCCT Gauss–Kronrod，维持 1e-10 请求精度，不以显示网格体积替代精确几何。拆分不改写业务 BREP、面身份或 Naming。耗时以报告和真实 Worker 日志为准；请求精度不是一般模型的严格误差保证，小样本不作为工业容量或 P95 声明。

Product 的独立结果通过 ReferenceDisplayScope.results 显示，保留 document/revision/InstancePath/featureId/resultId；不混入实体 Body 或机构求解的 ResolvedInstances。视区和缩略图消费同一结果及解析外观。READY ContextVariant 的独立结果求值、完整跨零件消费和测量仍是独立后续工作。

## 验证入口

- [NIST STEP corpus](../../../tests/exchange-corpus/README.md)：来源清单、SHA、49 份不同文件及完整 Jobs 导入。
- `services/cmd/occccad-jobs/exchange_corpus_integration_test.go`、`export_integration_test.go`、`import_integration_test.go`：真实 Worker/Router、空文档、警告、共享定义、显隐、协议及重试。
- `kernel/occt/tests/geometry_exchange_scenarios.cpp`、`exchange_mass_scenarios.cpp`：32 个唯一 Definition、外部引用隔离、颜色/透明度、解析体积/质心、尺度和 Loft。
- `web/apps/cad/browser/exchange-live.spec.ts`：真实上传、完成通知、消息中心恢复、导出选项/下载和 Product 独立结果显示。

这些入口不等于已经执行。当前运行报告在 build；外部 CAD 阅读器、PostgreSQL、完整 AP242 conformance 和超大装配容量需分别验证。
