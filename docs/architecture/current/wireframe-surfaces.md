# 线框与曲面 Feature

当前交付独立 Part 中的关联曲线、拉伸曲面、加厚实体，以及原实体扫掠对正式三维曲线的消费。它们共用 Part Feature 历史、参数、Domain Command、预览及制品管线。操作见[使用指南](../../guides/wireframe-surfaces.md)；后续曲面构造、修剪缝合及交换范围见[阶段设计](../../../plans/wireframe-surface-design.md)。

源码入口：Workspace `geometric_feature.go`、`geometric_reference.go`、`evaluation_projection.go`、`part_bodies.go`；协议 `geometry_worker.proto`；OCCT `geometry_feature.cpp`；Web `geometric-feature-editor.tsx`、`geometric-input-selection.ts`。先按操作名搜索，不默认展开整个实体 evaluator。

## 所有权、输入与结果

- `GEOMETRY` Feature 和“线框与曲面”标签新建的 Sketch 归 `geometricalSetId`；实体特征归 `bodyId`，两者互斥。普通几何图形集只组织和控制显隐，全 Part DAG 决定依赖，集合顺序不构成材料前驱。首次创建可原子建立默认集合；既有草图不隐式搬移。
- `features[].result = {id, domain, geometryKey}` 是派生引用，正式输出角色为 `result`；`POINT_SET / WIRE_SET / SHEET_SET` 独立于 Body。`THICKEN` 是原 Body 管线中的实体生成器，仍接纳非空有效 Solid 集，支持新 Body、添加、切除及交集的领域操作。
- 输入分别为固定 Sketch entity ID 集、整个动态 `FeatureId + ResultId` 结果、固定拓扑 `PersistentSelection` 或显式 DatumPlane。`sourceBodyId` 与 `sourceResult` 恰有一个，拓扑绑定记录来源 Revision/Feature 和证据；不能保存 localId、数组位置、显示名或屏幕坐标作为身份。
- 构造点可引用参考点、草图的稳定点/端点/圆心或正式点结果/顶点；方向可引用参考轴、坐标系轴、草图直线、精确直线边或平面法向。旋转轴另带来源原点，镜像及圆/圆弧支撑引用平面。所有引用进入原 DAG、删除保护及上游顺序检查。
- 控制点拥有稳定 ID；插点不挪用其他点的权重槽，替换来源保留本地点 ID。UI 只编辑半径、长度、角度、弧长比例及权重等量值/表达式，不创建独立坐标或向量参数。领域 API 仍可显式定义坐标曲线和数值框架；引用方式不为已关联方向额外建立 XYZ 槽。二维 PlaneGCS 不承担三维约束求解。

Worker 只接收冻结精确 BREP/Naming、规范曲线或显式平面；来源的 owner、阶段、BREP digest 和完整 Naming 必须一致。构造使用真实 Generated/Modified/continuation 和稳定构造角色，再做严格拓扑邻接闭包；相同稳定来源仍无法消歧则失败，不能按位置补名。

## 已实现范围

| 操作 | 当前语义与限制 |
|---|---|
| 直线、圆、圆弧 | 有限精确曲线；UI 选择端点，圆/圆弧选择圆心和支撑平面，设置半径/起止角；圆心须位于支撑上 |
| 三维样条 | 插值点或控制点方式；控制点次数、规范节点/重数和正有理权重；空点列起步，逐点拾取，支持插前/插后、替换、移除及权重，不提供任意三维约束求解器 |
| 曲线上点 | 单条有限边的归一化弧长比例；不是原生曲线参数 |
| 提取、边界 | 固定精确边/面或整个结果；边界提取自由边，周期缝和合法退化边不算自由边 |
| 组合曲线 | 精确成员组成连通无分叉 Wire，保留方向；不重拟合、不自动跨接断口 |
| 投影、交线 | 沿关联方向投影到选定有限面或无限基准平面；有限面之间或面与平面求交。不同稳定来源对可产生多个成员；同一来源对的多支路不能稳定区分时明确拒绝；相切点/重合域尚无独立结果语义 |
| 平面曲线偏移 | 关联基准面或精确平面面片、精确平面性检查及有符号距离；当前支持实现可判定的线、圆/椭圆及样条，不承诺任意空间法向偏移。正号沿输入方向的左侧，逆时针圆正偏移缩小半径 |
| G1 连接 | 第一曲线终点到第二曲线起点的三次 Bezier，方向与正切线尺度显式；不承诺 G2 或任意支撑匹配 |
| 分割、修剪、延伸 | 分割/修剪按单边归一化弧长点或保留区间；延伸限直线/圆的起端、末端或两端，圆不超过一周。尚非通用切割对象/点击保留区域式编辑 |
| 平移、旋转、镜像 | 选择对象后选择方向/旋转轴/镜像平面，调整距离或角度；保留关联框架和真实变换历史 |
| 拉伸曲面 | 固定草图曲线或正式有限 Wire 沿关联直线、参考轴或平面法向及长度产生侧面；开放或闭合轮廓均不自动封盖、不参与材料布尔 |
| 加厚 | 有界 Sheet 的有符号常厚度生成有效 Solid，再进入原 Body 操作与修改链；单面采用简单偏移，多面采用求交连接及封边，保存真实历史；自交、退化、开放壳或厚度失败拒绝 |
| 原实体扫掠 | 新增正式三维边/连通 G1 Wire 路径，精确冻结输入；截面起点和法向需匹配路径，曲率标架/修正标架可选。空间尖角拒绝，既有平面草图直线斜接另保留 |

