# 二维草图能力与验证入口

审核起点：`29e5186da56787106c6520538cf9724271745aa3`。本页分开记录生产能力、支持边界和验证入口；测试结果只覆盖实际运行的组合。

## 当前生产合同

以下描述当前源码中接入的生产路径；整轮交付状态及实际运行结果分开记录，未运行组合不视作验收完成。

| 功能 | 支持几何与合同 | 生产路径 |
| --- | --- | --- |
| 创建 | Point、Line/连续轮廓、Circle、signed Arc、Ellipse/EllipticalArc；矩形三方式、正多边形、三点圆/弧、连续线/相切弧；FIT 与 CONTROL Spline | 统一工具输入 → `EDIT_SKETCH` → 权威 Worker；复合创建是普通实体及正式约束 |
| 辅助几何 | `PROFILE` / `CONSTRUCTION`；独立点和辅助几何退出实体边界 | `UPDATE_ENTITY_ROLE`，Profile Builder |
| 编辑与拓扑 | 删除/内部引用复制；移动/旋转（不支持统一缩放）；解析或 OCCT 参数域分割/裁剪；受影响引用显式释放，原子失败 | `sketch_geometry_edit.go`、`sketch_curve_edit.go`、Worker `ComputeSketchCurves` |
| 逻辑约束 | 基础逻辑、Collinear；类型精确的相切/等值/同心；端点与中心引用；内部 MIRROR/SAME_SUPPORT 仍参加诊断 | 规范能力/类型过滤 → Go 严格验证 → PlaneGCS 原语；CONTROL 样条端切向是端部真实 D1，FIT 先显式转 CONTROL |
| 尺寸 | 真距离、signed ΔX/ΔY、支撑点线距离、非负平行线间距（可见 Parallel）、线长、半径/直径、线角度、椭圆半轴 | 统一定义 → 稳定 ParameterId/Quantity/checkedAST → 求解；零间距合法，非平行不冒充线段最近距离 |
| 尺寸生命周期 | 同一对话框编辑 source/名称/引用/驱动参考/停用/删除；普通逻辑定义原子改kind/refs/停用；参考退出 Solver/DoF，解后测量，不可测为空 | `sketch_dimension_lifecycle.go`；恢复驱动显式 ORIGINAL 或 MEASUREMENT；表达式读取参考参数目前明确拒绝 |
| 样条 | FIT 插值点和 CONTROL 极点身份分开；canonical degree/knots/multiplicities/weights/domain；合法结点插入/移除保持原曲线 | FIT 点编辑重插值；CONTROL de Boor/齐次 knot 操作；删除须通过系数逆操作验证，不采样重拟合 |
| 镜像 | 独立副本或正式关联 MIRROR；有向弧/椭圆/样条反射；轴上连接及自映射去重复 | Go 来源/身份映射 → Worker 关联求解 → 统一 Profile |
| 二维偏移 | 独立精确副本；Line/Circle/Arc、正式连通的开放/闭合混合链；signed 左法向侧；MITER/ROUND；半径非正/重叠/自交整体拒绝 | `OFFSET_ENTITIES` → `sketch_offset_edit.go` → Solve/精确 Profile/Extrude；不复制原驱动，不宣称持续距离关联；Ellipse/Spline 精确等距明确不支持 |
| 圆角/倒角 | 线线圆角/三种倒角；线弧/弧弧常见圆角；正式连接/相切/尺寸与保留支撑关系 | `sketch_corner_edit.go` → Worker → Profile；虚拟交点为受约束 Construction 点 |
| 外部引用 | 已有面支撑、ExternalGeometry、Reconnect、Detach；外部缓存几何只读，可为本地边界 | 原有 support/projection 合同继续使用 |
| 轮廓和拉伸 | 真 Line/Arc/Ellipse/EllipticalArc/canonical Spline；正式连接、孔岛/多个区域；闭环循环方向无关稳定身份 | 正式拓扑 Builder → Worker PROFILE 精确分类/OCCT Shape gate → 现有 Extrude；多个 Solid 不自动拆 Body |
| 分析与历史 | 后端结构化开放端/分支/重复/失效连接反馈；开放草图可保存；同命令原子修改、Undo/Redo、冷读取 | `sketch_exact_profile.go` 复用正式环及 kernel，`geometryVerified` 明确精确验证，Web 不另判闭合；最终 ChangeSet 包含求解坐标和参数定义 |

