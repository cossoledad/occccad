# 复杂导入件的切除性能

2026-11-09 UTC，本机 Release / OCCT 7.9.1，AMD Ryzen 9 9950X / WSL2，正常配置的 SQLite、LOCAL ArtifactStore 和生产 Router。完整数值、冻结输入摘要、二进制/源码指纹及计时边界见[机器样本](../../tests/test.data/performance/part-feature-performance.json)。这是指定输入的串行小样本，不代表 P95、整车容量或与 CATIA 的统一基准。

## 输入与操作

一个有效导入 Solid：827 Face、2169 Edge、1390 Vertex，包含 256 个三次 BSpline 面；冻结 BREP 约 4.94 MB，体积约 620928.47 mm³。圆草图半径 15.596120225694847 mm，原点零，X 方向 +X、支撑法向 −Y；反向切除 40 mm，再改为 41、42 mm。精确 Profile、默认 0.02 mm / 0.12 rad 显示策略保持一致。

原始模型不随仓库分发，也不以可清理的开发文档 ID 作为复现前提。输入 SHA-256 和完整草图平面记录在 JSON；没有同一 BREP 时，以下性能数字不能直接类比其他模型。

## 观察结果

| 边界 | 优化前 | 优化后 |
|---|---:|---:|
| 首次新切除：正式 API 到预览响应 | 89.125 s | 4.751 s |
| 深度 40 → 41：重新求值 | 85.631 s | 4.211 s |
| 深度 41 → 42：重新求值 | 未测 | 4.122 s |
| 完全相同输入 | 约 35 ms | 约 36 ms |
| 响应后的 GLB 预览呈现 | 6.170 s | 0.750 s |
| 呈现期间最长主线程任务 | 6.023 s | 0.575 s |

“首次”指新启动运行时中，已接纳导入件上的第一个新特征意图：实际执行生成器和材料布尔，恢复的是既有导入根证据，不是缓存的切除结果。它不包含最初文件导入，也不是 `ForceCold` 的根证据重建。后两种不同参数也实际执行，不以完全相同请求命中作为优化结论。

首次 Worker 分阶段为 base 0.521 s、exact 0.561 s、Naming 1.393 s、display 1.641 s；mesh 1.398 s 和 encoding 0.034 s 已包含在 display 中。一次局部材料积分、8 个新面证据、822 次面证据复用，邻接约 3.4 ms。Worker 阶段与整个 API 的计时边界不同，不用相减推断网络开销。

本次切除前后的 BREP SHA-256 完全相同，最终均为 828/2171/1392 个 Face/Edge/Vertex，740450 个三角形、395065 个显示顶点。质量由局部积分加减得出，与原整体积分的体积相对差约 8.33×10⁻¹¹；不是所有几何输入的误差上界。有效性、完整 Naming 和原积分精度策略继续生效，未降低显示精度。

原取消样本在 18 s 时中止客户端，原生计算随后仍消耗约 43.98 CPU s。优化后分别在 0.75 s、3 s 中止的新操作样本，额外 Worker CPU 约 0.31 s、0.38 s；含连续空闲确认的观察尾段约 1.20 s、1.40 s。取消时点不同，因此不能据此给出统一中断延迟。所有原模型测量均未提交或改变 Head。

## 验证与复建入口

- `mise run check --scope all --level unit --jobs 4`：共享 Proto/Worker 的单元覆盖；报告中的实际 case、失败和 skip 才是当次证据。
- `mise run check --scope services --level integration --match 'TestPartFeatureChainIncrementalAndColdThroughRouter|TestSolidBooleanLifecycleThroughRouter' --test-jobs 1`：正常数据库及 Router，八次切除、修改中间步骤、前缀执行计数、冷 BREP/Naming 摘要对照、预览只读、补偿历史。
- `TestEdgeVertexPersistentSelectionThroughRealRouter/import naming diagnostics`：冻结导入根恢复、首次操作与冷重建 BREP/Naming 等价、导入引用及真实 STEP/BREP 归一化。
- 在 `web/apps/cad` 运行 `pnpm exec playwright test --config playwright.live.config.ts browser/solid-feature-live.spec.ts --grep 'Hole preview keeps one shaded result'`：真实预览、拾取、取消、提交、Undo/Redo 及重开。

