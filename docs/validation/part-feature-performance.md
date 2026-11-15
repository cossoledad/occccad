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

## 草图隔离与长链（2026-11-14）

本机同一 Release / OCCT 7.9.1、当前 `.env` 的 SQLite / LOCAL，串行经过正式 Workspace Service、生产 Router 和真实 Worker。新的[机器样本](../../tests/test.data/performance/part-sketch-performance.json)记录源码/Worker 指纹、输入摘要、计时边界和用例入口。本轮没有远端约 10 MB 的汽车 B 柱；下列生成 STEP 不能替代该模型，也不能与前两节的冻结导入件直接比较速度。

20 个未被三维结果消费的草图（含面支撑）验证了预览与提升提交均为 **0 次 Body Worker 执行**，原 BREP/NAMING/VISUAL 引用保持；无关 Feature、Profile 和投影没有实际准备。已消费的上游草图修改仍更新实体，冷重建与 Undo/Redo 保持原门禁。浏览器另以实际选面、进入草图和画线验证 Body 对象、BufferGeometry、位置属性及 BVH 均保留，dispose 为 0；这个显示用例使用简单实体，不能作为复杂 STEP 的帧率或延迟验收。

### 首次 STEP 操作

生成 300×150×10 mm、450 个圆孔的机械板，真实导出 STEP 再导入：STEP 1,787,951 字节，BREP 355,183 字节，456/1362/908 个 Face/Edge/Vertex，127812 个三角形。显式卸载 Shape 后首次精确选面读取的 load 约 5.4 ms、query 约 1.0 ms。此前同 Worker 已完成源板生成和交换，制品/输入及其他缓存没有全局清空；这不是首次文件导入或新文件系统缓存的基准。

在顶面绘制 2×2 mm 矩形，首次向下切除 10 mm，再将深度改为 9 mm，两个结果都实际执行一个新生成器，体积分别减少 40、36 mm³；候选提升提交不重复 Body 执行，冷 BREP/Naming 摘要相同。

| 服务边界及 Worker 子阶段 | 首次切除 | 改深度 |
|---|---:|---:|
| PreviewCommand / ApplyCommand 返回 | 4.244 s | 4.112 s |
| exact | 0.904 s | 0.865 s |
| Naming | 0.273 s | 0.166 s |
| display | 2.877 s | 2.880 s |
| 其中 mesh | 2.684 s | 2.695 s |

此服务计时不含 HTTP 和浏览器显示；mesh/encoding 已属于 display。改深度复用了 454 个面、计算 7 个面，但 mesh 仍约 2.7 s，因此复用面数不能代表耗时复用比例。按该夹具的拓扑推断，下一步应逐面录制带大量孔的裁剪平面，优先研究昂贵变化面的剖分，而不是仅扩大未变面缓存。精确 Boolean 也仍有约 0.9 s 成本，尚未消除首次操作延迟。

### 固定拓扑的真实长链

从长方体开始，用同一矩形草图连续削去底部材料，完成 100 次真实切除、共 103 个 Feature。每个接纳结果始终为 6 Face / 12 Edge / 8 Vertex；第 100 步和末端改深度都只执行 1 个生成器并复用 100 个前缀阶段，随后通过冷 BREP/Naming 对照。

| 切除步数 | 本步服务返回 | Naming | 当前 Worker RSS |
|---:|---:|---:|---:|
| 1 | 66 ms | 2.3 ms | 36 MiB |
| 20 | 97 ms | 10.3 ms | 88 MiB |
| 50 | 157 ms | 21.2 ms | 224 MiB |
| 100 | 289 ms | 47.5 ms | 401 MiB |

末端改深度约 274 ms；这些是单次读数，不是 P95。精确运算仍约 6 ms，完整 Naming 打包、前缀展开和服务端模型/Revision 装配继续随历史增长。Stage 计量维持在约 128 MiB 预算内，但 RSS 仍增长；本轮没有证明长时间稳态内存。源板 STEP 用例的 peak RSS 还包含源板生成和前序操作，不能把它解释为一次切除的独立峰值。

20/100/300/1000 个未消费草图另有规划微基准；1000 草图的局部规划调整前后约 42.2 → 31.0 ms，分配约 38.0 → 34.7 MB。每档仅 3 次迭代、一次运行，且只计规划，不执行 OCCT；不代表已经验收 1000 个实体 Feature。

### 复验与限制

