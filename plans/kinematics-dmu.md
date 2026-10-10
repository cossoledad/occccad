# 机构与 DMU 应用及仿真转装配：实施与实机验收

当前事实见[当前合同](../docs/architecture/current/kinematics-dmu.md)，长期候选见[目标合同](../docs/architecture/target/kinematics-dmu.md)。本次交付同一 Product 应用会话、几何接合、独立驱动/研究/分析、冻结回放和有效帧的正式装配转换。浏览器实机验收由维护者完成；后端与 Node 证据、前端浏览器合同模拟和真实后端实机验收分别记录。没有改动主题、求解器算法、精度、部署拓扑、Proto 或迁移。

## 已实现链路

- 定义：空机构可保存，独立 Joint/Driver/Study/Analysis；Applications 树选择/高亮/编辑/重命名/删除，运行节点只读。正式 SAVE_KINEMATICS 及 Undo/Redo，不修改装配位姿。
- 几何：真实轴/圆柱面/圆边及定位平面拾取，稳定 InstancePath/PersistentSelection/Publication；Frame 派生，捕获横向角度基准，支持偏移、零位、方向和限位。
- 方程：所选机构 Ground、全部接合、显式补充关系及硬驱动；来源不重复加入。来源变更与几何失效分别处理。导入是用户确认的 Ground/Revolute 提案。
- 运行：现有 Worker 求解完整合格帧、单坐标试动、0°～360°连续角、闭环、整帧回放；独立干涉分析范围/间隙/问题帧。应用退出清理覆盖，后台 Job 独立，旧结果显式加载且树/几何/选择统一冻结。
- 转换：计划列出新增/复用/更新/冲突与姿态变化；默认保留旋转自由度，可明确锁角。完整目标装配方程验证、正式求解、计划 digest、Head CAS，一次事务保存约束/位姿/映射，一次撤销。没有每帧 Revision 或固定全部运动件代替接合。

主要实现位于 `services/internal/workspace/kinematics_{model,geometry,projection,import}.go`、`motion_{snapshot,apply,demo}.go`，API 入口 `internal/api/motion.go`，UI `motion-study-panel.tsx`、Workbench/树/CommandRegistry 与 `cad/assembly/motion-definitions.ts`。普通 Product 求值和历史重放仍走原有通道；修复整值 kinematics Undo 时旧 omitempty 字段残留的问题。

## 转换交付的后端与逻辑验证（2026-10-10，前轮已执行）

复用匹配当前未改动 C++ 源码的 CMake Release Worker、OCCT 7.9.1。新测试创建 fresh SQLite 和隔离测试制品 staging，不重置开发应用数据，不访问配置 S3。之前的 S3/OCCT 专项不作为本次新跑证据。

| 范围 | 实际证据 |
|---|---|
| 圆柱—螺旋桨完整集成 | 普通建模命令创建两真实 Part，带通孔及两叶片；普通插入/移动命令产生偏移。真实几何命名绑定到圆柱轴/孔轴和定位面，独立 Driver 在没有 Study 时试动 45°且不改正式模型；真实 Router/Worker/Artifact/Job 产生 25 帧 0°～360°；90°帧转换后 3 条正式约束、3 条映射、DOF=1，独立核验姿态不漂移 |
| 正式提交与历史 | 错误 digest 无提交、同版本两个并发转换仅一个提交、读回正式姿态、一次 Undo 完整恢复前值、Redo、重复转换 REUSE 无新增约束；明确锁角后 4 条约束且 DOF=0；保留旧角锁时转换另一个帧报告冲突，明确停用后 DOF=1；旧 version/CAS 拒绝，转换专项 Go race 检测通过，多个冲突处理项采用确定排序以固定计划 digest |
| 关系与选中依赖 | 原 Fix 冲突明确处理，用户 REPLACE 后只替换该关系；导入 Ground/Revolute 的来源不重复进方程；无关空机构不阻止所选研究 |
| 保留的运动/DMU集成 | 真实四杆 21 帧、OCCT 离散检查与独立结论、冻结后 Head 改变、取消保存部分帧；嵌套刚体、多 Body、共享几何独立 occurrence、scope/后代 Revision 门禁及 staging 清理 |
| 必要逻辑回归 | 几何 Frame/捕获角基准/语义基线、重复来源树节点唯一及来源修改/删除投影；Revolute/Prismatic/Rigid、闭环、限位、连续角、取消/预算、失败类别和并发冻结研究；API权限/幂等/陈旧计划/不合格结果拒绝；目录命令注册 |
| Web与构建 | Node 的 motion-application、motion-study、command-registration、publication-selection、assembly-publication：对象删除依赖、正式关系保留、Part/转发 Product Publication 选择、完整 path+Body、高亮身份、冻结整帧/实际 Three.js 刚体变换、只读运行节点与应用命令门禁；TypeScript/Vite生产构建和受影响 Go 可执行单元编译 |