内核邻接与独立两两 oracle 对照、冻结根拒绝错误身份/摘要、多个 Solid（分离/重叠）的整体积分回退、惰性拓扑属性、取消不返回部分网格的回归，位于 `kernel/occt/tests/geometry_exchange_scenarios.cpp`。原始运行报告在 `build/agent-check/`；本机原模型测量及 CPU profile 在 `build/performance/import-cut-analysis/`。这些可清理文件不是功能的持久输入。

2026-11-10 的增量优化继续降低了首次与改参数成本，见下节。首次仍需完整剖分，精确 Boolean、Naming 打包、制品读写和 OCCT 全进程内存仍有优化空间；单次积分及部分构造器仍不能中途取消。未对复杂导入件的所有局部修改器、INTERSECT 或长链规模给出性能保证，需要整体积分的分支仍可能较慢。算法边界归[Part 求值](../architecture/current/part-feature-evaluation.md#零件增量求值)，后续入口归[CAD 核心主线](../../plans/cad-core-roadmap.md)。


## 增量命名、显示与交互（2026-11-10）

同机同一冻结输入和精度，正常 SQLite / LOCAL / Release Router。新运行时的三个串行预览分别执行生成器和布尔，不是相同结果缓存命中；后两次允许复用未变化面的离散数据。测量期间没有并行构建/测试。全部 BREP SHA 与此前 40/41/42 mm 样本一致，原文档 Head 保持。

| 边界 | 本轮前 | 本轮后 |
|---|---:|---:|
| 首次新切除，API 至预览响应 | 4.489 s | 2.892 s |
| 40 → 41 mm | 4.087 s | 1.557 s |
| 41 → 42 mm | 4.104 s | 1.497 s |
| Naming（含内核与打包） | 1.404～1.436 s | 0.211～0.250 s |
| 改参数时的 mesh 子阶段 | 1.103～1.124 s | 0.060～0.064 s |
| 切除 GLB 的 HTTP 载荷 | 22.69 MB | 2.65 MB |

内核语义索引避免重复线性查找；检查点共享已完成的 FeatureResult 前缀，恢复时再展开。独立原生样本与调用链对照表明，一个主要墙钟损耗来自算法热循环反复查询 gRPC 取消状态：本轮改为 16 ms 有界轮询，本次首次实际查询 58 次、约 5.3 ms；接纳和发布仍强制查询。Naming 新计时分别给出内核约 0.129～0.144 s、制品打包约 0.082～0.107 s，不能将旧 Naming 整段都解释为命名算法 CPU。

改深度时 828 个面中 761 个剖分被保留，67 个由 OCCT 计算；结果仍为 740450 三角形。严格复用依赖同源精确 TShape/位置/朝向和精度，失败走原边界校验及冷算。首次没有 face packet，mesh 仍约 1.132 s。BREP/GLB 字节改为不可变共享所有权，驻留预算按共享几何并集计量；这些模块预算不是全进程 RSS 上限。

22.69 MB → 2.65 MB 是对授权的既有导入 GLB 做无损传输编码，包含索引常量偏移；不是降低精度或只校验部分文件。独立编码样本约 72 ms，浏览器正式 VisualRepository 路径已重建并校验目标 SHA。下载、Worker 解码及整件轮廓准备的两次样本约 0.59～0.68 s，和前节 commandPreview/GPU 呈现计时边界不同，不能直接作逐帧速度对比。

前端整件高亮原来每次扫描三角形并创建约 2159 个覆盖对象；现在面/边各一个批次，并复用不可变 mesh 的空间轮廓、精确子集索引和共享租借缓冲。相机变化做保守候选筛选，原 shader 与深度/剖切门禁作最终判断，材料保持原外观。一个零件及其两个重复 occurrence 的 Product 样本中：首次整件 hover CPU 约 35～37 ms，重复选择/hover 约 0.2～1.0 ms；本轮前对应操作约 2.6～4.9 s。大 GLB 与整件轮廓准备在 Web Worker，渲染直接借用紧凑 accessor，不再先建立大量嵌套面数组。

这里的 hover 数字只计引擎调用 CPU，不含 GPU 完成；Chromium/SwiftShader 的截图同步仍有明显成本，不能据此声称目标显卡 60 Hz。Product 测量选择其中一个完整 occurrence，并验证重复实例共享 mesh、各自覆盖；不是大装配或任意开放结果的容量验收。精确稳定引用、历史全量等价、冷算、取消和共享资源寿命继续独立检查。完整机器样本在同页 JSON 的 `incrementalOptimization20261110`；原始本机材料在 `build/performance/part-incremental/`。

本轮另在 1.2 s 和 0.75 s 中止新操作，客户端均被取消且源 Head 未变；观察包含空闲确认，不作为所有构造器的中断上界，详细 CPU 样本见 JSON。

新增验证入口：`MeshReusePreservesColdResultAndIsolatesSource`、`BorrowedBrepSurvivesEvictionAndKernelLifetime`，`TestVisualDeltaPreservesBytesAuthorizationAndVariantETag`，Web 的 `interaction-geometry` / `visual-decoder` / `mesh-glb` 场景；真实视图沿用 `surface-contours-live`、`Engineering surface display` 和 `Hole preview keeps one shaded result`。测试存在不代表执行，实际结果由本次 `build/agent-check/` 和浏览器报告确认。


## 局部高亮的邻接与投影

2026-11-12 UTC，同机 Chromium / SwiftShader，对已授权且完成解码的 741210 三角形、827 面导入件做一次串行对照。基线恢复 `5f8ee542` 的子集轮廓、覆盖及投影路径；新旧均从同一紧凑网格开始，计入引擎 `preselect` 和 CPU 轮廓投影，不计下载、解码、首次邻接准备及 GPU 完成。详细输入摘要、源码摘要、原始调用读数见同页 JSON 的 `highlightOptimization20261112`。

| CPU 交互 | 基线 | 当前 |
|---|---:|---:|
| 首次选择导入步骤的贡献 | 426.9 ms | 32.9 ms |
| 重复同一贡献 | 1.2 ms | 0.7 ms |
| 首次选择一个面 | 40.9 ms | 0.5 ms |
| 重复同一个面 | 0.4 ms | 0.1 ms |

首次贡献查询输出均为 15934 条面轮廓及 23857 条 CAD 边线，单面均为 130 条线。该次查询前后 `usedJSHeapSize` 差约 302 MB → 2 MB，派生几何缓存约 53.15 MB → 1.34 MB；这是调用前后读数，不是采样峰值、分配总量或 RSS。两组均已持有同一约 44.18 MB 的不可变显示邻接；它随解码结果共享，不能从上述差值推导加载阶段或所有模型的内存上界。

根因是子集选择重复焊接整份网格、建立大型 JS Map、预留过量双精度边法向，并先输出全部曲面候选后再投影。现在邻接在解码阶段用紧凑数组一次准备；面到候选边索引支持局部查询，选择与投影合并，只缓存当前投影几何。整件与覆盖全部面的贡献共享投影，引用计数保护重复 occurrence 和 selection/hover 的寿命；缓存按字节约束全部相机投影。首次邻接仍为线性工作，大制品在可取消的 Worker 中准备，并未改变显示精度或 CAD 持久身份。

此对照隔离交互算法，不是整个旧版本加载基准；原基线的整件预计算不在本夹具恢复范围，因此不比较其 Body 整件路径。单个串行样本不代表 P95、所有模型、GPU 帧率或用户观察到的另一模型的 500→1500 MB 堆峰值。
