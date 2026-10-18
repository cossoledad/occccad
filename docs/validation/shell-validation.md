# 抽壳算法与功能验收

2026-10-17，基线 `4fc1302` 加本次工作区修改。当前合同见[实体 Feature](../architecture/current/solid-features.md)；本页记录特定输入的执行证据，不代表任意曲面均可抽壳。

## 基线与修复

基线四组原生 Shell 测试中，两组失败：`ShellAfterLargeModifiersAndMixedLoft`、`ShellOffsetRectangleToLargerCircle`，均含偏置精度错误。原始报告：`build/agent-check/20261017T093618-c87383f9/report.json`。

混合尖角/相切放样被 OCCT 整体偏置拒绝；一次多参数分区也可能丢失重合壁层。补充路径改用开口局部框架中的独立面精确偏置、二元裁剪与顺序并集，合成实际历史。局部交点在既定误差内重定位，真实材料轮廓与孔洞保留；不增加厚度容差。UV 邻近合并源三维边界点还会造成显示边界不一致，消费端通过公开网格扩展点保留不同源采样，并继续执行全部显示门禁。详见 `kernel/occt/src/shell_junction.cpp`、`mesh_boundary.cpp` 和 `occt_kernel.cpp`。

## 原生专项

`invoke check --scope geometry --match Shell`：5/5 组通过，无跳过，报告 `build/agent-check/20261017T120334-73c5b631/report.json`。收尾后专项再次 5/5 通过，报告 `build/agent-check/20261017T121537-362f9b5b/report.json`。

- 长方体向内/向外，单开口/双开口：验证解析体积、一个 Solid、完整 Naming、选择顺序冷重算、上游不变；零厚度、负厚度及缺少开口被拒绝。
- 大倒角、全棱圆角、倒角后底棱圆角、矩形到圆放样：验证开口、底厚、法向两侧材料、边/点记录与实际 pcurve 偏差、全部面有网格、有限坐标、GLB、上游 BREP 不变及冷重算 Naming。
- 偏置矩形 `(-82.8172482034993,-87.47795914135841)` 到 `(111.25953546530698,69.55214784189991)`，圆半径 90、截面间距 100 mm：0.5/1/2 mm；另加旋转平移后的 1 mm。同样检查实际厚度与完整制品。

## 真实 Worker、应用与浏览器

| 验证 | 结果与证据 |
|---|---|
| `invoke check --scope geometry` | 原生 100/100；Go 集成 220 通过、4 跳过、0 失败。`build/agent-check/20261017T120545-8c29ff9f/report.json` |
| 真实 Worker | 三个前置模型 × 0.5/1/2 mm 共九组抽壳均通过，收紧包含性门禁后九组再次通过。检查阶段面引用、完整 Naming、非空 BREP/GLB、体积减少和同输入缓存复用。最终日志 `build/agent-check/shell/worker-delivery.log` |
| 收尾后专项 | `invoke check --scope geometry --match Shell`：5/5；日志 `build/agent-check/shell/shell-containment.log` |
| 失败显示/缓存键/抑制定向 | `go test ./internal/workspace -run 'TestFailedBodyDisplayProvenanceAndRecovery|TestPartGeometryKeyIncludesFeatureIdentityAndTopologyPolicy|TestSolidFeatureSuppressionUsesDefinitionHistory' -count=1`：通过；`build/agent-check/shell/workspace-final.log` |
| Chromium，真实 API/Router/Worker | `pnpm exec playwright test --config playwright.live.config.ts --grep 'loft shell|viewport Boolean|chamfered box fillet' --output ../../../build/acceptance/playwright-shell-final`：4/4；`build/agent-check/shell/browser-delivery.log` |

完整领域回归之后，收尾修复了失败 Visual 的重复投影、更新旧浏览器用例的对象子菜单定位、消除了 C++ 参数遮蔽警告，并禁止复用包含证明的分区路径重定位端点。该收尾由最终原生专项、九组 Worker、Workspace 定向和真实浏览器覆盖；未在收尾后重复整个领域回归，也未运行全仓测试。包含性门禁收紧后，两类放样的浏览器创建/失败恢复/编辑/Undo/Redo/重开再次 2/2 通过，日志 `build/agent-check/shell/browser-containment.log`。

四项跳过均为显式 `NOT_APPLICABLE`：当前存储不是 S3；装配性能由独立性能入口测量；两个大型 STEP 用例未选择可选 corpus。不存在抽壳测试跳过。

两类放样在视图区选择开口，验证 0.5 mm 预览不推进 Head、1000 mm 无效厚度清除旧候选并禁止提交、修复后创建、编辑为 1 mm、Undo/Redo 新 Revision 及重开持久选择。另两项覆盖布尔视图区拾取/编辑及倒角→圆角→抽壳的精确体积、上游失败、旧成功结果显示、对象子菜单抑制和恢复。