PlaneGCS 使用仓库锁定的 FreeCAD 1.0.2 commit `256fc7eff3379911ab5daf88e10182c509aa8052`；逐文件 SHA-256 清单位于 `workers/geometry/sketch/CMakeLists.txt`。不据上游最新接口推断当前能力。

## 支持边界与引用规则

- 交点、裁剪、分割、补弧和延伸使用真实参数曲线；点击选择区间、分支或自由延伸的解析投影目标。重叠、不存在交点、极短区间及无效半径明确拒绝；不以显示采样线段代替权威交点。
- 变换、复制及拓扑编辑保持实体和子元素身份映射。内部引用按新身份映射；跨选择集关系、Fixed、公式控制和被删除端点的关系必须明确保留、释放或拒绝，不按最近端点重连。复合操作原子提交。
- Equal 在圆/圆弧间表示半径相等，在椭圆/椭圆弧间表示两个半轴相等。Concentric 使用这些类型的中心。相切按支持类型及明确端点角色过滤；CONTROL 样条开放 clamped 端点可与直线建立真实 D1 切向。
- 线线尺寸表示非负平行支撑线间距；水平/垂直点点尺寸是有符号坐标差。参考尺寸不驱动几何，表达式读取参考参数目前明确拒绝，避免同草图测量/驱动循环。公式与只读来源不能被普通数值编辑覆盖。
- 关联镜像只保留正式派生关系，不复制源驱动尺寸。后续编辑触及关联引用时必须显式处理影响。轴上连接来自正式关系；自映射元素不生成重复轮廓。
- 偏移当前为独立副本，支持 Line/Circle/Arc 及其适用连续链；Ellipse/Spline 的精确等距、一般自由曲线间任意圆角、高阶 G2/G3、完整三维投影和 Engineering Connections 不在本轮范围。
- ExternalGeometry 保持只读，只能通过显式 Detach 独立编辑。开放草图可保存；闭合实体拉伸仍需正式拓扑和精确曲线验证。完全约束与闭合是独立状态。
- Slot 创建入口和专用工具已移除；构成它的通用线段、圆弧、相切和闭合能力继续保留。

样条合同：`ControlPoints` 在 FIT 中仍是插值点，不能当成极点。CONTROL 明确使用 `poles`。canonical 数据存在时显示优先 rational de Boor，非法 canonical 不回退成近似“成功”；FIT 未求解的临时工具预览可近似，不作为权威曲线。开闭 CONTROL 会构造明确首末重复极点与新的 clamped basis，因此是改形状操作；不承诺周期性或 seam tangent。FIT→CONTROL 是显式保留精确曲线的转换。任意极点删除不能绕过合法结点移除验证。

## 验证映射

| 关键合同 | 邻近测试入口 | 关键断言 |
| --- | --- | --- |
| 基础求解和诊断 | `workers/geometry/sketch/tests/solver_scenarios.cpp`，`invoke check --scope sketch --match <name>` | 各类型残差、退化、DOF/冗余/冲突区分、连续拖动 |
| 连接/Profile | `profile_builder_test.go`、`sketch_profile_analysis_test.go`，`TestBuildProfileRegions*` | 排序/反向身份；真实椭圆/样条曲线；孔岛、自交、重叠；明确开放端 |
| 参数及原子修改 | `sketch_dimension_lifecycle_test.go`、`sketch_editing_test.go`、`associative_design_test.go` | 线线间距反向/倾斜/零；参考尺寸、表达式；修改失败模型不变 |
| 历史/实体链 | `service_test.go` 中 `TestSketchPadSupportsTwoUndoAndTwoRedoModelSteps`、`TestSolvedSketchChangeSetUsesPersistedAfterValue` | 每类复合操作 Solve → Profile → Extrude → 修改更新 → Undo/Redo → 冷读取 |
| 真实工具交互 | `sketch-creation/edit/dimension-lifecycle/ellipse/spline-canonical/profile-analysis.scenario.mjs`，`pnpm test -- sketch` | 动态输入、选择组合、操作预览、最后目标、迟到与取消；标注拖动不改尺寸 |
| Product 上下文 | `product-sketch-visibility.scenario.mjs`，`sketch-session-policy.scenario.mjs` | 在 Product 内编辑 Part 后宿主、标签、显示稳定 |
| 数据库基线 | `migration_test.go`，`TestMigrationBaselinePreservesDocuments` | 重复迁移和重开不改文档、Revision、历史；checksum 或旧迁移记录错误时拒绝继续 |
| 独立几何集成 | `sketch_workflow_integration_test.go`，匹配源码的真实 Worker/隔离数据库 | 下表列真实生产路径与独立几何断言；不推断未运行组合已验收 |

