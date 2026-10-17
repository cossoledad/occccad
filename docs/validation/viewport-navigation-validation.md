# 视口、导航与透明边线验收

日期：2026-10-17；基线 `b35fbb29009a02e722fd48baae49a40520d89f4c`，本次工作树。当前规则见[对象属性与全局选项](../architecture/current/object-scene-properties.md)、[颜色材质](../architecture/current/surface-appearance.md)和[导航实现](../../web/apps/cad/src/cad/navigation/README.md)。

本轮只修改 Web 显示与输入；使用当前 `.env` 的 SQLite、LOCAL 制品、已有真实 API/Release Worker，以及已有 Chromium 软件 WebGL。没有重置开发数据、修改 Proto/持久模型或下载浏览器。

## 具体证据

| 范围 | 实际结果 |
|---|---|
| 完整 Web | `invoke check --scope web`：114/114 场景、类型检查和生产构建通过；代码固定后报告 `build/agent-check/20261017T060836-f9df9579/report.json`，日志 `viewport-refinement/web-delivery.log`。 |
| 定向浏览器 | `navigation`、`properties`、`body-color`、`viewport-rendering` 及机构属性操作共 12 项通过，含完整 6 项导航用例；`viewport-refinement/browser-accepted.log`。最后独立背景明暗判断调整后补跑场景、属性与渲染 4 项通过，`contrast-browser.log`。 |
| 真实浏览器 | 当前 API/Worker/数据库上的 2 项通过；`viewport-refinement/live-accepted.log`。 |
| 知识入口 | 文档审计 8 项、`invoke context-audit`、`git diff --check` 通过；`viewport-refinement/doc-tests.log`、`context-audit.log`。 |

以上日志相对 `build/agent-check/`。中途两次完整检查因检查期间继续修正源码而标记 `SOURCE_CHANGED`，未作为最终通过证据；最终报告的 source/sourceAfter 相同。

- 图标入口、固定间距/自动地面间隙、透明结构树、CATIA 主键按下选心与三维十字线均由实际工作台浏览器操作覆盖。
- `browser/viewport-rendering.spec.ts` 在真实 WebGL 中调用生产网格、材质、深度通道和宽线：顶视模型覆盖地面、下视地面线覆盖模型；透明表面后的细线/宽线亮度明显高于黑色无遮挡线。该部分是隔离渲染 fixture，不冒充 API/Worker 链路。另一个用例通过实际工作台验证顶点/面/空白取心、零选择副作用、SVG 光标加载及画布与透明树重叠。
- 真实 `properties-live.spec.ts` 2 项通过，覆盖真实建模、Body/面透明度与颜色、Preview 取消、Undo/Redo、CAS 保留草稿、自动负 Z 地面/世界模式、新建外观快照。自动高度断言按实际模型 bbox 计算 5% 间隙，并按 UI 的两位小数比较。日志 `build/agent-check/viewport-refinement/live-accepted.log`。
- 对指定文档 `01a14949-cfc1-7954-b280-aa632796fbaa` 实际打开斜面 `Sketch 2`、正对/退出草图、从下方旋转和切换明亮场景。前后 Head 均为 `01a149dc-ab21-74cc-a983-641f0c9ce352`，没有写入模型；没有 pageerror 或 GLSL 编译错误。证据 `target-acceptance.log`、`target-from-below.png`、`target-oblique-symbols.png`、`target-light-tree.png`，均在 `build/agent-check/viewport-refinement/`。

## 导航回归中的修正

真实浏览器事件记录表明，释放 CATIA 侧键后会出现仍带主键 buttons 的 `lostpointercapture`，此前将其当成整段手势取消，造成缩放锁存丢失。新的续接只允许仍持有主键且 pointerId 相符的 CATIA 导航；工具、pointercancel、失焦、隐藏和 Esc 仍取消。Node 的 `navigation-capture` 和原生浏览器组合键用例覆盖该路径；诊断日志 `capture-diagnostic.log` 同时记录关闭 SVG 光标后仍复现，因此没有将其归因于光标图形。

原导航用例另外存在过时假设。本轮保留选择、比例、旋转和投影断言，并修正验证方式：

- 用开发调试快照的完整精度比较投影，避免极远相机的四位小数四元数放大为像素误差；正交裁剪允许沿视线平移眼点，但必须保持显示投影。
- 等待草图进入动画最终姿态再记录基线。
- 消费后隐藏的草图先通过正式“显示”动作开启，再要求退出后的可见高亮；不绕过定义级隐藏。
- 当前拉伸命令支持无预选启动，因此验证清除选择后打开的是空轮廓选择，而不是要求旧式禁用按钮。

恒定像素辅助几何的世界原点仍参与 Fit，其当前装饰长度不参与，防止下一次 Fit 依赖上一次 zoom；邻近 Node 场景验证此不变量。

## 参考与限制

核对本机 B33 `control/BasEnglishC2.viewdoc` 与其引用的 `online/basug_C2/basugbt1201.htm`、`basugbt1208.htm`、`basugbt1305.htm`，以及 [3DEXPERIENCE 原生基本手势说明](https://help-3dexperience.aesvietnam.com/English/ComUserMap/exp3dbasics-r-BasicViewingCommands.htm)（达索说明的公开镜像）。中键居中/平移、双键旋转和释放侧键缩放来自说明；悬停点优先级、模型相对间隙、十字线尺寸与球面算法是本软件明确实现的规则，没有原版软件逐轨迹/像素对照。

透明边线目前区分最近可见表面前后关系，不是多层玻璃光学或 OIT；复杂透明面相交仍受常规排序限制。额外深度通道只在可见半透明 CAD 表面存在时执行，本轮没有性能/帧率基准。固定地面间距在极端缩小时允许抗混叠淡出，草图网格和捕获仍自适应。硬件 GPU、PostgreSQL/S3、全仓原生几何/交换未在本轮重验。
