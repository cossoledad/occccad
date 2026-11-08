# 草图与实体流程验证

对应[整体设计流程](../architecture/current/part-design-workflow.md)与[CAD 主线](../../plans/cad-core-roadmap.md)。当前行为由 current 主题维护；本页只记录必要检验与测量边界，原始日志/JSON 在 `build/agent-check/`，可按开发资源策略清理。

## 环境和证据范围

本轮使用当前 `.env` 的 SQLite、配置的本地 ArtifactStore、真实 Router/Release Geometry Worker（OCCT 7.9.1），以及预装 Chromium/SwiftShader；没有重置数据库/制品，也没有下载新浏览器。验证不是 PostgreSQL、全部平台或工业容量验收。模型来自测试中的正式 Command/Revision，不是截图推断或手册模型验证。

## 操作精简复验（2026-11-08 UTC）

本次按新的操作范围移除实体尺寸手柄与绘制时创建尺寸约束的数值框，精简实体表单，统一全局错误/修复提醒，扫掠及薄壁线链采用视区选择。Part、Assembly 与 Sketcher 增加测量、参数与接口分类；Sketcher 的测量分类使用现有尺寸工具，不接入仅支持 B-Rep 的三维测量。诊断面板采用约束/自由度表格。下方 2026-11-07 的执行记录属于旧快照，其中数值绘图与深度/角度手柄已被本次范围替代。

| 实际验证 | 结果 | 原始记录（`build/agent-check/` 下） |
|---|---|---|
| Web 生产构建与完整前端场景 | PASS：154 个场景，0 skip | `ui-simplification-web-final.log`；`20261108T060854.847197015/report.json` |
| Catalog 与相关服务单元测试 | PASS：42 个测试，0 skip；检查上下文实际加入分类、Sketcher 尺寸分类及命令可见/可执行边界 | `ui-simplification-catalog-final.log`；`20261108T060855.102214390/report.json` |
| Router/Worker 与服务集成 | PASS：1 个正式尺寸驱动实体的真实 Router/Worker 场景，4 个 Catalog/API/数据库相关场景，0 skip | `ui-simplification-services-final.log`；`20261108T060617.537416872/report.json` |
| 实际浏览器 | 14 个不同场景最终通过，分批执行；末批 3/3 通过 | `ui-simplification-browser-2.log`、`ui-simplification-browser-3.log`、`ui-simplification-browser-final.log`、`ui-simplification-browser-delivery.log` |
| 文档、合同目录与差异格式 | PASS：文档链接/入口、41 个合同完整性检查及 `git diff --check` | `ui-simplification-docs-final.log`、`ui-simplification-contract.log` |

浏览器覆盖孔预览；倒角预览失败、候选失效及全局恢复；Boolean 失效区域重选和迟到响应；工程孔数量校验、下游修复警告、孔组及阵列；指针绘图及无自动尺寸、分类和搜索隔离；诊断选中/定位/停用/启用；薄壁线链；新建/更换输入草图后返回实体草稿；依赖及健康历史对照后的修复；Sweep；G2 放样。初次分批运行的失败保留在日志中，已修复实际分类上下文遗漏、失效提示在加载时被过早清除的问题，并更新已删除手柄、数量框和说明文字的旧断言；相关失败项均已复测通过。不能把各批日志整体称作全部成功，或把 14 个场景称作整份浏览器套件。

`TestSketchDimensionsDriveSolidThroughRouter` 用显式正式尺寸与表达式检验 Preview、后续实体、参数更新、引用更换、Undo/Redo 与冷读取，不再声称绘图工具会生成这些尺寸。Web 场景另外检查绘图数字键不产生尺寸，并保留方向推断、捕获连接、权威候选和回执行为。全局反馈检查保留原 request/diagnostic、每个失败一个出口，以及同一幂等请求明确重试后仍可再次报告失败。

本次未重置开发数据库/制品，未运行全仓集成、全量浏览器、PostgreSQL 或原生 GPU 验收；没有新的性能或容量结论。截图位于 `build/acceptance/ui-simplification-final/`，仅对应末批实际执行的场景。

