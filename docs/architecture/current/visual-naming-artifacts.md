# Visual 2 / Naming 3 制品

这两个文件共享冻结 Geometry 的关联信息，但不共享职责。数据库只保存 Artifact 索引、摘要和业务状态。领域选择、历史及求值对象可以在内存展开；这些对象不是第二份持久数据格式。

## Visual: GLB 2.0 + OCCCCAD_cad schemaVersion 2

- 根扩展声明 `units=mm`、`coordinateSpace=PART_LOCAL`。可选 `association.geometryId` / `namingDigest` 关联权威制品；关联摘要不是 topology identity。
- 遍历全部 mesh 的全部 primitive；一个 Body 不限制为一个 primitive。节点 placement 不承载装配位姿：所有数值都已处于 Part-local 坐标，Product occurrence 在外部应用变换。
- FACE 使用 TRIANGLES (4)，EDGE 使用 LINE_STRIP (3) 或 LINES (1)；当前 C++ encoder 把全部边合并成一个 indexed LINES primitive，避免 JSON/accessor 数量随边数增长，VERTEX 使用 POINTS (0)。POSITION/NORMAL 为 FLOAT/VEC3；indices 为 UNSIGNED_INT/SCALAR。每个 primitive 的 `OCCCCAD_cad.kind/localIds` 指向 UNSIGNED_INT/SCALAR accessor，分别每三角形、线段、点一个 locator。
- 三种 localId 均从 1 开始；0 非法。`geometryKey + topology kind + localId` 仅在冻结结果内定位，不是持久身份。多个 primitive 可以重复同一个面/边的 locator，例如材质切分；localId 只在该 Body 的冻结 Geometry 内唯一；两个 Body 都可以具有 FACE 1。
- 所有 bufferView 位于 GLB BIN chunk，按 4 字节对齐。当前实现读取紧密排列的 FLOAT/UINT32 accessor；不接受外部 URI。未来 accessor 编码能力扩展必须同步消费者或升级版本。
- `OCCCCAD_visualization.schemaVersion=2` 的辅助显示 primitive 将 `positions` / `indices` 改为 accessor 索引，其余字段只保存选择与显示 metadata。辅助显示同时生成标准 glTF primitive；扩展中的所有权范围标记允许更新时替换二进制后缀。运行时会展开为原有显示对象，HTTP/WS DocumentView 不携带这些数组。
- 新写入的 `featureAssociationTable.version=1` 共享 Feature ID、重复的归属列表和阵列成员列表。行通过拓扑类型和冻结 locator 定位，解码后提供 origin、primary、modifier、support 和 pattern member。已冻结的 Visual 2 可继续读取原 `featureAssociations` 对象。续算只需读取配对 GLB 的 JSON 块，几何变体重新使用同一套显示归属。
- 删除 `stableIds`。SemanticTopologyRef 的序列化哈希既不是 stable ID，也不是 Visual 的职责。
- 每个 Body 或几何 FeatureResult 使用自己的 GLB；Wire/Point 允许仅有 EDGE/VERTEX primitive，不含零 count 面 accessor。拾取归属来自 Revision 的 `bodyId` / `result` binding，持久拓扑身份来自同一 owner 的 Naming。多个 mesh/primitive 为 Material、LOD 等组预留扩展；需要新解码语义的功能必须声明 required extension 或升级 schema，不能默默把 LOD 候选全部绘制。

可选 `OCCCCAD_cad.edgeProfile` 是[自然边线](natural-edges.md)的显示证据，独立 `schemaVersion=1, source=OCCT_EDGE_USE_V1`。`positions / normalA / normalB` 指向 FLOAT/VEC3 accessor，每渲染段两个点及双侧法线；`metadata` 指向 UNSIGNED_INT/VEC4 accessor，每段 `[edgeLocalId, faceA, faceB, flags]`。缺失 face 为 0；`flags & 255` 为 unknown/wire/boundary/manifold/seam/coplanar/non-manifold（0…6），`flags >> 8` 的 bit 0 表示法线缺失、bit 1 表示采样上限。零法线明确表示未知。原标准 EDGE 折线、拓扑 locator 和 Naming 保持不变，profile 只能细分原弦；它不是新的拓扑或持久选择格式。未知来源/版本忽略该可选证据并保守显示标准边；认识的 profile 必须通过数目、有限值和边/面关联检查。服务端添加 Visualization 二进制后缀时保留这些 accessor 与数据。

## Naming: PartTopologyManifest schema_version 3

文件 schema 与 Naming policy / evaluator 版本分离。存储布局调整不改变 SemanticTopologyRef 的业务身份、匹配容差和 history digest 算法。