## 交互与求值

“实体”和“线框与曲面”是同一 Part Design 工作台的并列 toolbar tab；切换不生成 Revision。草图仍有独立上下文；测量、参数与接口、视图和文档功能保持公共分类。

曲线/曲面及参考元素创建不复制命令前的选中对象，不填充示例曲线或默认支撑；必需的几何字段为空时不发预览、不能确认。输入按角色依次激活，在视图区或结构树拾取，连续点列按点击顺序绑定；切换字段、退出会话和 Revision 改变使未完成的绑定失效。参考点计入视图适配；固定屏幕尺寸的平面/轴图标不扩大适配范围。

创建与编辑共用紧凑拾取字段、数量/表达式和全局错误反馈，不使用来源下拉列表或“更多”折叠区；点列可定位已选点。命名沿用树的统一改名，容器沿用上下文/默认集合。

预览与提交走同一权威 evaluator；预览不写 Head，确认绑定 actor、Revision、payload 和 preview token，提交继续 CAS 与幂等。方向、支撑和点的引用在冻结模型内解析为 Worker 数值输入；缓存同时记录正式引用、解析框架及 BREP/Naming 来源快照。上游参数改变重建消费者；自身无效编辑拒绝，已有下游失效按原失败/阻塞合同呈现，用户可重选输入。Undo/Redo 使用 ChangeSet 补偿和重放。集合显隐、改名等组织动作不改变几何身份。

每项独立结果拥有 BREP / GLB / Naming，下载授权按冻结 Revision 的 Body 或 FeatureResult 引用检查。GLB 可只有线/点，不制造空面或假三角形；开放面双面显示并避让共面深度，不投射/接收阴影，避免与加厚实体叠加出条纹；曲线/点独立命中。`geometryDomain` 决定物理量是否适用：开放结果的内部 `volume=0` 只是存储占位，不能作为实体体积或质量展示。格式与门禁分别见[制品](geometry-representations.md)、[Naming](persistent-naming.md)。

## 验证入口与未交付边界

内核 `geometry_feature_scenarios.cpp` 检查解析面积/体积、完整拓扑命名、方向、周期边界、三维有理样条、投影/交线、参数更新、冷重建及原实体扫掠/修改。Workspace `geometric_feature_test.go`、`geometric_reference_test.go` 检查所有权/参数/依赖/历史；`geometric_feature_integration_test.go` 通过真实 Router/Worker 验证预览不写 Head、提交、参数传播、引用、Undo/Redo、冷读取及实体互用。`geometric-feature-live.spec.ts` 是真实浏览器创建、拾取、编辑、重开、取消和全局错误场景。执行结果以当次 `build/agent-check/` 与浏览器报告为准，不由测试文件存在推导通过。

当前只接入独立 Part 的线框/曲面显示及编辑；Product occurrence 的开放结果显示、READY ContextVariant、Publication/跨零件消费、完整测量和开放/混合几何 STEP/BREP 交换尚未接入。Surface Revolve/Sweep/Loft/Fill、曲面 Split/Trim/Extend、Join/Sew/Offset/Close 属后续阶段。法向/自由边专用叠加、UV 等参数线、曲率梳/斑马纹、有序集合及吸收显示尚未交付；源几何按集合可见性保留。尚无工业模型成功率、全局误差界或容量/性能承诺。
