# 场景、草图与结构树验收

当前算法见[场景选项](../architecture/current/object-scene-properties.md)、[语义结构树](../architecture/current/semantic-tree.md)和[抽壳算法](../architecture/current/shell-algorithm.md)。执行范围、结果及剩余限制由同页实测记录说明；原始输出归 `build/agent-check/viewport-tree/`，不依赖开发文档身份。

## 可重建验证入口

- `scene-ground-sketch`：有限地面、草图世界范围、相机独立尺寸、编辑时隐藏与语义配色。
- `tree-node-interaction`：可编辑节点及未来节点的命令分发、焦点行快捷键、权限、上下文变化与只读引用。
- `ground-grid.spec.ts`：真实 WebGL 的有限边界、大坐标、远裁剪及上下遮挡。
- `tree-sketch-scene.spec.ts`：工作台参数双击/Enter/菜单编辑、明亮草图及退出恢复。
- 完整非浏览器检查使用 `invoke check --scope all`；浏览器与真实 API/Worker 验收单独计入。

## 执行环境与结论

2026-10-18，在当前 `.env` 的 SQLite / LOCAL ArtifactStore 上执行；原生构建为 Release，浏览器使用已安装的 Chromium 与软件 WebGL。没有重置开发数据。真实端到端用例通过 Domain Commands 创建独立样本，不依赖预存开发文档。

**整体检查尚未全绿。** 显示、草图和结构树的受影响用例及真实链路通过；扩大检查发现共享队列测试隔离问题，以及尚未更新的基准面/机构 Mock 验收合同。具体结果如下。

| 范围 | 实际结果 | 证据与边界 |
|---|---|---|
| 开发工具 Python | 53 通过 | `all-check.log` 对应报告；不包含浏览器。 |
| C++ 构建及 CTest | 构建通过，402 通过，无跳过 | 包括抽壳几何、Naming 和网格场景；不是每个复杂输入均可抽壳的保证。 |
| Go packages | 1008 通过、9 跳过、1 失败 | `TestJobLifecycleRealtimeOutboxDatabase` 争抢共享队列；见下文。 |
| Go conformance | 4 通过，无跳过 | 在全量入口因 Go 失败停止后，单独执行 `tests/go` 的 `go test -json -count=1 -p1 ./...`。 |
| Web 场景及生产构建 | 120 个场景通过，类型检查/生产构建通过 | `web-final.log`；包含有限地面、草图配色及树编辑命令框架。 |
| 场景/树/导航及相关 Mock 浏览器 | 15 通过 | `ground-grid`、`tree-sketch-scene`、`navigation`、`viewport-rendering`、`properties`、`solid-feature` 对应用例；不是生产 Worker 验收。 |
| 机构 Mock 浏览器 | 12 个唯一用例中，7 个有通过证据，5 个仍失败 | `browser-motion-verified.log` 与 `browser-joint-owner.log` 合并覆盖；不是一次完整通过的运行。包含机构进入、子关系编辑、角色选择、创建/删除及失败恢复。 |
| 基准面 Mock 浏览器 | 1 失败 | 用例仍查找旧的中心坐标字段，未完成其后断言。 |
| 真实 API / Router / Worker / Chromium | 通过 | 参数预览/提交/重开、独立草图范围、明亮草图、地面隐藏/恢复、上下文输入编辑；见下文。 |
| 运行环境 | `invoke doctor --live` 通过 | `doctor-final.log`；只证明检查项，不替代功能验收。 |
| 文档 | 8 项文档审计测试、`invoke context-audit` 和 `git diff --check` 通过 | 检查链接、知识入口与差异格式，不证明运行功能。 |

全量入口报告为 `build/agent-check/20261018T011121-f45f822e/report.json`，Web 最终检查为 `build/agent-check/20261018T013008-3126b40e/report.json`。其他原始日志、样本脚本、截图及 JSON 位于 `build/agent-check/viewport-tree/`；这些是可清理制品，不是长期模型身份。

## 真实链路验证

可重建样本为 XZ 平面的矩形拉伸，以及位于更低 Z 区域的独立草图：实体底部为 −40 mm，独立草图延伸至 −100 mm。地面低于草图底部，确认草图参与世界包围盒，而非仅依据实体。改变缩放后，地面中心和正方形世界尺寸保持不变；在绘制草图时地面与世界网格隐藏，退出后恢复。

参数从树中双击编辑拉伸长度 8 → 10 mm。预览期间 Head 不变，提交后重新打开仍为 10 mm；真实 Worker 结果体积约 4000 mm³，实体包围盒的拉伸尺寸为 10 mm。另验证焦点草图的 Enter 编辑、明亮背景下的深色轮廓及可辨网格。上下文输入经双击和 Enter 打开，名称与“必需输入”通过正式命令提交成功。上述真实用例没有页面异常。

GPU 定向测试覆盖有限平面的四角、平移到边界以外、大坐标、上下视角的深度遮挡及超出模型远裁剪范围的网格。真实截图与这些断言一起构成视觉证据；调试状态属性本身不替代像素和几何检查。

## 遗留问题与未验收范围

- **共享队列测试隔离**：Go 用例入队后再自行 `Claim`，实际运行的 Jobs 消费者先取得了任务。数据库 attempt 的 owner 与测试 owner 不同，证明此处存在测试竞争；提高优先级不能提供隔离。需要为该测试建立明确的消费者互斥/所有权边界。未修改生产抢占规则，也没有把该失败记作通过。
- **过期浏览器合同**：四项机构用例仍使用旧的“运行仿真 / 仿真与回放”流程；另一项仍断言当前已支持的“刚性接合”不存在。基准面用例仍寻找旧中心字段，Mock 也未覆盖当前权威预览。需要按当前命令目录、双面板模拟和关联基准面合同重建验收。失败后的断言没有执行，对应回放、取消及迟到结果的端到端行为仍缺本次通过证据，不能仅凭旧名称解释为产品无缺陷。
- **环境边界**：9 项 Go 跳过分别属于未配置的 S3、PostgreSQL 专用合同、可选大型 STEP、专门性能入口及可选缩略图库。没有执行 PostgreSQL/S3、工业大模型、硬件 GPU、其他浏览器或多人并发验收；小样本不能说明稳定性能分位数。
- **算法边界**：本次没有扩展抽壳适用几何。复杂曲面、一般凹曲边和向外偏置限制见[抽壳算法](../architecture/current/shell-algorithm.md#性能与限制)。地面大小与偏移公式是本产品的尺度策略，CATIA 文档只提供参考行为，不证明参数完全一致。

未完成的验证维护进入[后续工作](../../plans/README.md)，当前算法不保留开发文档编号或排错过程。
