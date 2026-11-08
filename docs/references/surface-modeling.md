# 曲线与曲面建模参考

本页记录商业 CAD 的公开功能与操作依据；本项目的取舍见[线框与曲面设计方案](../../plans/wireframe-surface-design.md)。商业软件的专有内核算法没有据此验证，不能从功能名称反推其内部实现。本轮未运行 CATIA 或其他商业 CAD 的实际模型。

## CATIA：工作台定位与共同流程

依据本机 B33 文档，Wireframe and Surface Design 面向前期构造几何、基本曲面及与 Part Design 配合的混合建模；Generative Shape Design 面向更完整的线框、曲面构造与修改。两者有大量功能重叠：WFS 的曲面目录已经列出 Extrude、Revolve、Offset、Sweep、Fill、Multi-sections 和 Blend，也列出连接与曲率分析。不能把它们简单分成“没有连续性控制”和“具有连续性控制”两套工具。

GSD 目录进一步列出导向/隐式截面扫掠、变偏移、更多曲线派生、法则、外推及几何操作。具体配置/许可仍影响选项；例如 Multi-sections 页面对特定单截面/单导向配置另列 HD2 要求。本轮比较是手册行为范围，不是许可组合的完整审计。

| 能力族 | 手册中的功能与操作重点 | 对工程建模的意义 |
|---|---|---|
| 构造线框 | 点、有限线、圆/圆锥曲线、样条、螺旋线；曲线上点、投影、交线、平行/偏移、连接曲线、等参数曲线 | 将设计意图建立为可重用、有方向及关联来源的输入 |
| 基础曲面 | 拉伸、旋转、球/柱面、偏移 | 先形成独立面片，再用于其他曲面或实体 |
| Sweep | 区分 profile、guide 和 spine；显式截面可使用一/二条导向，spine 决定扫掠截面平面 | 路径、截面姿态与边界控制是不同输入职责 |
| Multi-sections | 截面有序；导向需与截面相交；支持闭合点、方向、coupling、端部支撑及连续性 | 减少扭转和错误对应，支持上游编辑后的重算 |
| Fill / Blend | 边界与支撑分别选择，逐边设置连续性；Fill 可加入内部通过点/曲线及偏差控制 | 补面需要明确约束和可接受误差 |
| Split / Trim | 选择保留/移除区域，多连通交线可能要求消歧；自动外推是显式选项 | 修剪同时具有几何、分区和用户意图语义 |
| Join / Extract / Boundary | 连接性、流形和相切检查；传播范围、自由边界、合并距离及结果方向可控制 | 整理输入集合、识别开口并建立后续稳定引用 |
| 分析与实体化 | 曲线/曲面连接检查、曲率分析；Part Design 的 Thick Surface、Close Surface | 从质量检查和曲面结果继续形成实体 |

### 值得借鉴的交互

- 输入按角色收集，截面/导向列表可定位、替换、反向与重排；三维标记与列表使用一致的颜色。闭合点、对应点和曲面侧向显式可见。
- Join 的预览可显示自由边界，错误对象在视区高亮；连接、流形、相切是不同检查。合并距离是算法输入，不能被当作模型本来就精确连接的证据。
- Sweep 的 guide 与 spine 不应合成一个含糊的“路径”字段。Multi-sections 的自动对应只是初始建议，已接受的人工对应应保留。
- Trim 有保留/移除侧及结果分区选择；靠近既有边、相切或产生多个交区时，需要明确消歧。
- Geometrical Set 是无序组织；Ordered Geometrical Set 还带有步骤、当前对象、吸收和可见/可访问范围。其插入/删除可触发引用重路由，是独立语义，不应只通过给普通集合加排序号模拟。
- Connect Checker 区分几何邻近与已连接拓扑，并提供距离、相切、曲率等分析。分析结果和构造时要求的连续性需要分别表达。

### 本机 SourceLocation

先检索 `control/WfsEnglishC2.viewdoc`、`SdgEnglishC2.viewdoc`，共用页面再核对 `CfyEnglishC2.viewdoc`，实体化核对 `PrtEnglishC2.viewdoc`。以下路径相对 `/mnt/c/UserApp/3D/DS/Dassault Systemes/B33doc/English/`；MCP 库标识为 `6d5aed22-2df7-427c-83ae-0b4926af15ef`，检索 revision 为 `9058175bec4c4d87f0f646ae`。临时检索 ID 不作为持久来源。

