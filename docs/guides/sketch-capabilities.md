# 二维草图能力与验证入口

本页提供可重建的验证入口与人工检查步骤；测试存在、历史通过和完整用户验收分别判断。

## 能力来源

[Part/Sketch](../architecture/current/part-sketch.md)拥有实体、约束、参数、Profile、支撑和外部几何的事实与限制；[实体 Feature](../architecture/current/solid-features.md)拥有后续建模能力。此页仅维护验证映射与人工步骤，不另复制模型合同。

PlaneGCS 的锁定来源与逐文件摘要由 [CMake](../../workers/geometry/sketch/CMakeLists.txt)拥有；不据上游最新接口推断当前能力。

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
| 独立几何集成 | `sketch_workflow_integration_test.go`，匹配源码的真实 Worker/当前开发数据库 | 下表列真实生产路径与独立几何断言；不推断未运行组合已验收 |

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

集成复现通过根 `.env` 加载开发数据库、匹配源码的真实 Worker 与制品目录，从 `services/` 运行 `go test ./internal/workspace -run '^TestSketchWorkflow' -count=1`。配置不足时按入口报告 Skip/环境阻塞，均不计通过；资源与清理边界遵循[开发环境](../development/environment.md)。

执行结果另存 `build/sketch-audit/` 派生输出；不把环境日志加入能力表。浏览器/WebGL 与完整链路需独立验收。

## 数据格式与目录迁移

Sketch 模型使用椭圆、canonical 样条、稳定端点/点身份、Reference 与关联镜像 selfMirrorMode，保持稳定 ParameterId、历史与严格引用校验。创建、编辑、约束和尺寸入口由 `internal/workbenchconfig/catalog.json` 定义，Mock 与真实 API 消费同一配置，并通过 Web CommandRegistry 执行。数据库只建立当前领域结构和必要管理员种子；两种后端的基线及显式重建规则见[数据库层说明](../../services/internal/database/README.md#迁移)。

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

索引：`/mnt/c/UserApp/3D/DS/Dassault\ Systemes/B33doc/English/control/CfyEnglishC2.viewdoc`；以下均在其 `online/cfyugdys_C2/` 条目下：

- `cfyugdys0501.htm`（Trimming Elements）：一条或两条修剪、点击位置选分支、端部关系影响。
- `cfyugdys0502.htm`（Breaking and Trimming）：Quick Trim 的删除命中、保留命中、分割保留三种语义，生成明确 Coincident。
- `cfyugdys0509.htm`（Creating Mirrored Elements）：选择源及线/轴；几何约束开关决定建立对称关联。
- `cfyugdys0313.htm`（Creating Corners）：相切圆弧、半径、双侧/单侧/不修剪、Construction 选项、多个角点。
- `cfyugdys0314.htm`（Creating Chamfers）：双长度、长度加角度、修剪选项及旧交点的尺寸意义。

这些页面仅用于用户语义参照，不是 occcccad 已交付能力或内部架构要求；本任务的原子失败、稳定引用等合同仍以本仓库为准。
