# 实体 Feature

返回[Part/Sketch](part-sketch.md)。这里记录实现合同，测试执行结果与人工验收必须另行区分。

## 定义与求值

实体生成、跨 Body 组合、局部修改复用 Part Feature、参数、ChangeSet、CAS、补偿历史和每 Body 的 BREP/Visual/Naming 制品。它们采用不同的 OCCT 算法，不把局部修改伪装成草图拉伸。

- 拉伸：有限长度、双侧独立长度、对称总长、贯穿切除。贯穿根据当前 Feature 上游输入的包围范围计算，不持久化一个任意的大长度。
- 旋转：稳定轴引用、角度、反向及真实扫掠 Naming；轴与轮廓边界重合时，只保留最终扫掠拓扑中的实际成员，不把 OCCT 的轴上辅助点或边写为悬空输出。长度、第二长度和角度使用既有 Quantity/表达式参数。
- Boolean：同一 Part 的并、差、交；并/差允许最多 32 个不同工具 Body，交集只允许一个。工具定义保存 `bodyId + featureId`，只读取该明确阶段。编辑可更换目标和工具。工具 BREP 和 Naming 必须属于同一个不可变快照。
- 圆角：常量半径与持久边选择，按稳定语义对边集排序。凸全平面实体上，共享支撑面、被一条倒角边隔开的两条正交直边，使用原始支撑面构造恒半径圆柱及角区裁剪，允许半径超过小倒角尺寸；圆柱周期缝放在保留圆弧之外，避免斜向实体产生额外显示边。构造来源与每步 Boolean 的真实历史合成后进入同一命名门禁。其他情况使用通用 OCCT 圆角；同时构造失败时，按稳定语义顺序逐步构造并合成 OCCT 历史，已被相邻圆角消耗的边以实际删除历史识别。倒角：等距距离与持久边选择。局部修改复制上游精确几何，避免内核构造修改不可变阶段。
- 拔模：持久面选择，中性平面可为标准/基准面或同一 Part 上游 Body 的任意平面，后者保存 PersistentSelection 和 source Revision/stage，重算以精确平面来源解析；恒角及反向。抽壳：移除所选面、恒定厚度、向内或向外。凸全平面实体的向内内腔通过各保留面的精确偏置半空间交集构造，允许小倒角面在偏置后消失；其他实体走通用 OCCT 抽壳。向内结果必须减少材料并完全包含在原实体内，防止过大厚度越过对侧壁。
- 放样：2–32 个有序闭合草图截面，直纹或平滑，逐截面方向和起始边；圆支持闭合点角度。当前要求单区域、无孔；同边数截面和矩形/圆截面组合可用。不同边数使用 OCCT CompatibleWires 拆分，并保留原始草图边到生成面的真实历史。创建时选取截面定义中首个有效边并将其 stable ID 保存为闭合起点；缺失的已保存起点必须失败，不能随求值排序改选。使用既有显式基准平面框架建立偏置截面。

`Body` 身份不随材料连通性改变。Boolean 默认消耗工具的最终输出，`keepTools` 可保留它；定义、制品和历史始终保留。派生 `consumed` 与 `visible` 独立，视口/拾取、Product 展开和 STEP 导出遵循输出状态。抑制 Boolean 恢复工具输出。

## 阶段、命名与历史

依赖连接上游 Feature 输出，而不连接 Body 的未来最终结果。局部修改先重建自身之前的 Body 前缀，再以保存的 source Revision、Body 和 PersistentSelection 解析当前输入。缺失、歧义或类型变化必须失败，不能用最近几何或持久化 local ID 恢复。显示层的 local pick 只用于创建正式持久选择。

生成历史、Boolean、same-domain unify 和局部修改的真实 Generated/Modified/Deleted/存活关系形成同一命名链。抽壳同时保留外面并生成内面时保留两类来源；多对多来源不能生成重复 semantic ref。未被同类型历史覆盖的边/点继续使用已有语义邻接闭包，完整门禁覆盖每个 Face、Edge、Vertex。控制面 evaluator 为 `part-solid-generators-v19-corner-fillets`，Naming policy 继续为 v6；缓存包含版本和完整来源历史。

Feature 定义编辑保持稳定 ID、顺序和名称；生成和局部修改保持所属 Body。参数 Source 的更新与新增/移除和定义共用一次 ChangeSet。结构树可抑制/解除抑制所有建模实体 Feature，专用 `SET_FEATURE_SUPPRESSION` 命令检查 definition digest 并只改变显式 suppressed 字段，沿用 entity ChangeSet 和 Undo/Redo；导入基础实体保持受保护。失败状态与诊断为派生值，不进入可补偿的 Feature 定义。

## 用户操作与编辑上下文

所有实体 Feature 面板共用视图区输入规则。几何对象不作为 UUID 下拉选项：轮廓、旋转轴、中性平面、边/面集、布尔目标/工具、放样截面和闭合起始边均直接拾取。运算、范围、直纹/平滑是参数选项，使用分段按钮。命令可以先选对象再启动，也可以先启动再选对象。