复现命令（仓库根，Worker 测试期间不要重链接同一二进制）：

```sh
OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/build/cmake/release/workers/geometry/occccad_geometry_worker" go -C services test ./cmd/occccad-jobs -run '^TestMotion' -count=1
OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/build/cmake/release/workers/geometry/occccad_geometry_worker" go -C services test ./internal/workspace ./internal/api ./internal/workbenchconfig -run 'TestMotion|TestCatalog|TestAssemblyOffset|TestAssemblyReferenceKey' -count=1
OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/build/cmake/release/workers/geometry/occccad_geometry_worker" go -C services test -race ./cmd/occccad-jobs -run '^TestMotionApplyGeometryFrameAtomicHistory$' -count=1
go -C services build ./cmd/occccad-server ./cmd/occccad-jobs ./cmd/occccad-control
pnpm --dir web/apps/cad test -- motion-application motion-study command-registration publication-selection assembly-publication
pnpm --dir web/apps/cad build
invoke context-audit
git diff --check
```

前一轮未运行浏览器；此次框架重构新增浏览器模拟，结果见下节。仍未做真实后端浏览器实机/人工视觉验收、无关单元测试、PostgreSQL 或配置 S3 专项。没有新增性能基线、P95、工业容量或 60 Hz 结论。

## 前端框架重构（2026-10-10）

- 移除硬编码 Product 应用 Tabs、重复命令按钮目录及 DMU Drawer/Modal。`catalog.json` 给根 Product 配置机构分类和定义/驱动/分析/关联/回放组，接合使用现有 variants；名称、说明、图标与搜索投影共用唯一声明。`rootTarget` 防止在嵌套编辑上下文建立宿主机构，`motionActive` 控制入口显示。
- `useWorkbenchTab` 共用原文档会话/工作台选择；没有新增应用布尔状态。跨机构分类边界取消交互命令，退出释放覆盖，不发模型命令。普通视图/文档分类保持原有行为。
- 定义与转换使用既有 `CommandDialog` 与 form 的 `CommandOperation`；运行/回放/任务取消是 handler，统一经 Registry 执行。视口下方 `WorkbenchLayout.activity` 保留机构、研究、分析及冻结运行选择、完整帧定位和 DMU 列表；Inspector 仍只读。后端 Job 生命周期独立。
- 树双击定义使用既有 `tree.edit` form 的完整操作生命周期；分离命令和结果请求代际；关闭转换计划保留当前回放，迟到拾取/提案/计划不改新草稿，退出后迟到运行不抢占视口。补充关系/解除发布关联改为确认后保存，取消不推进 Revision。

验证入口：

```sh
go -C services test ./internal/workbenchconfig ./internal/api -run 'Catalog|Toolbar' -count=1
pnpm --dir web/apps/cad test -- command
pnpm --dir web/apps/cad test -- toolbar
pnpm --dir web/apps/cad test -- motion
pnpm --dir web/apps/cad build
pnpm --dir web/apps/cad exec playwright test browser/motion-workbench.spec.ts browser/workbench.spec.ts -g 'one configured|geometry picks|frozen whole-frame|a late|canceling an apply|contextual commands|panel layout' --timeout=120000
invoke context-audit
git diff --check
```