| 真实生产场景 | 定向入口 | 独立断言 |
| --- | --- | --- |
| 半轮廓关联镜像修改/历史 | `TestSketchWorkflowLinkedArcMirrorExtrudeHistoryColdRead` | 源半径变化、关联坐标、闭合、体积变化、Undo/冷读取 |
| 椭圆弧精确分割 | `TestSketchWorkflowEllipseArcExactSplitExtrude` | 曲线参数/真椭圆、闭合及实体体积 |
| 平行线间距与参考 | `TestSketchWorkflowParallelSpacingReferenceAndReversal` | 真实支撑间距、反向、参考测量/DoF与参数模式 |
| 矩形多圆角改宽 | `TestSketchWorkflowRectangleBatchFilletsDimensionUpdate` | 连接/相切、来源改宽后各角仍有效、实体更新 |
| 重叠圆快速修剪 | `TestSketchWorkflowQuickTrimPeriodicCirclesClosedUnion` | 周期区间及圆并集真实闭合/面积/实体 |
| 延伸闭合 | `TestSketchWorkflowExtendEndpointClosesRectangle` | 明确目标交点、正式连接和拉伸 |
| Rational 样条 knot/分割 | `TestSketchWorkflowRationalSplineKnotSplitExtrude` | 合法 basis 操作保留原曲线及精确子段/实体 |
| 外环/孔/岛/多区域 | `TestSketchWorkflowHolesIslandsDisconnectedSolidsOneBody` | 精确包含、体积、多个 Solid 属于一个 Body |
| 混合链独立偏移 | `TestSketchWorkflowMixedChainIndependentOffsetExtrude` | MITER/ROUND 两方式的真线弧曲线、正式闭合及独立实体面积 |
| FIT 点编辑 | `TestSketchWorkflowFitPointEditRebuildsCanonicalAndSolid` | 拟合点变化引起真实 canonical 曲线与拉伸体积变化 |
| 自映射圆随轴修改 | `TestSketchWorkflowSelfMirrorCircleTracksAxisEdits` | 不复制对称自身的圆；轴移动/旋转后仍保持正式关联 |
| 复制/变换/删除及历史 | `TestSketchWorkflowCopyTransformDeleteExtrudeHistory` | 新身份/内部引用映射、刚性变换、缩放拒绝、原子删除、体积/Undo/Redo/冷读取 |
| 三模式倒角修改 | `TestSketchWorkflowChamferModesDimensionsAndExtrude` | 等长、双长度、长度加角度；正式虚拟交点、修改尺寸及实体更新 |
| 线弧/弧弧圆角修改 | `TestSketchWorkflowLineArcAndArcArcFilletsExtrudeUpdate` | 真圆弧、真实端部切向、半径修改后仍合法闭合并更新实体；未据此声称邻边修改组合已测 |
| 圆弧补弧/闭合及修改 | `TestSketchWorkflowArcComplementAndCloseExtrudeUpdate` | Arc/EllipticalArc 真实范围、补弧/闭合、尺寸修改与独立实体体积 |

上述 15 个入口已使用匹配源码的真实 Worker、隔离数据库和独立几何断言运行通过（28.773 s）。C++ 74 个定向测试、Go workspace/control/geometry/database 定向测试及构建、Web sketch 11/71 与 workbench/Product/session 3/71 场景及构建通过；数据库基线及重复迁移验证入口见上表。详细结果保留在 `build/sketch-audit/verification.md` 及派生日志，不推断全部曲线组合已验收。