- 面板中的输入按钮切换当前拾取角色。角色先过滤原始 hit，再按业务对象投影；轮廓选整个草图，旋转轴和闭合起始边保留草图的单条实体，边/面不能被邻近顶点抢走。所有输入限定同一所有者文档、Revision 和 occurrence。基准面/轴输入临时显示该 occurrence 的参考几何，退出恢复原显示偏好；保存选择的高亮携带已提交编辑会话的完整 InstancePath 和 contextVariantKey。
- 进入实体 Feature 命令不改变相机 fit。旋转拾轴阶段的整张轮廓只作淡色上下文，轴线选择和 hover 分别使用更明显的颜色和线宽，包含轮廓自身的直线。
- 圆角/倒角自动带入已选边，拔模/抽壳自动带入已选面。后续点击累加，再次点击取消，不要求 Ctrl 或“添加当前选择”。集合只显示计数；高亮显示真实几何。异步持久绑定顺序执行，不能因为快速连续点击丢掉输入。
- 放样截面按拾取顺序添加，支持移除、上移、逐截面反向、拾取闭合边和圆的闭合点角度；选好两个合法闭合截面立即预览。
- 输入完整或参数修改后，250 ms 合并请求既有权威预览命令；新输入中止旧请求，旧 generation/Revision 的结果不能显示或提交。仅当前输入获得成功预览后启用提交；预览失败移除旧候选，修正输入自动恢复。取消、文档/Revision/occurrence 变化结束会话。预览显示结果和上游参考线，同时仍允许继续拾取原始输入。
- 编辑局部特征时通过只读 `POST /api/documents/{id}/feature-input` 重建自身之前的 Body 阶段并解析原选择，高亮显示原边面。该接口也为阶段上的新点击绑定持久选择；它不推进 Head。新绑定使用 `sourceVersionId + sourceFeatureId` 指明不可变来源，重算仍由真实 Naming 解析，不保存局部序号。已保存的旧 Revision 选择继续按既有最终输出来源解析。
- 编辑布尔恢复目标上游阶段和已消耗工具的明确输出阶段；工具的临时可见性不修改持久 `visible/consumed`。输入阶段制品下载按文档权限、当前 Revision、Feature 和精确对象范围授权。

## 失败和预览

创建失败与预览失败原子拒绝。已有 Feature、参数或草图编辑引起的可识别实体求值失败可以提交 FAILED Revision：失败 Feature 为 FAILED，依赖后代为 BLOCKED，受影响 Body 不携带旧成功 GeometryKey。参数/定义修复或 Undo 重新求值；Redo 可恢复失败定义，显示元数据编辑也不能把 FAILED Revision 标为成功。预览不推进 Head，变更输入或 Revision 后不能采用旧候选。

失败 Body 可携带派生 `displayFallback{geometryKey,sourceVersionId}`，指向该 Body 最近成功修订的显示结果；连续失败或显示元数据编辑保持同一成功来源，恢复成功立即清除。Part/Product 视口以半透明红褐色显示旧 Mesh，树和视口提示 FAILED/BLOCKED；显示遵循 Body/occurrence 可见性。旧实体可用于相机导航，不能参与建模拾取。快照仅开放其 Visual 制品，BREP/Naming 下载、持久绑定和导出仍要求当前成功结果；fallback 不进入可补偿 Body 定义。

结构或参数验证错误仍拒绝候选；草图支撑和外部几何保留其既有失败合同。这里没有把所有数据库/网络错误归为可持久化的模型失败。

## 验证入口与限制

- `kernel/occt/tests/geometry_exchange_scenarios.cpp`：Boolean 精确体积与冷重算、拉伸范围、旋转 Naming、局部修改、放样后切除/倒角，检查完整拓扑覆盖。
- 同一内核 corpus 覆盖轴与轮廓边界重合的半周/整周旋转、长方体一条棱倒角 1 后相邻两棱半径 0.5/1/1.01/1.5/2/5 的圆角（含旋转平移、独立参考形体差集、精确半径、逐面网格覆盖和反向选择冷重算）、四条竖边倒角后的 1–8 条底边圆角、消失小倒角面的向内抽壳与冷重复历史。
- `services/internal/geometry/corner_fillet_worker_integration_test.go`：真实 Worker RPC 的倒角 1 → 双边圆角 1/1.5/2，按阶段 Naming 绑定原边，检查完整历史及 BREP/GLB 制品。
- `services/internal/workspace/solid_feature_test.go`：阶段合法性、参数生命周期、定义编辑和补偿。
- `services/internal/control/solid_feature_integration_test.go`：真实 Router/Worker、预览、三条建模链、上游修改、失败恢复与保存重开。
- `web/apps/cad/browser/solid-feature-live.spec.ts`：真实 API/Router/Worker 下的视图区布尔工具拾取/取消及消耗工具编辑恢复、倒角预选/编辑阶段拾取/失败预览恢复、矩形到圆放样、单条草图轴线旋转、自动预览、精确体积和页面重开；通过 `playwright.live.config.ts` 启动 API 模式前端，需运行应用及配置登录。
- `web/apps/cad/browser/solid-feature.spec.ts`：真实浏览器中的面板入口、未完成定义禁用提交、取消。Mock 浏览器用例不证明精确几何或真实后端拾取。
- `web/apps/cad/browser/datum-feature.spec.ts`：自由框架和参考复制预览、取消不写 Head、实体面中心、标准轴复制、属性与自定义基准树删除；精确绑定/求值仍由服务端与 Worker 测试证明。

未包含多工具交集、跨文档 Boolean、可变半径、非等距倒角、复杂拔模、带导轨/带孔/任意不同边数放样。偏置基准面目前保存明确框架，尚无关联偏置参数编辑。三条完整建模链的逐步鼠标拾取、全部参数组合和复杂实体的 WebGL 人工验收仍需独立证据。
