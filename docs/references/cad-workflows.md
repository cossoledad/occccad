# CAD 工作流参考索引

本页记录 CATIA B33、产品资料与 OCCT 页面定位，不保存完成日志或独立设计方案。页面内容使用前按具体问题核验；这些引用是用户流程/术语参照，不证明 occccad 已实现能力，也不约束内部架构。重新用于设计时先查 `/mnt/s/tools/DS/Catia/B33doc/English/control/*.viewdoc`，再读对应 `online/` 页面。相对 `online/` 路径均以该英文文档根为基准。

- `/mnt/s/tools/DS/Catia/B33doc/English/online/cfyugasm_C2/cfyugasmut0311.htm`：Editing Constraints、四种 traffic-light 状态、Object in Work。
- `/mnt/s/tools/DS/Catia/B33doc/English/online/cfyugasm_C2/cfyugasmrf0501.htm`：Constraint 属性、Supporting Elements 的 connected/disconnected 与 Reconnect。
- `/mnt/s/tools/DS/Catia/B33doc/English/online/cfyugasm_C2/cfyugasmut0319.htm`：Refreshing Constraints，Broken → NotUpdated 的显式刷新语义。
- `/mnt/s/tools/DS/Catia/B33doc/English/online/cfyugasm_C2/cfyugasmrf0101.htm`：孔类型变化删除支撑轴后约束 disconnected。
- `/mnt/s/tools/DS/Catia/B33doc/English/online/asmug_C2/asmugat0102.htm`：Reconnecting Constraints。
- `/mnt/s/tools/DS/Catia/B33doc/English/online/CAAScdAsmUseCases/CAAAsmCstOnPublish.htm`：约束基于 Publication，替换 component 后自动重连。
- `/mnt/s/tools/DS/Catia/B33doc/English/online/CATIA_P3_default.htm`：CATIA 文档总入口。
- [OCAF User Guide: Topological naming](https://dev.opencascade.org/doc/occt-7.2.0/overview/html/occt_user_guides__ocaf.html)
- [BRepTools_History](https://dev.opencascade.org/doc/refman/html/class_b_rep_tools___history.html)
- [BRepPrimAPI_MakePrism](https://dev.opencascade.org/doc/refman/html/class_b_rep_prim_a_p_i___make_prism.html)
- [BRepAlgoAPI_BuilderAlgo](https://dev.opencascade.org/doc/refman/html/class_b_rep_algo_a_p_i___builder_algo.html)
- [ShapeUpgrade_UnifySameDomain](https://dev.opencascade.org/doc/refman/html/class_shape_upgrade___unify_same_domain.html)
- [TNaming package](https://dev.opencascade.org/doc/refman/html/package_tnaming.html)
- [TNaming_Tool](https://dev.opencascade.org/doc/refman/html/class_t_naming___tool.html)
- `online/basug_C2/basugbt0502.htm`：Specification Tree 与 geometry focus 切换及树宽调整；本项目保留浏览器视口 focus，只采用可拖动树边界和跨会话记忆。
- `online/basug_C2/basugbt0508.htm`：Specification Tree 的层级展开/收起语义；仅保留层级语义参考，当前图标由 Web 实现决定。
- `online/basug_C2/basugcu0102.htm`：Toolbar 的独立命名、显示、内容和位置恢复；本项目据此把类别提升为稳定 ToolbarId，同时保持服务端目录与用户布局分层。
- `online/bascukwr_C2/bascuparameasure0400.htm`：全局 Units 选项；本项目进一步区分用户默认显示单位、用户侧文档覆盖与未来版本化团队文档属性。

## Product 方法参考

- [CATIA Assembly Design：支持 in-context part and assembly design](https://www.3ds.com/fileadmin/Products/catia/solution-builder/content/cross_industries/pdf/CATIA%20TEAM%20PLM.pdf)
- [CATIA Product Design Expert：使用 Publication 管理 contextual links](https://www.3ds.com/assets/edu/document/course-catalog-v5-6r2018-to-v5-6r2023.pdf)
- [CATIA V5R9：Publication、替换与版本更新](https://www.3ds.com/newsroom/press-releases/ibm-and-dassault-systemes-announce-catia-version-5-release-9)
- [3DEXPERIENCE IRPC：Reference、Instance 与 Occurrence](https://3dswym.3dexperience.3ds.com/wiki/solidworks-news-info/getting-started-with-mbom-management_HOJ-oxIVRRqJF1KiTpPDHg)
- [CATIA 设计访谈：assembly context 与 skeleton methodology](https://www.3ds.com/cloud/resources/designing-impactful-innovation-podcast/ep13-designing-beyond-limits-industrial-design)

当前实现见[当前架构](../CURRENT_ARCHITECTURE.md)，后续工作只在[统一路线](../../plans/README.md)维护。

## Assembly Design 来源

2026-09-21 按本机 Asm/Cfy viewdoc 索引读取用户给出的 `online/CATIAfr_C2/asmugCATIAfrs.htm` 及其关联菜单/六类约束任务页、几何兼容参考页、Activate/Deactivate 与 Manipulate 页面。逐页路径、语义与文档疑点集中见[六类约束合同](../architecture/current/assembly-constraints.md)，不再在本索引复制参数矩阵。该次核对不刷新上方其他外部产品资料，也不表示新能力已经实现。

## 机构与 DMU 工作流来源

本设计按本机 `control/KinEnglishC2.viewdoc`、`control/FitEnglishC2.viewdoc` 定位任务页。用户给出的 `online/CATIAfr_C2/kinugCATIAfrs.htm` 是入口，下面的具体页面用于核对流程。

| 页面 | 可确认的用户行为与术语 |
|---|---|
| `online/kinug_C2/kinugbt0202.htm`，Simulating with Commands | 独立命令试动对话框支持游标、数值及直接操纵；不能在此独立命令内录制，录制使用 Simulation |
| `online/kinug_C2/kinugbt0203.htm` | 按请求更新的试动方式，是交互计算而非自动扫描全部位置 |
| `online/kinug_C2/kinugbt0302.htm`，Recording Positions | Simulation 创建树对象，打开 Kinematics Simulation 与 Edit Simulation 两个窗口；自动记录初始帧，Insert 记录位置，再回放记录 |
| `online/kinug_C2/kinugbt0501.htm`、`kinugbt0513.htm` | 机构创建固定零件与命令；可模拟资格关联整个机构及驱动后剩余自由度 |
| `online/kinug_C2/kinugbt0504.htm` | 命令方向可在创建或编辑时设置 |
| `online/kinug_C2/kinugbt0701.htm`、`kinugbt0801.htm` | 旋转接合的两轴、两定位平面；轴重合与平面偏移组成关系 |
| `online/kinug_C2/kinugwd1000.htm` | 机构、接合、命令、固定零件、规律与模拟的树分类 |
| `online/kinug_C2/kinugbt0205.htm`、`kinugbt0206.htm` | 退出与复位不同；修改后位置及再次进入时的命令读数按当前构型处理 |
| `online/fitug_C2/fitugbt0304.htm`，Animating a Viewpoint | Insert 同时记录位置和当前视点；Animate viewpoint 控制回放，可在录制后切换 |
| `online/fitug_C2/fitugbt0302.htm`，Recording Automatically | Automatic insert 可按移动过程记录，精度／阈值影响记录密度；被拒绝的碰撞位置不记录 |
| `online/fitug_C2/fitugbt0300.htm` | Simulation 记录和生成 Replay 制品是不同动作，Replay 可以独立查看 |

表中缩写文件名沿用同单元格的目录。Fit 页面属于 DMU Fitting，提供录制／视点操作参考，不能据此推断 Kinematics 求解器的内部实现、关键帧持久模型或闭环插值算法。Recording Positions 的多机构例子也不代表本项目首个切片支持多机构。

维护者 2026-10-11 的 CATIA 实践补充：驱动接合不必直接连接固定件；命令目标不被满足时，模型与游标保持最后正确位置；正式模拟由用户通过命令面板取得位置，再手动或自动插入记录。该失败保持细节属于用户实践，不扩展为 CATIA 在任意数值失败下的内部状态机承诺。

本项目据此选择[命令试动与录制模拟统一设计](../architecture/target/kinematics-dmu.md)：稳定操作零位、同一约束内核、双面板单拥有者、合格检查点、明确确定／取消及独立产品转换。两套关系独立、以当前 confirmed 按目标关系切换、Revision/CAS/Undo 和近似显示边界都是本项目决策，不声称照搬 CATIA 的约束自动更新或内部架构。实现状态见[当前代码事实](../architecture/current/kinematics-dmu.md)。

空间分析扩展参考 `control/SpaEnglishC2.viewdoc` → `online/spaug_C2/spaugbt0302.htm` 的 clash/contact/clearance 流程；其网格／sag 判定不作为本项目 OCCT 实体精度依据，也不代表当前机构应用已有干涉分析入口。
