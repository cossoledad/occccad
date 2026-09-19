# P9 后 Product 中心的关联设计方案

状态：设计基线，尚未代表已实现能力  
适用范围：P9 Publication/ContextReference 基线之后，P10 Assembly M3 与产品级版本发布之前

## 1. 结论

下一阶段不应继续扩展“选择任意 Document，再手工填写 PublicationId”的入口，而应把普通关联设计收敛为一个 **Product Design Session**：

- 用户打开根 Product，在同一视口中激活某个 Part/Product occurrence 并原位编辑；
- 可引用范围默认只包含该根 Product 当前冻结结构中可达、可见权限允许且合同兼容的 Publication；
- Part/Product 继续用 `PublicationId`、`InstanceId`、`InstancePath` 和 Revision 作为权威身份，但普通用户只看可读名称和路径；
- Part 声明可复用的输入端口与输出 Publication，根 Product 拥有 occurrence 之间的 `ContextBinding`；
- 接受上游变化由根 Product 生成一次完整 Update Plan，按依赖 DAG 重算 Part context variant、装配引用与求解；
- 创建 Product Version/Release 时冻结完整依赖闭包，而不是留下会随 Workspace Head 漂移的引用。

这不是把所有数据塞进 Product。Part 仍是独立可复用的参数化定义，Product 是结构、上下文连线、occurrence pose、更新决策和发布快照的协调根。

## 2. CATIA/3DEXPERIENCE 参考边界

本方案借鉴 CATIA 的工作流语义，不复制其文件格式或历史兼容负担：

- Dassault Systèmes 将 assembly context 中创建/修改零件作为正式 Product Design 能力；
- CATIA 培训把“用 Publication 管理 Product 文档之间的 contextual links”与参数驱动产品设计放在同一方法中；
- Publication 是明确的工程接口，可位于产品结构不同层级，并支撑版本更新或组件替换后的依赖协调；
- 3DEXPERIENCE 的 Reference/Instance/Occurrence 区分表明：Reference 是复用定义，Instance 是父级拥有的直接使用关系，Occurrence 是从根展开的完整路径；
- Skeleton 方法用一个 Part 发布主要参数、点、轴和曲面，再驱动周围详细零件。

参考资料：

