# P10 Product 工作台交互收敛

状态：已完成自动化实现，待浏览器人工验收
日期：2026-09-19

本文承接 P10A–P10H 的领域能力，收敛 Product 内就地设计、引用更新、发布选择与版本发布的工作台语义。它不改变不可变 Revision、Resolution Snapshot、SolveManifest 或 ProductRelease 合同；UI 只驱动既有版本化 Domain Command。

## 1. 设计原则

1. Product 是默认设计上下文。根 Product 负责装配显示，Active Occurrence 的 Reference Document 负责草图、约束、参数和特征命令；两种视图不可混用。
2. `FOLLOW_HEAD` 是日常编辑默认值。来源 Head 变化后，打开 Product 的客户端按叶到根自动接受带 digest 的 `ProductUpdatePlan`，每一级仍产生普通不可变 Revision，绝不令旧 Revision 漂移。用户仅在需要冻结依赖时把 Instance 切换为 `PINNED`，并可恢复跟随。
3. Publication 是面向人的稳定接口。选择已发布元素时，视图区几何、所属 PartBody（折叠时最近可见祖先）和 Publication 节点共同高亮；装配约束面板优先显示 `ReferenceName(InstanceName) / PublicationName`，UUID 只保留在诊断细节中。
4. 基准几何是屏幕空间辅助对象。三条正半轴和位于正象限的三个小基准面保持固定屏幕尺寸、关闭深度测试并始终可选，不随模型包围盒放大；面片与轴线保留屏幕空间间隙，轴线采用专用虚线及较小命中容差，避免压住模型边时抢选。
5. Product Release 是不可变产品里程碑，不是 STEP/BREP 导出对话框。版本中心负责 gate、冻结、列表与 replay；Exchange 后续作为独立工作流呈现。
6. Toolbar 的稳定边界是用户意图类别，不用单个大型 Toolbar 内的分隔符模拟多个类别；目录 ToolbarId 是唯一分类来源，命令注册仍保持 UI 与领域命令解耦。

## 2. 实施批次

### UX-A：Active Occurrence 草图上下文

- 视口保留根 Product 场景，但所有草图实体命中、框选、尺寸拖拽、约束预选和编辑均读取 `editingView`。
- 增加 Product 内激活 Part 的场景测试，防止再次退回根 Product 的空 `part` 投影。

### UX-B：引用模式与自动更新

- Instance 右键提供“固定当前版本/恢复跟随最新版本”。
- 服务端结构树 capability 决定入口；命令目标是该 Instance 的 owning Product，而不是根 Product 或 Reference Part。
- FOLLOW_HEAD 的 stale closure 通过现有 `ProductUpdatePlan` 自底向上自动接受；失败保留诊断且不部分推进单个计划。

### UX-C：基准、树与发布选择

- 基准轴/面使用独立材质和屏幕稳定缩放，`depthTest=false`、高 render order。
- Instance 显示 `ReferenceName(InstanceName)`；分支节点使用一个圆形爆炸控件表达收起/展开，不使用平台折叠箭头，也不拆成四个独立四分之一圆弧。
- Publication 选择投影补全 Publication identity，驱动三处高亮和装配 `PublicationRef`。

### UX-D：约束、发布版本与 Toolbar

- 约束支持元素采用可读的 occurrence、Publication/拓扑类型和语义锚点两级文案。
- Product 版本中心替代“Release + Exchange 导出”混合对话框。
- Toolbar Catalog 补齐稳定分组；组间有明确视觉分隔。

### UX-E：全局偏好、工作区布局与分类工具栏

- 标题栏提供统一用户偏好中心。鼠标操作模式、三维选择/草图吸附和用户默认显示单位从 Workbench 瞬态 Store 移入版本化 UI preference；捕捉和导航不再重复出现在各工作台 Toolbar。
- 文档可覆盖用户默认长度显示单位。当前实现是按稳定 DocumentId 保存的用户侧显示/新输入偏好，不改写参数表达式和内核规范毫米值；若后续需要团队共享单位约定，必须新增版本化 Domain Command，而不是把本地偏好伪装成 Revision 属性。
- 结构树右缘提供可键盘操作的水平 resize separator，宽度限于 220–640 px 并跨会话记忆；树分支使用圆形爆炸状态。
- 文档新建/切换/关闭入口从视口左下角迁入全局标题栏，避免遮挡模型与状态栏。
- Catalog 改为一类一栏：选择、草图入口、实体特征、参数与接口、基准、草图会话、外部几何、基本元素、轮廓、两类草图约束、产品结构、产品接口、组件定位、装配约束、历史、协作、视图和诊断。Publication、Product Release 与标准视图使用各自语义图标。
- 默认工具栏按停靠位置分行，用户拖动后的坐标与方向继续覆盖默认布局。

## 3. 验收边界

- 自动化：Go workspace/API 测试、Web scenario、TypeScript build、相关领域检查。
- 浏览器：Product 新建 Part → 激活 → 草图约束；Publication 三处高亮；遮挡状态选择基准；Pin/Follow 与上游修改；约束面板文案；创建并 replay Product Release；偏好重载、文档单位覆盖、结构树拖宽、标题栏文档切换以及新 Toolbar 默认布局。
- 当前不新增任意历史版本 picker、Configuration/Design Table、柔性子装配或 Exchange 格式管理器。

## 4. 本地 CATIA V5 对照

- `online/basug_C2/basugbt0502.htm`：Specification Tree 与 geometry focus 切换及树宽调整；本项目保留浏览器视口 focus，只采用可拖动树边界和跨会话记忆。
- `online/basug_C2/basugbt0508.htm`：Specification Tree 的层级展开/收起语义；本项目以圆形爆炸状态承载同一层级动作。
- `online/basug_C2/basugcu0102.htm`：Toolbar 的独立命名、显示、内容和位置恢复；本项目据此把类别提升为稳定 ToolbarId，同时保持服务端目录与用户布局分层。
- `online/bascukwr_C2/bascuparameasure0400.htm`：全局 Units 选项；本项目进一步区分用户默认显示单位、用户侧文档覆盖与未来版本化团队文档属性。

以上页面均位于 `/mnt/s/tools/DS/Catia/B33doc/English/`。它们只用于确认用户流程和术语，不作为内部实现或当前能力证据。
