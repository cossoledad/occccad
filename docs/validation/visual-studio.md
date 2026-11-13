# 曲面、光照与缩略图的定向验收

2026-11-13 的本地样本。当前实现与边界归[显示策略](../architecture/current/visual-studio.md)，本页只保存测量条件、必要数值和复现入口。图片及原始 JSON 在 `build/agent-work/visual-studio/comparison/`，日志在 `build/agent-check/visual-studio-*.log`；不把单机样本当作硬件帧率或工业容量保证。

## 输入和画面对照

AMD Ryzen 9 9950X（16 核 / 32 逻辑 CPU）、WSL2 Linux 6.18、GCC 15 release、OCCT 7.9.1。浏览器为 Chromium 153 / ANGLE Vulkan SwiftShader，1280×900、DPR 1。视口测量串行执行，未与本次编译或其他测量任务并发；开发后台保持正常运行。

原生 `SurfaceDisplay` 构造 20 mm 立方体、全边 R3 圆角块、R10 球、R10×H20 圆柱、60×40 浅弧 Bezier 面和鞍面。浅弧面为 `z=4−x²/225`，`x∈[−30,30]`，由解析法线核对。单件颜色含 `#98a8b8 / #fafafa / #151b24 / #cf664d`，另有 16 / 64 个不同姿态的球实例。

对比采用三列：旧细网格与原默认照明；新曲面法线、材质与照明但保留细网格；新默认较粗网格与阴影缓存。基础颜色、模型、相机与尺寸固定，地面阴影开启，网格和 AO 关闭。旧、新 legacy GLB 的摘要逐项相同；测试内 legacy 输出只将细网格法线重建为旧的三角平均法线，不进入生产代码。原版视口记录于改动光照前的 `baseline-sync`；`final` 包含新三阶段数据，`reference` 是提交 `6389c840` 的原树组件在同一测试宿主中的记录。

实际图像检查覆盖浅弧面亮带、鞍面明暗、圆角连续性、白 / 深 / 彩色零件和球面接缝。白色顶面保留层次，深色件保留轮廓；较粗网格在常规视距仍有连续受光，但近看轮廓仍可能呈多边形。没有用生成图、后期修图或伪造模型替代真实视口。

阴影缓存与强制每帧重绘的相同视图 PNG 逐像素一致；移动一个实例后，再次核对缓存与强制重绘一致。Node 场景另覆盖显隐、透明度、几何替换、位置缓冲版本、裁剪、光源、草图模式、context restore 失效及释放监听；context restore 未独立进行真实 GPU 丢失 / 恢复验收。

## 几何与法线

以下为一次原生串行剖分样本，**不是重复采样中位数**；不含 RPC、上传、Naming 或前端加载。每个模型两种精度均重新验证源 BREP 序列化不变，精度不同不会复用前一种精度的面包。

| 模型 | 细 / 默认三角形 | 细 / 默认 GLB B | 细 / 默认剖分 ms |
|---|---:|---:|---:|
| 立方体 | 12 / 12 | 4,820 / 4,820 | 0.74 / 0.55 |
| R3 圆角块 | 6,836 / 3,348 | 287,168 / 160,124 | 19.65 / 9.70 |
| 球 | 8,120 / 3,650 | 237,604 / 109,996 | 36.49 / 14.03 |
| 圆柱 | 416 / 276 | 43,240 / 29,732 | 2.45 / 1.52 |
| 浅弧面 | 662 / 262 | 26,704 / 13,752 | 2.29 / 0.97 |
| 鞍面 | 1,460 / 582 | 53,132 / 25,068 | 4.62 / 1.76 |

细精度为 0.02 mm / 0.12 rad，默认为 0.05 mm / 0.18 rad。另生成 0.08 / 0.24 的计数候选，未作为默认视觉验收。默认球体的曲面法线误差约 `1.48×10⁻⁶°`，浅弧面约 `1.21×10⁻⁶°`，相对于已知解析法线；此前三角平均法线的最大误差分别约 7.15° 和 0.80°（旧细网格）。不能据此推广为任意曲面的误差上界。新默认球的法线阶段约 0.072 ms，圆角约 0.081 ms。测试还覆盖 Face 反向、Location、无 UV、取消及 0.001 / 1000 倍几何缩放；缩放测试同时缩放显示线性偏差，以隔离法线正确性。

## 视口与缩略图耗时

每视口场景预热 2 帧后取 5 帧，中位数如下。`submitMs` 仅围绕 `renderer.render`；每次 render 后另计 `gl.finish` 加同步 1 px readback 的等待。后者包含 SwiftShader、驱动和读回，不是隔离的 GPU 耗时；RAF 墙钟值另存 JSON，不换算成稳定 FPS 或 P95。

| 场景 | 旧 / 新 draw | 旧 / 新绘制三角形 | 提交 CPU ms | 完成等待 ms |
|---|---:|---:|---:|---:|
| 单圆角块 | 5 / 4 | 17,708 / 6,088 | 0.50 / 0.50 | 69.7 / 64.2 |
| 单球 | 5 / 4 | 16,562 / 3,864 | 0.50 / 0.60 | 43.2 / 33.9 |
| 16 球实例 | 66 / 50 | 394,852 / 120,164 | 0.90 / 0.70 | 135.4 / 79.5 |
| 64 球实例 | 258 / 194 | 1,579,396 / 480,644 | 1.70 / 1.60 | 429.3 / 210.8 |

