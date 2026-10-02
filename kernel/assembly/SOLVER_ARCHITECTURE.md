# 装配求解器边界

kernel/assembly 处理冻结刚体、body-local descriptor、编译约束、目标与 SolverProfile，不访问 Product、数据库、OCCT、Workbench 或制品。接口/运行见[README](README.md)，实际方程、图编译、Jacobian、解选择、零空间与诊断见[算法](SOLVER_ALGORITHMS.md)。

## 责任

Domain 保存公共定义、Quantity/表达式、模式/激活、稳定支持与组成员。Go 解析单位/occurrence，编排组阶段、接纳、Session、只读局部搜索、候选/CAS/Manifest。Worker 消费纯值数学请求，不查询业务真相。Web 生成累计目标、合并请求并显示 accepted frame，不修正权威姿态或猜精确支持。

[Product](../../docs/architecture/current/product-assembly.md)拥有跨模块语义，[六类合同](../../docs/architecture/current/assembly-constraints.md)拥有组合、参数、branch 和失败边界，不在本页复制矩阵或路线。

## 保证与限制

硬几何、软运动偏好、rank/瞬时自由度、branch 与诊断证据分别报告；静态 reference/nominal 与交互 driven/held 显式区分。nominal 不随 warm start 漂移，零空间试步须真实方程恢复和验收。数值 gauge 不是物理 Fix。

局部非线性求解不保证全局最近点、分支枚举或有限运动可达。普通未收敛不是 UNSAT，受限鼠标目标不是设计冲突。dense augmented QR/SVD 与 cluster/ground/component 编译控制规模；单次分解不能中途取消。稀疏/增量、flexible 与跨主机只有负载证据后再设计；工程连接暂缓，已有 Frame/Contact/组与有限运动回归不回退。

验证入口见[装配测试](../../tests/assembly-contract/README.md)：原生数学与跨层事务、历史、真实 Router/Worker 各有证据，不把历史通过复制为当前全量通过。
