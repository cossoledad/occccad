# 自然边线的定向验收与测量

2026-11-13 的开发环境记录；设计、公式和当前限制由[自然边线](../architecture/current/natural-edges.md)拥有。本页记录具体样本，不能作为任意模型、硬件或当前全量测试通过声明。日志与派生图片位于 `build/agent-check/natural-edges/` 和 `build/agent-work/natural-edges/`。表格所引单次原始样本保存在 comparison 下的 `reference-geometry.json / reference-measurements.json`；重跑会生成新的时间样本。

## 输入与观察范围

AMD Ryzen 9 9950X（16 核/32 逻辑 CPU）、WSL2 Linux 6.18.58.1、OCCT 7.9.1、GCC 15 release。原生 fixture 使用真实 BREP：20 mm 立方体、全边 R3 圆角块、R10×H20 圆柱、R10 球、缝合共面双面、连续变化连接面、20×20 开口面；另有旋转/镜像立方体和边长 0.02/20000 的尺度样本。连续变化面沿公共直边 `x∈[-10,10]` 满足 `z=y×1.4(x+10)/20`，用于检查局部法线与连续权重，而不是只测整条直边的平均值。

Chromium 153.0.8010.12、ANGLE Vulkan SwiftShader WebGL，1280×900、DPR 1，原有工程光照与环境，关闭地面、阴影、网格和 AO。单件颜色包括 `#fafafa / #151b24 / #db553b / #2877cf`，roughness 0.72、metalness 0。每个模型三阶段用相同 GLB 摘要、相机和材质，预热后各记录 4 帧。PNG 由浏览器实际视口生成，未用图片生成模型或图像修饰代替实际渲染。

13 组、39 张三阶段图片覆盖全部要求：立方体、圆角块、圆柱、球、共面双面、变化连接、开口、深色立方体和深/红/蓝圆角块，以及不同姿态的 16/64 实例。基础与优化的 13 对 PNG 逐像素一致；4 种圆角表面的局部像素检查确认只加深、不增亮，RGB 比率偏差在像素量化允许范围内。相同相机重复绘制的 PNG 一致，背面边像素保持遮挡。32 步相机旋转没有重新构建边线静态属性；透视/平行与 DPR 1/2 保持 1.35 CSS px 设置。球面隐藏接缝的悬停、选中与取消恢复通过实际 overlay 图像检查。

另一个 Live 用例把 7 种 BREP 通过真实 API 导入任务提交给 Router/Worker，经制品存储、Naming 关联和 VisualRepository 进入正式文档；还通过正式命令创建 Sketch、矩形和 LinearExtrude，核对参数化求值输出，并验证四个不同姿态的真实装配实例共用一份边线几何。它与无业务写入的渲染夹具是两种证据；后者不能独立证明 Router 或持久链路。Live 创建的文档在测试后删除，未重置数据库、既有文档或共享制品存储。

补充检查覆盖错误 seam 标记、缺失法线时的结构线降级、0.5…2 倍相机缩放以及 `[0.5,2,0.5]` 的 GPU 非均匀缩放；缩放复用原静态证据。解码 Worker 检查仅向真实圆角 GLB 的 JSON chunk 添加空白以跨过 512 KiB 门槛，验证实际 Worker 转移并返回原始法线/位置 buffer view；它不是大模型吞吐测试。

## 测量样本

原生单次运行，`edge_profile_ms` 仅为采样/分类子阶段，**不等于开关该功能后的端到端差值**；EdgeUse 建立、初始平面法线、剖分和编码另计。D1 次数不含恒定平面的初始求值。

