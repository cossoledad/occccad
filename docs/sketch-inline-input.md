# 草图当前动作与 inline 参数输入

草图当前动作继续由工具的 `SketchCommandState` 推进。状态栏显示 actor 的当前 role、接受/所需数量、字段入口与完成入口；不从中文提示解析阶段或重建选择状态。ToolManager 拥有工具生命周期，Viewport 只转发状态与动作，Workbench 仅显示投影。

`presentation=inline` 时，视口只显示 `input.fieldIndex` 对应字段的引线数值，不显示常驻输入卡片；点击数值或直接键入时展开无边框编辑，Enter 接受，失焦不提交。`input.id` 表示焦点请求代际，hover 更新不重置输入焦点或数值草稿。数值、单位和表达式原文通过 field action 回到 actor；长度使用当前文档单位，角度为 deg；已打开草稿以 actor 的 displayUnit 保持输入单位，不因外部偏好改变重新解释数值。Tab 切换字段，Enter 单次确认，Esc 退出整个所属工具；失焦不取消工具，IME 和重复按键不触发确认。没有输入请求时不常驻参数面板。直接镜像/倒角的次级定义通过状态栏“选项”按需弹层修改；双距离/距离角度的第二字段可由状态栏字段入口或 Tab 切入，仍通过同一 actor action 保存原文。普通单半径/单距离不增加空选项面板。需要完整高级选项的工具仍显式使用 advanced 面板。

所有草图按钮单击单次、双击明确请求连续。尺寸的选择、放置和数值阶段由尺寸工具推进；首线在定义阶段仍可接受明确第二对象，锁定长度或空白放置才接受单线定义；进入 Placement 后引用冻结。镜像先选轴再选来源集合，Enter/完成入口提交；修剪默认删除命中，hover 只展示候选区间。确认仍走既有正式 Sketch 命令，未知结果保留原 requestId/operations 并查询回执，阻止新编辑，不因输入 UI 消失或退出工具宣称请求已撤销。

悬停引用、已接受选集、尺寸预览和修剪区间具有独立生命周期。统一层级使选集与候选高于尺寸引线/文字；尺寸标记采用透明文本，不绘制大块背景或额外尺寸图标。视图引用反馈只高亮几何，不显示 #1/#2 编号。提交回执继续由 Engine 管理，正常等待不显示底部提示框，未知回执在状态栏提供恢复入口。

## 定向验证

- `sketch-inline-ui.scenario.mjs`：实际 React SSR 和生产控件 props，当前动作/count/completion、只显示当前字段、数值聚焦与按需编辑、单位/表达式原文、Enter 防重、Esc、Tab、IME/repeat、pending/unknown 输入锁定及单击/双击策略。
- `sketch-input-session.scenario.mjs`、`sketch-edit-session.scenario.mjs`：真实 InputManager/Router/ToolManager 的阶段、完成、取消和未知回执。
- `sketch-direct-workflow.scenario.mjs`、Go `TestSketchDirectProductionToolReplay`：真实工具手势 → 正式操作 → 权威 Worker 快照 → Profile/Pad 主闭环，新增真实回放结果须单独确认。
- `sketch-feedback.scenario.mjs`：真实 Engine 场景对象、选集元数据、独立 hover/尺寸层、缩放与镜像/尺寸工具角色；不创建引用序号。
- `sketch-dimension-lifecycle.scenario.mjs`：数量/表达式/稳定 ParameterID、参考草稿、原请求重试及编辑对话框。

从 `web/apps/cad/` 执行 `pnpm test -- sketch`、`pnpm typecheck` 和 `pnpm build`。本轮未运行浏览器自动测试；SSR 和 Node 场景不证明真实焦点轨迹、布局或 WebGL 已通过验收。

## 人工步骤

1. 激活镜像/修剪/圆角，观察状态栏当前动作及已接受数量；空白鼠标移动不重新解释阶段，也不出现完整常驻选项面板。
2. 选够对象后输入半径、偏移或尺寸，检查引线数值；点击或键入进入当前字段编辑。尝试表达式和 in/cm；hover 不清空或重新全选正在编辑的文字。
3. Tab 切字段、Enter 确认一次、IME 输入不确认；输入框 Esc 应直接退出所属工具。单纯点击别处或失焦不自动退出工具。
4. 连续修剪/分割/圆角成功后继续下一组；Mirror 单次成功回选择，双击模式可连续。失败保留草稿，未知响应只查询原请求，不能再次生成重复操作。
5. 打开高级编辑和已有尺寸编辑，确认选项、引用更换、参考模式、取消与未知回执屏障仍可用。在 Product 内编辑 Part 草图重复以上步骤，宿主与标签保持。

完整十步主闭环和引用/回执检查见[草图交互验收](sketch-ux-02.md#人工验收)。
