# Part Feature 求值与草图上下文

本文拥有实体求值阶段顺序及 Sketcher 的显示边界。Part/Body/Sketch 模型见[Part/Sketch](part-sketch.md)，各特征范围及失败 Revision 合同见[实体 Feature](solid-features.md)，命名历史与解析规则见[Naming](persistent-naming.md)。

独立曲线/曲面共用 Part DAG、参数、候选与制品流程，但不执行材料布尔及 Solid 体积门禁；其域、归属与限制归[线框与曲面](wireframe-surfaces.md)。`THICKEN` 将有界 Sheet 显式实体化后进入下述原 Body 管线，后续修改继续使用同一门禁。

## 权威求值链

```text
typed Feature definition
  → 解析参数、上游阶段与稳定引用
  → 精确 Profile / Tool Shape
  → BodyOperation + generator / Boolean history
  → same-domain normalization + history
  → BREP / Body policy validation
  → semantic adjacency closure
  → complete-shape Naming gate
  → 不可变 BREP / Visual / Naming 制品
  → evaluator 最终模型的 ChangeSet / CAS
```

OCCT IsDone 不是接纳条件。结果须非空、有效、满足多 Solid Body 规则，并让每个最终 Face/Edge/Vertex 都有唯一语义输出及 lineage。生成、Boolean 和同域合并先保留同类型真实历史；仅未覆盖的最终交线/点等可进入有版本的语义邻接闭包。完整规则统一归 [History 生成与制品门](persistent-naming.md#history-生成与制品门)。

成功闭包可以报告 `TOPOLOGY_HISTORY_DERIVED_CLOSURE:<count>`；缺失、重复、悬空、类型不明及 live/tombstone 冲突仍拒绝。Worker 保留 Feature、求值阶段与稳定错误码，不能通过丢弃历史或接纳无名实体恢复。

创建与编辑的失败接纳规则、FAILED/BLOCKED、displayFallback 及预览资格统一见[失败和预览](solid-features.md#失败和预览)。局部特征的逐项输入状态、显式替换/移除、必需输入门禁及定向证据统一见[输入修复边界](solid-features.md#输入修复边界)。

[调整特征顺序](solid-features.md#调整特征顺序)和[历史位置创建](solid-features.md#在历史位置创建)按候选列表重建 Body 前驱和全 Part 依赖，使用候选阶段输入与缓存；不沿用旧图的隐式前驱边，不自动搬移或改绑输入。两者都必须通过全 Part 引用与几何检查；插入直接创建完整候选，不拆成创建后移动两笔事务。

## 零件增量求值

普通 Part 命令在求值前使用既有 DAG、已接纳 manifest、evaluator/单位及节点输入 digest 形成请求内执行计划。真实变更及依赖闭包驱动草图/Datum 准备、独立几何和 Body 执行；删除末端 Feature 另检查 Body 执行成员顺序，不能因节点已不存在而漏掉更新。未变支撑及外部投影仅在定义、策略和全部上游依赖不变时复用。Undo/Redo 的补偿求值使用相同依赖范围，避免无关支撑被重新绑定到另一显示变体而破坏下一次历史摘要校验。冻结 Revision 的既有 Body Tip 可直接作为源阶段，历史中间阶段仍按其合法边界求值。`ForceCold`、缺少合格 manifest 及上下文变体继续使用完整路径。

受影响的面支撑仍重新解析。若恢复后只是 exact-only/显示制品的 GeometryKey 不同，仅当旧制品也通过完整 Naming、BREP 归属校验，且 manifest、策略及 evidence 摘要与新解析相同时，保留原冻结依赖快照。真实源变化继续更新；不以相同坐标或单面的几何相似性代替证明，也不放宽 ChangeSet 的冲突校验。

未被三维结果消费的 `EDIT_SKETCH` 预览只产生草图候选；提交携带同一候选、操作 ID 和基础 Revision，复用仍通过原候选提升/CAS 门禁。被实体或曲面消费的轮廓修改继续执行真实下游。Body 制品不携带草图图元；DocumentView 和冻结 occurrence 的 `sketchVisualization` 从对应 Revision/变体投影，缩略图与视口使用同一归属及可见性。未变化的 BREP/NAMING/VISUAL 引用不因草图绘制更新。

Body、Stage 和按面网格缓存的费用复用已序列化 BREP 的保守估算，不为记账重新编码驻留形体并集。Stage 历史账本按缓存根及分叉共同保留的不可变前缀并集计费，根租借与前驱边分别计数；新增/释放只遍历新接入/失去最后所有者的前缀尾部。共享历史不因多个检查点重复收费。形体及构造上下文估算仍可能重复计费；缓存预算不是全进程 RSS 上限。输入 BREP 在请求与有界缓存间共享不可变字节。

导入件的首次新操作可从该 Body 已接纳的不可变 Naming 制品恢复 ImportBody 根 transition。输入绑定冻结 BREP SHA-256、Feature/Body、导入身份分配、OCCT/策略版本、完整历史及正体积；体积来自原始导入制品，不读取已切除后的 Body Tip。Worker 验证制品摘要和归属，内核逐项验证 typed locator 与随机身份覆盖。没有可恢复的根时执行完整初始化；`ForceCold` 显式绕过恢复和阶段缓存。此优化不需要曾经计算过同一个切除结果。

输入与工具均为单 Solid 时，ADD/REMOVE 复用本次 Boolean 的交集分区，在新增或移除的材料区域执行原精度体积积分；以输入体积加减变化量得到结果体积。无材料变化仍按原阈值拒绝，边界接触不作为材料，空结果仍拒绝。差值接近整体数值抵消时，回退为结果整体积分。多个 Solid（包括重叠分量）的补集可能改变有向体积的分量重数，保留原输入/结果整体积分；INTERSECT 也保留输入/结果体积检查。结果仍执行有效 Solid 集、无游离拓扑、同域合并及完整 Naming 门禁；已验证的同一结果不重复检查。

一次请求内按精确 OCCT occurrence（包含朝向）复用质量与几何证据；同一语义引用且拓扑未变时可沿用已接纳证据，邻接按本次结果重建。最终邻接使用包含/共享边界索引，不做全部拓扑两两扫描。语义引用索引按完整 typed ref 查找，并保留重复候选的原顺序；不在每个输出/lineage 上线性扫描全部输入。上述索引不是持久身份，规则见 [Naming](persistent-naming.md)。

前缀检查点带输入与策略 digest、历史、质量、命名及构造上下文。`FrozenFeatureResult` 用持久 AVL 序列共享完全相同的 lineage 行及连续片段，历史输出以紧凑绑定借用行中的完整引用/证据；例外输出原样保存。 完整有序 source ID 列表按值与前驱共享，只读复制不再深拷贝字符串数组；写访问分离，已有可变借用也不污染副本。历史计费按新保留的行、来源存储及最终树节点计算，不累计已释放的 join/rotation 临时节点。`FeatureResultSequence` 与检查点共享冻结记录，恢复仅读取 metadata 和借用证据，不展开全部历史。需要完整 `FeatureResult` 的读者按需展开并持有租借，检查点只保留弱展开引用；淘汰不使读者失效。完整历史、输出顺序、删除、歧义及每个阶段的 semantic outputs 继续可读。工具、Body 阶段及范围基底的构造上下文共享不可变 ToolBuild，复制阶段时只复制索引；实际需要重执行的工具再按需复制。持久 Naming 的完整历史、顺序和摘要保持原合同。完整工具上下文会挤出最近前驱时，优先精简当前检查点；必要时也精简前驱，使两者在原预算内共存。精简不改变 Shape、历史或质量。缺少的构造上下文状态随普通后续步骤传播，不能被误标为完整；只有尚未执行的后缀包含 `SOLID_PATTERN` 时才要求完整上下文，否则允许复用已完成阵列之后的精简检查点。新阵列确需缺失来源时仍重建完整上下文。取消后不接纳检查点；淘汰与冷重建不改变结果语义。

历史摘要排序借用记录并预计算排序键，避免深拷贝证据/邻接；canonical 历史摘要、身份、单位与匹配容差不变。Worker 的公共值编码器接受原生只读记录及 protobuf 输入，只按完整 typed ref/evidence 值 intern；新增 Evidence 字段必须同步完整值比较/散列和冻结旧编码器的逻辑字节对照。持久 Naming 已改为 v3 根及不可变历史块：根只保存当前 tip locator，块保存变化行及无损 CARRY 布局，见[文件合同](visual-naming-artifacts.md#naming-parttopologymanifest-schema_version-3)。新增或编辑末端共享既有块，Worker 的 32 MiB 编码 memo 仅优化转换；`naming_source` 是已接纳根的发布复用提示，校验归属后省去已持久块的重复写入，不能参与或替代精确求值。

Boolean 和剖分通过 OCCT progress 协作取消，算法内观察本地单调取消标记，真实 transport 状态至多每 16 ms 轮询一次；检查点/网格接纳和结果发布边界强制查询真实状态，制品提交仍检查取消和过期资格。单次几何积分及部分构造器仍可能在返回后才响应取消，不承诺所有原生算法立即中断。

`PartRuntimeStats` 分开记录 base、exact、Naming、display、mesh、encoding、邻接、积分次数及证据复用；Naming 再区分内核与制品装配，另记真实取消查询、网格复用/计算面数和缓存记账耗时/编码次数。GetTopology 另报排队、精确加载与查询时间，客户端接入请求计时。mesh/encoding 属于 display，缓存记账也包含在 RPC 总时长内，不能重复相加。指定复杂导入件及生成 STEP 的测量与边界见[零件性能测量](../../validation/part-feature-performance.md)。

### 通用特征性能与增长边界

新增 Feature 应接入既有依赖执行计划、规范输入/策略键、不可变结果、完整 Naming、显示制品及接纳门。Body 链共享前驱检查点；独立曲线/曲面共享结果容器、摘要、编码与服务端解析。公共层不按“孔/切除”选择是否优化；具体生成器仍拥有精确算法及真实 history。冷算、取消、淘汰、上游修改、重排/删除、阵列上下文、补偿历史及过期候选必须分别验证。公共层优化不意味着每类精确构造具有相同加速比。

设第 i 步最终拓扑数为 Tᵢ，完整逻辑历史仍有 H=ΣTᵢ 行。原生层共享未变化的行/片段，文件层用 CARRY 表达同引用、同完整证据的自延续；只读查询使用有序事件及独立当前 tip，不先展开 H 行。新增 Feature 自动复用结果容器、编码、发布、依赖闭包与查询设施；精确构造和本步完整拓扑验证仍随本步几何量增长，不承诺常数时间。

真正改变的证据、邻接、分裂/合并、删除和歧义均须保存；变化量本身可能很大。每块使用局部引用表，跨块仍重复一些引用和证据；首次完整读取要校验所有声明块，单个当前根的依赖总字节可能大于旧单文件，不能把根文件大小当成整个历史大小。当前一次一块的读取也未对 S3 冷 I/O 给出低延迟保证。跨块字典、分组/有界预取及按恢复预算设置的 checkpoint 仍待测量设计。

数据库已记录 Naming 根→全部历史块、块→前驱的不可变依赖并提供可达性审计，共享前缀不会因删除另一个根而被直接回收。自动物理 GC **尚未启用**，须完整纳入 Revision、跨文档引用、预览及活动 Job 根。通用持久 Body/tool 检查点恢复、Part 参数规范化、模型 JSON/digest、视图投影及 Revision 快照也尚未全面增量化。实施入口统一归[CAD 核心主线](../../../plans/cad-core-roadmap.md)。

## Sketcher 的显示上下文

进入草图改变交互，不创建第二份 BREP 或改写 Part 真相。视口叠加三个层次：当前 Revision 的只读 Body 制品、活动草图/约束/暂态预览、世界及支撑平面网格等参考元素。

显示与可选择性分离。Body 参与导航与遮挡，普通草图工具只选活动实体和约束；只有显式 ExternalGeometry 工具可捕获上游 Face/Edge/Vertex，并将原始命中转为正式持久引用。草图曲线、点和尺寸应在重合支撑面上方可见；显示状态不进入求值输入。

同一零件几何引用不变时，视口保留 Body 对象、GPU 属性、轮廓资源与 BVH，只重建变化的草图对象并更新语义 Revision；根零件输入上下文切换也遵守此规则。草图每帧着色使用场景中的草图子树索引，不遍历实体边线层级。真实几何变化仍经过显示接纳屏障。主颜色绘制仍提交完整可见场景；尚无静态颜色层或跨 GLB 版本的通用逐面 GPU 更新。

## 源码、验证与限制

- `services/internal/workspace/solid_feature.go`、`evaluation_projection.go`：正式定义、参数及求值编排。
- `services/internal/workspace/part_execution_plan.go`、`sketch_display.go`：依赖执行范围、支撑复用和 Revision 草图显示投影。
- `services/internal/workspace/document_history_evaluation.go`：补偿求值、冻结支撑与原历史冲突门禁。
- `services/internal/workspace/feature_association.go`：显示贡献的分步归并、未变集合复用；不承担持久拓扑解析。
- `workers/geometry/src/naming_artifact.hpp`、`naming_history.hpp`：公共值表、历史块、当前根及编码 memo；冻结旧编码器检验完整展开后的逻辑字节。
- `kernel/api/include/occccad/kernel/immutable_sequence.hpp`、`feature_history.hpp`、`shared_vector.hpp`、`kernel/occt/include/internal/retained_prefix_budget.hpp`：持久序列、紧凑冻结历史、读者租借及共享前缀计费。
- `services/internal/workspace/naming_history*.go`、`naming_query_charge.go`、`services/internal/artifact/dependencies.go`：块/根校验、精简查询视图、所有权计费及制品依赖。
- `kernel/occt/tests/naming_growth_scenarios.cpp`：持续新增拓扑的原生基准、旧摘要 oracle、分叉所有权及冷重建；120 步性能需显式设置 `OCCCCAD_TEST_NAMING_STEPS=120`。
- `kernel/occt/src/occt_kernel.cpp`：精确构造、历史与门禁；先按操作符号搜索。
- `kernel/occt/tests/geometry_exchange_scenarios.cpp`：多区域/孔岛、ADD/REMOVE、boss/pocket、merge/split/delete、完整拓扑覆盖与冷重建。
- `services/internal/control/solid_feature_integration_test.go`：真实 Router/Worker、预览、提交、历史及重开。
- `services/internal/control/part_feature_performance_integration_test.go`：多步切除、前缀复用、上游编辑、冷 BREP/Naming 等价和 Undo/Redo。
- Web 的 `product-sketch-visibility`、`multi-body` 场景验证上下文过滤；真实深度、遮挡与相机行为仍须浏览器/WebGL 验收。

测试入口存在不是当次执行通过；几何、持久身份、历史和浏览器操作分别举证。执行方法见[测试路由](../../../tests/README.md)。
