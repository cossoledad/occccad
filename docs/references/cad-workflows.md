# CAD 工作流参考索引

本页保留已归并计划引用过的 CATIA B33、产品资料与 OCCT 页面定位，不保存完成日志或独立设计方案。页面内容没有在本轮重新核验；这些引用是用户流程/术语参照，不证明 occccad 已实现能力，也不约束内部架构。重新用于设计时先查 `/mnt/s/tools/DS/Catia/B33doc/English/control/*.viewdoc`，再读对应 `online/` 页面。相对 `online/` 路径均以该英文文档根为基准。

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
