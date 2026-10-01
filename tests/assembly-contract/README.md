# 六类装配约束可执行合同

这里是 CONSTRAINT-CONTRACT 的跨实现执行入口，不是第二套持久 Constraint 模型。唯一生产目录在 [services/internal/assemblycontract/catalog.json](../../services/internal/assemblycontract/catalog.json)；本目录 `catalog.json` 是指向它的相对 symlink，不维护另一份矩阵。数学/产品预期见[目标合同](../../docs/architecture/target/assembly-constraints.md)，当前设施与执行记录见[当前 Product](../../docs/architecture/current/product-assembly.md#六类约束可执行合同目录)，剩余开发见[装配计划](../../plans/assembly-evolution.md)。

## 文件与证据职责

- `catalog.json`：`schemaVersion=1`、`contractVersion=assembly-six-families-v2`，六族、子类型、unary/binary/group 角色、八类 descriptor/稳定派生支持、参数单位/依赖、交换/方向/branch、模式/激活、秩/退化与分阶段失败合同。Point–Curve 明确为 Line/Circle，Point–Surface 为 Plane/Cylinder/Sphere/所选叶 Cone；Contact 11 个解析分支与一般秩已冻结，不声明任意曲线/曲面。规模随新增专用 case 变化，以 validate 输出和派生报告为准，不在 README 永久锁定数量。
- `runner.py`：选择并校验同一目录；C++ 映射到现有 GTest，Go 调用真实包内函数/既有测试，TypeScript 读取目录调用现有规则；集成走正式 Router/Worker。纯模型与带 scalar test double 的编排测试各有 `evidenceKind`，不冒充几何或数据库集成。
- `baseline.json`：目标/案例摘要、实现声明下限及测试断言源摘要的回归锁，不另维护覆盖表。目录新增可以扩展；删除目标、改预期、降声明、改已有断言会失败。C++/Go 的 `fixture.assertionHelperSymbol` 必须指向实际调用的断言 helper，其源码纳入同一锁，不能只锁空包装 TEST 而遗漏共享几何/生命周期断言。真实合同演进或合法测试调整必须连同目标依据与锁的变更一起审查，不用自动 bless 绕过失败。
- `test_runner.py`：目录/执行器的完整性与负例检查，不计为产品能力验证。

实现字段与执行 verdict 独立。`implemented` 仅指该层的窄组合；`partial/missing/unknown` 不可提升为完整产品能力。`specific-combination` 与 `representative-shared-foundation` 明确区分，复用生命周期或入口测试不代表每种几何组合都完成。`caseIds` 是可追溯映射，fixture 保留在原模块，不复制 solver/command 逻辑。

六个默认必需层为 `ui/domain/resolution/workerSolver/lifecycle/historyReplay`。即使当前公共 v2 声明这些层 implemented，也必须实际执行该 capability 对应的专用断言才能验收；生产能力不依赖 PASS 数量，源码/UI、模型编排、真实数值、Router/Worker/数据库历史仍各自标记证据层。

报告根据 `requiredLayers`（默认六层）和适用的 specific-combination case/`requiredCaseIds` 分别推导 `targetStatus` 实现覆盖、`verificationStatus` 执行完整性与 `acceptanceStatus`。缺具体层测试、NOT_RUN、ENVIRONMENT_BLOCKED、真实失败都不成为完整验收；拒绝未实现项和共享基础测试不能替代具体组合证据。实现补齐可提升覆盖，不再由 profile 名称永久固定 PARTIAL。集成测试按实际 native selector 分别运行，避免一个历史测试失败把另一真实通过 fixture 误标失败。

## 执行

从仓库根目录执行，依赖沿用既有 Debug CMake、Go、pnpm/TypeScript 和 Python 3 标准库：

```sh
python -m unittest discover -s tests/assembly-contract -p 'test_*.py'
python tests/assembly-contract/runner.py validate
python tests/assembly-contract/runner.py baseline --output build/assembly-contract-baseline
python tests/assembly-contract/runner.py gaps --output build/assembly-contract-gaps
python tests/assembly-contract/runner.py composition --output build/assembly-contract-composition
```

`baseline` 只执行目录中已纳入既有验证基线的 case；尚未纳入的目标断言显示 NOT_RUN。`gaps` 执行全部选中目标断言，实际失败返回 1；未来未实现项目继续保留为 TARGET_NOT_IMPLEMENTED，缺少目标测试显示 MISSING_TEST，不改成 PASS。两种检查都要求实际执行且成功的断言；空选择、拼错 ID、过时映射或零测试不能返回绿色结果。未执行、skip 或缺环境独立记录；选中的必需基线 case 未运行返回 2。可选集成环境阻塞不使数学/模型基线失效，但报告始终保留阻塞，不能称作集成通过。

`composition` 是更严格的收口门：全部选中 capability 的 `acceptanceStatus` 必须为 ACCEPTED，所有必需实现层和具体证据均齐全且实际通过。缺测、NOT_RUN、ENVIRONMENT_BLOCKED、缺 fixtures 或集成 skip 都不能返回绿色 COMPOSITION。筛选后的成功只证明该选择范围；里程碑必须运行无 capability/family/layer/adapters 缩范围的完整入口。baseline 通过不等于完整六族通过。

定向选择可组合 capability、family、layer 和 case；未知值与交集为空报错：

```sh
python tests/assembly-contract/runner.py baseline --family Angle
python tests/assembly-contract/runner.py baseline --capability offset.point-axis
python tests/assembly-contract/runner.py baseline --layer domain
python tests/assembly-contract/runner.py baseline --case composition.finite-joints
python tests/assembly-contract/runner.py gaps --case offset.plane-plane.first-normal-editor
python tests/assembly-contract/runner.py baseline --family Offset --output build/offset-after-baseline
python tests/assembly-contract/runner.py gaps --family Offset --output build/offset-after-gaps
```

`--adapters` 可限定实际执行设施；排除必需 case 仍报告 NOT_RUN，不获得完整基线通过。`--build-type Release` 使用既有对应构建目录，执行器不会下载依赖或启动新平台。C++ 每次先构建 scenario target；Go 使用 `-count=1` 及结构化事件，子测试跳过不能被父 PASS 隐藏；Web 复用 `pnpm test` 的场景发现与零匹配失败规则。新增 Web 场景同时调用目录/锁校验，因此 `pnpm test -- assembly` 会发现目录映射退化；Go/C++ 新断言也进入原模块的测试入口，没有修改共享构建系统或 `invoke check` 路由。

要运行真实 Router，先完成统一 Worker 和解析 fixture 构建，生成实际 B-Rep/STEP，再执行目录；不能在集成运行期间重新链接同一 Worker：

```sh
cmake --build build/cmake/debug --target occccad_geometry_worker occcad_geometry_scenarios --parallel 2
export OCCCCAD_ASSEMBLY_FIXTURE_DIR="$PWD/build/constraint-composition/analytic-fixtures"
build/cmake/debug/kernel/occt/tests/occcad_geometry_scenarios \
  --gtest_filter=AssemblyExactSupport.ExportAnalyticRouterFixtures
export OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/build/cmake/debug/workers/geometry/occccad_geometry_worker"
python tests/assembly-contract/runner.py baseline --case integration.router-worker
```

数据库/历史/Release case 另要求明确配置 `OCCCCAD_TEST_DATABASE_URL`。既有集成 fixture 要求可丢弃数据库；不自动指向应用开发库，不静默切换 Mock，不清库。具体要求见[开发环境](../../docs/development-environment.md)。

本批使用独立 `occccad_offset_contract_test`（已创建并保留），不是根 `.env` 的应用数据库。重复执行时显式设置指向可丢弃测试库的 URL 和当前 Worker，随后运行上面的 Offset baseline/gaps；未设置时报告 ENVIRONMENT_BLOCKED。不要为了测试重置应用数据或重建全部制品。

COMPOSITION 的真实 fixture 同时校验 URL 数据库名与 `current_database()` 均为上述专用库。显式配置其连接 URL（不要复制应用库 URL）后，可运行：

```sh
: "${OCCCCAD_TEST_DATABASE_URL:?请先设置专用 occccad_offset_contract_test 的连接 URL}"
python tests/assembly-contract/runner.py composition --output build/constraint-composition/contract-composition
```

FixtureExport 输出 sphere/cylinder/cone 的不同半径/尺度 B-Rep 与 STEP，可复用于真实 Router 和实机导入；没有目录环境变量时该测试只验证构造，不会导出文件。缺少安全环境时保留 ENVIRONMENT_BLOCKED，不自动连接应用库、不降级 Mock，不调用数据 reset。

## 报告与已定位差异

每次输出 `report.json`、`summary.md` 和实际工具日志，默认在忽略提交的 `build/assembly-contract/`。报告携带 HEAD、工作区状态/diff 摘要、目录/执行输入摘要、命令/环境存在性、scope、预期/观察、各层实现、测试层级、未实现/缺测试/未运行/环境阻塞和后续任务。报告是派生结果，不提交手工维护的完整覆盖表；同一次底层测试可支持多个合同 case，报告另列去重映射数。

`offset.plane-plane.first-normal-editor` 保持原目标/断言，现已通过并进入 baseline。新增 `offset.plane-plane.signed-rotated-exchange`、`offset.single-plane.signed-order` 调用实际 solver，独立计算最终几何/秩；Quantity、编译边界、模式、候选身份和编辑回填有实际 Go/TS 测试。三个 `offset.plane-plane.router-*` 复用一个真实数据库/Router/Worker fixture，覆盖 Datum/材料外法向、嵌套 occurrence、同 Part 不同 occurrence 与第二 CAD Body、跨零、表达式引用重命名/UndoRedo、不可测清值、冷读、manifest 与冻结 Release；不是三次独立或全矩阵验收。浏览器未运行。

此前 OFFSET 切片的锁更新仅覆盖其新映射、编辑器原目标和已交付窄层；原执行记录见 current。本轮进一步锁调整必须逐类记录理由：descriptor/placement/Worker 的坐标和半径统一 mm，Quantity 源值 SI；Contact 11 解析分支的目标公式/一般秩和非法/退化门；Point–Curve/Surface 的明确有限子类与 Frame 子元素；公共 v2/共享参数/组阶段/第三轴及相应真实测试映射。宽泛项展开可追溯子类，不能删除困难目标或降 requiredLayers。helper 引用与源码进入锁，不能只因新的包装 TEST 存在就提升覆盖；最终 baseline 变更须随具体 diff 审查，不全量自动 bless。

历史断言按既定准入合同核对，而非要求“任何失败都抛错/不保存定义”：检查 NotUpdated/Impossible/Broken、无可晋升 token、失败位姿不采用、被冻结失败定义及试算证据、未偷偷 Suppressed、接纳子集与完整定义分别重放。legacy manifest 测试验证旧字段形状/摘要及旧符号可读、篡改拒绝，不把新字段默认值倒写旧 Revision/Release。真实支持的历史路径使用专用数据库与当前 Worker；这不是降低几何要求，也不以 Mock、源码检查或拒绝测试代替完整历史证据。

Contact 和多成员 Fix Together 已有实际生产实现及专属正向测试。保留的旧 `CONTACT/FIX_TOGETHER` 拒绝测试现在检查非法输入：缺接触分支或缺组成员，无候选/ChangeSet；它们不再表示合法能力未实现，也不计入正向验收证据。Pair Rigid、零 Distance、Preview/null-space、TREE-03 编辑会话仍不能替代真实组、Contact 或 M4。

平行 EDGE 距离切片新增两个基线 case：`offset.axis-axis.parallel-off-origin-motion`（正距离/交换/Fix 构型，独立几何、偏好及物理 rank/DOF）、`offset.axis-axis.router-part-edges-history`（两个独立 Part 的真实 EDGE，Preview/提交/编辑/UndoRedo/冷读）。其旧阶段为 v9；本轮 v10 新增零交线/平行 chart/显式 Parallel 耦合、尺度 LM、严格可行性恢复和 Axis–Plane Measured 恢复的 90° 驻点测试。原符号、物理方程/秩和容差不改；此前零目标探索失败不倒填为旧阶段通过，新结果由本轮实际 case 报告给出。

复现入口：`runner.py baseline --case offset.axis-axis.parallel-off-origin-motion`；真实数据库入口：`runner.py baseline --case offset.axis-axis.router-part-edges-history`（显式测试库与当前 Worker）。新通过记录在 `build/edge-offset-verified-{baseline,gaps}/`，受影响合同记录在 `build/edge-offset-affected-baseline/`；初次 Worker 重链接期间的启动失败记录独立保留在 `build/edge-offset-baseline/`。执行前先等待 Worker 构建完成，不与重链接并行启动集成。应用数据库仅作只读诊断，不重置数据。

本轮原生统一执行 **177/177 scenarios、21/21 corpus 通过**，正式 XML 为 `build/constraint-composition/native-composition-final.xml`、`native-corpus-final.xml`。完整 `gaps-verified/report.json` 实际 610 PASS、332 个去重映射、58 项自动证据齐全，无 skip/环境阻塞；据此将新增案例纳入 baseline，原 100 个 ID、目标与断言保留。旧 Web catalog fixture 与逐能力 fixture 分批隔离，修复 selector 接线而非放宽期望。组的动态秩仍为 `6*(N-1)`，仅把已冻结的 DAG/重叠阶段规则替换旧开放边界。最终重新生成的 baseline/gaps/composition 报告位于 `build/constraint-composition/{baseline-verified,gaps-final-verified,composition-verified}/`，以实际 verdict 为准，不用基线标志宣称通过。

报告记录核对 commit/工作区、精确 Worker 可执行文件 SHA256 与测试数据库名（不保存凭据），`acceptanceScope=AUTOMATED_CONTRACT_ONLY` 与独立 `manualAcceptance=PENDING_MAINTAINER`。初轮真实失败保留在 `gaps/` 与 `gaps-final/`，Contact 显式 branch 校验、Publication Redirect Redo、第三角度轴组阶段、DIRECTED→FREE 分支污染等复现日志保留在同一输出目录。几何共享回归有一项 ImportedSolidRepairCorpus 环境缺 corpus 而跳过，与本轮合同目录无 skip 的结果分别记录。

最终执行器完整性共 20 项实际通过。额外负例锁住 Go 子场景的祖先 FAIL/SKIP：子场景 PASS 不能认证失败或未完整执行的包含 fixture，原始子事件仍保留；真正无关 fixture 的失败不污染本案例。此收紧未修改任何产品预期或 solver 容差。

最终交叉审查还补齐修剪圆弧的真实 inspection→UI 门禁：几何已解析与合同可用分开，compile/inspection 共用同一前提，Web 未明确收到 eligible 时不能提交，派生目标不被来源诊断误阻。增加 7 项纯支持域、生产 UI 和真实导入球面 seam/整圆求解历史证据，分别实际通过于 `arc-targeted/<caseId>/`，据此纳入基线而非自动 bless。最终目录为 617 项，最终完整报告使用 `build/constraint-composition/final-{baseline,gaps,composition}/report.json`，旧 610 项报告保留其真实执行范围。

最终完整三门各 **617 PASS / 335 去重映射 / 58 项自动 ACCEPTED**，没有合同内未运行、skip、缺测或环境阻塞。`GOFLAGS=-count=1 invoke check --scope assembly --verbose` 四步骤实际通过，日志 `final-assembly-domain.log`；全 Go 编译、相关 API/模型包、TypeScript/生产构建、TREE-03 四场景、context-audit 和 diff 检查通过。共享几何外部 repair corpus 跳过，通用大型 STEP/传输健康缺 corpus 未执行，均不是合同 PASS；未运行浏览器、全仓无差别单测或容量验收。准确环境、Worker SHA256、逐项预期/观察和原失败日志由上述派生报告保留。

## 维护者实机验收

维护者此前仅明确确认 OFFSET 改造使用验证通过；没有为本轮新 Contact/组或全部组合代签。本轮不运行浏览器自动测试。下列步骤复用导出的解析 STEP fixture，在新测试 Product 中操作，不需清理应用数据：

1. 重新构建并按仓库正常流程重启应用（不 reset-data），用上述 FixtureExport 生成数据，导入球/圆柱/圆锥 STEP，插入同 Part 的两个 occurrence，再加入嵌套 Product。核对支持顺序、精确类型、派生圆心/球心/轴/锥顶/Frame 子元素；来源未解析时不能默认开放错误组合。修剪圆弧应提示显式 Underlying Circle，未选时禁用整圆关系 Preview/确认；选择圆心/轴/平面或 Underlying Circle 后允许相应合法关系，保留来源参数域而非伪 Broken。
2. 分别创建 Coincidence、Offset、FREE/DIRECTED Angle、SPACE/RELATIVE Fix，令初始姿态明显不满足。检查 Preview/取消/提交、参数表达式、支持替换/重开编辑器，第一法向 Offset 不翻号，独立第三参考轴不随选择交换换 owner；Measured 不移动组件。
3. 对 Contact 逐项检查 Plane–Plane face、Plane–Cylinder line、Plane–Sphere point、Cylinder–Cylinder line/face、Sphere–Sphere face、Sphere–Cone/Sphere–Circle ring、Cone–Cone line/face、Cone–Circle ring。明确材料侧/所选叶和合法尺寸；Sphere–Sphere face 用等半径同球心，不是外切。Sphere–Circle 检查两高度及赤道，Cone–Circle 检查整圆，非法半径/材料分支应显示诊断且不采用失败姿态。轴平行法向/Plane face 反极点初态应实际转动到目标，不改成已满足场景。
4. 创建 2/3/N 成员 Fix Together，编辑增删成员、嵌套/重叠、激活/停用及解散；检查内部独立约束不被停用，内部更新先解、整组参加外部 Contact，失败不重新捕获关系或提交候选。选择/树/Inspector 能定位成员，Product 宿主与标签规则保持 TREE-03。
5. 对六族执行单项/批量停用恢复、适用的 Measured→Suppressed→恢复、空活动集合、来源变化/断裂/显式重连、Undo/Redo、重新打开和 Release 后改变 Head/replay。AXIS–PLANE 测量期间可移到不可测构型，应清旧值；恢复 Driving 应实际转回合法方向并保留驱动参数。检查失败定义状态与用户停用正交，冻结 Release 不随新 Head 换来源/符号/组关系。

记录实际操作的版本、场景与观察后再作人工确认。M4 最近可行拖拽、M5/M6、任意曲面接触和动力学不属于本批交互验收；现有 interaction-driver MOVE 不承诺 M4 行为。