真实 Router 入口为 `TestDenseSTEPFaceSketchAndFirstCutThroughRouter`、`TestLongPartChainWithFixedTopologyThroughRouter`、`TestPartSketchEditingIsolationThroughRouter` 和既有八次切除/上游编辑用例。扩大回归覆盖多 Body、持久选择、曲面关联、任意节点删除及恢复。连续撤销面支撑草图的回归也验证补偿求值使用相同依赖范围，不改写无关支撑快照或放宽历史摘要校验。

全 scope 单元检查通过，历史修复后补跑 Workspace 单元与相关真实集成；两项可选缩略图/原生测量导出跳过。原始报告在 `build/agent-check/`，本轮测量 JSON 保留实际运行入口与报告位置。真实浏览器验收入口为 `solid-feature-live` 的 `face sketch drawing retains accepted Body buffers and BVH`，并结合曲面、支撑修复、属性生命周期和阵列用例；具体本次结果见机器样本。

当前已去掉无关草图/Body 执行、整件显示重建及记账并集编码；首次复杂剖分、完整 Naming/Revision 数据装配和 OCCT 进程峰值仍是后续重点。Router RSS 软预算只约束扩容、推测预备和空闲额外副本回收；主颜色绘制仍提交可见场景，尚无通用逐面 GPU 更新或静态颜色合成层。后续工作继续归[CAD 核心主线](../../plans/cad-core-roadmap.md)，不据本轮有限样本承诺工业容量或 CATIA 等价延迟。

## 平面剖分与长链算法（2026-11-14）

同机 Release / OCCT 7.9.1、正常 `.env` 的 SQLite / LOCAL 与生产 Router，对基线 `a03ea2ec` 和本轮实现分别做三次串行运行。显示策略保持 **0.05 mm / 0.18 rad**。下表是相同机械板配方和操作的服务返回中位数，不含 HTTP 或浏览器，测量期间没有并行构建或其他测试。每次生成独立文档/Feature 身份，BREP 的序列化遍历可能不同；它是配方级对照，不能声称六次结果逐字节相同。冻结输入的单独算法对照见后文。完整读数、源 BREP 摘要、源码/Worker 指纹和报告在[本轮机器样本](../../tests/test.data/performance/part-algorithm-performance.json)。

| 服务边界 | 基线中位数 | 本轮中位数 | 加速比 |
|---|---:|---:|---:|
| 450 孔 STEP，首次新切除 | 4259 ms | 1755 ms | 2.43× |
| 同件切除深度 10 → 9 mm | 4066 ms | 1581 ms | 2.57× |
| 固定拓扑长链第 100 次切除 | 260 ms | 201 ms | 1.29× |
| 100 次切除后的末端编辑 | 270 ms | 197 ms | 1.37× |

机械板仍为 300×150×10 mm、450 个半径 1 mm 的贯通孔，经真实 STEP 导出/导入；输入均为 456/1362/908 个 Face/Edge/Vertex、127812 三角形。顶面 2×2 mm 矩形切除 10、9 mm，分别减少 40、36 mm³。首次新切除实际执行一个生成器，候选提升不重复 Body 执行；改参数也实际执行，均非相同结果命中。长链始终保持 6/12/8 个 Face/Edge/Vertex；第 100 次切除及其末端编辑各执行一个阶段、复用 100 个前缀。每个样本内另做冷算，BREP 和完整 Naming 摘要一致。

### 算法定位与实现

逐面计时定位到机械板上下两个各有 451 条 wire 的裁剪平面。独立试验用同一份冻结的 STEP 派生 BREP（355183 字节，SHA-256 `80e930ef1607f71b48aa2adf45e8e29807c1da225f12556d2434fd7725b42979`），每种策略从无剖分的深拷贝开始：

| 单次原生算法对照 | 剖分耗时 | 三角形 | 有向边界节点使用数 |
|---|---:|---:|---:|
| 原默认 OCCT 工厂 | 2828 ms | 127812 | 129648 |
| 只取消平面全曲面偏差控制（诊断对照） | 3269 ms | 127812 | 129648 |
| 仅平面改用内置 Delabella | 206 ms | 127812 | 129648 |

全部面的覆盖面积差不超过 1e-4 mm²，状态为成功，全部面及有向边界多边形存在。两个大平面的算法调用各约 2.69 s → 0.059 s，包含并行重叠，不能相加当作总墙钟。三角形数一致是此夹具的观察，不是任意输入的承诺。生产实现只切换平面基底，曲面细化、共享边采样、法线、邻接校验、取消及失败门禁保留。正式服务首次 mesh 子阶段中位数 2703 → 199 ms，改深度 2650 → 146 ms；这些子阶段属于 display，不能与总时长重复相加。