## 组合与专项入口

| 场景 | 关键检查 | 可执行入口 |
|---|---|---|
| 主草图与壳体 | 开放主草图仍可消费独立区域；Shell 和关联偏移环体共享壁厚，更新/Undo/Redo/重开/冷 Naming | `TestMasterSketchRegionsDriveShellAndLinkedOffsetBodies` |
| 重复 occurrence | 独立默认高度10、已接受20/30分别求值；薄壁贯穿、重执行、Sweep 新 Body、孔组；位姿、冻结输入及冷结果 | `TestSketchSolidCombinedWorkflowAcrossRepeatedOccurrences` |
| 历史位置 | Sweep 两个输入先于生成器，插入后继续原 Chamfer；体积及冷历史 | `TestInsertSweepBeforeModifierKeepsInputsAndColdHistory`；既有历史生成器/范围保护用例 |
| Sweep | 光滑弯管、直线斜接、闭合圆环、内孔；路径缺失、方向及显式修复 | `SweepSmoothMiterClosedAndAnnularHaveColdStableNaming`、`TestSweepFormalPathThroughRouter`、浏览器 Sweep 场景 |
| 工程孔 | 多点、三种孔型、钻尖两种深度、表达式、标注/几何边界和逐点修复 | `EngineeringHoleGroupDrillDepthAndColdIdentity`、`TestEngineeringHoleGroupThroughRouter`、浏览器孔组场景 |
| 放样 | 完整边界基底导数与解析支撑、无效支撑拒绝、参数/缺失修复及冷热 Naming | `LoftEndJetsMatchExactCylindricalSupportsAndColdNaming`、`TestLoftBoundarySupportsThroughRouter`、浏览器 G2 场景 |
| 成员重执行 | 五类生成器真实构造次数、斜顶逐位置终止、镜像、每成员材料变化、取消、稳定引用/槽位修复 | `PatternReexecution*`、`TestPatternReexecutionPerMemberTerminationRepairAndIdentity`、浏览器重执行场景 |
| 设计输入与诊断 | 正式尺寸/公式、区域、推断/自由度、依赖/健康历史对照、薄壁与关联终止、深度/角度草稿 | 对应 control integration 与 `solid-feature-live.spec.ts` 场景 |

## 本轮执行结果（2026-11-07）

| 验证层 | 实际结果 | 原始记录（`build/agent-check/` 下） |
|---|---|---|
| 全域单元与构建 | PASS：Assembly 233、Geometry 144、Sketch 67；Workspace 723、Services 965、Tooling 89、Web 156；契约41、conformance 2；生产构建与 codegen 一致性通过 | `20261108T005633.355875091/report.json` |
| 定向 Router/Worker 集成 | PASS：48个主用例及其子场景，共97条通过记录；另1项相关服务用例通过 | `20261108T003810.386115298/report.json` |
| 功能浏览器 | PASS：21个场景，含创建/编辑/取消、公式、历史/输入往返、诊断修复及新生成器 | `solid-sketch-final-browser.log` |
| 浏览器性能与历史 | PASS：1个独立场景、5次真实提交、5次视觉下载；取消/Undo/Redo/重开保持正式模型 | `solid-sketch-browser-performance-final.log` |

全域命令为 `mise run check --scope all --test-jobs 4`。Services 包含 Workspace，Tooling 也有重叠，不能累计成独立测试总数。Services 另有1项 `TestThumbnailGallery` 按既有规则跳过：可选缩略图库导出，不计入通过。集成不是全部集成注册表，浏览器也不是全部 browser-live；执行记录中的未选项不算通过。报告目录名使用 UTC，本页日期使用本机时区。

主线状态与执行结果同步；入口存在不表示后续修改自动通过。全域单元、定向集成、实际浏览器与文档/契约是不同证据层，不相互替代。

## 性能测量边界

