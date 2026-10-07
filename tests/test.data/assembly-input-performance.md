# 装配输入与失败快照：定向性能记录

2026-10-07，在 Linux x86_64、Intel Core i7-4900MQ 上测量。Go 基线为 `4c5fcc6`，C++ 为同一基线求解逻辑加同口径计时/查找计数；本轮代码使用 GCC 15.2.0、Eigen 3.4.0、Debug 构建。输入、硬约束/偏好阶段、迭代数和最终残差一致。数值算法没有大范围更换。

## 输入与方法

- Go：32 刚体、32 点、31 重合约束，每个旧几何 ID 含 256 次引用文本；冻结另含 warm start。ReferenceKey 单独使用 16 层、完整 document/revision/instance 字段的 occurrence 路径。属于压力输入，不能将缩小比例推广到所有日常模型。
- Go benchmark 每项 3 次、每次 300 ms，表中为各指标中位数。RPC 包括共享编译和 protobuf 序列化；Replay 计完整数值 JSON 生成；Freeze 包括校验、所有权复制、私有 ID 编译和摘要；Digest 单独计摘要。基线 ReferenceKey 提取原 resolver 的逐字段 JSON/拼接代码，未改变行为。
- C++：5/15/30 刚体 plane-chain，每个案例连续 3 次。总耗时是三次平均，阶段计时/查找量为最后一次；RSS 是该进程截至该案例的累计峰值。分别使用正常短 ID 和 8192 字符前缀 ID。基线仅加测量，没有改数值步骤。
- RSS 来自实际 benchmark 二进制的 `/usr/bin/time -v` / `getrusage`，不含 Go 编译器。Go bytes/op、allocs/op 是 Go allocator 指标；未单独测 C++ allocator 次数，不宣称其所有分配归零。计数为端点查询处理的字符串字节，不是所有字符串操作总量。
- 数据是未压缩 RPC/JSON。运行日志保留在忽略提交的 `build/performance/assembly-input/`。最终采样串行运行；CPU 频率/后台负载未固定，三次样本不构成工业容量或稳定延迟结论。

## Go 结果

| 操作 | ms/op：前 → 后 | bytes/op：前 → 后 | allocs/op：前 → 后 |
|---|---:|---:|---:|
| Freeze | 22.697 → 1.676 | 5,896,476 → 565,327 | 7,001 → 9,165 |
| Digest | 13.196 → 1.491 | 3,689,504 → 452,019 | 6,501 → 8,615 |
| ReferenceKey | 0.012 → 0.007 | 8,207 → 1,856 | 3 → 2 |
| RPC | 0.436 → 0.150 | 768,251 → 64,106 | 401 → 442 |
| Replay | 1.764 → 0.515 | 499,350 → 139,897 | 2,853 → 2,317 |

RPC **726,855 → 4,246 B**；数值重放 JSON **17,224 → 11,151 B**。冻结分配字节减少约 90%，摘要约 88%，RPC 约 92%，重放约 72%。冻结/摘要仍需完整内容校验，精确整数字段规范化增加了小对象分配；冻结、摘要和 RPC 的**分配次数增加**，不能说所有分配优化完成。

Workspace benchmark 进程峰值 RSS **32,552 → 28,160 KiB**；Geometry benchmark **33,764 → 25,736 KiB**。这是固定测试进程，未测常驻 API/Worker 的生产峰值或完整大 Product 导出内存。

同一 CAD 构型的真实 PostgreSQL Product（两 occurrence、Fix、已接受 Offset、冲突的 NotUpdated Offset），完整导出 **27,628 → 12,106 B**，减少约 56%。两次生成使用新文档/Revision ID，构型相同而非 UUID/时间戳逐字节相同；新版还包含一个实际失败请求。默认文件无完整 Part 历史、无重复 manifest/request 数值来源。当前样本为 [assembly-notupdated-product.3dreplay](assembly-notupdated-product.3dreplay)。

## C++ 结果

8192 字符前缀输入：

| 刚体数 | 总 ms/op：前 → 后 | Residual ms：前 → 后 | Jacobian ms：前 → 后 | Compile ms：前 → 后 | 峰值 KiB：前 → 后 |
|---|---:|---:|---:|---:|---:|
| 5 | 29.714 → 39.833 | 4.590 → 5.866 | 5.734 → 7.763 | 0.262 → 0.409 | 7,168 → 7,296 |
| 15 | 573.184 → 594.852 | 25.895 → 17.368 | 51.432 → 46.156 | 0.825 → 1.169 | 8,568 → 8,704 |
| 30 | 8,543.175 → 7,761.258 | 49.125 → 38.475 | 183.465 → 174.007 | 2.033 → 2.383 | 11,528 → 11,824 |

对应 Residual/Jacobian 评估次数仍为 **23/10、25/11、27/12**。热循环端点字符串查询 **264 / 1,008 / 2,262 → 0**；查询字符串处理字节 **2,165,856 / 8,269,956 / 18,558,969 → 0**。这是一次绑定索引的直接证据。

正常短 ID 的总 ms/op：**31.192 / 469.046 / 8,465.924 → 42.185 / 591.696 / 7,470.038**；30 刚体 Residual **45.417 → 38.231 ms**、Jacobian **192.843 → 174.084 ms**、Compile **2.556 → 1.745 ms**，峰值 RSS **9,520 → 9,680 KiB**。

小规模总耗时没有改善，新增绑定与诊断计量有成本；长 ID 的编译阶段和 C++ 峰值 RSS也增加。dense 分解仍占大部分总成本。本轮证明 payload、重复字符串工作和 Go 分配字节减少，不承诺普遍求解加速、C++ 总内存下降或 60 Hz 拖拽。

## 验证与复现

实际完成：C++ assembly / assembly-corpus 全部 **230 项**；Go 的 workspace、geometry、valuecopy、control、API 定向测试及 `go build ./cmd/...`；生产 Worker 离线测试；真实 PostgreSQL → Router → Worker 的嵌套来源、几何缺失、组内失败/多阶段回放、移动、预览提交、NotUpdated 保存重开、模式/抑制、第三轴、组约束和 Undo/Redo；既有 windmill、反向孔轴与接触偏好回放。离线测试另覆盖明确固定构型矛盾、超时分类、矩阵/轨迹截断、字段/大整数往返、输入不变和派生文件不覆盖。

```sh
# services/
go test ./internal/workspace ./internal/geometry -run '^$' \
  -bench BenchmarkAssembly -benchmem -count=3 -benchtime=300ms

# 仓库根目录：既有基线入口及长 ID 压力输入
invoke performance-baseline --count 3
build/cmake/debug/kernel/assembly/tests/occcad_assembly_solver_benchmark --long-ids
ctest --test-dir build/cmake/debug -R '^(assembly|assembly-corpus)/' --output-on-failure
```

离线文件加载、模式/初值/预算/分支调整及 API/测试用法见 [回放工具](../../services/cmd/occccad-3dreplay/README.md)。未运行浏览器自动测试、无差别全仓单测；未清理用户数据。缺失或保留预算外的历史现场无法重建。矩阵、轨迹和大量默认摘要仍有采集成本；详细 trace 不是完整优化器执行轨迹或通用非线性无解证明。