研究依据为 Shewchuk 的[三角剖分算法与数据结构对照](https://www.cs.cmu.edu/~quake/tripaper/triangle2.html)及[稳健几何谓词](https://www.cs.cmu.edu/~quake/robust.html)：算法、点定位与存储组织应单独比较，速度不能靠削弱数值判定获得。它们是研究依据，不证明 OCCT 某个函数的渐近复杂度。本轮复用已有依赖的 [OCCT 7.9.1 Delabella 工厂](https://github.com/Open-Cascade-SAS/OCCT/blob/V7_9_1/src/BRepMesh/BRepMesh_DelabellaMeshAlgoFactory.cxx)，没有引入 Triangle、新依赖或修改精确 BREP 算法。

长链另有三处实现调整：

- 显示贡献按历史 transition 归并，每步读取此前状态、统一提交本步写集，去掉反复复制全部状态的 `∑|Sᵢ|` 成本；同引用的 UNCHANGED 共享不可变归属集合。完整值键只在调用内准备，成员别名才需要反向摘要索引；摘要字符串一次构建，避免逐段重复复制。
- Naming v2 逐条装配，省去整份 protobuf FeatureResult 中间数组；完整字节键哈希查找仍按首次出现赋号。冻结原编码器的字节摘要和原显示投影摘要，验证格式、归属、成员及排序未变。
- 检查点只复制共享构造上下文的索引，工具、Body 阶段及范围基底由不可变对象持有；实际重执行才复制所需工具。完整原生历史、最终 Naming 和部分索引复制仍存在，未实现全链常数开销。

`BenchmarkFeatureAssociationHistory` 的 1000 步合成 Naming 微基准为 922 → 41 ms，累计分配约 1.62 GB → 80 MB，每档仅 3 次迭代、一次运行。它只执行 26 个关联元素的历史归并，不能当作 1000 个实体 Feature 的验收。真实 100 次切除夹具的独立 Go pprof 中，贡献索引累计分配读数约 1713 → 231 MB，总累计分配约 5740 → 4129 MB；采样包括建模与冷验，且不计入上表三组计时。它不是 live heap、单次峰值或 RSS。第 100 步当前 Worker RSS 基线约 402～571 MiB、本轮约 287～401 MiB，样本有明显波动，尚不能据此断言长期稳态内存。

### 复验与剩余瓶颈

正式配方仍由 `TestDenseSTEPFaceSketchAndFirstCutThroughRouter` 和 `TestLongPartChainWithFixedTopologyThroughRouter` 执行。`SurfaceDisplay.PlanarMeshingAlgorithmsPreserveTrimmedArea` 默认生成 450 孔平面，也可用 `OCCCCAD_TEST_MESH_BREP` 指定冻结 BREP 作上述诊断；路径只供原生测试，不进入领域模型。Naming 及贡献索引测试验证原字节/投影摘要、同一步读取隔离、合并/删除、阵列成员与支撑归属。面支撑历史回归增加真实上游编辑、Undo，以及 40 次草图绘制/创建的 Undo 和 40 次 Redo；只在完整精确 manifest 相同时保留原依赖制品，不放宽摘要冲突。

本次 `check --scope all` 单元层通过：含 200 项原生几何/Naming、749 项 Workspace、171 项 Web 场景及生产构建；服务检查中的可选缩略图画廊、原生测量导出两项跳过。另有正式 Router 五项回归及曲面/关联 Workspace 集成 17 个 case 通过，无跳过。

六项真实浏览器用例另行通过，涵盖曲面整体拾取、工程曲面/点/缩略图、草图显示复用、孔预览、阵列归属及轮廓高亮；工程曲面用例同步了默认收起的属性面板前置操作和已有 0.38 材质基线后复验。实际 450 孔导入件的 127828 三角形、索引缓冲/BVH、整件 hover/select、面命中和材质保持也通过，操作未改变 Head。浏览器使用 Chromium / SwiftShader，本轮未计 GPU 帧率或复杂场景交互延迟。

本轮后的首次 exact 中位数约 0.90 s，改参数约 0.86 s；它包含生成、材料 Boolean、同域合并、结果验证等，尚未细分到足以把全段归因于某一个算法。后续优先分解这些阶段，再评估空间候选筛选、可证明的局部处理和并行策略，保留完整拓扑历史及有效性。长链的完整历史输出、参数规范化、模型 JSON/digest 与 Revision 装配仍有分配；集合内容本身也随历史增长，不能把分步归并称为全流程 O(n)。远端约 10 MB B 柱仍未取得，本轮不提供该模型、CATIA 等价延迟、GPU 帧率或 P95 结论。
