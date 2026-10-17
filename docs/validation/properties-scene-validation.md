# 属性、外观与场景验收

日期：2026-10-17；基线 `02cade78c08fa403ef1a6a1432ee22cc75981cb3`，本次工作树。当前行为见[统一属性](../architecture/current/object-scene-properties.md)和[外观](../architecture/current/surface-appearance.md)，本页只记录实际执行及限制。

使用根 `.env` 的 SQLite / LOCAL 制品、Release Worker 和已有 Chromium（软件 WebGL），没有新建测试数据库或下载浏览器。实施中按授权停止旧 API/Jobs/Worker，清理当前 `services/data/occccad-local.db` 的开发表及 `services/data` 中开发制品/暂存，再迁移并重启最新运行；没有清理其他数据库、S3 桶或无关目录。后续验收使用独立文档，运行日志和截图在 `build/`。

## 已执行证据

| 范围 | 结果与边界 |
|---|---|
| 原生 XDE | `invoke check --scope geometry --match 'GeometryExchange.Xde'`：11 个用例通过，含整体/分面/共享实例 RGBA、完全透明及重复往返；`build/agent-check/20261017T021428-0e37268b/report.json` |
| 真实交换链路 | Jobs `TestXdeImportSharedDefinitionsAndRoundTrip` 通过：4 个共享定义、201 个叶实例、颜色/透明度导入，单个嵌套实例名称与分面覆盖，真实 STEP 写出/读取及再导入；`build/agent-check/properties-exchange-integration.log` |
| 属性与缩略图 | 定向 `TestObjectProperties* / TestFaceInherit* / TestThumbnailTransparency*` 通过，含元数据同事务提交、预览无写入、两步撤销/重做、幂等与过期版本、替换/复制所有权、稳定面继承遮罩和透明合成；`build/agent-check/properties-go-final.log` |
| Go 全包 | `go -C services test -json -count=1 -p 1 ./...`：993 个 test/subtest verdict 通过、9 个跳过；唯一失败族是下述 Shell 包装用例；不能称全包通过。日志 `build/agent-check/properties-go-all.jsonl` |
| Go conformance | `go -C tests/go test -json -count=1 -p 1 ./...`：4 个用例通过；`build/agent-check/properties-conformance.jsonl` |
| Web 全套 | `invoke check --scope web`：111/111 场景及生产构建通过，报告 `build/agent-check/20261017T025615-94449805/report.json`。随后仅对齐属性框的系统色样，补跑 `surface-appearance`、类型检查/生产构建与 3 个 Mock 用例通过；`properties-final-swatch.log`、`properties-final-build.log`、`properties-mock-final.log` |
| Mock 浏览器 | `body-color.spec.ts`、`properties.spec.ts` 共 3 个用例通过，覆盖右键/Alt+Enter、命名、颜色/历史、场景确定/取消/刷新持久化；`build/agent-check/properties-mock-final.log` |
| 真实浏览器 | 最新运行上的 `properties-live.spec.ts` 通过：真实建模、Body/面赋色和透明度、几何 Key 不变、服务端预览取消、撤销/重做、并发 CAS 拒绝与草稿恢复、自动负 Z 地面/世界地面、光照取消不产生 Revision；`build/agent-check/properties-live-latest-runtime.log` |
| 环境与文档 | `invoke doctor --scope services,web --browser --json` 通过；文档审计、8 个审计测试、`context-audit` 与差异空白检查通过，原始输出在 `build/agent-check/properties-*` |

真实浏览器截图：`build/acceptance/properties-real-face.png`、`properties-real-scene.png`。这证明本次软件 WebGL 下的具体交互，不代表硬件 GPU、规模或帧率验收。场景设置的本地持久化由 Mock UI 测试覆盖，真实浏览器另外覆盖权威模型底部和 Revision 边界。

## 全量检查的失败与跳过

`invoke check --scope all` 已运行：开发工具 53 个测试和 C++ 构建通过，CTest 400 个场景中 398 个通过、2 个失败，后续步骤因此未执行。报告：`build/agent-check/20261017T024931-2f2e845c/report.json`。之后分别补跑上述 Go、conformance 和 Web，不将 NOT_RUN 当作通过。

失败场景为 `GeometryExchange.ShellAfterLargeModifiersAndMixedLoft`、`GeometryExchange.ShellOffsetRectangleToLargerCircle`，诊断含 `SHELL_OFFSET_TOLERANCE_EXCEEDED`。在 `build/agent-check/properties-baseline` 的未修改 HEAD 检出中，使用相同 Release/Conan 工具链独立构建并运行这两个场景，复现相同失败；证据：`properties-baseline.log`、`properties-baseline-shell.log`。Go `TestCppWorkerShellAfterFilletAndMixedLoft/offset-rectangle-larger-circle` 复现同一原生失败。本次没有改 Shell 算法、断言或容差。临时基线检出已清理，基线构建及原始失败日志保留在 `build/agent-check/`。

9 个 Go 跳过项涉及 S3、PostgreSQL、性能、外部大 STEP/传输健康及缩略图图库的可选条件；本次没有把它们纳入通过声明。当前 SQLite/LOCAL 路径有实际验证，PostgreSQL/S3 仍需配置对应环境单独执行。

Web 增量显示/选择用例使用部分 Engine fixture；场景地面接入后，为这些 fixture 补齐空 `refreshSceneGround`，原断言保留。装配合同锁只更新已定向执行通过的 `composition.constraint-display.incremental-ui` 源摘要，数学预期、容差和覆盖下限未变化。

浏览器仍输出原有 Ant Design 弃用提示和程序环境 PMREM 采样提示；本次浏览器用例没有未捕获 pageerror。复杂透明曲面的常规排序伪影、物理折射/透明阴影、图片材质和共享场景保存均不在当前交付契约内。
