# 六类装配约束可执行合同

这里是 CONSTRAINT-CONTRACT 的单一逻辑目录与跨实现执行入口，不是第二套持久 Constraint 模型。数学/产品预期见[目标合同](../../docs/architecture/target/assembly-constraints.md)，当前设施与执行记录见[当前 Product](../../docs/architecture/current/product-assembly.md#六类约束可执行合同目录)，剩余开发见[装配计划](../../plans/assembly-evolution.md)。

## 文件与证据职责

- `catalog.json`：版本化的 58 个 capability、100 个 case，六族、子类型、unary/binary/group 角色、descriptor/派生支持、参数单位/依赖、交换/方向/branch、模式/激活、秩/退化与分阶段失败合同。共享 profile 按引用展开；宽泛 Curve/Surface 和 Contact 未冻结的秩显式为边界/问题，不填猜测值。
- `runner.py`：选择并校验同一目录；C++ 映射到现有 GTest，Go 调用真实包内函数/既有测试，TypeScript 读取目录调用现有规则；集成走正式 Router/Worker。纯模型与带 scalar test double 的编排测试各有 `evidenceKind`，不冒充几何或数据库集成。
- `baseline.json`：目标/案例摘要、实现声明下限及测试断言源摘要的回归锁，不另维护覆盖表。目录新增可以扩展；删除目标、改预期、降声明、改已有断言会失败。真实合同演进或合法测试调整必须连同目标依据与锁的变更一起审查，不用自动 bless 绕过失败。
- `test_runner.py`：目录/执行器的完整性与负例检查，不计为产品能力验证。

实现字段与执行 verdict 独立。`implemented` 仅指该层的窄组合；`partial/missing/unknown` 不可提升为完整产品能力。`specific-combination` 与 `representative-shared-foundation` 明确区分，复用生命周期或入口测试不代表每种几何组合都完成。`caseIds` 是可追溯映射，fixture 保留在原模块，不复制 solver/command 逻辑。

报告根据 `requiredLayers`（默认六层）和适用的 specific-combination case/`requiredCaseIds` 分别推导 `targetStatus` 实现覆盖、`verificationStatus` 执行完整性与 `acceptanceStatus`。缺具体层测试、NOT_RUN、ENVIRONMENT_BLOCKED、真实失败都不成为完整验收；拒绝未实现项和共享基础测试不能替代具体组合证据。实现补齐可提升覆盖，不再由 profile 名称永久固定 PARTIAL。集成测试按实际 native selector 分别运行，避免一个历史测试失败把另一真实通过 fixture 误标失败。

## 执行

从仓库根目录执行，依赖沿用既有 Debug CMake、Go、pnpm/TypeScript 和 Python 3 标准库：

```sh
python -m unittest discover -s tests/assembly-contract -p 'test_*.py'
python tests/assembly-contract/runner.py validate
python tests/assembly-contract/runner.py baseline --output build/assembly-contract-baseline
python tests/assembly-contract/runner.py gaps --output build/assembly-contract-gaps
```

`baseline` 只执行目录中已纳入既有验证基线的 case；尚未纳入的目标断言显示 NOT_RUN。`gaps` 执行全部选中目标断言，实际失败返回 1；未来未实现项目继续保留为 TARGET_NOT_IMPLEMENTED，缺少目标测试显示 MISSING_TEST，不改成 PASS。两种检查都要求实际执行且成功的断言；空选择、拼错 ID、过时映射或零测试不能返回绿色结果。未执行、skip 或缺环境独立记录；选中的必需基线 case 未运行返回 2。可选集成环境阻塞不使数学/模型基线失效，但报告始终保留阻塞，不能称作集成通过。

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

要运行真实 Router，先按既有构建入口保证 Worker 与当前代码一致，再指定它：

```sh
cmake --build build/cmake/debug --target occccad_geometry_worker --parallel 2
OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/build/cmake/debug/workers/geometry/occccad_geometry_worker" \
  python tests/assembly-contract/runner.py baseline --case integration.router-worker
```

数据库/历史/Release case 另要求明确配置 `OCCCCAD_TEST_DATABASE_URL`。既有集成 fixture 要求可丢弃数据库；不自动指向应用开发库，不静默切换 Mock，不清库。具体要求见[开发环境](../../docs/development-environment.md)。

本批使用独立 `occccad_offset_contract_test`（已创建并保留），不是根 `.env` 的应用数据库。重复执行时显式设置指向可丢弃测试库的 URL 和当前 Worker，随后运行上面的 Offset baseline/gaps；未设置时报告 ENVIRONMENT_BLOCKED。不要为了测试重置应用数据或重建全部制品。

## 报告与已定位差异

每次输出 `report.json`、`summary.md` 和实际工具日志，默认在忽略提交的 `build/assembly-contract/`。报告携带 HEAD、工作区状态/diff 摘要、目录/执行输入摘要、命令/环境存在性、scope、预期/观察、各层实现、测试层级、未实现/缺测试/未运行/环境阻塞和后续任务。报告是派生结果，不提交手工维护的完整覆盖表；同一次底层测试可支持多个合同 case，报告另列去重映射数。

`offset.plane-plane.first-normal-editor` 保持原目标/断言，现已通过并进入 baseline。新增 `offset.plane-plane.signed-rotated-exchange`、`offset.single-plane.signed-order` 调用实际 solver，独立计算最终几何/秩；Quantity、编译边界、模式、候选身份和编辑回填有实际 Go/TS 测试。三个 `offset.plane-plane.router-*` 复用一个真实数据库/Router/Worker fixture，覆盖 Datum/材料外法向、嵌套 occurrence、同 Part 不同 occurrence 与第二 CAD Body、跨零、表达式引用重命名/UndoRedo、不可测清值、冷读、manifest 与冻结 Release；不是三次独立或全矩阵验收。浏览器未运行。

本批锁更新经过定向核对：六个 Offset capability 的缺口/证据映射更新、11 个新 case、原编辑器 case 进入 baseline（预期与断言不变），仅双平面 UI/domain 声明提升；具体 Workspace/Worker descriptor 长度单位修正为代码真实 mm，目标符号/交换公式细化，未来 Contact descriptor 仍保留其目标单位。旧 `integration.product-history-release` 断言按既定准入合同修正：从“冲突 Preview 必须抛错/最终数值 replay 必须失败”改为检查 NotUpdated、无可晋升 token、位姿/Head 不变、冻结失败定义、未偷偷 Suppressed、失败试算持久证据及已接纳子集 replay 隔离。这暴露并修复了目标 Offset 未 Verified 却生成候选的真实缺陷；不是放宽几何期望。其他族目标、case 预期与旧数值断言不变；不提供自动 bless。

Contact 的各接触分支和多成员 Fix Together 保留目标及任务；已有命令对 `CONTACT/FIX_TOGETHER` 的明确拒绝可测试，通过拒绝断言不代表目标实现。Pair Rigid、零 Distance、Preview/null-space、TREE-03 编辑会话均不能替代这些目标或 M4。

平行 EDGE 距离反馈新增两个基线 case：`offset.axis-axis.parallel-off-origin-motion`（12 个正距离/交换/Fix 构型，独立几何、偏好及物理 rank/DOF），`offset.axis-axis.router-part-edges-history`（两个独立 Part 的真实 EDGE，Preview/提交/编辑/UndoRedo/冷读）。仅新增对应 case/断言锁，更新 axis-axis 缺口说明；现有 `integration.router-worker` 严格 build 断言升至 v9，`offset.mode.expression-suppression` 增加旧 v8 manifest 可读检查。没有修改目标公式、覆盖下限或放宽原断言。零目标探索仍停滞，明确保留该缺口，基线绿不代表零值/耦合矩阵均通过。

复现入口：`runner.py baseline --case offset.axis-axis.parallel-off-origin-motion`；真实数据库入口：`runner.py baseline --case offset.axis-axis.router-part-edges-history`（显式测试库与当前 Worker）。新通过记录在 `build/edge-offset-verified-{baseline,gaps}/`，受影响合同记录在 `build/edge-offset-affected-baseline/`；初次 Worker 重链接期间的启动失败记录独立保留在 `build/edge-offset-baseline/`。执行前先等待 Worker 构建完成，不与重链接并行启动集成。应用数据库仅作只读诊断，不重置数据。