- `naming.pb` 为根：`geometry_id` / `brep_sha256` 绑定冻结结果；`coordinate_space=PART_LOCAL`、`length_unit=mm`。位置为 mm，方向无量纲，measure 显式为 SI。根的 `transitions` 为空，`history` 顺序声明块的 SHA-256、大小、Feature、Geometry 和完整 lineage 行数。
- `bodies` 或 `results` 恰有一个 owner，互斥。根保存当前 `tip_feature_id`、全链完整性与 typed locator → semantic/evidence 映射；Body 和几何 FeatureResult 不能混合。一个 Naming 根不组织整个 Part。
- 每个 `NamingHistoryBlock` 包含前驱 SHA / 行数、完整行数、一个 Feature 的 metadata、literal 变化行、删除、歧义和有序 `layout`。前驱为空的块从零行开始。后续块与根声明的直接前驱及行数绑定。codec 1 的邻接引用位于本块。codec 2 的偶数 token 引用本块，奇数 token 引用已绑定前驱完全展开后的谱系结果行。两者都从 1 编号，完整展开保留邻接顺序。已接纳前缀可以混合两种 codec。
- LITERAL span 按序读取本块 literal 表；CARRY span 读取已验证前驱行的 result 和完整 evidence，重建 `UNCHANGED(result ← result)`。只有同引用、同完整证据的自延续可编码为 CARRY；类型变化、其他来源、删除或歧义不省略。span 中的 offset 是已校验块内位置，不是拓扑身份。边界、计数、重复继承范围及有序语义须验证；完整行序列可无损重建。
- 块的 `values` 和根各有局部引用表与 `evidence` 表，均从 1 编号。当前编码使用 `reference_dictionary` 的字符串符号和有序来源列表。解码恢复完整的 `SemanticTopologyRef` 值。可证明与 tip 定位证据一致时，根还提供按展开历史行排列的定位表。根的定位与证据可独立读取，历史块记录拓扑变化与证据继承。
- Evidence 保留 Plane / Cylinder / Line / Circle / Point / Spline / Other typed 数据、公共质心、SI measure、digest、endpoint role 和邻接；不伪造 evaluator 未提供的值。Curve range 保留 LINE_MM、ANGLE_RADIANS 或 NATIVE_CURVE_PARAMETER 的原生约定，不能当成统一弧长。
- Loader 校验根/全部块摘要及大小、policy/evaluator、Frame/单位、Feature/Geometry、单 owner、前驱、layout、表引用和 locator 唯一性。source/target Revision 及授权另行校验。部分缺失不能降级为“完整历史”，也不能按最近坐标改绑。

Worker 直接借用紧凑原生记录；32 MiB memo 按不可变记录、完整策略及前驱字节复用编码。一次请求先租借已有匹配项，再补算缺失项，防止长链顺序扫描淘汰稍后可命中的尾部；前驱 SHA 仍逐项核对。`PartEvaluationManifest.naming_history_artifacts` 只返回本次需要发布的块引用，根声明整个闭包。`EvaluatePartRequest.naming_source` 是已接纳根的发布复用提示，排除在精确计算缓存键外；它不能改变计算、身份或接纳门禁。导入根恢复另传根明确声明的 `PartBaseStage.root_history`，Worker 校验其摘要/归属。

数据库的 `artifact_dependencies` 原子记录根→尾块及块→前驱。共享前缀保留相同的块和依赖。父引用限制直接删除仍被引用的块。可达性审计已提供；自动物理 GC 尚未启用，仍须设计完整 Revision、跨文档、预览和活动 Job 根集合。完整 Naming 不在 RPC 或数据库复制保存。

tip 绑定独立读取当前定位表。需要沿历史解析时校验全链并保留有序事件及当前 tip 证据。已接纳前缀续算可按证据接纳新后缀。内核与测试的显式完整展开另有读者租借、预算和取消门禁。语义查询与缓存归[Naming](persistent-naming.md#有界解析视图与按需查询)，原生所有权与剩余增长归[Part 求值](part-feature-evaluation.md#通用特征性能与增长边界)。

## 演进与验证边界

当前 Naming 根为 v3，历史块支持 codec 1 和 2。新制品输入键分别绑定 Naming 存储版本和显示策略版本。已接纳 Revision 继续引用原制品与摘要。持久选择继续使用 v8 策略，预览继续引用临时制品。

`NamingArtifact.HistoryBlocksKeepLegacyBytesAndSurviveEvictionAndColdRebuild` 用冻结旧编码器比较完整展开后的逻辑 v2 字节。Router 验证各自冻结的 BREP、Naming 根及历史块摘要。恢复圆曲线后，OCCT 文本可将轴分量的零写成相反符号，连带改变 BREP 和证据摘要。对应回放测试核对 BREP 数值及拓扑排列、完整 Naming 的语义引用、证据、定位和有序历史，并逐份验证原摘要。Go 测试还覆盖旧扫描器、删除/重新出现/歧义顺序、错误闭包、缓存淘汰/取消、共享前缀依赖和精简查询视图。
