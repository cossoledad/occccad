# Part Feature 求值与草图上下文

本文拥有实体求值阶段顺序及 Sketcher 的显示边界。Part/Body/Sketch 模型见[Part/Sketch](part-sketch.md)，各特征范围及失败 Revision 合同见[实体 Feature](solid-features.md)，命名历史与解析规则见[Naming](persistent-naming.md)。

独立曲线/曲面共用 Part DAG、参数、候选与制品流程，但不执行材料布尔及 Solid 材料门禁；其域、归属与限制归[线框与曲面](wireframe-surfaces.md)。`THICKEN` 将有界 Sheet 显式实体化后进入下述原 Body 管线，后续修改继续使用同一门禁。

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

Body 和 Stage 的形体费用复用已序列化 BREP 的保守估算；按面网格只计实际拥有的三角化/边界包及索引，不为记账重新编码驻留形体并集。Stage 历史账本按缓存根及分叉共同保留的不可变前缀并集计费，根租借与前驱边分别计数；新增/释放只遍历新接入/失去最后所有者的前缀尾部。共享历史不因多个检查点重复收费。形体及构造上下文估算仍可能重复计费；缓存预算不是全进程 RSS 上限。输入 BREP 在请求与有界缓存间共享不可变字节。

导入件的首次新操作可从该 Body 已接纳的不可变 Naming 制品恢复 ImportBody 根 transition。输入绑定冻结 BREP SHA-256、Feature/Body、导入身份分配、OCCT/策略版本、完整历史及 `occccad.geometry.brep-valid.v1` 校验事实，不读取已切除后的 Body Tip 或保存的体积。Worker 验证制品摘要和归属，内核逐项验证 typed locator 与随机身份覆盖。没有可恢复的根时执行完整初始化；`ForceCold` 显式绕过恢复和阶段缓存。此优化不需要曾经计算过同一个切除结果。

可接纳的普通 Body 中间结果按 Body、Feature 和完整 DAG 输入摘要登记，随正式 Revision 在同一事务内发布。追加或编辑时，候选计划从最后一个合格的已接纳前驱续算。Worker 统一从冻结 BREP 解码形体，并从 Naming tip 的已验证全量定位关系恢复命名与证据，仅执行受影响后缀。已接纳的历史块继续以原摘要引用。阵列后缀若需要更早的工具或材料范围，或冻结制品缺失及证明校验失败，进入完整前缀求值。取消和新特征的失败继续走原诊断路径。冷算只保留本次阵列实际依赖的原生工具、阶段和范围基底。