| 页面 | 证据范围 |
|---|---|
| `online/wfsug_C2/wfsugpr01.htm`，Overview / Wireframe and Surface in a Nutshell | WFS 定位与混合建模 |
| `online/wfsug_C2/wfsugbt0100.htm`、`wfsugbt0200.htm`、`wfsugbt0300.htm` | WFS 线框、曲面、工具/分析目录 |
| `online/sdgug_C2/sdgugpr01.htm`，Overview / Generative Shape Design in a Nutshell | GSD 定位 |
| `online/sdgug_C2/sdgugbt0100.htm`、`sdgugbt0200.htm`、`sdgugbt0300.htm` | GSD 线框、曲面及操作目录 |
| `online/sdgug_C2/sdgugbt0204.htm`，Creating Swept Surfaces | profile / guide / spine、支撑与连续性限制 |
| `online/cfyug_C2/cfyugloft.htm`，Creating Multi-sections Surfaces | 截面、导向相交、支撑、coupling 与闭合点 |
| `online/sdgug_C2/sdgugbt0206.htm`，Creating Fill Surfaces | 边界/支撑、连续性、内部约束与偏差 |
| `online/cfyug_C2/cfyugtrimGSD.htm`，Trimming Geometry | 保留/移除、相交分区与外推 |
| `online/cfyug_C2/cfyugjoin.htm`，Joining Surfaces or Curves | 连接性、流形、相切、容差与自由边界 |
| `online/cfyug_C2/cfyugboundary.htm`、`cfyugextract.htm` | 边界及提取、传播范围 |
| `online/cfyug_C2/cfyugorderedgeomsets.htm`，Managing Ordered Geometrical Sets | 无序集合与有序/吸收语义 |
| `online/cfyugfss_C2/cfyugfssut0302.htm`，Checking Connections Between Surfaces | 连接分析类型、间隙与拓扑/几何连接 |
| `online/prtug_C2/prtugbt0710.htm`、`prtugbt0709.htm` | Thick Surface、Close Surface |

MCP 返回的关键原文已取至 `has_more=false`；目录页另外读取了本机 HTML。手册中的默认数值只证明参考软件行为，不成为本项目默认容差或验收阈值。

## 其他商业 CAD

| 软件 | 已核对的公开行为 | 本项目可采用的设计要点 |
|---|---|---|
| SOLIDWORKS | Boundary Surface 具有两个方向的输入与相切/曲率条件；列表、曲线标记及连接对应联动。[Boundary Surface](https://help.solidworks.com/2025/english/solidworks/sldworks/c_Boundary_Surface_Concept_surfaces.htm)、[PropertyManager](https://help.solidworks.com/2024/English/SolidWorks/sldworks/r_boundary_pm.htm) | 区分沿截面放样和两个方向的边界曲面；逐边支撑与对应控制直接呈现 |
| Creo Parametric | Quilt 表达非实体曲面结果；Boundary Blend 使用一/二方向边界和约束；Thicken / Solidify 将曲面用于材料操作。[Quilts](https://support.ptc.com/help/creo/creo_pma/r12/usascii/surfacing/surface/About_Quilts.html)、[边界条件](https://support.ptc.com/help/creo/creo_pma/r12/usascii/surfacing/surface/Setting_Boundary_Conditions.html)、[Thicken](https://support.ptc.com/help/creo/creo_pma/r12/usascii/part_modeling/part_modeling/About_the_Thicken_Feature.html) | 曲面结果需有独立类型、可见性和下游用途；实体化须是明确操作 |
| Onshape | Composite Curve 可产生多条独立曲线；Boundary Surface 使用 U/V 边界，Fill 支持多段闭合边界和逐边条件，且区分精确导向与采样导向。[Composite Curve](https://cad.onshape.com/help/Content/PartStudio/composite_curve.htm)、[Boundary Surface](https://cad.onshape.com/help/Content/PartStudio/boundary_surface.htm)、[Fill](https://cad.onshape.com/help/Content/PartStudio/fill.htm) | 支持稳定多结果和链选择；记录实际精度/近似方式，不能将采样拟合标为精确满足 |
| NX | Siemens 的 June 2025 高级能力说明提到 Through Curve Mesh / Studio Surface 的边界拆分及多面输出。[官方说明](https://blogs.sw.siemens.com/designcenter/whats-new-nx-advanced-capabilities/) | 一个构造特征可以产生多面结果；不要强制“一个曲面特征等于一个 Face”。本次未审计 NX 全部选项 |
| Rhino | Trim 保留底层曲面及裁剪边界；MatchSrf 提供位置、相切、曲率匹配，Surface Tools 提供偏移、混接、延长和收缩。[Trim](https://docs.mcneel.com/rhino/8/help/en-us/commands/trim.htm)、[MatchSrf](https://docs.mcneel.com/rhino/8mac/help/en-us/commands/matchsrf.htm) | 底层参数曲面与裁剪 Face 要分开；自由编辑的便利性值得参考，历史/引用规则仍按本项目参数化合同设计 |

这些来源覆盖可观察功能、对象模型及操作，并不构成相同输入下的质量、速度或鲁棒性横向测试；不同版本/许可可能提供不同范围。

## 算法候选的证据边界

本地构建依赖的 `Standard_Version.hxx` 为 OCCT 7.9.1。本轮核对了该版本头文件及官方源码：[BRepFill_Filling](https://github.com/Open-Cascade-SAS/OCCT/blob/V7_9_1/src/BRepFill/BRepFill_Filling.hxx)、[BRepOffsetAPI_MakePipeShell](https://github.com/Open-Cascade-SAS/OCCT/blob/V7_9_1/src/BRepOffsetAPI/BRepOffsetAPI_MakePipeShell.hxx)。前者声明带支撑的 G1/G2 边界约束及误差查询，后者区分扫掠壳构造与 MakeSolid；本地 Sewing 头文件提供自由边、多重边及修改历史查询。

这只证明候选 API 存在。算法接纳范围、完整 Naming、误差上界、取消响应及工程模型成功率仍需后续实现与检验。OCCT 当前在线文档会重定向到较新版本，不能把其页面版本当成本项目运行版本。Onshape 的公开 [FeatureScript 几何操作文档](https://cad.onshape.com/FsDoc/library.html)也仅用于理解 wire/sheet 输出及边界输入合同，不用于推断其专有内核实现。
