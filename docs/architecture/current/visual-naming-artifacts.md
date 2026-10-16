# Visual / Naming Artifact v2

这两个文件共享冻结 Geometry 的关联信息，但不共享职责。数据库只保存 Artifact 索引、摘要和业务状态。领域选择、历史及求值对象可以在内存展开；这些对象不是第二份持久数据格式。

## Visual: GLB 2.0 + OCCCCAD_cad schemaVersion 2

- 根扩展声明 `units=mm`、`coordinateSpace=PART_LOCAL`。可选 `association.geometryId` / `namingDigest` 关联权威制品；关联摘要不是 topology identity。
- 遍历全部 mesh 的全部 primitive；一个 Body 不限制为一个 primitive。节点 placement 不承载装配位姿：所有数值都已处于 Part-local 坐标，Product occurrence 在外部应用变换。
- FACE 使用 TRIANGLES (4)，EDGE 使用 LINE_STRIP (3) 或 LINES (1)；当前 C++ encoder 把全部边合并成一个 indexed LINES primitive，避免 JSON/accessor 数量随边数增长，VERTEX 使用 POINTS (0)。POSITION/NORMAL 为 FLOAT/VEC3；indices 为 UNSIGNED_INT/SCALAR。每个 primitive 的 `OCCCCAD_cad.kind/localIds` 指向 UNSIGNED_INT/SCALAR accessor，分别每三角形、线段、点一个 locator。
- 三种 localId 均从 1 开始；0 非法。`geometryKey + topology kind + localId` 仅在冻结结果内定位，不是持久身份。多个 primitive 可以重复同一个面/边的 locator，例如材质切分；localId 只在该 Body 的冻结 Geometry 内唯一；两个 Body 都可以具有 FACE 1。
- 所有 bufferView 位于 GLB BIN chunk，按 4 字节对齐。当前实现读取紧密排列的 FLOAT/UINT32 accessor；不接受外部 URI。未来 accessor 编码能力扩展必须同步消费者或升级版本。
- `OCCCCAD_visualization.schemaVersion=2` 的辅助显示 primitive 将 `positions` / `indices` 改为 accessor 索引，其余字段只保存选择与显示 metadata。辅助显示同时生成标准 glTF primitive；扩展中的所有权范围标记允许更新时替换二进制后缀。运行时会展开为原有显示对象，HTTP/WS DocumentView 不携带这些数组。
- 删除 `stableIds`。SemanticTopologyRef 的序列化哈希既不是 stable ID，也不是 Visual 的职责。
- 每个 Body 使用自己的 GLB；glTF 的多个 mesh/primitive 为 Material、LOD 等组预留扩展。拾取的 Body 归属来自 Revision 的 Body/Geometry binding，持久拓扑身份仍来自对应 Body 的 Naming。需要新解码语义的功能必须声明 required extension 或升级 schema，不能默默把 LOD 候选全部绘制；当前没有新增材质、LOD 或 PMI 功能。

## Naming: PartTopologyManifest schema_version 2

文件 schema 与 Naming policy / evaluator 版本分离。改变存储布局不改变 SemanticTopologyRef 的业务身份、匹配容差和 history digest 算法。

- `geometry_id` 与 `brep_sha256` 共同校验对应 Body 的冻结结果和精确 BREP，`coordinate_space=PART_LOCAL`，`length_unit=mm`。坐标/原点/质心采用 mm，方向是无量纲向量；measure 明确为 SI（AREA=m²、LENGTH=m、NONE 无量纲）。
- `semantic_refs`、`evidence` 是共享表，引用使用 1-based ID，0 无效。只按完整值 intern，不按几何 hash 合并不同 Feature/Definition 身份。Evidence 邻接只引用 semantic table ID。
- Evidence 使用 Plane / Cylinder / Line / Circle / Point / Spline / Other oneof。公共质心、SI measure、digest、endpoint role 与几何专属 frame/range 分离。当前只保存 evaluator 已提供的证据，不伪造半径等尚未输出的数据。
- Curve range 保留解析曲线的原始参数：直线 LINE_MM；圆/椭圆 ANGLE_RADIANS；Spline 及其余曲线 NATIVE_CURVE_PARAMETER，指该冻结曲线的原生参数域，不是归一化弧长。位置和参数不能混用。
- `transitions` 保存 Feature/Body/input/profile 身份、输入/输出 Geometry、lineage/split/merge/deleted/ambiguous、完整性与诊断。source/result/evidence 全部通过表引用。
- `bodies` 当前严格要求恰好一项，显式列出该 Body 的有序 transition ID、tip_transition 和最终 tip 的 typed locator → semantic/evidence 映射；serializer/loader 拒绝跨 Body transition。一个 Naming 文件不能组织整个 Part。只有该 Body 当前 tip 保存完整 locator snapshot；历史 Feature 不重复保存整个拓扑快照。
- 历史语义恢复沿 transition；不是按 localId 或最近几何匹配。当前没有任意历史节点几何 snapshot 查询接口；需要历史 Revision 时读取该不可变 Revision 的 Naming Artifact。未来可增加可选 checkpoint/delta，不能退回每个 Feature 无条件全量快照。
- Loader 拒绝越界表引用、错误 Body 归属、重复 locator、未知 schema/单位/参数约定。内存 domain view 按 Body Tip 查询，不能使用全局最后一个 Feature。

## 演进与边界

旧 v1 文件不兼容读取；开发数据通过维护者运行 `invoke data.reset --yes` 后重建。新字段遵循 protobuf 保留字段号、glTF extension 和显式 schemaVersion；消费者不能把未知必需语义当成已支持。Preview 仍是 transient representation，可使用同一个显示 schema，但不成为正式 Naming/业务真相。
