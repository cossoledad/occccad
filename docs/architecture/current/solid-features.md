# 实体 Feature

返回[Part/Sketch](part-sketch.md)。这里记录实现合同，测试执行结果与人工验收必须另行区分。

## 定义与求值

实体生成、跨 Body 组合、局部修改复用 Part Feature、参数、ChangeSet、CAS、补偿历史和每 Body 的 BREP/Visual/Naming 制品。它们采用不同的 OCCT 算法，不把局部修改伪装成草图拉伸。

- 拉伸：有限长度、双侧独立长度、对称总长、贯穿切除。贯穿根据当前 Feature 上游输入的包围范围计算，不持久化一个任意的大长度。
- 旋转：稳定轴引用、角度、反向及真实扫掠 Naming。长度、第二长度和角度使用既有 Quantity/表达式参数。
- Boolean：同一 Part 的并、差、交；并/差允许最多 32 个不同工具 Body，交集只允许一个。工具定义保存 `bodyId + featureId`，只读取该明确阶段。编辑可更换目标和工具。工具 BREP 和 Naming 必须属于同一个不可变快照。
- 圆角：常量半径与持久边选择。倒角：等距距离与持久边选择。
- 拔模：持久面选择、基准中性平面、恒角及反向。抽壳：移除所选面、恒定厚度、向内或向外。向内结果必须减少材料并完全包含在原实体内，防止过大厚度越过对侧壁。
- 放样：2–32 个有序闭合草图截面，直纹或平滑，逐截面方向和起始边；圆支持闭合点角度。当前要求单区域、无孔、各截面边数相同。创建时选取截面定义中首个有效边并将其 stable ID 保存为闭合起点；缺失的已保存起点必须失败，不能随求值排序改选。使用既有显式基准平面框架建立偏置截面。

`Body` 身份不随材料连通性改变。Boolean 默认消耗工具的最终输出，`keepTools` 可保留它；定义、制品和历史始终保留。派生 `consumed` 与 `visible` 独立，视口/拾取、Product 展开和 STEP 导出遵循输出状态。抑制 Boolean 恢复工具输出。

## 阶段、命名与历史

依赖连接上游 Feature 输出，而不连接 Body 的未来最终结果。局部修改先重建自身之前的 Body 前缀，再以保存的 source Revision、Body 和 PersistentSelection 解析当前输入。缺失、歧义或类型变化必须失败，不能用最近几何或持久化 local ID 恢复。显示层的 local pick 只用于创建正式持久选择。

生成历史、Boolean、same-domain unify 和局部修改的真实 Generated/Modified/Deleted/存活关系形成同一命名链。抽壳同时保留外面并生成内面时保留两类来源；多对多来源不能生成重复 semantic ref。未被同类型历史覆盖的边/点继续使用已有语义邻接闭包，完整门禁覆盖每个 Face、Edge、Vertex。evaluator/policy 已提升到 v5。

Feature 定义编辑保持稳定 ID、顺序和名称；生成和局部修改保持所属 Body。参数 Source 的更新与新增/移除和定义共用一次 ChangeSet。抑制是显式定义字段；失败状态与诊断为派生值，不进入可补偿的 Feature 定义。

## 失败和预览

创建失败与预览失败原子拒绝。已有 Feature、参数或草图编辑引起的可识别实体求值失败可以提交 FAILED Revision：失败 Feature 为 FAILED，依赖后代为 BLOCKED，受影响 Body 不携带旧成功 GeometryKey。参数/定义修复或 Undo 重新求值；Redo 可恢复失败定义，显示元数据编辑也不能把 FAILED Revision 标为成功。预览不推进 Head，变更输入或 Revision 后不能采用旧候选。

结构或参数验证错误仍拒绝候选；草图支撑和外部几何保留其既有失败合同。这里没有把所有数据库/网络错误归为可持久化的模型失败。

## 验证入口与限制

- `kernel/occt/tests/geometry_exchange_scenarios.cpp`：Boolean 精确体积与冷重算、拉伸范围、旋转 Naming、局部修改、放样后切除/倒角，检查完整拓扑覆盖。
- `services/internal/workspace/solid_feature_test.go`：阶段合法性、参数生命周期、定义编辑和补偿。
- `services/internal/control/solid_feature_integration_test.go`：真实 Router/Worker、预览、三条建模链、上游修改、失败恢复与保存重开。
- `web/apps/cad/browser/solid-feature-live.spec.ts`：真实 API/Router/Worker 下的布尔面板预览、提交、工具消耗、精确体积和页面重开；通过 `playwright.live.config.ts` 启动 API 模式前端，需运行应用及配置登录。
- `web/apps/cad/browser/solid-feature.spec.ts`：真实浏览器中的面板入口、未完成定义禁用提交、取消。Mock 浏览器用例不证明精确几何或真实后端拾取。

未包含多工具交集、跨文档 Boolean、可变半径、非等距倒角、复杂拔模、带导轨/带孔/不同边数放样。偏置基准面目前保存明确框架，尚无关联偏置参数编辑。三条完整建模链的逐步鼠标拾取、全部参数组合和复杂实体的 WebGL 人工验收仍需独立证据。