首次浏览器组合链停在已移入对象子菜单的“抑制”定位（旧脚本仍在顶层查找），同次日志还暴露失败显示的 `visual naming digest mismatch`。重复投影测试先复现摘要变空，再修复为保留已有摘要；最终浏览器额外断言该摘要非空且实际 Visual 下载成功，没有通过绕过来源检查解决。旧失败日志保留于 `build/agent-check/shell/browser-final.log`。

最终留存文档：

- 小型放样：`/documents/01a14b4b-31d1-7a20-8f3f-f620c49312c8`。
- 偏置大截面放样：`/documents/01a14b4b-6963-74cc-bea0-dadb6f3eb632`。
- 两者的 `shell-reopened.png` 位于 `build/acceptance/playwright-shell-containment/` 对应用例目录；组合链的 `failed-body-display.png` 位于 `build/acceptance/playwright-shell-final/`。已检查开口、内壁及失败显示截图。

## 指定 Part2 底面 2 mm 验收

2026-10-17，本轮基线 `dc85844`。目标为 `/documents/01a14aba-ed9d-733d-beea-f7db4116ed41`，输入 Revision `01a14ad0-8ca9-7190-a9bf-33c8c2e04ec5`。模型为三截面放样（底部大五边形、中间小五边形、倾斜 30° 的半径 4 mm 圆），顶部半径 1 mm、深 3.5 mm 的盲孔，孔底圆角 0.1 mm。移除 Z＝−52 mm、法向 −Z 的 `LOFT_START_CAP`，向内抽壳 2 mm。

目标精确 BREP 可生成有效抽壳和完整 Naming，但一次 General Fuse 分区留下纯 INTERNAL 边用途的辅助线框，网格拒绝为 `VISUAL_MESH_FAILED:status=65`。对分区结果复用既有原生 ReShape 清理，合成真实删除历史，保留材料轮廓和孔洞。该路径禁止端点重定位，继续复用构造包含证明；不改变厚度、容差或网格警告门禁。evaluator 更新为 `part-solid-generators-v36-shell-partition-boundaries`。

本轮仅运行目标原生用例 `GeometryExchange.ShellBottomOfThreeSectionLoftWithFilletedBlindHole`，完整重建放样→盲孔→圆角→底面抽壳，检查底部开口、壁内 1 mm 有材料/3 mm 无材料、一个 Solid、完整 Naming、逐面网格覆盖、GLB、BREP 重读及上游不变。功能断言通过，日志 `build/agent-check/shell-bottom/native-target-final.log`，报告 `build/agent-check/20261017T124454-aefb5137/report.json`；后续只整理了该用例的排版。

另外用单个 Chromium 会话连接真实 API/Router/Worker，在原文档上完成持久底面绑定、2 mm 预览、确认 Head 未变、正式提交及重开，并实际读取 BREP/Naming/Visual。没有运行浏览器套件或全量单测。执行脚本和证据位于 `build/agent-check/shell-bottom/`：`accept.mjs`、`acceptance.json`、`acceptance.log`、`accepted.brep`、`accepted.glb`、`accepted.naming.pb`、`reopened.png` 和 `bottom-opening.png`。已检查从底部看到的开口及内壁。脚本通过正式领域命令提交；底面保存为 PersistentSelection，不保存面序号。

- 新 Revision：`01a14b66-f514-7445-92d8-cfd440f422d3`。
- 新特征：`shell-bc79ee46-20a0-5d72-ab6f-67e2026b4d98`，厚度 2 mm。
- 结果：体积 `21177.546343297454 mm³`，1 Solid、20 Faces、48 Edges、32 Vertices、3584 Triangles。
- 本次单次预览至提交约 4.94 秒；其中 Worker 预览约 3.67 秒，不作为稳定性能结论。

本轮未运行完整几何领域、其他模型或全量浏览器/单元测试；前文领域回归属于上一轮证据，不代表本次重复执行。应用已用新版本重启，未重置开发数据。

## 证据边界

共享当前 `.env` 的开发数据库、ArtifactStore 与 Worker 构建，未重置数据、未下载浏览器或修改第三方 OCCT。新增测试的对象键使用独立命名空间；浏览器文档留存供查看。最终九组 Worker 单次运行中，复杂放样的三个厚度冷 RPC 约 6.0/9.8/15.1 秒，缓存命中约毫秒级；当时与原生专项并行，属于单次样本，不是稳定延迟分位数。复杂曲面的冷计算仍需优化。专项耗时含冷重算及多次原生候选，不能据此声明吞吐或任意复杂曲面的工业可靠性。


## 编辑移除孔底圆角后的失败与耗时

2026-10-17（诊断 UTC 2026-10-18），继续基于 `dc85844` 及上述工作区修复。诊断 `01a14aba-ed9d-733d-beea-f7db4116ed41/20261018T050058.609993627Z-4b68947ac0524b00` 对应输入 Revision `01a14d60-85ce-7be2-9d2e-980f9fe94e0e`。用户已删除孔底 0.1 mm 圆角，当前输入为三截面放样与半径 1 mm、深 3.5 mm 的盲孔；目标仍为移除 `LOFT_START_CAP`、向内抽壳 2 mm。输入体积 118683.6910660475 mm³。

