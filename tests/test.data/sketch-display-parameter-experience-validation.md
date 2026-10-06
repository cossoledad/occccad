# 草图、对象显隐与参数输入验收

本轮复用 SelectionIndex、VisibilityResolver、参数 AST 和版本化显示命令；未修改几何算法。当前实现见[显示模型](../../docs/architecture/current/model-display.md)、[参数与历史](../../docs/architecture/current/model-history.md)及[草图](../../docs/architecture/current/part-sketch.md)。

## 统一规则

- 显示制品复用同步根 Part 的当前 Revision；只有原 GeometryKey 仍存在的拓扑才同步。occurrence 按各自权威路径解析，保留 PINNED 版本。基准位置/方向/增删改变刷新参考图形和拾取对象。
- 连续轮廓的起点、中间顶点进入屏幕像素捕获器，提交时生成真实端点 Coincident。草稿捕获只存在于会话，结束/取消清除。
- Origin、基准面/轴、轴系、原点及 XYZ 子轴是 Part 显示属性；约束标记是 Product 显示属性。继承隐藏不覆盖子对象本地值；隐藏退出拾取，不改变求解、支撑或约束抑制。Datum 编辑保留显隐，显示属性不进入几何依赖摘要。
- 场景地面/草图网格分别保存在前端偏好，关闭网格不关闭捕获；投影可临时揭示显示偏好关闭的参考元素，不能揭示定义隐藏对象。
- 新参数别名使用可读英文基名、下划线和首个未用正整数，按定义顺序分配；保留已有别名及 ParameterId。树右键复制别名，HTTP 剪贴板提供文本选择复制回退。列表省略技术 ID，属性面板保留。
- `r_1 + 5` 的无单位数值项采用该输入的默认单位；`r_1 * 2` 的倍数仍无量纲。显式单位和参数比值保留量纲检查，持久 AST 与值继续使用 SI。

## 已验证

- Go 定向测试：表达式默认单位、合法/非法量纲、唯一稳定别名、Datum/参数/阵列依赖、显示属性补偿历史和冷反序列化通过。
- TypeScript 类型检查及 Vite 生产构建通过；相关 Node 场景覆盖投影旧 Revision、替换拓扑拒绝、occurrence、闭合连续轮廓、显示/拾取、网格偏好、参数和剪贴板。
- 真实 Router/Worker：Bottom Support 的 `r_2 = r_1 + 5` 驱动基准→投影→草图→实体，验证支撑平面/Origin/X 轴隐藏、Undo/Redo、冷重开及几何保持。
- 真实 Router/Worker：Signed Offset 装配约束隐藏保留已接受状态和位姿，Undo/Redo、冷重开、参数表达式与多个 occurrence 通过。
- 独立临时 SQLite/ArtifactStore；未清理或修改用户文档。未进行浏览器自动测试或无差别全量单测。

复现：Go 在 `services/` 下使用上述两条 `TestBottomSupportParametricDatumLifecycleThroughRouter`、`TestOffsetSignedProductHistoryThroughRouter`；设置 `OCCCCAD_TEST_GEOMETRY_WORKER` 为当前构建的 Worker，后者另指定独立的 `OCCCCAD_TEST_DATABASE_URL=sqlite:/绝对路径/assembly.db`。Web 在 `web/apps/cad/` 运行 `pnpm build` 和 `pnpm test -- sketch-interaction assembly-occurrence-snapshot visibility-resolver ui-preferences parameter-editor workbench-inspector semantic-tree selection-index projection viewport visual-repository clipboard`。

## 人工验收

1. 后台 API/Jobs/Worker 已按当前代码重建重启，刷新前端。创建草图、修改尺寸后不刷新页面，反复进入投影；从视区和树选原点、标准轴、基准轴及实体边/顶点。切换 occurrence 后不得串选。投影仍不接受面或基准面作为直接投影源。
2. 连续轮廓靠近起点闭合，靠近中途顶点捕获；确认闭合 Profile 可拉伸，Esc/取消后下一条绘制不捕获旧草稿。
3. 隐藏 Origin，单独隐藏 X/原点/基准，恢复父项时子项本地状态保持；重开和 Undo/Redo 验证。装配隐藏约束标记时零件不能移动或失去约束。
4. 新建基准轴/面和特征，复制树参数别名；输入 `r_1 + 5`、`r_1 * 2` 并修改 r_1，确认传播。参数列表没有技术详情，选中参数后属性面板仍有 ID。
5. 分别切换地面和草图网格，确认只控制显示；关闭网格后捕获设置仍生效。

浏览器/WebGL 实机交互尚未验收；自动场景和 Worker 验证不代替上述操作。

## 投影扩展与运行版本核对

实机诊断 `01a1109b-cc20-7e29-a6b2-d126f7ec28ac/20261006T111325.259622156Z-5adb050550cf1c4d` 的平面隐藏失败来自旧 API（10 月 5 日构建），实际错误为 `PLANE has no independent display result`。本轮更新运行进程，并修复显隐附带基准求值槽导致 Undo/Redo 重算的问题。别名、复制菜单、`p111 + 4` 沿用已有实现，未增加兼容分支。

投影新增圆弧、椭圆、椭圆弧；圆斜投影为精确椭圆，侧向退化为含区间极值的线段。使用真实 Naming 的轴、X 方向、半径和角度区间，不用显示折线拟合；二维快照供求解、测量、显示和解除引用共用。圆心显示为可捕获、可约束的 CENTER 子元素。

定向入口：C++ `ExternalGeometryProjector.*`；真实 Router/Worker `TestConicProjectionLifecycleThroughRouter`（完整圆/圆弧/斜圆弧/椭圆/椭圆弧、圆心 Coincident、解除引用、Undo/Redo、冷重开），并回归 `TestBottomSupportParametricDatumLifecycleThroughRouter`、`TestPerpendicularEdgeProjectionThroughRouter`、`TestLoftLinearBoundaryProjectionThroughRouter`。前端 `projection` 场景验证椭圆弧数据、圆心捕获；TypeScript 与生产构建验证调用合同。

人工补充：分别投影完整圆、修剪圆弧、椭圆弧，选择投影圆心建立重合约束；改变上游后查看关联更新，解除引用再撤销。隐藏支撑平面后 Undo/Redo 和重开，确认实体不变；创建关联 Plane 后复制参数别名，输入 `p111 + 4` 并确认数值采用输入默认单位。Face 边界组、一般 BSpline、Section/Silhouette 尚未实现；未执行浏览器/WebGL 自动验收。
