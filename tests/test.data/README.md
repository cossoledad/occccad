# 测试数据

本目录统一保存仓库内文件型测试输入、回归语料和人工验收记录。测试实现、执行入口和生产能力合同仍留在所属模块；运行产生的日志、诊断归档及临时制品不作为静态 fixture 搬入这里。

- 根目录五份 STEP：目标建模验证集与交换回归共用输入，配套 `modeling-assembly-*.md` 与 `analysis-assets/`。
- `assembly-screenshots/`：原根目录 `images/` 中的五张装配实机截图。
- 根目录 `*.3dreplay`：不依赖数据库或 BREP 的数值内核重放；`windmill-cylinder-reference.3dreplay` 对应圆柱同轴回归与同名诊断报告；`assembly-antipodal-holes.3dreplay` 与 `assembly-contact-point-preference.3dreplay` 保留实际反向平面/双孔同轴和精确接触/点重合的全部 double 输入及原失败结果，由装配 native 与真实 Router 回归覆盖。
- `assembly-notupdated-product.3dreplay`：真实 Product 的全部约束/原始状态、当前数值表及关联失败请求，供生产 Worker 无数据库离线测试；模式与预算见 [回放工具](../../services/cmd/occccad-3dreplay/README.md)，实测数据见 [输入性能记录](assembly-input-performance.md)。
- [装配求解器性能记录](assembly-solver-performance.md)：第一轮优化构建的 Native/Router/Session 基线，以及一键命令的 Release 内核复测、历史对比和证据限制。
- `sketch/`：草图求解回归输入。
- `assembly-contract/`：测试组合索引、证据映射和回归锁；生产语义仍来自 `services/internal/assemblycontract/catalog.json`。

新增文件型测试输入放在本目录，按用途建立子目录，并同步所属测试的加载路径。已有验收记录区分已验证结论与未验证项，不代表完整产品验收。
