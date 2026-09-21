# 实体特征扩展契约

> 目标契约，不等于已交付能力。返回[目标架构目录](../../TARGET_ARCHITECTURE.md)；当前事实见[当前架构](../../CURRENT_ARCHITECTURE.md)，实施顺序只在[统一路线](../../../plans/README.md)维护。

### 5.4.10 抽壳 Shell

Shell 是对已有 Solid 的原子修改：删除指定开口面，并对其余边界做等距偏移和连接。

```proto
message ShellFeature {
  FeatureOutputRef input_body = 1;
  repeated PersistentSelection faces_to_remove = 2;
  LengthValue thickness = 3;
  ShellSide side = 4; // INWARD | OUTWARD
  ShellJoin join = 5; // ARC | INTERSECTION
  optional ToleranceOverride tolerance = 6;
}
```

`thickness` 始终为正数；INWARD/OUTWARD 单独编码，避免 signed length 与面朝向混淆。P0 要求 `faces_to_remove` 非空且唯一解析为 input Body 的 Face；“无开口偏置实心”应是独立 Offset Body 功能。

实现使用 [BRepOffsetAPI_MakeThickSolid::MakeThickSolidByJoin](https://dev.opencascade.org/doc/refman/html/class_b_rep_offset_a_p_i___make_thick_solid.html)。OCCT 文档明确指出全局 `SelfInter`/`Intersection` 算法选项并未完整实现，因此目标架构不暴露这些布尔开关来制造虚假的能力：

- P0 固定 `SelfInter=false`，不声称支持自交修复；
- P0 固定全局 `Intersection=false`；它与 `GeomAbs_Intersection` 面连接类型不是同一个选项；
- `join=ARC` 首先交付；`join=INTERSECTION` 只有通过独立 corpus 后才开放；
- offset sign 由 ShellSide 和外壳方向转换，转换规则纳入 evaluator version；
- 读取 `Modified` 历史并与被删除开口面 tombstone 合并；
- 结果必须是单一、闭合、有效 Solid，所有残留 sliver/开放边都视为失败。

典型失败必须可解释：厚度超过局部曲率半径、窄槽塌陷、相邻偏置面无法连接、开口面选择不连续、非流形输入或偏置自交。诊断至少给出 `problematic_selection_ids`、估算失败区域 bbox 和 OCCT status；不得只返回 `MakeThickSolid failed`。

P1 可增加 `MultiThicknessShellFeature`，但它不是简单给 face 列表附不同厚度：必须定义厚度分区边界、过渡规则和连接连续性，未设计前不放进当前 schema。

### 5.4.11 拔模 Draft

Draft 以明确的中性元素、拉模方向和面组修改 Body：

```proto
message DraftFeature {
  FeatureOutputRef input_body = 1;
  PullDirection pull_direction = 2;
  repeated DraftGroup groups = 3;
}

message DraftGroup {
  repeated PersistentSelection faces = 1;
  NeutralElement neutral = 2; // DATUM_PLANE | PLANAR_FACE
  AngleValue angle = 3;
  DraftMaterialSide material_side = 4;
  DraftPropagation propagation = 5; // NONE | TANGENT
}
```

使用 [BRepOffsetAPI_DraftAngle](https://dev.opencascade.org/doc/refman/html/class_b_rep_offset_a_p_i___draft_angle.html)。每个 group 解析出中性平面和拉模方向后，对面逐一 `Add`；每次检查 `AddDone`，失败时读取 `ProblematicShape/Status`。整个 Feature 原子化：任一面失败就回滚所有 group，不允许“前五个面成功、第六个失败”的半成品成为 Body Tip。

- `angle` 保存绝对值与明确的 material side；P0 限制 `0 < abs(angle) < π/2 - angular_epsilon`；
- Pull Direction 是脱模运动方向，不等于任意所选面的法向；
- Neutral plane 表示几何保持位置，必须与 Pull Direction 的关系满足内核算法要求；
- `TANGENT` propagation 只扩展到版本化 tangent tolerance 下连续的面，最终扩展集合写入结果，不能在重算时不可见地变化；
- 同一 Face 不能属于语义冲突的两个 group；不同 group 的顺序进入 canonical definition；
- 修改历史通过 `Modified/Generated` 汇总，问题 Face 返回持久选择证据。

拉伸自带斜度可以在内部复用 Draft evaluator，但它仍是一个 `LinearExtrudeFeature`，不在用户 Feature Graph 中偷偷插入第二个 FeatureId。

### 5.4.12 多截面实体 Loft

多截面实体是按用户给定顺序穿过多个截面构造 Solid，不按世界坐标自动排序：

```proto
message LoftFeature {
  repeated LoftSection sections = 1;
  LoftContinuity continuity = 2; // C0 | C1 | C2
  LoftMode mode = 3;             // SMOOTH | RULED
  SectionMatching matching = 4; // EXPLICIT | AUTO_COMPATIBLE
  BodyOperation operation = 5;
  optional uint32 max_degree = 6;
}

message LoftSection {
  oneof geometry {
    ProfileSelection profile = 1;
    PersistentSelection wire = 2;
    PointRef terminal_point = 3;
  }
  optional PersistentSelection seam_anchor = 4;
  repeated SectionMarker markers = 5;
}
```

使用 [BRepOffsetAPI_ThruSections](https://dev.opencascade.org/doc/refman/html/class_b_rep_offset_a_p_i___thru_sections.html) 构造 Solid。规则如下：

1. 至少两个 section；Point 只能作为首/末截面，内部必须是闭合 Wire；
2. 所有 Wire 的开闭属性一致；实体模式要求闭合 Wire；
3. `sections[]` 顺序是领域语义，服务端不按质心、法向或轴向重排；
4. 默认 `matching=EXPLICIT`：每个截面提供 seam anchor 与可选 markers，建立边段对应；
5. `AUTO_COMPATIBLE` 只用于交互建议/简单截面，实际采用的方向、起点和分段映射必须固化为 `ResolvedSectionMatching` 并计入求值结果；
6. 调用内核时设置输入不可变或传入副本，避免 compatibility 检查修改上游 Wire；
7. `SMOOTH` 才使用 continuity、smoothing weights 和 max degree；`RULED` 在相邻截面间生成直纹面，忽略平滑参数；
8. P0 不支持最后截面回接第一截面的周期 Loft；该能力需要独立的周期参数化与 seam 规则，不能用普通 `ThruSections` 冒充；
9. 截面自交、顺序折返、对应边交叉或结果非流形均失败；
10. Guide curve 与 centerline/spine 不混入 P0 Loft；它们属于带独立相交约束的 Guided Loft 或 Sweep 扩展。

截面对应是多截面实体可编辑性的关键。只依赖 OCCT 自动兼容会在截面加一条边后产生不可预测旋转或扭结。`SectionMarker` 应引用 Region edge/vertex 的稳定身份，解析后形成：

```text
ResolvedSectionMatching
  section_id -> orientation + seam_vertex
  interval_id -> [section_0 edge range, section_1 edge range, ...]
  inserted_split_vertices[]
```

若为了兼容而需要拆分 Wire，这些 split 只存在于 Feature 的求值副本，并把 `inserted_split_vertices` 记录为派生拓扑；绝不能反向修改 Sketch 或上游 Face。输出血缘使用 `GeneratedFace(interval_id)`、`StartCap`、`EndCap` 和 terminal point fan，不用结果面序号。

### 5.4.13 后续特征如何扩展

| Feature | 分类 | 复用能力 | 需要新增的关键语义 |
|---|---|---|---|
| Sweep/Pipe | Generator | Profile、Axis/Path refs、BodyOperation | path frame、扭转、角点过渡、guide |
| Rib/Web | Composite generator | open Profile、extrude、boolean | 薄壁方向、到面终止、拐角连接 |
| Hole | Semantic composite | Axis、limits、REMOVE | 标准孔型、螺纹 metadata、沉头/沉孔 |
| Fillet | Modify | PersistentSelection、history | 半径 law、边链、rolling-ball 失败诊断 |
| Chamfer | Modify | PersistentSelection、history | distance-angle、reference face、链传播 |
| Pattern/Mirror | Replication | Feature refs、BodyOperation | 实例身份、跳过实例、合并策略 |
| Boolean Body | Combine | BodyOperation | 多 Body ownership、工具 Body 保留策略 |

这些能力应增加新的 `oneof` 分支和 evaluator，不增加通用脚本字段。孔虽然可由草图+切除组合生成，仍值得成为语义 Feature，便于制造信息、标准件配置和孔表；其内部子操作只出现在 provenance，不产生用户不可编辑的隐藏节点。