原来的独立法向壁层在圆柱孔壁与平面孔底的尖锐凹圆边处仅沿边相接，缺少恒定边距的过渡材料。一次分区得到两个 Solid，孔底薄片体积约 6.28319 mm³；后续八次二元工作域重试均以自干涉失败。不是已保存的开口 PersistentSelection 丢失。原诊断从创建到失败约 51 秒，同输入、生产四线程策略的原生重放 53.10 秒后复现同一错误。

实现以真实邻接识别完整凹圆形平面/圆柱交界，构造源圆外侧、半径等于厚度的四分之一圆扇区旋转体，只补缺失象限，并加入既有的一次分区。该构造在厚度小于、等于或大于孔半径时都不需要自交环面。原边到生成面的真实历史合成进入 Naming；输入副本、包含性、精度、实体及显示门禁保留。成功停在一次分区，避免八次无效回退；未新增缓存，也未提高容差。evaluator 为 `part-solid-generators-v37-shell-circular-corners`。

定向原生测试 `GeometryExchange.ShellBottomOfThreeSectionLoftWithFilletedBlindHole` 扩展为保留圆角的 2 mm 和删除圆角的 0.5/1/2 mm，共四种状态。检查单 Solid、完整 Naming、BREP 重读、自干涉、开口、普通壁厚及圆形连接体内外的独立材料采样、逐面网格覆盖、GLB 和上游 BREP 不变。执行 `invoke check --scope geometry --match ShellBottomOfThreeSectionLoftWithFilletedBlindHole` 通过（1/1 组、无跳过），日志 `build/agent-check/shell-edit/native-sector.log`，报告 `build/agent-check/20261017T222135-d35515a1/report.json`；随后仅整理 include 和测试邻近排版，Worker Release 构建通过。

### 串行性能记录

Release、OCCT 7.9.1、生产 Worker 相同的 4 线程池；各样本串行执行、强制冷求值，使用保留的前后原生可执行文件及相同 BREP。以下原生求值包含精确构造与 Naming，不含随后网格化和 API/数据库；`exact_ms` 包含其内部 wall_partition 等子阶段，不应相加。日志及汇总：`build/agent-check/shell-edit/*-four-*.log`、`baseline-failed-four.log`、`performance.json`。

| 输入 | 修复前 | 修复后 |
|---|---|---|
| 删除圆角，2 mm | 单次进程 53.10 s 后失败（原诊断真实预览约 51 s） | 三次冷求值 3433.40 / 3464.00 / 3447.96 ms，中位 3447.96 ms；含输入读取和网格化的进程约 3.5 s，均成功 |
| 保留圆角，2 mm | 三次冷求值 4181.53 / 4174.79 / 4173.12 ms | 三次冷求值 4216.30 / 4202.76 / 4217.33 ms；该原本成功路径没有明显提速 |

主要收益是该尖锐凹连接场景不再耗尽失败回退。修复后目标的分区仍约占 3.2 秒，是冷计算的主要耗时。这里仅有三个成功样本及一个失败基线，不推导稳定分位数或任意模型的加速比例；默认大线程池的探索日志不混入上述生产四线程对照。

### 原文档真实预览、提交和再次编辑

单个 Chromium 会话连接真实 API/WebSocket Domain Command、Router 和更新后的 Worker，未运行浏览器套件。以只读精确属性将底面绑定为 `LOFT_START_CAP` 的 PersistentSelection，完成 2 mm 创建预览与提交，再以正式 `EDIT_FEATURE`（definition digest）执行 2→1→2 mm 预览与提交。每次预览都检查 Head 不变，提交保持同一 Shell ID；最终重开、下载成套 BREP/Naming/Visual 并检查底部视图。

- 创建预览 3302.30 ms，采用预览提交 114.10 ms；编辑为 1 mm 预览 4323.50 ms、提交 68.50 ms；恢复 2 mm 预览 725.80 ms、提交 74.20 ms。后者可复用已有运行时结果，不作为冷算法加速证据。这些是客户端实测的完整调用耗时，不与原生 exact 子阶段混用。
- 最终 Revision `01a14d77-fabf-7623-b9dc-bdceda09fdca`；Shell `shell-09b602a2-3d23-58bb-aeac-3e9b795ea0be`，保留 2 mm。
- 最终体积 21178.62612591042 mm³，1 Solid / 20 Faces / 48 Edges / 32 Vertices，3366 个三角形；几何键 `sha256:dfc0b6cf7424a5ef02e0f14b8d430d2d137821ceb9eb08807dc9154caf2df4d7`。
- 脚本、记录及制品在 `build/agent-check/shell-edit/`：`accept.mjs`、`acceptance.json`、`acceptance.log`、`accepted.brep`、`accepted.glb`、`accepted.naming.pb`、`reopened.png`、`bottom-opening.png`；已检查两张截图，无页面运行错误。

本轮仅运行上述目标及厚度变体、单文档真实验收、文档审计和必要构建；未运行全仓、完整几何领域或浏览器套件。适用补充仍限完整凹圆形平面/圆柱连接，其他凹边、多角交会和一般曲面性能不由此保证。