`TestPatternReexecutionPerMemberTerminationRepairAndIdentity` 输出 `CAD_WORKFLOW_SAMPLE`。模型包含5条轮廓曲线、3个实体特征、声明3槽位/活动2成员；各5个热/冷样本，每次真实改变阵列步距并验证解析体积及 Head 不变。小样本仅用于定位重复调用，不计算稳定 P95、60 Hz 或工业容量。

- serviceTotalMs：直接 Service PreviewCommand 的总墙钟时间，包含准备、权威求值、制品与读投影；不含浏览器。
- sketch-rpc / sketch-kernel：客户端求解 RPC 与 Worker 原生求解器调用；RPC 包含传输/转换，原生计时不含 Proto 转换。
- part-rpc：客户端 EvaluatePart RPC，包含 Worker 工作；Worker exact/naming/display/inputPrepare/artifactIO 分别来自运行统计。父子阶段可能重叠，不能相加当作总耗时。
- 浏览器 `sketch performance line commit to toolbar ready and visual classification` 单独测量5次实际点击提交到显示/工具恢复，并记录 hydrate/renderReady；软件 WebGL 与浏览器调度影响这组数据。

初始测量发现仅改阵列参数时，完整草图求解已缓存，分量诊断仍重复调用。将同一冻结输入下的完整分量诊断纳入既有32 MiB有界准备缓存，保持单位、硬约束、DoF、失败、取消与每请求冷重建语义。最终版本首次准备以外的4个热样本不再发生草图 RPC，冷端仍执行权威求解；测试直接断言调用消除、解析体积与 Head 不变。

最终版本各5个样本的阶段中位如下，原始数据为 `build/agent-check/workflow-phase-measurements-final.json`。热 Service 范围63.8–73.2 ms，冷71.0–73.8 ms；热端复用2个实体阶段并实际构造1个成员生成器，冷端执行3阶段/3次生成器，均保持相同精确结果及 Naming。

| 测量阶段 | 热（ms） | 冷（ms） |
|---|---:|---:|
| Service 总墙钟 | 64.6 | 73.4 |
| Part RPC | 20.8 | 28.9 |
| Worker 精确构造 | 7.5 | 13.1 |
| Worker Naming | 4.4 | 7.6 |
| Worker 显示生成 | 5.4 | 5.3 |
| Worker 制品 I/O | 1.33 | 1.38 |

初期热 Service 中位73.8 ms、缓存首次复测73.0 ms；此后积分算法及环境运行状态也有变化。这些小样本不能把最终差异单独归因于缓存，不据此宣称稳定提速。确定的优化结论仅是同一冻结草图输入下的重复诊断 RPC 已消除。

浏览器在其他验证停止后独立采样5次：点击至本次提交、视觉加载及工具恢复的观察墙钟中位643.7 ms，范围555.1–681.7 ms；包含自动化等待、浏览器调度和软件 WebGL，不能等同于纯服务耗时。`sketchOperations` 调用中位176.0 ms，`renderReady` 中位2.8 ms。15次可能重叠的 hydrate 调用共用5次实际视觉下载，最终每次提交只调用一次 renderReady；这些阶段不能相加。原始数据为 `sketch-browser-measurements-final.json`。没有旧版本同条件的浏览器对照，不宣称浏览器提速。

另修正高阶有理放样的固定阶质量属性积分误差：体积与正式测量使用按 B-spline 区间划分的 OCCT 自适应 Gauss-Kronrod 积分，面积证据使用自适应积分，测量 evaluator 版本隔离旧结果。该修正以解析体积、完整边界导数及测量/实体体积一致性检查证明，不能用降低精度换取性能。积分选项参考 [OCCT BRepGProp 官方接口](https://www.occt3d.com/dev/doc/refman/html/class_b_rep_g_prop.html)，本轮验证来自上述实际模型。

## 明确未覆盖

未声称任意自由曲面 G2、螺旋螺纹几何、任意局部特征范围重执行、完整放样导向曲线、跨 Body 历史创建或所有 Feature×上下文×失败组合。长期运行/大模型压力、远程部署、硬件 WebGL 与 PostgreSQL 需要各自验收；已实现范围见唯一能力表，未运行项不写成通过。
