/** Explicit local validation, separate from untrusted service exceptions. */
export class CommandInputError extends Error {readonly name="CommandInputError";}
export type OperationFailure={message:string;details:string;code:string;requestId?:string;diagnosticId?:string;phase?:string;retryable:boolean;severity:"error"|"warning"};
export function normalizeOperationFailure(error:unknown):OperationFailure {
  const value=error as {message?:unknown;diagnostic?:unknown;diagnosticCode?:string;code?:string;requestId?:string;correlationId?:string;diagnosticId?:string;phase?:string;retryable?:boolean;terminationStage?:string;terminationReason?:string};
  const raw=typeof value?.message==="string"?value.message:typeof value?.diagnostic==="string"?value.diagnostic:String(error),code=value?.code??value?.diagnosticCode??raw.match(/\[([A-Z_]+)\]/)?.[1]??"OPERATION_FAILED";
  const details=value?.terminationStage||value?.terminationReason?JSON.stringify({message:raw,terminationStage:value.terminationStage,terminationReason:value.terminationReason}):raw;
  const retryable=value?.retryable??["ASSEMBLY_INTERACTION_UNCONFIRMED","TIMEOUT","CONNECTION_CLOSED","REALTIME_UNAVAILABLE","DATABASE_BUSY","REALTIME_BUSY"].includes(code);
  const reasons:Record<string,string>={
    REPLACEMENT_FRAME_INCONSISTENT:"替换后的组件位置与关联输入无法保持一致，请检查约束和接口。",
    REPLACEMENT_VERSION_REQUIRED:"替换需要明确的装配和新组件版本，请重新选择组件",
    REPLACEMENT_DIRECT_INSTANCE_REQUIRED:"只能替换当前装配的直接组件",
    REPLACEMENT_TYPE_MISMATCH:"请选择同类型的替换组件",
    REPLACEMENT_CYCLE:"替换会形成循环装配引用",
    REPLACEMENT_RECEIVER_UNSUPPORTED:"只支持替换当前装配中的直接接收零件",
    REPLACEMENT_INPUT_MAPPING_REQUIRED:"仍有已关联输入未指定新零件输入",
    REPLACEMENT_INPUT_INCOMPATIBLE:"对应输入的类型、契约或量纲不兼容",
    REPLACEMENT_LOCAL_INPUT_UNSUPPORTED:"接收零件含本地输入，请先恢复默认或重新关联后再替换。",
    CONTEXT_LOCAL_INPUT_UNSUPPORTED:"此输入尚不能独立保存为精确几何，请保留关联。",
    CONTEXT_LOCAL_INPUT_INCOMPATIBLE:"本地输入与当前零件输入契约不兼容，请恢复默认或重新关联。",
    CONTEXT_LOCAL_INPUT_HAS_NO_SOURCE:"本地输入没有来源关系；请先重新关联再设置固定版本。",
    CONTEXT_INPUT_ALREADY_LOCAL:"此输入已是本地输入。",
    CONTEXT_INPUT_ASSIGNMENT_MISSING:"该实例输入已不存在，请刷新后重新选择。",
    CONTEXT_INPUT_ACCEPTED_RESULT_UNAVAILABLE:"当前已接受结果不可用，请先修复输入或上游特征。",
    CONTEXT_INPUT_FORWARD_FAILED:"当前输入无法恢复对外接口，请检查下游特征。",
    CONTEXT_BINDING_REVISION_CHANGED:"装配已更新，请重新预览后确认。",
    CONTEXT_ASSIGNMENT_CONFLICT:"输入赋值状态冲突，请重新打开操作。",
    REPLACEMENT_INPUT_DUPLICATE:"多个输入不能对应同一本地目标，请重新选择",
    REPLACEMENT_REQUIRED_INPUT_MISSING:"新零件有必需输入尚未满足",
    REPLACEMENT_INPUT_UNAVAILABLE:"已接受的输入快照不可用，请先修复关联",
    REPLACEMENT_CHAIN_UNSUPPORTED:"该接收零件同时提供其他关联输入，本轮不迁移",
    REPLACEMENT_PLACEMENT_UNAVAILABLE:"无法从独立安装支撑取得初始位置，请检查安装接口",
    REPLACEMENT_DEEP_REFERENCE_UNSUPPORTED:"关联使用了深层路径，请先整理为直接组件的公开接口",
    REPLACEMENT_PINNED_INPUT:"组件存在固定输入，请先明确处理该关联后再替换",
    REPLACEMENT_MECHANISM_UNSUPPORTED:"组件属于运动机构，本轮不迁移机构关系",
    REPLACEMENT_MAPPING_REQUIRED:"仍有已使用接口未指定对应接口",
    REPLACEMENT_IMPACT_CHANGED:"接口影响范围已变化，请重新预览",
    REPLACEMENT_GEOMETRY_FAILED:"关联零件无法完成替换后的求值，请检查输入及下游特征",
    REPLACEMENT_RAW_SUPPORT_UNSUPPORTED:"约束支撑未使用公开接口，请先重新指定支撑。",
    REPLACEMENT_CONTRACT_INCOMPATIBLE:"对应接口的类型、契约或量纲不兼容，请重新选择。",
    REPLACEMENT_PUBLICATION_UNAVAILABLE:"所选新接口尚不可用，请修复来源或选择其他接口。",
    REPLACEMENT_FORWARDING_FAILED:"对外接口无法恢复，请检查对应接口。",
    REPLACEMENT_CONSTRAINT_FAILED:"装配约束无法迁移到新组件，请检查支撑和接口。",
    NESTED_REFERENCE_UPDATE_REQUIRED:"来源装配的内部关联尚待更新或修复，请先在来源装配接受更新。",
    CONTEXT_PUBLIC_INTERFACE_REQUIRED:"请选择直接子零件的发布接口，或子装配的公开接口。",CONTEXT_DIRECT_RECEIVER_REQUIRED:"请在当前装配中激活直接子零件接收关联输入。",
    PUBLICATION_SKETCH_ENTITY_UNAVAILABLE:"来源草图对象不可用，已接受输入保留；可固定当前输入或重新连接。",PUBLICATION_CONTRACT_INCOMPATIBLE:"来源接口缺失或类型不兼容，请重新连接符合要求的接口。",REFERENCE_SOURCE_MISSING:"来源文档暂不可用，已接受结果保留；请检查来源或重新连接。",
    CONTEXT_INPUT_VALUE_READ_ONLY:"此值由产品关联驱动，请定位来源或在关联管理中更换输入。",CONTEXT_CONSUMER_UNSUPPORTED:"此发布类型暂不支持所选本地输入。",PUBLICATION_TARGET_DOCUMENT_MISMATCH:"请选择当前零件中的对象。",PUBLICATION_TARGET_OCCURRENCE_MISMATCH:"所选对象不属于当前零件实例，请重新选择。",PUBLICATION_TARGET_REVISION_CHANGED:"所选对象已更新，请重新选择发布目标。",CONTEXT_PRODUCT_REVISION_CHANGED:"产品已更新，请重新打开操作。",CONTEXT_VARIANT_CHANGED:"关联输入已更新，请重新打开操作。",CONTEXT_BASE_REVISION_CHANGED:"零件已更新，请重新打开操作。",
    BOOLEAN_REGION_MISSING:"原结果区域已失效，请重新选择或移除。",BOOLEAN_REGION_AMBIGUOUS:"分区变化后无法唯一对应，请重新选择。",BOOLEAN_REGION_SELECTION_EMPTY:"至少保留一个结果区域。",BOOLEAN_REGION_DUPLICATE:"结果区域不能重复保留。",BOOLEAN_REGION_PARTITION_FAILED:"实体分割失败，请检查目标与工具。",BOOLEAN_REGION_LIMIT_EXCEEDED:"结果区域过多，请简化输入实体。",NO_MATERIAL_CHANGE:"所选结果没有改变目标实体，请调整运算或保留区域。",EMPTY_RESULT:"没有可保留的实体材料，请调整输入。",INVALID_RESULT:"结果实体无效，请调整保留区域或输入。",

    PATTERN_SEEDS_REQUIRE_SAME_BODY_OPERATION:"种子必须属于同一实体，并具有一致的添加或切除语义。",PATTERN_SEED_SET_INVALID:"种子集合无效，最多选择32个同实体的实体特征。",PATTERN_SEED_DUPLICATE:"同一种子不能重复添加。",
    MISSING_SUPPORT:"尚未选择支撑，请选择所需对象。",INSTANCE_UNAVAILABLE:"支撑所属零件已不可用，请检查关联零件或重新选择支撑。",SUPPORT_SNAPSHOT_STALE:"原支撑需要重新检查，请刷新后重新选择。",RESOLUTION_UNAVAILABLE:"当前支撑尚未完成检查，请重新计算或重新选择。",SOURCE_UNAVAILABLE:"引用来源暂不可用，请检查上游对象。",NOT_CONNECTED:"引用已断开，请重新选择支撑。",SUPPORT_UNAVAILABLE:"支撑暂不可用，请检查来源或重新选择。",
    PROFILE_REGION_MISSING:"原轮廓区域已不存在，请重新选择区域。",PROFILE_REGION_AMBIGUOUS:"轮廓变化后无法唯一对应，请重新选择区域。",FAILED_SUPPORT:"草图支撑已失效，请重新指定支撑。",SKETCH_SUPPORT_UNAVAILABLE:"草图支撑不可用，请重新选择支撑。",TYPE_MISMATCH:"所选几何类型不符合要求，请重新选择。",AMBIGUOUS:"存在多个对应对象，请明确重新选择。",OUTSIDE_CURRENT_TIP:"原选几何已不存在，请重新选择支撑。",PROFILE_NOT_CLOSED:"轮廓尚未闭合，请补齐或修复草图。",NO_MATERIAL_INTERSECTION:"切削范围没有与目标实体相交，请检查方向、位置和深度。",HOLE_RECESS_DIAMETER_INVALID:"沉孔直径必须大于主孔直径。",HOLE_RECESS_DEPTH_INVALID:"沉孔深度必须为有效正值。",HOLE_COUNTERSINK_ANGLE_INVALID:"锥面夹角必须大于 0° 且小于 180°。",HOLE_RECESS_EXCEEDS_DEPTH:"沉孔或锥段深度必须小于盲孔总深度。",HOLE_DIAMETER_INVALID:"孔直径必须为有效正值。",HOLE_DEPTH_INVALID:"孔深度必须为有效正值。",HOLE_KIND_INVALID:"孔型不受支持。",HOLE_POINT_MISSING:"定位点已丢失，请重新选择草图点。",TERMINATION_TARGET_UNAVAILABLE:"终止平面不可用，请重新选择。",MECHANISM_COMPLETE_INPUT_UNAVAILABLE:"部分接合或支撑需要修复，暂时无法确定自由度。",ANALYSIS_POSE_CHANGED:"当前构型尚无可靠的运动分析结果。",NUMERICAL_NONCONVERGENCE:"尚未找到满足当前要求的结果，请检查参数、支撑和位置。",EXECUTION_FAILURE:"计算未能完成，请重试。",FORBIDDEN:"没有执行此操作的权限。",NOT_FOUND:"所需对象已不存在，请刷新后重新选择。",ASSEMBLY_INTERACTION_UNCONFIRMED:"移动的保存结果尚未确认，可重试确认，或取消恢复。",VALIDATION_FAILED:"当前输入无法完成操作，请检查参数与所选支撑。",VALIDATION_ERROR:"当前输入无法完成操作，请检查参数与所选支撑。",
  };
  const reason=Object.keys(reasons).find(key=>code===key||raw.includes(key));
  const known=[...Object.values(reasons),"当前没有执行此操作的权限。","操作未完成，可重试。","模型已发生变化，请重新预览后确认。","操作未完成，原因尚无法确定。可重试或检查当前输入。"];

  const message=error instanceof CommandInputError?error.message:reason?reasons[reason]:known.includes(raw)?raw:retryable?"操作未完成，可重试。":
    ["STALE_VERSION","VERSION_CONFLICT","CAS_CONFLICT","PREVIEW_STALE","PREVIEW_CANDIDATE_STALE_OR_MISMATCHED","MECHANISM_ANALYSIS_STALE"].some(key=>code===key||raw.includes(key))?"模型已发生变化，请重新预览后确认。":"操作未完成，原因尚无法确定。可重试或检查当前输入。";
  return {message,details,code,requestId:value?.requestId??value?.correlationId,diagnosticId:value?.diagnosticId,phase:value?.phase,retryable,severity:retryable?"warning":"error"};
}

export function engineeringProblem(error:unknown):string{return normalizeOperationFailure(error).message;}
