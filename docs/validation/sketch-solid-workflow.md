# 草图与实体检验与测量

功能语义见[整体流程](../architecture/current/part-design-workflow.md)，可执行场景由[测试路由](../../tests/README.md)维护。本页保留检验边界及必要性能样本；旧交付清单、失败重试和逐次通过数由 Git 与 `build/agent-check/` 报告追溯，不作为当前保证。

## 功能检验边界

| 层次 | 检查内容 |
|---|---|
| 工具与界面 | 指针绘图、正式尺寸编辑、约束/自由度诊断、分类与命令隔离、视区拾取、全局反馈和取消 |
| Router/Worker | 参数驱动草图及实体、主草图合法区域、共享参数、多 Body、重复 occurrence、历史插入、失效修复、冷热重建和 Naming |
| 浏览器 | 实际树/视区选择、输入草图往返、工程孔与放样、阵列重执行、预览/提交及 Undo/Redo |

`TestSketchDimensionsDriveSolidThroughRouter` 使用显式正式尺寸与表达式，不假定绘图自动生成驱动尺寸。尺寸约束属于草图；三维测量及其输出不改变这一分类。全局失败检查保留原 request/diagnostic、一次失败一个出口和明确重试的恢复行为。

运行 `mise run check --scope sketch`、`--scope workspace`、`--scope web`，按测试路由选择真实集成与浏览器场景。Node/Mock、Router/Worker、浏览器及合同完整性是不同证据层；零匹配、skip、未执行和旧快照通过都不能替代本次验收。

## 测量环境

下列样本来自2026-11-07的 SQLite、本地 ArtifactStore、Release Router/Geometry Worker（OCCT 7.9.1）及 Chromium/SwiftShader。模型经正式 Command/Revision 创建，无数据库/制品重置或新浏览器下载。不是 PostgreSQL、原生 GPU、远程部署或工业容量验收。

## 性能样本（2026-11-07）

`TestPatternReexecutionPerMemberTerminationRepairAndIdentity` 输出 `CAD_WORKFLOW_SAMPLE`。模型包含5条轮廓曲线、3个实体特征、声明3槽位/活动2成员；各5个热/冷样本，每次真实改变阵列步距并验证解析体积及 Head 不变。小样本仅用于定位重复调用，不计算稳定 P95、60 Hz 或工业容量。

- serviceTotalMs：直接 Service PreviewCommand 的总墙钟时间，包含准备、权威求值、制品与读投影；不含浏览器。
- sketch-rpc / sketch-kernel：客户端求解 RPC 与 Worker 原生求解器调用；RPC 包含传输/转换，原生计时不含 Proto 转换。
- part-rpc：客户端 EvaluatePart RPC，包含 Worker 工作；Worker exact/naming/display/inputPrepare/artifactIO 分别来自运行统计。父子阶段可能重叠，不能相加当作总耗时。
- 浏览器 `sketch performance line commit to toolbar ready and visual classification` 单独测量5次实际点击提交到显示/工具恢复，并记录 hydrate/renderReady；软件 WebGL 与浏览器调度影响这组数据。

该快照的32 MiB有界准备缓存包含同一冻结输入的完整分量诊断；首次准备之外的4个热样本没有草图 RPC，冷端仍执行权威求解。测试同时检查解析体积与 Head 不变。

该快照各5个样本的阶段中位如下，原始数据为 `build/agent-check/workflow-phase-measurements-final.json`。热 Service 范围63.8–73.2 ms，冷71.0–73.8 ms；热端复用2个实体阶段并实际构造1个成员生成器，冷端执行3阶段/3次生成器，均保持相同精确结果及 Naming。

| 测量阶段 | 热（ms） | 冷（ms） |
|---|---:|---:|
| Service 总墙钟 | 64.6 | 73.4 |
| Part RPC | 20.8 | 28.9 |
| Worker 精确构造 | 7.5 | 13.1 |
| Worker Naming | 4.4 | 7.6 |
| Worker 显示生成 | 5.4 | 5.3 |
| Worker 制品 I/O | 1.33 | 1.38 |

前后运行的积分算法和环境状态不同，不能把中位差异单独归因于缓存，也不据此宣称稳定提速；可确认的是这些热样本未重复调用草图诊断 RPC。

浏览器在其他验证停止后独立采样5次：点击至本次提交、视觉加载及工具恢复的观察墙钟中位643.7 ms，范围555.1–681.7 ms；包含自动化等待、浏览器调度和软件 WebGL，不能等同于纯服务耗时。`sketchOperations` 调用中位176.0 ms，`renderReady` 中位2.8 ms。15次可能重叠的 hydrate 调用共用5次实际视觉下载，最终每次提交只调用一次 renderReady；这些阶段不能相加。原始数据为 `sketch-browser-measurements-final.json`。没有旧版本同条件的浏览器对照，不宣称浏览器提速。

精度检查覆盖高阶有理放样：体积与正式测量使用按 B-spline 区间划分的 OCCT 自适应 Gauss-Kronrod 积分，面积证据使用自适应积分，测量 evaluator 版本隔离旧结果。解析体积、完整边界导数及测量/实体体积一致性共同约束结果。积分选项参考 [OCCT BRepGProp 官方接口](https://www.occt3d.com/dev/doc/refman/html/class_b_rep_g_prop.html)，证据来自上述实际模型。

## 覆盖限制

上述样本不覆盖长期运行/大模型压力、所有 Feature×上下文×失败组合，亦不证明任意自由曲面 G2、建模螺纹、完整放样导向、任意局部范围重执行或跨 Body 历史创建可用。现有功能限制以[实体能力表](../architecture/current/solid-features.md#feature-能力表)为准。
