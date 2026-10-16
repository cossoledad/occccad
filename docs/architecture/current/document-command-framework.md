# 文档接入与命令注册

> 返回[当前架构目录](README.md)。本页解释当前实现，不声明插件市场、运行时代码替换或新的建模算法。

## 文档公共流程

`services/internal/workspace/document_registry.go` 注册 Part/Product 的 `DocumentAdapter`。公共 Service 管理创建/复制、ACL、显式打开、Workspace、不可变 Revision、幂等事务、CAS、历史及 Outbox。普通读取不会打开标签。文档类型未注册、接口不完整或重复 ID 会被拒绝。

适配器分别提供初始化、模型校验、参数/依赖准备、命令适配、求值、Preview、History 的属性值读写与求值、领域投影和结构树、额外关系持久化。Part 保留 Body/Feature/Sketch、Naming 与增量 evaluator；Product 保留实例、约束、引用解析与装配 solver。原有 transport command 经适配器进入现有 typed handler；新领域操作继续注册到 `modelcore.Registry`。公共服务不创建另一套参数、历史或缓存。

前端 `cad/document/document-registry.ts` 提供文档投影校验、工作 Body 默认值及结构树接入。编辑目标来自既有 `EditSession`：宿主 Document 决定打开标签和场景；target Document 与完整 InstancePath 决定命令目标；working Body 和 active Sketch 决定域内输入。Product 中编辑 Part/Sketch 仍保留 Product 宿主。PINNED、Revision 与 activation generation 校验沿用现有上下文编辑合同。

当前默认类型仍为 Part/Product。接入新文档领域需要注册完整适配器、领域模型/typed handlers，并满足持久模型合同；配置不把任意字符串变成可保存的文档类型。

## 唯一呈现配置

`services/internal/workbenchconfig/catalog.json` 是文档、Tab、命令组、命令名称/说明、图标与显示条件的唯一来源。后端嵌入并严格解析，通过 `GET /api/ui/toolbars` 返回配置与派生 Toolbar 投影；Mock 与本地 SVG fallback 引用同一文件。修改配置后需重新构建，不提供热替换。

关系为 `documents → adapter`、`contexts.when → tabs`、`placements.tabId → groupId`、`groups.commands → commandId`。命令定义只存一次；下拉命令族名称由组的 `variants` 配置提供；命令在组内的位置、组在 Tab 内的位置各自拥有顺序。同一命令/组可以出现在多个位置。所有引用使用稳定 ID。配置校验拒绝重复 ID、缺失引用、未知条件、错误字段和不安全图标；前端在发布命令区前检查每个声明都有实际实现。

图标支持配置内的白名单 SVG 基元或 `/assets/` 本地资源，不执行脚本；前端无需增加按图标名分派的代码。未知图标和资源加载失败使用统一占位。数据库迁移只退役旧 `ui_toolbars`/`ui_toolbar_items` 应用种子表；文档、Revision 和用户状态不参与目录加载，布局/工具变体偏好仍由现有浏览器偏好管理。

条件表达式支持 `fact/equals`、`all`、`any`、`not`。事实来自现有状态：hostType、targetType、sketchActive、canEdit、rootCanEdit、busy、selectionKind、selectionCount、hasWorkingBody、moveReceiptPending、isMock、rootTarget、motionActive、canDeleteMotionResult。每次状态改变在前端同步计算，不向后端请求选择变化。显示、可执行及活动态分别投影；隐藏入口不会禁止合法快捷键调用，执行仍检查条件、实现 capability 与后端 ACL。

Toolbar Tab 按文档会话及工作台保存当前分类，唯一 `WorkbenchCommands` 使用 `useWorkbenchTab` 读取选择。普通分类切换只改变呈现。根 Product 的机构入口及 Mechanism 树双击进入显式应用编辑会话，catalog 的互斥 contexts 根据 `motionActive` 显示 KINEMATICS_DMU 分类并隐藏装配分类；切换该应用内部的视图/文档分类不退出机构。“机构”入口编辑明确选中的机构，否则通过 SAVE_KINEMATICS 新建；退出清理本会话没有确认操作的空机构，已有机构保留。进入/退出从当前已确认位置按目标应用的关系权威求解，完整显示帧交接，见[机构合同](kinematics-dmu.md)。嵌套 Product 编辑使用原装配分类，不提供宿主机构入口。后台读取和刷新保持既有打开标签顺序。

## 实现注册与操作生命周期

`cad/command/command-registry.ts` 接受普通 `CadCommand` 对象与批量注册，无继承要求。声明的 `implementation` 为 handler/tool/form；交互工具继续使用 ToolManager/Input，参数面板继续使用现有领域表单与 Preview 会话。`features/workbench/workbench-command-registration.ts` 汇集已迁移实现，并通过 ref 读取当前上下文，不在每次选择改变时重新注册。

Toolbar、搜索、快捷键和结构树菜单经同一个 `execute` 入口；菜单节点继续携带 owner Document、InstancePath、Revision 和 capability。结构树执行前校验当前节点身份，不能按名字/数组位置推断目标。主工作台仍负责既有领域面板和查询编排，新增独立命令不需要修改它的条件分支。

