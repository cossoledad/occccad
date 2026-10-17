# 视图动画与斜视深度精度验收

日期：2026-10-17。起点是 `96d8cbaafe526a9314e39233dc5002238e056c86` 加上一轮已验证、尚未提交的球面导引及缩放修改；该轮检查报告为 `build/agent-check/20261017T081425-21f619d1/report.json`。不能仅用 HEAD 还原本次故障基线。当前行为见[导航实现](../../web/apps/cad/src/cad/navigation/README.md)。本轮只修改前端视图变换、裁剪与验证，没有重置数据或下载浏览器。

## 变更与根因

Fit、顶/前/右视图及等轴测共用现有 `ViewTransition`，280 ms 平滑过渡，支持连续命令从当前帧接续、输入打断及 reduced motion。在克隆相机上准备目标，避免命令执行时先跳到终态；首次打开文档的初始化 framing 仍立即完成。相机距离与 zoom 按几何比例插值，重定位随可见跨度推进，避免从高倍率细节执行 Fit 时先移走细节或穿过表面。正对平面、草图进出也复用相同入口，MotionViewpoint 播放保留自身时钟。

用户反馈几乎所有文档在放大后都可能出现线框感。复现表明：上一轮 near 修复将包围盒跨过相机眼点等同于实体表面贴近眼点，直接把 near 压至 `1e-7`。普通斜视下，眼点仍在实体外也能满足这个条件；深度精度下降后，本应被遮挡的背面边线穿过不透明表面，形成锯齿和线框感。材质的 `wireframe` 始终为 false。上一轮轴向简单盒体验证没有覆盖这个情形。

修复保留有限 near/far，不移动透视眼点。包围盒接近或跨过眼点时，视口通过已有 `NavigationPicker` 查询屏幕中心的最近可见 Mesh 表面，使用其视深度与关注深度确定 near；没有表面命中时回退到关注深度。正向包围盒仅作保守约束，不因负深度角点直接耗尽深度精度。普通 Fit 姿态不额外查询表面，导航事件使帧失效，裁剪在渲染帧统一更新。

## 可重复的渲染证据

生产 `CadMaterialFactory` / `CadShaderLibrary` 材质，20×20×20 不透明盒体，内部后方 z=-9 放置 17 条横线与 17 条竖线，320×320 WebGL、35° 透视、目标为原点。线段全部应被遮挡。数据和截图位于 `build/agent-check/view-motion-precision/`。

| 情形 | near | far | 暗色边线像素 |
|---|---:|---:|---:|
| 基线眼点 (3,-3,13) | 0.0000001 | 273.495887 | 7245 |
| 基线同姿态手动 near=0.001 | 0.001 | 273.495887 | 0 |
| 基线同姿态关闭 polygonOffset | 0.0000001 | 273.495887 | 7245 |
| 修复后轴向 (0,0,13) | 0.0013 | 260 | 0 |
| 修复后斜视 (3,-3,13)，实际表面参考 | 0.000315572 | 273.495887 | 0 |
| 修复后包围盒深度接近零，实际表面参考 | 0.000334619 | 277.022402 | 0 |

原始故障输出保存在 `depth-before.json`（初次运行 stdout）。`depth-baseline.mjs` / `depth-baseline.log` 是复现脚本和首次日志；脚本读取当前源码，重新生成故障数据须恢复完整基线。`depth-after.json` 是修复后不传表面解析回调的中间验证，不能与最终视口回调路径混为同一测量。

最终浏览器 `oblique-depth.json` 和 `oblique-depth-comparison.png` 同时展示三种正确遮挡姿态及强制 near=`1e-7` 的故障对照。该对照仍透出 7245 个暗像素，正确姿态为 0；每幅都有超过 80000 个着色像素。截图已检查。数值只代表此确定 fixture，不表示所有模型或 GPU 的误差范围。

## 实际验证

| 范围 | 结果与证据 |
|---|---|
| 定向 Node | `focused-final.log`：5/5，包含新视图命令场景、透视缩放与既有平面动画。 |
| 完整 Web | `web-accepted.log`：118/118 场景、生产构建通过；`typecheck.log`：类型检查通过。完整报告 `build/agent-check/20261017T085545-b4d5075b/report.json`，检查前后 source digest 一致。 |
| Mock/WebGL 浏览器 | `browser-accepted.log`、`browser-report.json`：16/16，0 skipped、0 flaky；现有 Chromium + SwiftShader。 |
| 动画 | 两种投影下实际点击 Fit/前/右/顶/等轴测，验证活动中间帧、Esc 打断后保持当前视图、reduced motion 立即完成。`fit-animation-frames-perspective.json`、`fit-animation-frames-parallel.json` 与两张 `view-command-*.png`。 |
| 近距遮挡 | 新斜视渲染测试；既有 `perspective-zoom` 浏览器测试验证连续真实 wheel 输入后保持实体外表面，包括眼点距表面 0.0001、关注点仍在盒内的姿态。 |
| 既有交互 | 六项导航、结构树/光标/球面、地面连续性、透明边线、投影偏好回归通过。 |

Node 另外覆盖两种投影的精确终态、重定向不跳变、初始化不动画、高倍率细节 Fit，以及 0.001/1/1000 三种模型尺度、遮挡点和大步长缩放。首次专项浏览器运行被已有 tooltip 挡住相邻命令点击；调整测试为移开指针并等待 tooltip 消失后重跑，最终 3 项专项与上述 16 项回归通过，没有强制点击或修改 tooltip 产品行为。

文档审计、`invoke context-audit` 和 `git diff --check` 的日志为 `doc-tests.log`、`context-audit.log`、`diff-check.log`。本轮没有运行真实 API/Worker、后端全量、PostgreSQL 专项或全部浏览器用例。中心表面参考改善当前显示精度，不是覆盖整个视锥的相机碰撞系统；Mock 工作台与生产材质 WebGL 证据不能代替所有现场文档验收。