浏览器用例通过测试页局部 API fixture 验证前端，fixtures 不进入应用 Mock adapter，不作为求解/OCCT/正式 CAS 的证据；正式链路仍采用上节实际 Worker 集成结果。模拟覆盖一个配置 Tab 区、搜索门禁、保存与 Esc/分类退出取消、树编辑、选择向接合草稿绑定、整帧回放、计划关闭保留预览、所选帧转换请求/返回装配，以及退出后的迟到结果。最终串行运行 **7/7 通过（5.3 min）**：5 个机构用例及原工作台的命令/草图、布局/文档切换 2 个用例，Chromium + SwiftShader，单 worker；120 s 是软件 WebGL 下的单测试预算，不是请求或求解性能指标。Node `command` 6/6、`toolbar` 3/3、`motion` 3/3；目录/API 的 Catalog/Toolbar Go 测试、服务端构建、TypeScript/Vite 生产构建、context-audit 与 diff-check 通过。本轮未重新运行未改动的数值 Worker 集成。

验证发现并修复：提交转换后 Head 刷新过早结束 form，导致不能自动返回装配；现在保留已发起转换的命令至完成，但显式取消仍阻止迟到完成切换应用。树双击定义转入已有 `tree.edit` form，避免沿用瞬时 activate handler 的操作。原浏览器用例按当前代码修正了“拉伸必须预选草图才可打开”和“窄屏参数始终位于当前 Ribbon 页”的过期假设；没有修改对应建模行为。构建保留既有大 chunk 提示，浏览器保留既有 Ant Design 弃用/软件环境采样提示，没有页面异常。

## 圆柱与螺旋桨实机步骤

1. 使用最新构建启动后端 `invoke run.app --build-type=Release`，另一终端 `invoke run.web --mode=api`。新建并打开空 Product，切到“机构与 DMU”Tab。Mock 不支持真实计算。
2. 在机构 Ribbon 的示例组点击“圆柱与螺旋桨示例”（窄屏可用“搜索工具”找到命令）。服务通过普通 CREATE_SKETCH / EDIT_SKETCH / PAD_SKETCH、INSERT_INSTANCE / MOVE_INSTANCE 等命令创建圆柱轴（半径 4 mm、长 30 mm）和螺旋桨（轮毂外半径 12、孔半径 4.5、厚 4 mm；两叶片伸至 ±40 mm、宽 6 mm），螺旋桨初始偏移 (22,15,7) mm。它不创建机构、接合、驱动或动画；后续全走通用功能。演示生成是多条跨文档命令，中断可能留下已创建 Part，换空 Product 重试。
3. “新建机构”输入名称并保存空机构；“固定件”选择圆柱轴并保存。Applications 树中应出现机构与固定接合。
4. “旋转接合”：第一端拾取圆柱轴的外圆柱面及其底部端面；第二端拾取螺旋桨通孔的内圆柱面及轮毂底部平面（不是叶片侧面）。两端都选原始 XY 草图侧的底面。这个演示的真实派生轴沿 -Z，设轴向偏移 **-10 mm**，零位 0°、方向 +1、限位留空；保存。应不要求填写四元数。拾取按钮后点击几何；也可以先选几何再用“使用当前选择”。若选择的是另一端面/反向轴，须按所选面和轴方向调整偏移。
5. “驱动”选择旋转接合并保存；可先用“单坐标试动”直接选择 Driver 求解 45°，不必先创建研究，查看后恢复正式姿态。“仿真”选择该驱动，起止 0°/360°，时长 **4 s**，帧数 **25**，预算 60000 ms，保存；在已保存仿真下拉框选择研究，点击“运行仿真”。应得到 25 个运动学合格帧，连续坐标最终 360°。播放、暂停、单步、定位、复位应使用完整帧，Revision 不变。
6. 暂停并定位 **t=1 s**（第 7 帧，对应 90°）。点击“应用到装配”，默认不勾选“锁定当前角度”；“生成／刷新转换计划”应列出圆柱 Fix、轴相合、轴向定位与螺旋桨位姿变化，DOF=1。确认“一次提交并返回装配”：两者同轴、轮毂底面 z=10 mm，螺旋桨采用选中帧角度。
7. 重新打开 Product 应保留正式姿态。普通装配求解认可这三条约束，移动工具仍可沿允许旋转方向试动。回到 DMU 显式加载同一冻结运行，再应用同一帧，应 REUSE 且约束数量不增。直接在首次转换后一次 Undo 应恢复转换前的全部约束、偏移位姿和映射；Redo 恢复转换。若做了重复转换，先撤销最后一次命令，再撤销首次转换。
8. 可另行选择“锁定当前角度”生成计划，应多一个静态角约束、DOF=0。随后选择另一个帧、取消锁角，旧角锁必须列为冲突；明确停用或采用接合替换，再刷新计划后提交。已有螺旋桨 Fix 或定位角的情形同理，不能自动丢弃。