每次执行获得 `CommandOperation`，提供 AbortSignal、current 和 `own(dispose)`。交互命令切换、目标上下文改变和模块释放会取消旧操作，释放资源；迟到的异步显示结果必须检查 operation 或原有领域 generation。普通视图 handler 不打断正在运行的建模工具。工具转入装配约束定义时，使用既有 pending/editing 状态保留操作，面板结束后才清理。ToolManager 与领域表单保留自己的 Esc/多步骤/连续执行规则，不新增全局 Esc 监听。

模型工具／表单执行前通过 `CommandOperation.prepare` 等待视口的 AssemblyInteraction 结束屏障。准备期间，旧工具清理产生的 select 或尚未打开的表单不触发自动完成；显式取消、上下文切换及模块释放仍可立即取消。等待结束后复核 operation 代际再启动当前实现，不将旧手势预览作为下一命令的基线。

机构定义、仿真对话框及转换计划使用同一 `CommandDialog` / form `CommandOperation`。Mechanism 树双击进入应用，其他可编辑定义转入已有 `tree.edit` form。“运行仿真”和模拟下指定结果的树双击打开同一个对话框，计算、停止后台任务、播放/暂停、单步和时间定位是对话框局部控件，不是 Registry 命令或 Ribbon/Search 项。局部操作不能取消持有该对话框的 CommandOperation；普通视图 handler 仍可执行。

视口不增加常驻机构活动区域；提示使用底栏 WorkbenchStatus，诊断走统一 OperationFeedback。旋转接合按轴线重合和轴向定位显示支撑来源、替换/清除、偏移与平面方向；自动选择/替换字段复用 FeatureSelectionSession，精确 inspect 后绑定，树和视口没有额外鼠标控制器。命令取消与后台 Job/冻结结果加载分别使用代际门禁。个人模拟结果的删除按终态 Job capability 与任务所有者 ACL 执行，canDeleteMotionResult 独立于 Product 编辑权限；当前 Product 只读时仍可删除自己的终态结果。命令对话框统一“确定／取消”页脚。回放确定通过 SAVE_KINEMATICS 采用当前时间对应的合格计算帧，失败保留对话框；取消、标题关闭和 Esc 恢复机构 accepted，后台完成不重开显示。帧转换从工具栏进入，是回放子流程；CadCommand.continues 显式声明 dmu.apply 可续接 dmu.run / dmu.apply，Registry 保留同一 CommandOperation，初始进入不执行取消清理。取消计划返回原仿真及原帧，转换成功回装配设计。机构约束与产品约束继续独立，确认不先释放到 Product 初始位置。

模型修改仍由现有正式命令提交；Preview 不推进 Head，确认提交当前成功候选，失败、过期、取消继续受服务端 gate 约束。表单 adapters 复用各自既有参数、选择过滤、预览/提交/关闭实现，公共注册层不会强迫拉伸、草图约束和适应视图使用相同步骤。

## 最小扩展示例

现有 `commands/selection-summary.ts` 是不写模型的独立工具。增加声明并在目标组添加位置：

```json
{"id":"example.summary","name":"选择摘要","helpText":"显示当前编辑目标和选择数量。","iconKey":"select","repeatable":false,"implementation":"handler","visibleWhen":{},"enabledWhen":{}}
```

```json
{"commandId":"example.summary","groupKey":"primary","order":200}
```

实现只需通过注册入口接入：

```ts
registry.register({
  id: "example.summary",
  execute: () => show(context().documentId + " · " + context().selectionCount),
});
```

新组/Tab 用配置的 `placements`/`contexts` 接入，不复制命令定义。新图标加入 `icons` 字典。注册失败是整体错误，不默默忽略缺失实现。`command-registration.scenario.mjs` 在独立 registry 中增加配置和实现，实际验证其出现在指定位置并执行，且不修改主工作台、工具栏或服务分派。

## 验证入口与边界

Go 配置/注册测试验证失败封闭、已有命令保留及无需数据库/Worker 的目录读取；迁移测试验证两种数据库的历史保留。Node 场景覆盖 Part/Product/Sketch 事实、fit/coincident/pad、完整默认注册、扩展、取消和迟到结果，以及原有菜单、标签、Preview 与装配路径。真实链路验证仍使用现有 Router/C++ Worker、数据库和制品接口。

这些测试不替代浏览器/WebGL 操作验收，不表示所有装配组合或工业规模验收。实际执行结果、既有失败与未运行项由当次交付报告列出。

装配创建/编辑表单由 `workbench/assembly-constraint-fields.tsx` 拥有呈现和表单字段；工作台保留查询与命令编排。`AssemblyDialogLifecycle.bindCallbacks` 同时绑定两条实际提交路径，按对话框寿命处理成功/失败；自身提交触发的权威缓存刷新不会关闭回调资格，关闭或切换上下文后的迟到回调不影响新会话。相邻场景通过实际 MutationObserver 时序验证。