- [CATIA Assembly Design：支持 in-context part and assembly design](https://www.3ds.com/fileadmin/Products/catia/solution-builder/content/cross_industries/pdf/CATIA%20TEAM%20PLM.pdf)
- [CATIA Product Design Expert：使用 Publication 管理 contextual links](https://www.3ds.com/assets/edu/document/course-catalog-v5-6r2018-to-v5-6r2023.pdf)
- [CATIA V5R9：Publication、替换与版本更新](https://www.3ds.com/newsroom/press-releases/ibm-and-dassault-systemes-announce-catia-version-5-release-9)
- [3DEXPERIENCE IRPC：Reference、Instance 与 Occurrence](https://3dswym.3dexperience.3ds.com/wiki/solidworks-news-info/getting-started-with-mbom-management_HOJ-oxIVRRqJF1KiTpPDHg)
- [CATIA 设计访谈：assembly context 与 skeleton methodology](https://www.3ds.com/cloud/resources/designing-impactful-innovation-podcast/ep13-designing-beyond-limits-industrial-design)

## 3. 当前 P9 基线与缺口

P9 已经正确建立以下不可推翻的底座：

- Publication 是 Revision 内的稳定实体，rename 不改变 `PublicationId`；
- ExternalParameterRef/ContextReference 冻结 resolved Revision、合同和值/几何证据；
- 来源 Head 变化只产生 `UPDATE_AVAILABLE`，接受更新形成新 Revision；
- Product Publication 可以转发子 occurrence 的 Publication；
- ContextReference 已保存根 Product、source/owning InstancePath、Context Variant 和坐标变换；
- 依赖必须是单向 DAG，共享 Part 的 occurrence-specific context 不能被隐式接受。

当前问题不是 resolver 本身，而是领域入口和所有权尚处于试点形态：

| 当前形态 | 问题 | 目标形态 |
|---|---|---|
| 从全部 Document 中选择来源 | 脱离当前产品结构，权限、版本和语义范围过宽 | 只浏览当前 Product context 中可达 occurrence |
| 手工输入 `PublicationId` | 对用户不可读，也容易把错误类型/版本拼进请求 | 通过名称、树和类型过滤选择，客户端提交稳定 ID |
| ContextReference 直接属于消费 Part | 产品特定 wiring 可能污染可复用 Part Reference | Part 声明 ContextInput，Product 拥有 ContextBinding |
| 每个 Part 各自 Update References | 不能先展示整个产品的重算顺序和失败边界 | 根 Product 生成并接受 Product Update Plan |
| Product 只冻结若干局部 snapshot | 不能单独证明一次完整产品发布可重放 | Product Version Manifest 冻结完整依赖闭包 |

## 4. 领域模型

### 4.1 四类对象

1. **Reference**：Part/Product 的可复用定义及其不可变 Revision。
2. **Instance**：某 Product Reference 直接拥有的成员边，名称只要求在该父级内唯一。
3. **Occurrence**：从当前根 Product 经 InstancePath 展开的上下文对象。
4. **Design Session**：用户当前打开的根 Product、配置、active occurrence、Workspace heads 和交互状态；它不是 Revision，也不是稳定身份。

同一个 Wheel Product Reference 可以在 ToyCar 中出现四次。四个 Instance 有不同名称和 placement；内部 Rim/Tire 的 occurrence 路径也各不相同，但复用同一套定义。

### 4.2 输出、输入与连线

把现有单一 `ContextReference` 概念拆成三层：

```text
Part/Product Reference
  Publication       typed output port, stable PublicationId

Part Reference
  ContextInput      typed input port, stable ContextInputId

Root Product Revision
  ContextBinding    source occurrence Publication -> owning occurrence ContextInput
```

`ContextInput` 不保存具体来源，只声明：

- 名称、稳定 ID 和 expected contract；
- 消费槽位：ParameterId、Datum input、Sketch ExternalGeometry input 或后续 Feature input；
- 默认值/是否必须绑定；
- isolate 后如何物化成本地值或几何；
- 对求值图的 phase 和 dependency edge。

`ContextBinding` 由最低共同 Product ancestor 拥有，至少保存：

```proto
message ContextBinding {
  string binding_id = 1;
  RelativeInstancePath owning_occurrence = 2;
  string context_input_id = 3;
  RelativeInstancePath source_occurrence = 4;
  string publication_id = 5;
  InterfaceContract expected_contract = 6;
  ReferenceSelector selector = 7;
  BindingResolutionSnapshot accepted = 8;
}
```

持久层不得以 `Body.1/Face1` 字符串解析这条边。名称只用于展示；命令接受时已经解析成 typed path 与稳定 ID。

### 4.3 Context Variant

同一个 Part Reference 在多个 occurrence 中可能收到不同输入。系统不得把其中一个 occurrence 的值写回共享 Part：

```text
ContextVariantKey = hash(
  base Part Revision,
  root Product Revision/configuration,
  owning InstancePath,
  accepted ContextBinding snapshots,
  evaluator/policy versions
)
```

- 相同 base Revision 和相同 binding digest 可以共享求值制品；
- 输入不同则产生不同的派生 variant/evaluation，不暗改 Part Reference；
- 用户需要把某个 variant 变成可独立复用零件时，执行显式 “Derive Part from Context”；
- standalone Part 编辑器只显示定义和未绑定输入，不能猜测任意 Product context。

## 5. 名称规则：面向人，不能取代身份

### 5.1 名称空间

| 名称 | 唯一范围 | 权威身份 | rename 影响 |
|---|---|---|---|
| Document name | 项目中允许重复 | DocumentId | 不影响引用 |
| Instance name | 直接 owner Product 的 sibling 集合 | InstanceId | 不改变 InstancePath canonical identity |
| Publication name | 一个 Part/Product Reference 的全部 Publication | PublicationId | 不改变消费者 |
| ContextInput name | 一个 Part Reference 的全部 ContextInput | ContextInputId | 不改变 Product binding |
| Parameter key | 一个 Part Reference | ParameterId | AST 仍绑定 ParameterId |

所有需要唯一的名称由服务端以 `trim + Unicode normalization + case fold` 生成 comparison key。显示值保留用户大小写；空白、控制字符、路径分隔符和保留名称被拒绝。最终 normalization profile 必须版本化并进入 schema/conformance tests。

### 5.2 默认分配

- Instance：`<ReferenceName>.1`、`<ReferenceName>.2`，延续当前已实现规则；
- Face/Edge/Vertex target：`Face1`、`Edge1`、`Point1`；
- Datum/Feature output：`Plane1`、`Axis1`、`Frame1`、`Body1`；
- Parameter：优先采用参数 key，例如 `overall_width`，冲突时追加序号；
- 转发 Publication：优先继承子 Publication 名称，冲突时追加序号；
- ContextInput：按目标槽位给出 `WidthInput`、`MountPlaneInput` 一类候选名，仍允许直接编辑。

序号由所属 Reference 的分配状态产生，在同一历史分支中不因删除而回收，避免新对象在审计和评论中冒充旧 `Face1`。复制/派生 Reference 时重新建立自己的名称空间。

### 5.3 名称解析规则

- Web 的树、搜索、表达式提示和 picker 展示名称；诊断区可复制稳定 ID；
- picker 返回完整 typed selection，普通表单不再暴露 `DocumentId`/`PublicationId` 输入框；
- rename 命令按 ID 指向对象，并在 owner 当前候选模型中验证新名称唯一；
- 服务端 API 可以提供 name-to-ID query 供自动化使用，但 query 结果必须带 root Revision/context digest，后续持久命令仍提交 ID；
- 历史名称只用于审计和搜索提示，不作为隐式 fallback，避免 rename 后产生多义解析。

## 6. Product Design Session 与 UX

### 6.1 默认入口

- “新建设计”默认创建 Product；用户在 Product 树中创建/插入 Part 或子 Product；
- standalone Part 仍用于标准件、供应商件、模板、交换数据和不依赖装配上下文的详细设计；
- 双击 occurrence 在当前 Product 视口中激活编辑，根 Product 和周围零件继续显示；
- “在新窗口打开”有两个明确模式：`Open Definition` 与 `Open in This Context`。后者携带原 root Product/context token，前者不允许解析产品特定输入。

### 6.2 会话状态

`ProductDesignSession` 至少包含：

- session ID、actor 和权限；
- root Product Workspace、base Revision 和 configuration snapshot；
- active InstancePath/Reference Workspace；
- 当前加载的 occurrence/representation policy；
- visibility、selection 与 preview 等非持久交互状态；
- context catalog digest。

激活、视图和普通 selection 不写 Revision。真正创建 Publication、ContextInput、ContextBinding 或 Feature 时才提交 Domain Command。

### 6.3 Product Context Catalog

引用 picker 不查询全站 Document catalog，而查询：

```text
ContextCatalog(root snapshot, active occurrence, expected contract)
```

返回当前根 Product 中：

- 可从 root 展开的 occurrence 与其层级路径；
- 用户有权读取的 Part/Product Publication；
- 与目标 ContextInput 类型、量纲、对称性和版本兼容的候选；
- cycle check 的预判结果和不可选原因；
- Product 转发接口与可选的 source target 预览。

隐藏 occurrence 仍可搜索，因为 visibility 是视图状态；suppressed/不属于当前 configuration 的 occurrence 不可绑定。根 Product 之外的对象必须先通过“插入组件/添加受控依赖”进入结构，不能在普通 picker 中旁路 Product scope。

### 6.4 选择未发布对象

默认只允许跨 occurrence 选择 Publication。用户点中兄弟 Part 的未发布 face/parameter 时，UI 提供一次组合操作：

1. 在来源 Reference 创建带自动名称的 Publication；
2. 在消费 Part 创建或选择 ContextInput；
3. 在 root Product 创建 ContextBinding；
4. 预计算所有候选 Revision/Artifact；
5. 对涉及的 Workspace heads 做统一 CAS 后原子提交。

这需要 `ProductDesignTransaction` 支持多 Workspace 子命令。长几何计算仍在数据库事务外完成；最终短事务只有所有 expected heads 均匹配才追加全部 Revision/ChangeSet/Outbox。任一 Head 变化则整体拒绝，不留下半条连线。

### 6.5 Product Design Inputs 面板

根 Product 提供面向用户的 Design Inputs 面板，但不复制一份参数真相。第一版把 Skeleton 或其他 occurrence 中标记为 `DESIGN_INPUT` 的 PARAMETER Publication 转发为 Product Publication，并按 Product Publication name 展示：

- 用户看到 `body_length`、`wheelbase`、`wheel_outer_diameter` 等名称、单位、范围和当前值；
- 每一项仍指向唯一的 source ParameterId/PublicationId，面板不保存第二份数值；
- 编辑动作通过当前 Product session 定位来源 occurrence，并向 source Part Workspace 提交正常参数命令；
- 提交后 Product 只显示 `UPDATE_AVAILABLE` 与完整 Update Plan，用户接受后才更新其余 binding snapshot；
- 同名参数来自不同 occurrence 时必须由 Product Publication 重新命名，不能依赖树中碰巧显示的短名称。

后续若 Product 自身需要 Configuration/Design Table 参数，再增加真正的 Product Parameter entity；不能先用 UI 聚合值冒充领域模型。

## 7. 产品更新与发布

### 7.1 Product Update Plan

根 Product 检测到来源 Workspace Head 变化时，先产生只读计划：

```text
changed source Publication
  -> affected ContextBindings
  -> affected Part ContextVariants
  -> forwarded Product Publications
  -> affected assembly constraints
  -> root solve / release gates
```

几何/参数派生边必须是 DAG；Assembly constraint graph 可以有闭环，二者不能混成同一种 cycle。更新按 DAG 拓扑序计算候选，最后再构建一次冻结 SolveManifest。

每条 binding 同时报告三个正交状态：

- connection：`CONNECTED | BROKEN | INCOMPATIBLE`；
- currency：`CURRENT | UPDATE_AVAILABLE | UPDATE_BLOCKED`；
- evaluation：`READY | FAILED | BLOCKED_BY_UPSTREAM`。

Product 聚合状态由证据派生，不允许一个 `OK/ERROR` 覆盖所有含义。

### 7.2 接受更新

- 用户先查看影响范围、预计创建的 Revision/variant 和失败诊断；
- 接受动作使用一个幂等 Product update transaction；
- 计算阶段不持有数据库锁，提交阶段对全部相关 Workspace sequence 做 CAS；
- 默认全有或全无。未来的 partial update 必须是显式 scope，并确保未更新边界仍有冻结 snapshot，不能静默混合新旧来源；
- Undo/Redo 作用于这次根 Product design transaction，而不是让用户逐个撤销内部 Part。

### 7.3 Product Version/Release Manifest

“发布一个新的玩具车版本”不是只给根 Product Revision 起名字，而是创建不可变 manifest：

- root Product Revision 与 configuration snapshot；
- 完整 occurrence InstancePath、resolved Part/Product Revision 和 placement；
- ContextBinding 与 accepted resolution snapshots；
- ContextVariant/EvaluationManifest/GeometryId；
- Product Publication contracts；
- Assembly SolveManifest、solver/evaluator/kernel/policy versions；
- Release Gate 结果和 provenance。

编辑态可以 FOLLOW Workspace Head；Version/Release 中不得留下未解析 selector。旧版本必须在所有 Workspace Head 移动、服务重启和缓存清空后仍可重放。

## 8. 玩具车纵向场景

### 8.1 产品结构

```text
ToyCar (Product)
├─ Skeleton.1 (Part)
├─ Body.1 (Part)
├─ Wheel.FL (Wheel Product Reference)
│  ├─ Rim.1 (Part)
│  └─ Tire.1 (Part)
├─ Wheel.FR (same Wheel Product Reference)
├─ Wheel.RL (same Wheel Product Reference)
└─ Wheel.RR (same Wheel Product Reference)
```

四个 Wheel 是同一个 Product Reference 的四个 Instance，不复制 Wheel 定义。`Wheel.FL` 等是用户编辑后的 sibling-unique Instance name，持久 occurrence identity 仍是 InstanceId path。

### 8.2 Skeleton 接口

Skeleton 作为普通 Part，发布：

- 参数：`body_length`、`body_width`、`body_height`、`wheelbase`、`track_width`、`hole_diameter`、`wheel_outer_diameter`、`wheel_width`、`rim_diameter`；
- 几何：`BodyFrame`、`WheelFLFrame`、`WheelFRFrame`、`WheelRLFrame`、`WheelRRFrame`。

Body 的 ContextInput 绑定车身尺寸和四个 wheel frame，生成长方体及四个圆柱切除，并发布 `WheelFLAxis` 等装配接口。Wheel Product/Rim/Tire 的输入绑定轮径、宽度和轮毂直径；Wheel Product 转发 `MountAxis` 与 `MountPlane`。

装配约束把四个 Wheel occurrence 的 `MountAxis`/`MountPlane` 分别绑定到 Body 的四组已发布接口。依赖方向保持：

```text
Skeleton -> Body geometry/publications -> Product constraints
Skeleton -> Wheel/Rim/Tire geometry -> Product constraints
```

不得建立 Body 读取 Wheel 几何、Wheel 又读取 Body 几何的环。四个 Wheel 若获得完全相同输入，应命中同一 context variant/artifact；位置差异由 occurrence placement 与装配约束表达。

### 8.3 用户流程

1. 新建 ToyCar Product，在树中创建 Skeleton、Body 与 Wheel Product；
2. 在 Product 页面依次激活这些 occurrence 原位建模；
3. 从 Skeleton 的参数/几何创建 Publication，系统自动分配名称，用户可直接改名；
4. 编辑 Body 时，从 Product context picker 选择 `Skeleton.1 / body_length` 等名称建立 binding；
5. 创建一个 Wheel Product 定义后插入四次，并重命名 occurrence；
6. 用转发 Publication 创建四组装配约束；
7. 在 ToyCar 的 Design Inputs 面板修改 `wheel_outer_diameter`、`wheelbase` 或 `body_length`；
8. Product 显示完整 Update Plan，接受后重算 Body、共享 Wheel variant、四个 occurrence 和 assembly solve；
9. Release Gate 通过后创建新的 ToyCar Version；旧 Version 保持可重放。

### 8.4 必须通过的验收

- 普通流程没有手填 DocumentId/PublicationId；
- picker 只出现 ToyCar 当前 context 内的兼容 Publication；
- Publication/Instance rename 后 binding 与约束保持连接；
- 同名 Publication 在不同 Part 中通过 occurrence path 清晰区分；同一 Part 内拒绝重名；
- 一个 Wheel 定义更新一次，四个 occurrence 一致更新，不生成四份定义；
- 不同 occurrence 输入会产生显式 context variant，不能污染共享定义；
- 更新失败明确定位到 source Publication、binding、consumer feature 和 phase；
- 删除/替换/重定向/Undo/Redo/服务重启/缓存清空后结果确定；
- 新旧 ToyCar Version 可独立打开、求值、求解和导出。

## 9. 建议重排 P10

原 P10 直接从 typed InstancePath 进入 SolveManifest，会把 P9 的试点入口固化到正式装配合同中。建议保留 P10A，重排后续批次：

### P10A：typed nested InstancePath 与命名合同

- 完成嵌套 rigid Product、relative path、reparent/replace rewrite；
- 增加 Publication/ContextInput rename 与作用域唯一性；
- 服务端默认名称分配、normalization profile 和并发 CAS corpus；
- 名称只进入 display projection，canonical path 继续只用稳定 ID。

### P10B：Product Design Session 与 in-context activation

- root Product session、active occurrence、breadcrumb、原位编辑；
- `Open Definition` / `Open in This Context`；
- 权限、订阅和 command target 都从 session/typed path 解析；
- 不新增持久几何语义。

### P10C：Context Catalog 与人类可读 picker

- root-snapshot-scoped catalog API；
- 按 expected contract、权限、configuration 和 cycle feasibility 过滤；
- 参数编辑、ContextReference、Publication 转发与装配 constraint 移除手填 ID；
- 未发布对象提供受控的 Create Publication 引导。

### P10D：ContextInput、Product ContextBinding 与多 Workspace transaction

- 将 occurrence-specific wiring 从 Part ContextReference 提升为 Product-owned binding；
- Part 新增 typed ContextInput；
- 完成跨 Workspace 预计算、统一 CAS、ChangeSet 和 Undo/Redo；
- 项目未发布，直接修改唯一 schema/调用方，不保留 P9 试点双写。

**P10A–P10D 实施记录（2026-09-19）**：上述四阶段已落地。稳定 occurrence identity 使用 typed segment/InstanceId chain，名称采用 `nfkc-casefold-v1` 作用域规则；Product Design Session 与 Context Catalog 以不可变 root snapshot 为边界；Part ContextInput 与 Product ContextBinding 通过 `product_design_transactions` 组织跨 Workspace Revision。创建、Undo 与 Redo 都先预计算候选，再统一锁 Head/sequence 并原子提交；嵌套 owning Product 链会自底向上产生新 Revision，FK projection 在全部 Revision 建立后统一写入。P9 ContextReference 不参与这条新提交链，也不存在双写。

### P10E：Product Update Plan 与 Context Variant

- 构建产品级依赖 DAG、影响闭包和三维状态投影；
- variant key、共享制品、Isolate/Derive Part from Context；
- 全有或全无更新、失败隔离和可审计 preview；
- 冷重建与并发更新 corpus。

### P10F：ResolutionSnapshot 与 SolveManifest Builder

- 承接原 P10B；
- 输入只来自正式 typed path、Publication、ContextBinding snapshot 和 variant evaluation；
- 冻结 branch/tolerance/build/scope，builder 阶段拒绝 broken/ambiguous/incompatible。

### P10G：M3 replay、Router 与唯一路径迁移

- 合并原 P10C/P10D；
- replay、provenance、deadline/cancel 与 request-specific lookup；
- 所有正式 solve/preview/commit 走 M3，删除开发旁路。

### P10H：ToyCar Product Version/Release 验收

- 实现 Product Version Manifest 与最小 Release Gate；
- 以本文件第 8 节作为浏览器、服务、Worker 和持久化综合场景；
- 冻结成为后续 P11 Feature 扩张和 P15 Configuration/Design Table 的回归基线。

依赖关系：

```text
P9G
 -> P10A -> P10B -> P10C -> P10D -> P10E
                                  P10E -> P10F -> P10G -> P10H
P11 Feature naming corpus -----------------------------> P10H
```

## 10. 明确不做的捷径

- 不把 Publication name、Instance name 或树路径升级成数据库身份；
- 不让普通关联 picker 浏览整个租户的所有 Document；
- 不在 Part 中持久保存某个任意 Product occurrence 的隐式上下文；
- 不让 evaluator 在求值时查询可变 Head；
- 不用四份 Wheel 文档规避共享 Reference/context variant 语义；
- 不为当前实验数据增加 adapter、双写或 name fallback；
- 不在 Product update 成功前逐个提交半成品 Revision；
- 不把 assembly constraint cycle 误判成禁止的几何/参数依赖 cycle；
- 不在缺少冻结 dependency closure 时宣称 Product 已发布。

## 11. 实施前需要冻结的少量决策

进入 P10A 时应通过代码与 schema spike 冻结：

1. Unicode normalization/case-fold profile 及数据库唯一索引表达；
2. ContextInput 是 Part model entity，还是 Feature input slot 的统一 facade；
3. Context Variant 只作为派生 Evaluation，还是需要独立可寻址 Revision subtype；
4. 多 Workspace transaction 的数据库原子提交边界与 artifact orphan GC；
5. Product Version/Release 的首个 schema 是否沿用现有 Version 命名对象，或新增独立 Release entity。

这些决定影响稳定身份、历史和发布合同，必须在 P10A/P10D 前通过最小 corpus 冻结，不能在 UI 实现中临时决定。