## 干涉、关联与生命周期补充验收

- “干涉检查”保存 Analysis，可不关联研究检查当前正式姿态，也可关联上述研究并选择完整 occurrence + Body 的范围、间隙。选择圆柱与螺旋桨，间隙 0.4 mm 预期分离且通过；0.6 mm 预期间隙不足（孔半径与轴半径之差为 0.5 mm）。运动学状态与干涉结果应独立显示，结果行高亮到两个实例的 Body，问题帧可定位。
- “装配关联”可预览已正式发布的 Ground/Revolute 提案，在另一空机构确认导入；导入后的补充关系列表不应允许再选来源约束。更新来源、解除来源关联、删除接合都不应直接删除 Product 约束；正式约束和接合双方变化时转换需冲突处理。
- Applications 对象可选择/高亮、编辑、重命名、删除，语义展开项和来源/发布项只读。删除接合清理依赖驱动/研究，保留正式装配约束；关联 Analysis 转为独立静态分析。
- 运行中退出 DMU 或关闭命令，后台任务继续；仅“取消后台任务”发取消请求。重新进入可显式加载结果。退出、切换版本或晚到结果不得留下/抢占视口覆盖；历史回放的树、几何和选择一致使用冻结版本。
- 空 Product 可用“四杆示例”检查闭环及离散 DMU。它仍使用已有数值 Frame 定义，不具备可发布所需的真实轴/平面引用，不能直接“应用到装配”。这不是圆柱演示链路的替代。四杆铰接处几何有意重叠，运动学合格但干涉可能 VIOLATION。

## 已知限制与待验收

- 浏览器模拟仅验证真实 UI 组件与测试页 API 合同交互；真实后端 WebGL 拾取、冻结几何加载、高亮及正式页面视觉效果仍待维护者实机验收。
- 自动导入提案当前覆盖 SPACE Fix 与轴相合＋有向平面定位的 Revolute；Prismatic/Rigid 可手动创建，不做广义约束反推或 Engineering Connections。
- 根 Product 应用上下文、直接子 Product 内部刚体冻结；不支持可动嵌套机构。运行要求接地后一个独立自由度，驱动后局部确定；局部 DOF/硬见证不证明全局运动可行，奇异位形可能停止而不等于无解。
- 正式发布需要稳定轴/平面支持和捕获角度基准；没有支撑的旧数值定义不能发布。旧研究必须具备独立 Driver 定义；未增加开发数据兼容适配。
- DMU 仅有效实体及离散帧，正常铰接重叠也报告；无连续碰撞、动力学、气动、螺旋副、多驱或自动避碰。本次没有新增跨帧 B-Rep 缓存或通用收敛优化。
- 结果列表为当前用户最多 100 项 Job，未做共享/分页研究库；进程崩溃前未持久化的帧不能保证恢复。正式计划求解使用有界预算，失败不产生部分正式提交。
