# Web 工作台与交互边界

> 2026-09-21 文档核对。返回[当前架构](../../CURRENT_ARCHITECTURE.md)。本页描述实现，Product/WebGL 人工验收已由维护者确认通过，自动化测试与限制见[完成记录](product-assembly.md#accept-product-完成记录)。

## 状态与模块职责

```mermaid
flowchart TD
    Pages["Auth / Document Center / Workbench"] --> Query["TanStack Query: 服务端状态"]
    Pages --> Store["Zustand: 短期交互状态"]
    Pages --> Command["Command Registry"]
    Command --> Tool["Tool Manager"]
    Tool --> Interaction["Input / Navigation / Selection"]
    Interaction --> Engine["Viewport Engine"]
    Engine --> Three["Three.js / BVH"]
    Query --> Adapter["Mock / HTTP / Realtime adapter"]
```

页面通过 Viewport Engine 操作视口，不直接拥有 Three.js Scene/Renderer。模型、权限、Revision 与最终求值属于服务端；camera、hover、selection、工具采集和渲染插值是可丢弃状态。Mock 用于 UI 调试，不证明后端权限或几何正确性。

工作台由独立结构树、视口和属性/历史面板组成。树筛选保留祖先与 stable key，虚拟树支持键盘和集合选择；Inspector 关闭时卸载，按页签请求数据，读取失败明确报错。Document tabs 位于全局标题栏，支持切换、关闭、新建、排序及会话恢复。

## 根场景与编辑上下文

Product 始终显示根装配场景，Active Occurrence 的 Reference Workspace 决定命令、历史和属性目标。`ViewportEditContext`/`editingView` 将活动 Part 草图的拾取、约束、尺寸拖拽和编辑从根 Product 投影中分离；切换激活状态不产生空 Revision。创建 Part、Context Catalog、Pin/Follow 和产品版本中心的领域行为见[Product 架构](product-assembly.md)。

进入 Sketcher 保留权威 Body 作为只读环境，活动 Sketch、约束与预览叠加其上。普通工具只接受活动 Sketch；显式 Projection 才能进入上游 Body Edge/Vertex 选择并绑定 PersistentSelection。ExternalGeometry 独立、只读，使用同一捕捉/约束路径。退出后选择提升为 Sketch Feature；未消费草图保持可见，已消费草图在选择或重新编辑时临时显示。

## 显示制品与稳定选择

Part 求值输出 schema v1 `VisualizationManifest` 并写入 GLB 的 `OCCCCAD_visualization` 扩展。Point、曲线、约束 glyph 和尺寸引线携带稳定 entity/constraint/Feature identity、role、求解状态和关联实体；它们是可重建显示制品。Part 与 Product 共用 renderer 与 selection identity builder，Product 只施加 occurrence Transform，不维护另一份 Sketch 模型。

InputManager、Tool、Selection 和 Overlay 共用完整 pointer down/move/up/cancel、capture/lost capture、Esc/blur 生命周期。hover、正式选择和工具保留引用独立呈现；切换状态先释放旧覆盖再重建。隐藏对象及其子孙退出拾取候选。工具声明 geometry/instance 选择模式，在 hover/select 之前完成语义投影；树和视口按稳定 identity 同步，树祖先高亮不反向扩大精确拓扑选择。

最终 Body 制品绑定结果 Feature；当前没有历史中每个 Feature 的独立可视 Result。`geometryKey + local topology ID` 只用于当前制品拾取证据，持久引用仍由服务端绑定。Publication 同时关联视口、所属树节点及发布节点；无法解析约束显示锚点时回退 occurrence 中心，保持 Broken 可选、可修复。

捕捉过滤与 Selection 独立。三维类型过滤、草图网格/端点/中心/中点/曲线投影共用候选排序；禁用候选不能遮挡后方可用对象。显示折线上的投影仅是交互近似。命中稳定点时，同一编辑批次显式添加 Coincident，不能只保存相同坐标。

## 命令、预览与提交

Toolbar 来自服务端版本化 Presentation Catalog，稳定 ToolbarId 表达用户意图类别；命令仍需本地 CommandRegistry 注册并满足上下文 capability。未知命令不可执行，目录不承载远程代码。默认命令区按工作台/锚点呈现；导航、捕捉和显示单位属于偏好。全局快捷键避开输入框、IME、重复按键和对话框；上下文帮助点击不执行命令。

工具单击完成一个逻辑操作后回到选择，双击进入连续模式；Polyline/Spline 用 Enter 或双击完成多点采集。尺寸工具按引用选择、真实几何测量初值、放置、内联输入、提交运行；拖动尺寸位置和实体点只在 pointerup 形成一个 Domain Command。显示/输入单位在 UI 转换，权威数量和表达式由服务端验证。

非模态 CommandDialog 允许继续拾取。Part preview 复用正式 adapter、handler、参数/Sketch/Feature evaluator，返回带 base Revision/provenance 的精确结果；它不产生 Revision/历史/Outbox。提交可携带 verified candidate token，仅当 actor、document、base head/sequence、命令类型与 payload digest 全部匹配时提升候选，否则重新求值。当前候选和 warm-start cache 有 45 秒 TTL、256 项进程上限；丢失只影响性能。

交互以 interactionId 和单调 previewSequence 取消旧请求、丢弃迟到响应。装配前端 actor 管理草拟/预览/确认/取消，服务端 workflow 管理解析/求解/应用/失败；二者不替代 Revision 或 solver 数值状态。nominal pose 来自 Revision，前次权威解只作短期 initial guess。数值输入通常在 blur/Enter 请求权威预览，成功后才能提交。

实体 Feature preview 显示后端完整结果并临时替换当前 occurrence 的旧实体；取消恢复原可见性。Placement 动画只插值 TRS，旋转使用 quaternion slerp，同批 occurrence 共用时钟；新目标从当前显示帧接续，最终精确落到权威姿态。手柄与 Instance 使用同一确认帧。MOVE 的约束能力仍受临时 Fix 路径限制，不能据此宣称最近可行拖拽。

## 导航、布局与偏好

默认正交 ISO 朝向，Fit 按可见内容计算；普通刷新保留视图，草图进入/退出保存并恢复相机上下文。网格独立于模型包围盒、Fit 和拾取。Default、CATIA 和 SOLIDWORKS 导航由独立状态机实现；适用手势、参考资料与验证限制见[导航 README](../../../web/apps/cad/src/cad/navigation/README.md)。

schema 5 的 `occccad.ui-preferences.v1` 保存 Inspector、面板/工具条布局、树宽/显隐覆盖、导航、捕捉以及默认/按 DocumentId 的显示单位。显示单位影响格式和新输入，不重写已有表达式或变成共享 Revision 属性；隐藏为本地显示，抑制是持久领域状态。当前树使用独立侧栏、全高 resize separator 和展开箭头，不再采用旧 UX 计划中的圆环图标与角形 grip。

基准轴/面是屏幕稳定的辅助几何，关闭深度测试并有专用拾取规则，避免被实体遮挡后不可选择。语义色集中在 visual tokens，selection/hover/preview、求解诊断和坐标轴保持不同含义；精确像素、色值和动画时长由实现及视觉场景维护，不作为架构契约复制。

Insert 复用 ACL 文档搜索和缩略图 API，支持范围、文件夹、搜索、分页及失败重试；提交只传稳定 DocumentId，循环引用由服务端验证。诊断下载使用文档 ACL/CSRF，导出命令、Workspace、manifest 和日志关联，排除凭据、无关文档日志和原始 B-Rep。

## 实现与验证入口

- [Web 运行与测试](../../../web/apps/cad/README.md)
- [编辑上下文](../../../web/apps/cad/src/features/workbench/product-edit-context.ts)
- [Product 场景](../../../web/apps/cad/src/features/workbench/testing/product-edit-context.scenario.mjs)
- [Publication 选择](../../../web/apps/cad/src/features/workbench/testing/publication-selection.scenario.mjs)
- [偏好 schema](../../../web/apps/cad/src/state/ui-preferences.ts)
