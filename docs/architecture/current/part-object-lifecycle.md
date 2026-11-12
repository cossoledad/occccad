# Part 对象、容器与删除

Part 的 Body、几何图形集、Feature、用户参考面/线/点共用对象操作入口。结构树表达定义及状态，视图区表达当前有效结果；树中的输入引用不是另一份定义。图形属性见[对象属性](object-scene-properties.md)，几何能力见[线框与曲面](wireframe-surfaces.md)。

源码入口：Workspace `part_object_lifecycle.go`、`part_containers.go`、`object_graphics.go`；Web `object-operations.ts`、`part-delete-dialog.tsx`、`part-container-dialog.tsx`、`command-selection-seed.ts`。测试入口为对应 Workspace 单测、`part_objects_integration_test.go` 和 `part-objects-live.spec.ts`，执行结果由当次报告确认。

## 工作集合与通用操作

- 编辑会话分别保存工作 Body 和工作几何图形集，不改变模型历史。集合可在树的对象菜单中设为工作集合，以名称下划线标识；新的独立几何、参考元素和曲面工作区草图进入该集合。集合被删除后，优先使用仍存在的祖先，再使用剩余集合。
- “移入集合…”可组织独立 Feature、草图及用户参考面/线/点，也可调整集合父子关系；“解散集合”保留内容并移到父集合或指定集合。检查循环，禁止把材料 Feature 移出 Body。这些操作不重算 B-Rep、不改变对象身份；草图移出 Body 后仍可被原实体引用。
- 命令启动前冻结选集，按命令的合法输入角色依序带入。顺序点列、截面和多元素字段可带入多选，单输入字段只取一个兼容项。使用与交互拾取相同的 Document/Revision/occurrence 及类型过滤；编辑现有定义不覆盖原输入。取消或上下文变化使尚未消费的选集失效。无合法预选时保持空字段，不创建示例几何。
- 显隐、属性、定位、改名、移动和删除沿同一 typed object target 与能力投影路由；对象类型决定适用字段。标准基准面仍受保护，起源轴系和各类子节点不因此自动获得删除能力。

## 删除影响与失效定义

可删除任意位置的 Feature，以及非空 Body、几何图形集、用户参考面/线/点。删除前读取当前 Revision 的依赖图，展示删除对象、保留并更新对象及受影响接口；参数随所属对象一并清理。Body/集合删除包含其定义成员及子集合，不把树中的输入引用视为独立删除目标。

默认保留下游定义。显式输入消失时直接消费者为 `FAILED`，其依赖为 `BLOCKED`；删除材料步骤后的隐式材料顺序可以重新求值，实际失败仍由 evaluator 诊断。不会拿旧成功结果冒充当前精确结果，也不会按邻近形状或显示名称改绑。用户可以编辑现有定义重选合法上游输入；修复成功后恢复求值。

删除对话框提供“同时删除依赖特征”和“清理独占的草图与参考几何”。前者按依赖图展开；后者保留仍被其他特征（包括抑制中的定义）、基准、公式、测量或接口使用的输入。发布和 ContextInput 定义保留并报告目标缺失，不向其他文档执行破坏性级联。受影响列表按名称筛选、分页并可定位。

`POST /api/documents/{id}/delete-plan` 是只读 Editor 接口，要求当前 `versionId`；返回选项、影响列表和规范计划 digest。确认经普通 `DELETE_NODES` Domain Command，核对 Revision、选项和 digest 后在一个 ChangeSet/CAS 中执行。计划不调用几何 Worker，不建立 Revision。直接 typed 删除命令也进入同一个删除引擎。

尚被幸存定义引用的已删除对象保存为 `part.retired-references` 中的 typed、不可用描述；它只保留缺失来源的身份、名称和验证所需定义，不能作为活动几何、参数值或 Worker 输入。未知身份仍非法。参数引用保留类型而无旧数值，公式明确阻塞；修复后清理无引用描述，并记录补偿变化。Undo/Redo 恢复原对象、参数及容器身份，不抹除历史。

删除创建新 Body 的 Feature 时，若该 Body 仍有定义则清除创建者关联并保留 Body；空的 Feature 所有 Body 一同删除。删除最后一个 Body 后允许 Part 无 Body，不补造一个伪实体。

## 边界

本流程覆盖 Part 内对象；不删除跨文档消费者，也不承诺自动修复其更新。显式拓扑/阶段引用失效必须按 Naming 解析或由用户重选，不能把“材料顺序可拼接”扩大为任意拓扑可恢复。独占输入清理以现有正式依赖为准，不推断未建模的设计意图。工业长历史、超大删除闭包和多用户并发仍需扩展样本，定向验收不构成容量保证。
