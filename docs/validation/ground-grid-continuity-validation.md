# 地面网格连续性与旋转中心验收

日期：2026-10-17；基线 `6b00e9a5b8394ae8a83ec08d13d0017ae8a8cbce`，本次工作树。当前规则见[全局场景选项](../architecture/current/object-scene-properties.md)和[导航实现](../../web/apps/cad/src/cad/navigation/README.md)。

本轮仅修改 Web 显示、地面间隙计算及邻近验收；没有修改模型、服务端协议或相机导航手势。没有重置开发数据库、下载浏览器或修改 Kernel。

## 根因与实现

原地面仅画基准线和十倍主线：近看一个格子内部可没有线，远看两级线都可能因抗混叠淡出；片元超出模型 far 还会被直接丢弃。基线 WebGL fixture 复现了两种情况，证据在 `build/agent-check/ground-grid-continuity/baseline.json`。

- 自动间隙从单一对角线比例改为三向尺寸的几何均值，并用中间尺寸约束范围。模型等比例缩放时，间隙同比例缩放；细长件不再由长边主导。自动、世界、指定与锁定模式保留原所有权。
- 基准世界间距保持不变，近远时平滑补充十进制细分线或粗级线。每级以文档原点为锚，CPU 为重定位后的各级坐标计算相位；草图网格/捕获继续使用自己的自适应间距。
- 无限地面在裁剪范围内保留真实深度，范围外将辅助网格深度饱和到近/远边界，不扩大实体 far 或移动相机。后方网格仍被实体覆盖，前方网格仍可覆盖实体；水平视角使用可见边界射线重定位。背向射线、用户关闭/零强度、视点恰在平面内等情形不承诺可见网格。
- CATIA 十字总跨度为旋转球直径的 1/3.5，随视口尺寸变化；X/Y 留白、短黄色 Z、中心固定和世界朝向保留。HUD 相机深度同步扩展，防止放大的线段自身被 HUD 裁剪。

以上间隙系数与显示比例是本项目策略及用户要求，不声称是所有 CATIA 版本的参数。

## 实际验证

日志相对 `build/agent-check/ground-grid-continuity/`。完整源码检查、Mock/WebGL 与真实 API/Worker 是不同证据层。

| 范围 | 结果 |
|---|---|
| 定向 Node | `focused.log`：plane-grid、projection-camera、surface-appearance 共 3 项通过；含小/大模型同比例间隙、杆/薄片、所有网格级别的世界相位、三种视口尺寸的十字比例。 |
| 完整 Web | `invoke check --scope web`：115/115 场景、类型检查和生产构建通过；`build/agent-check/20261017T072901-7b5d2f6c/report.json`，检查前后 source 相同，`web-accepted.log`。 |
| Mock/WebGL 浏览器 | `browser-accepted.log`：10 项通过，ground-grid、viewport-rendering 和全部 navigation 用例。 |
| 真实浏览器 | `live-accepted.log`：1 项通过，现有 SQLite/LOCAL、真实 API/Release Worker、现有 Chromium；真实建模/外观历史/分面/冲突流程及自动地面高度、世界地面与取消恢复。 |
| 知识入口 | 文档审计 8 项、`invoke context-audit`、`git diff --check` 通过；`doc-tests.log`、`context-audit.log`。 |

新 `browser/ground-grid.spec.ts` 在生产网格的真实 WebGL 中验证 10 个距离样本 `10^-4…10^5`，固定基准间距均为 10；近看格子内部、远看密集网格均有可见线。另覆盖十亿量级坐标、far=120 时深度=150 的地面、上下两侧与实体遮挡、水平视角天空不画地面。截图在 `build/acceptance/playwright-mock/`。

独立前后对照使用 320×320、35° 透视、基准 10、视点横向偏离网格交点，统计红通道大于 5 的像素：原实现距离 `10^-4…10^-1` 与 `10^5` 均为 0，修改后这些样本均为 7536；原实现 far 外为 0，修改后为 5056。完整数据为 `baseline.json`、`after.json`，脚本为同目录 `baseline.mjs`、`after.mjs`，分别在基线和修改后的 Vite 源码上执行；脚本读取当前运行源码，重跑前需选择对应源码版本。这是隔离渲染 fixture 的可见性证据，不是所有模型/设备的保证，也不是帧率或性能结论。

首轮导航断言比较了投影转换保留的比例与新的正交 Fit，得到 9/10；验收改为先在所选投影建立明确 ISO Fit 基线，再检查旋转/缩放后的 Fit 往返，最终 10/10。没有修改 Fit 算法或放宽投影/相机断言。第一次真实测试过滤器未匹配用例，不计通过；修正过滤条件后的实际用例执行结果列于表中。

未运行服务端全量、PostgreSQL 专项或所有浏览器用例；本轮没有对应代码改动。
