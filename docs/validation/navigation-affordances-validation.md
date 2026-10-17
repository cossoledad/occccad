# 导航十字、光标与结构树手柄验收

日期：2026-10-17；基线 `d05065dce57062769bc28151590b7f9a727ca4e1`，本次工作树。当前规则见[导航实现](../../web/apps/cad/src/cad/navigation/README.md)与[Web 工作台](../architecture/current/web.md)。

本轮仅调整 Web 显示与面板手势，没有修改相机导航数学、模型、Revision 或服务端协议，没有重置开发数据或下载浏览器。

## 实现与边界

- 原球面导引投影固定三维角段，在球边发生缩短。现在两条屏幕弧各保持 52 CSS 像素弧长，曲率随球面位置变化并在边缘有界；指针越界时不吸附到圆周。显示几何由 `hud/navigation-hud-geometry.ts` 构造，轨迹球输入算法保持独立。
- 中心世界方向十字的 X 跨度继续为球直径的 1/3.5，Y 为 X 的 17/22；X/Y 正半轴有箭头，负半轴压暗，保留中央间隙和短黄色 Z。固定屏幕中心与相机逆旋转仍由 HUD 控制；长度比例是世界轴的固有比例，屏幕投影随观察方向变化。
- 结构树删除独立边缘手柄，复用收起按钮。4 像素移动阈值区分点击和拖动，拖动后抑制本次 click；取消恢复按下前的持久宽度。按钮保留方向键、Home/End、Enter/Space 与展开后的焦点交接。折叠使用 `display:none` 保留 React 状态，避免重新聚焦时继承隐藏状态；响应式宽度上限和画布覆盖布局沿用现有规则。
- 光标取消白色底描边，改为透明背景、单层圆角线。旋转箭头与圆弧共享端点，缩放镜片与热点对齐；根据全局背景明暗选深色或暖色，32×32 尺寸及 (16,16) 热点不变。

这些像素规格、轴长比例、曲率和配色是本项目的显示策略，不是 CATIA 专有算法的数值复刻。

## 实际验证

日志、数据和截图位于 `build/agent-check/navigation-affordances/`。

| 范围 | 结果与证据 |
|---|---|
| 定向 Node | `focused.log`：navigation-hud、projection-camera 共 2 项通过；覆盖 4 种球半径、中心/边缘/越界的弧长与连续性、轴向标记、颜色及三种视口的中心尺寸。 |
| 完整 Web | `web-accepted.log`：`invoke check --scope web`，116/116 场景、类型检查、生产构建通过；报告 `build/agent-check/20261017T075025-ad5d491a/report.json`，检查前后 source digest 一致。 |
| Mock/WebGL 浏览器 | `browser-rerun.log`：12 项通过，另 1 项旧标签排序前置条件失败；修正该前置条件后 `tab-drag-corrected.log`：1 项通过。共覆盖 13 个不同用例，使用现有 Chromium 和 SwiftShader。机器可读结果为 `browser-report.json`、`browser-corrected-report.json`。 |
| 视觉证据 | `hud-cursor-measurements.json`、`hud-and-cursors.png`、`catia-optional-sphere.png`、`tree-shared-handle.png`；已检查独立 HUD 的上/下方朝向、球边弧线和工作台实际显示。 |

浏览器原生拖动验证按钮宽度调整、持久化、不误收起、画布尺寸不变与最小/响应式宽度；原生键盘验证 Esc、方向键、Home/End、Enter/Space 和焦点交接。pointercancel、意外丢失捕获和 window blur 使用事件注入验证取消路径。已有六项导航用例以及三项视口显示用例全部通过。

400×400 HUD fixture 中，两条弧在中心均为 52 像素，在球边离散折线长度为 51.9974…51.9978 像素；上/下方观察都保留可见 HUD。四个旋转/缩放 SVG 的明暗版本均为 32×32，白色像素数为 0，外边界非透明像素数为 0。这是限定 fixture 的渲染证据，不代替所有设备或原生系统光标的验收。

首轮发现展开后聚焦发生于隐藏状态，调整折叠显示及聚焦时机后，新增手柄用例通过。旧标签用例试图拖动当前激活并被固定到首位的 Product；修正为激活 Part 后验证普通排序，未改变产品标签规则或放宽排序断言。完整 Web 检查后仅修改这一浏览器前置条件及验证文档。

知识入口完成文档审计、`invoke context-audit` 和 `git diff --check`；日志为 `doc-tests.log`、`context-audit.log`。本轮没有运行真实 API/Worker、服务端全量、PostgreSQL 专项或全部浏览器用例；Mock/WebGL 结果仅证明前端显示与交互。
