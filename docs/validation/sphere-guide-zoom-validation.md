# 球边导引与近距离缩放验收

日期：2026-10-17；基线 `96d8cbaafe526a9314e39233dc5002238e056c86`，本次工作树。当前规则见[导航实现](../../web/apps/cad/src/cad/navigation/README.md)。本轮仅修改前端显示、导航拾取和透视 near 计算，没有重置开发数据、修改持久模型或下载浏览器。

## 参考、复现与修改

本机 CATIA B33 `control/BasEnglishC2.viewdoc` 引用的 `online/basug_C2/basugbt1702.htm`（Setting Depth Effects）说明默认关闭深度效果，缩放时不主动裁剪几何，Near/Far Limit 随全局包围盒调整；`basugbt1205.htm` 描述增量与连续缩放，`basugbt1507.htm` 区分透视/平行投影。说明文档没有公开内部相机、十字曲线或缩放防穿透公式；以下为结合本项目调用链确定的实现。

- 中心十字删除箭头，仅保留 10 个轴线顶点。X 橙色、Y 青色，正半轴明亮、负半轴压暗；保留较短 Y、短黄色 Z、中央间隙、固定屏幕中心和相机逆旋转。
- 球面导引由启发式曲率改为球面大圆的投影。指针按轨迹球映射限制到球内；周向曲线为 `ρ cos(θ) · 径向 + R sin(θ) · 切向`，其中 `ρ≤R`。求屏幕弧长对应的参数范围，维持 52 CSS 像素；`ρ=R` 时曲线就是外圆。径向线接近边界后向内移，保持可见长度，全部顶点留在圆内。输入轨迹球算法不变，越界指针仍可控制滚转。
- 默认滚轮不再使用旋转中心的点/边晋升结果，改用 `NavigationPicker.pickSurface` 的最近 Mesh 表面；空白沿用关注深度的视平面。点/边拾取的像素容差按最近表面的视深度及 camera.zoom 计算，避免近距离时仍按整个模型的中心距离放大容差。
- `updatePerspectiveClipping` 同时计算包围盒的最小和最大视深度，near 受最近深度约束，保留原最小正值；裁剪更新只改平面，不移动相机。

基线在 Mock 的 72×38×16 简单零件上对三个锚点各连续放大 40 次，没有复现眼点进入实体。因此不将后续隔离 fixture 的根因直接认定为所有现场模型的唯一原因。

另用生产 NavigationController/Picker/Rig 构造 20×20×20 盒体，眼点 Z=10.01，遮挡点 Z=9.995，滚轮每次 -120：基线将后方点晋升为目标，第 3 次事件后前表面先被 near 切掉，第 4 次后眼点进入盒内。修复后目标为 Z=10 的表面，第 8 次眼点约为 Z=10.001、near 约为 0.0001，保持在表面前。原始数据见 `occluded-before.json`、`occluded-after.json`；脚本读取当前源码，重新生成基线数据须先切换到基线版本。

## 实际验证

日志、数据和截图相对 `build/agent-check/sphere-zoom/`。

| 范围 | 结果与证据 |
|---|---|
| 定向 Node | `focused-final.log`：5/5，navigation-hud、perspective-zoom、projection-camera、orthographic-view、navigation。 |
| 完整 Web | `web-accepted.log`：117/117 场景、类型检查与生产构建通过；报告 `build/agent-check/20261017T081425-21f619d1/report.json`，检查前后 source digest 一致。 |
| Mock/WebGL 浏览器 | `browser-accepted.log`、`browser-report.json`：13/13，含新增缩放、球面/光标/结构树、全部六项导航、三项视口显示与地面连续性。现有 Chromium + SwiftShader，没有下载浏览器。 |
| 渲染数据 | `hud-cursor-measurements.json`、`close-surface-zoom.json`；截图为 `hud-and-cursors.png`、`exterior-after-zoom.png`、`catia-optional-sphere.png`，已检查独立 fixture 和工作台显示。 |

Node 覆盖四种球半径、中心/边缘/越界、全部顶点在圆内、球边周向弧与外圆重合、无箭头与颜色区分；缩放覆盖 0.001/1/1000 三种模型尺度、50 次连续缩放和极大单次输入、遮挡/悬停点、隐藏几何、离中心锚点不漂移、缩放极限后的缩小响应、near 独立约束和 projection zoom 下的点容差。

400×400 WebGL fixture 的球半径为 192：球边及越界样本中，周向弧的半径变化小于 0.000016 像素，最大顶点半径约为 192.000007（Float32 误差）；两段离散线长度均在 52±0.001 像素内。浏览器通过生产 InputManager 接收 24 次真实 wheel 事件，包含一次 -1,000,000 大输入；眼点始终在盒体外，near 小于表面距离，中心像素保持外表面颜色 `[85,141,204,255]`。将眼点设到表面前 0.0001、中心设回盒内后，near 约为 0.00001，外表面仍显示。

知识入口执行文档审计、`invoke context-audit` 和 `git diff --check`；日志为 `doc-tests.log`、`context-audit.log`。没有运行真实 API/Worker、后端全量、PostgreSQL 专项或全部浏览器用例。本轮证明限定 fixture 及外部起始姿态下的缩放路径，不是漫游/旋转碰撞系统或对所有现场模型的保证。