集成复现需设置 `OCCCCAD_TEST_DATABASE_URL`（专用隔离数据库）、`OCCCCAD_TEST_WORKER_ADDRESS`（匹配本次源码的真实 Worker）和 `OCCCCAD_TEST_ARTIFACT_ROOT`（隔离本地制品目录），然后从 `services/` 运行 `go test ./internal/workspace -run '^TestSketchWorkflow' -count=1`。任一配置缺失会明确 Skip；Skip 不计通过，也不指向应用数据库或 S3 进行清理。

执行结果另存 `build/sketch-audit/` 派生输出；不把环境日志加入能力表。本轮不运行浏览器自动测试或无差别全量单测；WebGL 和人工操作由维护者实机验收。

## 数据格式与目录迁移

Sketch 模型使用椭圆、canonical 样条、稳定端点/点身份、Reference 与关联镜像 selfMirrorMode，保持稳定 ParameterId、历史与严格引用校验。创建、编辑、约束和尺寸入口由 `internal/workbenchconfig/catalog.json` 定义，Mock 与真实 API 消费同一配置，并通过 Web CommandRegistry 执行。数据库只建立当前领域结构和必要管理员种子；两种后端的基线及显式重建规则见[数据库层说明](../services/internal/database/README.md#迁移)。

## 可操作的人工验收

1. 创建两条平行线，直接选择两条线创建间距；编辑 5 mm、0 mm、表达式及名称，反转线端点仍测同一非负间距。切参考后改变几何并检查测量更新；恢复驱动分别验证原定义与当前测量。
2. 创建有线及有向弧的半轮廓，以内置轴做关联镜像，检查轴上连接及不重复的自映射元素。确认后端闭合分析再拉伸；改源尺寸/轴，再检查闭合和实体更新。
3. 矩形多个角点做圆角及三种倒角；改变半径/长度及原邻边尺寸。短边/过大半径请求须整体失败；取消和 Undo/Redo 不留下半角点。
4. 重叠圆按点击区间快速修剪，依次验证删除命中、保留命中及仅分割。尺寸/对称引用影响必须明确，不能就近重新绑定。
5. 选择 Line/Circle/Arc 连续链，以正/负距离分别创建独立偏移，验证左法向侧与 MITER/ROUND连接；半径归零/变负、凹角交叉或自交整体拒绝。源尺寸改变不驱动独立偏移，用户可明确看到独立副本语义。
6. 椭圆弧/样条端点接线后闭合拉伸；FIT 插入/删除保持其余点身份。显式转 CONTROL 后控制端部切向；插入并合法移除结点保持真实曲线，非法移除明确拒绝。
7. 外环、孔、岛和两个独立区域一次拉伸，检查一个 Body 可有多个 Solid。撤销、重做并重新打开后，尺寸来源、端点/极点身份和实体结果一致。
8. 在 Product 内编辑 Part 草图执行以上局部流程，更新时宿主、标签、显示和编辑归属保持一致；拖动尺寸文字只改变位置。
9. 对有向圆弧和椭圆弧执行补弧与闭合，检查真实曲线方向/范围；修改半径或半轴后检查轮廓和实体更新。

人工验收覆盖实际鼠标/键盘、WebGL 呈现和更多分支组合；共享基础测试不代替这些验证。

## 本机 CATIA 用户语义依据

索引：`/mnt/s/tools/DS/Catia/B33doc/English/control/CfyEnglishC2.viewdoc`；以下均在其 `online/cfyugdys_C2/` 条目下：

- `cfyugdys0501.htm`（Trimming Elements）：一条或两条修剪、点击位置选分支、端部关系影响。
- `cfyugdys0502.htm`（Breaking and Trimming）：Quick Trim 的删除命中、保留命中、分割保留三种语义，生成明确 Coincident。
- `cfyugdys0509.htm`（Creating Mirrored Elements）：选择源及线/轴；几何约束开关决定建立对称关联。
- `cfyugdys0313.htm`（Creating Corners）：相切圆弧、半径、双侧/单侧/不修剪、Construction 选项、多个角点。
- `cfyugdys0314.htm`（Creating Chamfers）：双长度、长度加角度、修剪选项及旧交点的尺寸意义。

这些页面仅用于用户语义参照，不是 occcccad 已交付能力或内部架构要求；本任务的原子失败、稳定引用等合同仍以本仓库为准。