普通生成、局部修改、Boolean、曲面实体化、导入及输出摘要不计算整体或差量体积。Body 校验检查有效 BREP、每个 Solid 的有界/方向及严格内部材料见证，拒绝空结果和游离拓扑；同一结果的校验事实传入后续阶段。对于已接纳前驱上的单实体 ADD/REMOVE，可在工具体内取点，并对输入、工具和实际结果进行严格分类。只有输入与结果在该点的材料归属确实改变才接纳。其余情况复用本次 Boolean 的分区检验，边界接触不算材料；INTERSECT/Feature 范围需要集合差时检查分区材料及其在原始操作数中的分类，不用两个整体体积的大小关系替代空间关系。分类未决显式失败，不当作空集。Shell 保留原生构造、边界精度、壁材料及实际空腔见证；其特殊顺序组装的包含性限制见[抽壳算法](shell-algorithm.md#性能与限制)。

实体边界校验使用已接纳前驱和当前 OCCT 形状。非破坏运算中，同一 TShape、位置和方向的面可以继承面内检查；变化面及其边、点与线环仍检查。每个结果壳重新检查连通性、闭合和方向，每个实体仍检查壳在实体中的关系及材料。私有副本上的圆角、倒角等无法以副本身份继承原面。无合格前驱、完整冷算及特殊拓扑走完整分析器。

对于具有大量直线或圆弧内轮廓的平面，校验器按扩张了容差的解析 UV 包围盒寻找空带。分开的轮廓无法相交或互相包含。每一组都保留同一条真实外轮廓，交给 OCCT 检查组内的相交、包含、方向和曲线在面上的一致性。检查用面只在内存中存在。曲面、周期边、共享边界或无法安全分组的平面使用原面检查。对于已验证前驱，若原平面、位置、面容差和外环一致，分组中的每个内环又逐边沿用原来的有向边及位置，已隔离的整组可复用面校验。重建的闭合环允许变换起始边；有改动的分组带原外环接受原生检查。壳、实体和材料检查继续针对完整结果执行。实现见[实体边界](../../../kernel/occt/src/body_validation.cpp)与[平面轮廓](../../../kernel/occt/src/planar_boundary.cpp)。

体积、面积、质心和边长是查询派生量，不是模型真相或 Feature 制品的必填属性。`Artifact`、数据库及 Worker 求值响应不保存体积；Naming 不为证据计算面积/边长/质心。属性栏展开时才查询选中对象，折叠时选择/悬停不发属性请求；关闭、切换选择或页签取消请求。完整 Body 查询使用 `topology-properties?kind=BODY&localId=0`，按文档/冻结快照授权后复用既有精确测量 RPC；历史阶段可显式请求 `FeatureInput.includeProperties`：未指定 Kind 时返回整体 Body，指定 Kind/localId 时返回授权输入阶段的单个拓扑元素；默认编辑上下文不查询。结果只进入有界易失缓存，绑定 GeometryId/BREP/求值策略。显式测量、分析或用户保存的测量定义保持原精度及来源合同，不由显示三角形估算代替。

精确整体属性查询仍可能昂贵：既有 ITEM 测量会计算面积、体积及质心。前端关闭或更换选择会取消请求并丢弃过期响应，但部分 OCCT 数值积分本身暂不可中断，不能把客户端取消等同于即时释放 Worker。它不在普通 Feature/折叠属性栏的选择路径执行。

一次请求内按精确 OCCT occurrence（包含朝向）复用有效性事实与几何证据；同一语义引用且拓扑未变时可沿用已接纳证据，邻接按本次结果重建。最终邻接使用包含/共享边界索引，不做全部拓扑两两扫描。语义引用索引按完整 typed ref 查找，并保留重复候选的原顺序；不在每个输出/lineage 上线性扫描全部输入。上述索引不是持久身份，规则见 [Naming](persistent-naming.md)。

网格包绑定完整不可变 BREP SHA 与该快照内部 Face/Edge 地址，不保留 BREP/TShape，不把这些临时地址当成持久引用。跨快照只接受原生未变复制对应，或真实一对一 typed history 后继且完整支撑/裁剪证据严格相同；不能按同名、局部编号或近邻几何搜索建立对应。位置/朝向、分裂合并、实际裁剪或未知变化使复用失效。面三角化与两向边界 polygon 共享不可变载荷，复制进私有剖分副本；继续检查共享边界并保留冷剖分回退。首次只剖分结果时可沿反向已证对应填充输入包；输入 BREP 卸载后仍能在相同 SHA 快照中恢复。128 MiB 包预算及最近使用的输入/结果双帧限制不等于进程 RSS 上限。取消/失败不接纳新候选包。

前缀检查点带输入与策略 digest、历史、校验事实、命名及构造上下文。`FrozenFeatureResult` 用持久 AVL 序列共享完全相同的 lineage 行及连续片段，历史输出以紧凑绑定借用行中的完整引用/证据；例外输出原样保存。 完整有序 source ID 列表按值与前驱共享，只读复制不再深拷贝字符串数组；写访问分离，已有可变借用也不污染副本。历史计费按新保留的行、来源存储及最终树节点计算，不累计已释放的 join/rotation 临时节点。`FeatureResultSequence` 与检查点共享冻结记录，恢复仅读取 metadata 和借用证据，不展开全部历史。需要完整 `FeatureResult` 的读者按需展开并持有租借，检查点只保留弱展开引用；淘汰不使读者失效。完整历史、输出顺序、删除、歧义及每个阶段的 semantic outputs 继续可读。工具、Body 阶段及范围基底的构造上下文共享不可变 ToolBuild，复制阶段时只复制索引；实际需要重执行的工具再按需复制。持久 Naming 的完整历史、顺序和摘要保持原合同。完整工具上下文会挤出最近前驱时，优先精简当前检查点；必要时也精简前驱，使两者在原预算内共存。精简不改变 Shape、历史或有效性事实。缺少的构造上下文状态随普通后续步骤传播，不能被误标为完整；只有尚未执行的后缀包含 `SOLID_PATTERN` 时才要求完整上下文，否则允许复用已完成阵列之后的精简检查点。新阵列确需缺失来源时仍重建完整上下文。取消后不接纳检查点；淘汰与冷重建不改变结果语义。

历史摘要排序借用记录并预计算排序键，避免深拷贝证据/邻接；canonical 历史摘要、身份、单位与匹配容差不变。Worker 的公共值编码器接受原生只读记录及 protobuf 输入，只按完整 typed ref/evidence 值 intern；新增 Evidence 字段必须同步完整值比较/散列和冻结旧编码器的逻辑字节对照。持久 Naming 已改为 v3 根及不可变历史块：根只保存当前 tip locator，块保存变化行及无损 CARRY 布局，见[文件合同](visual-naming-artifacts.md#naming-parttopologymanifest-schema_version-3)。新增或编辑末端共享既有块，Worker 的 32 MiB 编码 memo 仅优化转换；`naming_source` 是已接纳根的发布复用提示，校验归属后省去已持久块的重复写入，不能参与或替代精确求值。

Boolean 和剖分通过 OCCT progress 协作取消，算法内观察本地单调取消标记，真实 transport 状态至多每 16 ms 轮询一次；检查点/网格接纳和结果发布边界强制查询真实状态，制品提交仍检查取消和过期资格。显式质量属性查询的单次积分及部分构造器仍可能在返回后才响应取消，不承诺所有原生算法立即中断。

`PartRuntimeStats` 分开记录 base、exact、Naming、display、mesh、encoding、邻接、积分次数及证据复用；Naming 再区分内核与制品装配，另记真实取消查询、网格复用/计算面数和缓存记账耗时/编码次数；`volume_integrations` 统计统一积分入口的实际调用，普通 Feature 求值应为零。GetTopology 另报排队、精确加载与查询时间，客户端接入请求计时。mesh/encoding 属于 display，缓存记账也包含在 RPC 总时长内，不能重复相加。指定复杂导入件及生成 STEP 的测量与边界见[零件性能测量](../../validation/part-feature-performance.md)。

默认 OCCT 包在组壳与面定向操作内，按有向面的实际查找深度为大边表建立临时索引。Part 求值键和检查点接纳条件包含该内核的 recipe revision。唯一输出与历史行按同一次生成的行序绑定，共用证据；有重复引用或独立输出的输入仍可表达各自证据。网格沿已证对应传递，边显示采样还要求原生边与支撑面相同，且边界、方向、UV 和参数一致。首次从新结果反向补齐前驱的采样后，同一前驱的下次编辑也可复用。GLB 直接遍历有序点和采样段生成字节。

显示剖分检查同一面上的边界环时，先用两个 UV 空间线段树已有的根包围盒排除不可能相交的环对。包围盒沿用原段查询的容差；可能重叠的环对及环自身仍执行原生逐段检查。恢复 Body 检查点时，Worker 按制品中的 tip 与历史行顺序绑定共用的冻结证据，直接读引用字典；网格显示关联由服务从 GLB 头按需解析，共享的来源列表按实际保留的数组计入缓存预算。

精确阶段再记录生成器准备、Body 运算、Boolean Build、材料变化、同域合并、有效性、BREP 快照和未变子形状对应。有效性包含 B‑Rep 分析器及实体材料检查，前者对超过 128 个待检面的检查集合使用 Geometry Worker 的四线程 OCCT 池。显示记录边界提取与轮廓时间，以及计算和复用的边数。明细与其所属阶段存在包含关系。

### 通用特征性能与增长边界

新增 Feature 应接入既有依赖执行计划、规范输入/策略键、不可变结果、完整 Naming、显示制品及接纳门。Body 链共享前驱检查点；独立曲线/曲面共享结果容器、摘要、编码与服务端解析。公共层不按“孔/切除”选择是否优化；具体生成器仍拥有精确算法及真实 history。冷算、取消、淘汰、上游修改、重排/删除、阵列上下文、补偿历史及过期候选必须分别验证。公共层优化不意味着每类精确构造具有相同加速比。

设第 i 步最终拓扑数为 Tᵢ，完整逻辑历史仍有 H=ΣTᵢ 行。原生层共享未变化的行/片段，文件层用 CARRY 表达同引用、同完整证据的自延续；只读查询使用有序事件及独立当前 tip，不先展开 H 行。新增 Feature 自动复用结果容器、编码、发布、依赖闭包与查询设施；精确构造和本步完整拓扑验证仍随本步几何量增长，不承诺常数时间。

真正改变的证据、邻接、分裂/合并、删除和歧义均须保存。历史块的邻接可借用直接前驱行的精确语义引用。首次完整读取仍须校验每个声明块，统计存储需计入根与整个可达块集合。更长周期的字典共享、分组、有界预取及恢复预算仍待设计。当前逐块读取尚无 S3 冷 I/O 的性能验收。

数据库以 Naming 根→尾块、块→前驱登记不可变依赖及可达性，共享前缀继续由各根保留。自动物理 GC 尚未启用，仍需完整纳入 Revision、跨文档引用、预览及活动 Job 根。Part 响应由同一已解码模型生成详情和结构树。Revision 的结构依赖边集合可以共享已接纳记录，依赖输入及结果摘要独立保存。参数规范化、模型 JSON/digest 和 Revision 快照仍有全量工作。实施入口统一归[CAD 核心主线](../../../plans/cad-core-roadmap.md)。

## Sketcher 的显示上下文

进入草图改变交互，不创建第二份 BREP 或改写 Part 真相。视口叠加三个层次：当前 Revision 的只读 Body 制品、活动草图/约束/暂态预览、世界及支撑平面网格等参考元素。

显示与可选择性分离。Body 参与导航与遮挡，普通草图工具只选活动实体和约束；只有显式 ExternalGeometry 工具可捕获上游 Face/Edge/Vertex，并将原始命中转为正式持久引用。草图曲线、点和尺寸应在重合支撑面上方可见；显示状态不进入求值输入。

同一零件几何引用不变时，视口保留 Body 对象、GPU 属性、轮廓资源与 BVH，只重建变化的草图对象并更新语义 Revision；根零件输入上下文切换也遵守此规则。草图每帧着色使用场景中的草图子树索引，不遍历实体边线层级。真实几何变化仍经过显示接纳屏障。主颜色绘制仍提交完整可见场景；尚无静态颜色层或跨 GLB 版本的通用逐面 GPU 更新。

共享 Feature 编辑器的确认会立即启动尚在合并窗口中的当前候选，或等待同一运行中/已完成任务；不强制先展示预览，也不重复启动求值。候选须仍匹配当前输入 key、基础 Revision、取消状态及有效 previewId，最终继续走既有提升、幂等与 CAS。自动预览仍按最新输入合并，不能把过期结果当作最终确认。

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

已复现两处多 Body 限制：跨 Body 分区布尔后连续倒角、圆角，冷重建的部分 Naming 证据摘要与提交结果不同；撤销删除 Body 后，恢复的 Body 制品摘要可能发生变化。[复现入口与差异范围](../../validation/part-feature-performance.md#多环面剖分与前驱命名恢复)

测试入口存在不是当次执行通过；几何、持久身份、历史和浏览器操作分别举证。执行方法见[测试路由](../../../tests/README.md)。