| 模型 | 三角形 | 渲染边段 | 自适应 D1 次数 | 采样/分类 ms | 无 profile GLB / 新 GLB |
|---|---:|---:|---:|---:|---:|
| 立方体 | 12 | 12 | 0 | 0.006 | 3,112 / 4,820 B |
| R3 圆角块 | 6,836 | 672 | 6,616 | 0.397 | 227,308 / 287,168 B |
| 圆柱 | 416 | 211 | 1,060 | 0.074 | 23,876 / 43,240 B |
| 球 | 8,120 | 53 | 532 | 0.033 | 232,036 / 237,604 B |
| 变化连接 | 850 | 19 | 59 | 0.011 | 26,108 / 28,512 B |

所有样本未触及采样上限；变化连接以 64 个内部点/渲染段对实际 GPU 的“端点权重插值”做稠密解析对照，最大权重误差小于 0.025。该结论只覆盖这一个解析构造。小模型的相对字节开销较高，圆柱增加约 19 KB；尚未做压缩/法线量化。

| 场景 | 基础版 draw / 优化版 draw | 基础边线 geometry / 优化 geometry | 新建派生属性数组 |
|---|---:|---:|---:|
| 单圆角块 | 4 / 3 | 1 / 1 | 37,632 / 5,376 B |
| 16 实例立方体 | 65 / 49 | 16 / 1 | 10,752 / 96 B |
| 64 实例立方体 | 257 / 193 | 64 / 1 | 43,008 / 0 B（已热缓存） |

draw 统计含背景和既有多实体深度通道；省去的是每个实例零 alpha 的拾取线 draw。派生属性仅计新建法线/分类数组，不含源 GLB、位置、表面、拾取几何和驱动缓存，不能称为总内存/显存减少量。16 实例优化为一次构建、15 次共享；64 实例复用此前缓存，未发生新属性分配。

一次 4 帧 CPU `renderer.render` 提交中位数为：单圆角块约 0.25→0.35 ms，16 实例 0.80→0.65 ms，64 实例 1.55→1.80 ms。重复运行有波动，优化版并非每组 CPU 样本都更快。这里包含驱动可能的阻塞，时间分辨率有限、样本很少；没有隔离 GPU 时间，没有稳定 FPS/P95/工业容量结论。`preparationMs` 另含下载、场景构建和 RAF 等待，不能当作纯算法耗时。可靠的优化结论是确定性的 draw 减少、缓冲共享与画面不变。

## 复现

两个浏览器套件都会先调用同一个注册原生检查，生成当次代码的真实 fixture；不依赖手工保存的二进制模型。也可先单独运行生产者（环境变量只控制本测试的可选导出）：

```bash
OCCCCAD_EDGE_FIXTURES="$PWD/build/agent-work/natural-edges/fixtures" mise run check --scope geometry --match EdgeProfile
mise exec -- pnpm --dir web/apps/cad test natural-edges mesh-glb surface-appearance display-settings viewport-depth
mise exec -- pnpm --dir web/apps/cad exec playwright test --config playwright.edges.config.ts
mise exec -- node web/apps/cad/scripts/edge-comparison-report.mjs
```

生成 `build/agent-work/natural-edges/comparison/index.html`，可切换模型查看三幅同条件图、绘制统计和原始 JSON。`geometry.json`、`measurements.json`、`stability.json` 分别保存原生采样、渲染与稳定性记录。原生测试还验证 BREP 不变、正反 EdgeUse、尺度/镜像、缺失证据、退化段与取消。

启动当前真实后台后运行 Live（复用根环境与已安装 Chromium）：

```bash
mise run dev --profile release --jobs 4 --no-web
# 另一个终端
mise exec -- pnpm --dir web/apps/cad exec playwright test --config playwright.live.config.ts natural-edges-live.spec.ts
mise exec -- node web/apps/cad/scripts/edge-comparison-report.mjs
```

真实链路图为 `live-*.png`，对应制品摘要与类型在 `live-chain.json`。它记录单次验收对象，不是可依赖的长期业务 ID。构建和 Node 测试不代替上述 WebGL/Live 检查；所有范围之外的硬件、容量、极端曲面与透明排序限制仍按设计文档保留。
