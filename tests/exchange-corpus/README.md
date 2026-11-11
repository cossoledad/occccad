# NIST STEP 交换语料

本目录只保存固定来源和 SHA-256，不提交第三方 CAD 二进制。当前合同见[导入与导出](../../docs/architecture/current/exchange.md)，执行代码归 `services/cmd/occccad-jobs/exchange_corpus_integration_test.go`。

## 来源与覆盖

来源为 [NIST Download Free CAD Models, STEP Files, and Test Results](https://www.nist.gov/ctl/smart-connected-systems-division/smart-connected-manufacturing-systems-group/mbe-pmi-0)。NIST 允许使用这些测试数据；使用不代表 NIST 对本软件的认可。清单固定于 2026-11-11，更新来源包后应重新清点和校验，不能按同名文件去重。

| 发布包 | 不同 STEP 文件 |
|---|---:|
| NIST-PMI-STEP-Files | 33：AP203 geometry only 11、AP203 PMI 5、AP242 17 |
| FTC Definitions | 6 |
| CTC Definitions | 5 |
| D2MI | 5 |
| 合计 | 49 |

页面直接链接的 FTC/CTC 原生 CAD 包另有 11 个 STEP 条目，其 SHA 均已被以上 49 份覆盖；STC、HTC、MTC 原生包没有 STEP。总计 60 个 STEP 条目、49 份不同内容。`manifest.json.additionalArchives` 保存这些额外包的来源、归档 SHA 和重复条目对应关系；准备命令不重复下载没有新增 STEP 的原生包。

FTC/CTC Definitions 中若干文件与主包重名但字节不同，分别保留。FILE_SCHEMA 按文件头记录，文件名中的 e1/e2/e3 不能充当 schema 事实。solidCount 只是文本中的 MANIFOLD_SOLID_BREP 数，不是独立 CAD 阅读器的几何正确性证明。

## 准备与执行

使用当前 `.env` 的数据库、ArtifactStore、Worker 及测试资源目录，不另建测试连接。

```bash
mise run setup --component tooling
mise run exchange:prepare --json
mise run cpp:build --target occccad_geometry_worker --jobs 4
mise run check --scope services --level integration --match '^TestNistSTEPImportCorpus$' --test-jobs 1
mise run check --scope services --level integration --match '^TestReadableUnsupportedContentCreatesEmptyPartAndWarnings$|^TestExchangeExportFiltersHiddenGeometryAndOccurrences$' --test-jobs 1
```

准备命令从四个固定发布包提取选定 STEP，先验证归档 SHA，再验证文件 SHA 和长度；只按内容 SHA 写入 `<OCCCCAD_TEST_RESOURCES_DIR>/nist-step/`，不按 ZIP 路径解压。完整文件复用已有缓存；损坏文件必须重新校验/恢复。网络请求响应、取消和失败不会报告准备成功。默认目录在 build/test-resources，所有原始输出与本地 ZIP 均为可再生成资源。

回归逐文件经过真实上传 Artifact → InspectExchange → ImportExchange → 正式文档/命名命令 → Product 提交 → Job 成功报告 → 冷重新打开。检查有支持实体的文件没有静默变为空 Part，实际精确输出具备稳定 Naming；空 Body 的纯显示 Artifact 不被冒充精确拓扑。每文件耗时、结果和报告写入 `nist-step/import-results.json`。缺失语料时明确显示未适用/缺准备；直接 go test 的 skip 或退出码不能代替完整语料验收。

## 接纳边界

49 份文件都应成功创建文档；不支持内容通过持久警告说明。`nist_ftc_08_asme1_ap242-e1-tg.stp` 只有离散几何，当前策略跳过并创建空 Part，不能把该文件的成功状态称为完整精确几何支持。PMI、图层分组和其他不支持元素不创建领域对象。实体、开放结果及其颜色、结构和命名分别由邻近的解析/往返用例补充验证。

体积回归 `GeometryExchange.ExactVolumeSpans*` 使用解析圆柱、孔洞、尺度、质心和有理修剪面。可选的真实 NIST 样条交叉核对：

```bash
OCCCCAD_TEST_EXCHANGE_STEP="$PWD/build/test-resources/nist-step/ae0049980cb11cb224447e1738b499d8e1c79423782b7a8e8489a4d5316660cc.stp" \
  build/cmake/linux-gcc15-x86_64/release/kernel/occt/tests/occcad_geometry_scenarios \
  --gtest_filter=GeometryExchange.ExactVolumeSpansHandleTrimmedRationalGeometry
```

示例使用默认测试资源路径；修改过 OCCCCAD_TEST_RESOURCES_DIR 时，改用当前配置的绝对路径。另一种积分算法本身也可能提前收敛，因此解析用例以解析值断言。语料运行不替代外部 CAD、完整 AP242/PMI 标准认证，也不形成稳定 P95 或超大装配容量结论。