draw 包含背景、表面、自然边线和按需表面深度通道；省去每实例一遍暖态方向光阴影。不能将这些数值当作一个模型的唯一三角形数或总显存。小场景的 CPU 提交与墙钟并非都更快，可靠结论是三角形、制品字节与暖态 draw 减少，以及缓存画面相同。

缩略图为真实 GLB、640×400、2 倍超采样，1 次预热后取 3 次中位数；三个阶段独立计时，不含 GLB 下载 / 解码或任务排队。新方案同时改变网格、法线复用、照明、颜色空间和边线，因此表格不是单一优化开关的归因实验。

| 模型 | 准备 ms：旧 / 新 | 光栅 ms：旧 / 新 | PNG ms：旧 / 新 |
|---|---:|---:|---:|
| 圆角块 | 4.79 / 1.24 | 11.10 / 8.08 | 6.07 / 5.51 |
| 球 | 5.37 / 1.20 | 13.28 / 7.75 | 6.43 / 6.35 |
| 浅弧面 | 0.43 / 0.10 | 4.66 / 4.68 | 5.48 / 5.94 |
| 鞍面 | 0.90 / 0.21 | 5.26 / 4.90 | 5.93 / 5.57 |

新路径对完整 GLB 法线跳过三角法线哈希，逐顶点照明和投影复用；12 bit 线性输出表避免逐像素幂计算。缩略图不模拟实时 PMREM / 粗糙度响应 / 地面阴影，也不声称颜色逐像素匹配。

树的单次 UI 微基准为 5,001 个合成节点、330 px 宽、900 px 高，45 次提示变化。原树渲染 48 行、新树 44 行；React commit 中位数 7.10 / 7.70 ms，样本最大值 31.20 / 15.40 ms。不能由此声称整体树更快或尾延迟稳定改善；已交付的是更清楚的排版、统一行距及不再为关闭菜单构建项目。没有交付服务器分页、增量索引或大装配容量保证。

## 正式链路

`visual-studio-live.spec.ts` 使用当前 `.env` 的 API、Router、Worker、数据库与 ArtifactStore。真实导入圆角块、浅弧面、鞍面，核对实际 3,348 / 262 / 582 三角形和 GLB 曲面法线 / Naming；浅弧面实际解码法线另与解析方向核对。经正式 Sketch / Circle / LinearExtrude 命令创建带孔零件，确认新 Part 保存 `#8e9faf / 0.50 / 0.02` 创建快照。8 个真实装配实例使用不同姿态，再提交移动并等待当前显示构型。

预览 GET 等待实际 `THUMBNAIL_RENDER` 返回 ready，保存 PNG 并核对 640×400、浅背景像素、ETag 与文档 Head 不变。工作台覆盖树菜单 / Escape、树展开收起时画布不变，以及透视 / 平行、renderer DPR 1 / 2 的接收面位置和 Revision 不变。DPR 是渲染器设置覆盖，不是多个真实显示器测量。实例拥有 8 个表面几何包装，共用一组 CPU 顶点 / 法线数组和一份自然边线几何；没有据此声称全部 GPU 缓冲共享或表面合批。

本次测试创建的文档在结束后删除；未重置当前数据库、其他文档或共享 ArtifactStore，测试生成的 Jobs / 制品按既有保留策略存在。真实链路截图和当次摘要在 `comparison/live/chain.json`，不是长期业务引用。

## 复现

```bash
OCCCCAD_VISUAL_STUDIO_DIR="$PWD/build/agent-work/visual-studio/fixtures" mise run check --scope geometry --match SurfaceDisplay --jobs 4
OCCCCAD_VISUAL_STUDIO_DIR="$PWD/build/agent-work/visual-studio/fixtures" OCCCCAD_VISUAL_STUDIO_PHASE=final mise run check --scope services --match '^TestStudioMeasurements$'
OCCCCAD_VISUAL_STUDIO_PHASE=final mise exec -- pnpm --dir web/apps/cad exec playwright test --config playwright.studio.config.ts
mise run dev --profile release --jobs 4 --no-web
# 另一个终端，使用上述当前后台
mise exec -- pnpm --dir web/apps/cad exec playwright test --config playwright.live.config.ts visual-studio-live.spec.ts
mise exec -- node web/apps/cad/scripts/studio-comparison-report.mjs
```

当前测试重建新的样本，不自动替代历史基线。比较页还需要改动前保存的 `baseline-sync` 视口、`baseline` 缩略图和 `reference` 树记录；它们属于可清理的 build 制品。重测旧版时从 `6389c840` 隔离归档前端及 `services/internal/workbenchconfig/catalog.json`，复用已安装依赖，复制当前 `tree-studio` 测试宿主，用 `OCCCCAD_STUDIO_REFERENCE_WEB` 指向该 CAD 目录运行树测试；旧缩略图须用该提交的 renderer。不要把当前代码输出重命名为旧版结果。

完整根 unit 检查与文档检查由标准路由执行；上述定向 WebGL / Live 证据独立于它们。尚未验收真实硬件 GPU、大型装配容量、任意极端修剪曲面、屏幕误差 LOD、透明阴影、表面合批和服务器树分页。
