export type OperationFailure={message:string;details:string;code:string;requestId?:string;diagnosticId?:string;phase?:string;retryable:boolean;severity:"error"|"warning"};
export function normalizeOperationFailure(error:unknown):OperationFailure {
  const value=error as {message?:unknown;diagnostic?:unknown;diagnosticCode?:string;code?:string;requestId?:string;correlationId?:string;diagnosticId?:string;phase?:string;retryable?:boolean;terminationStage?:string;terminationReason?:string};
  const raw=typeof value?.message==="string"?value.message:typeof value?.diagnostic==="string"?value.diagnostic:String(error),code=value?.code??value?.diagnosticCode??raw.match(/\[([A-Z_]+)\]/)?.[1]??"OPERATION_FAILED";
  const details=value?.terminationStage||value?.terminationReason?JSON.stringify({message:raw,terminationStage:value.terminationStage,terminationReason:value.terminationReason}):raw;
  const retryable=value?.retryable??["ASSEMBLY_INTERACTION_UNCONFIRMED","TIMEOUT","CONNECTION_CLOSED","REALTIME_UNAVAILABLE","DATABASE_BUSY","REALTIME_BUSY"].includes(code);
  const reasons:Record<string,string>={
    INSTANCE_UNAVAILABLE:"支撑所属零件已不可用，请检查关联零件或重新选择支撑。",SUPPORT_SNAPSHOT_STALE:"原支撑需要重新检查，请刷新后重新选择。",RESOLUTION_UNAVAILABLE:"当前支撑尚未完成检查，请重新计算或重新选择。",SOURCE_UNAVAILABLE:"引用来源暂不可用，请检查上游对象。",NOT_CONNECTED:"引用已断开，请重新选择支撑。",SUPPORT_UNAVAILABLE:"支撑暂不可用，请检查来源或重新选择。",
    PROFILE_REGION_MISSING:"原轮廓区域已不存在，请重新选择区域。",PROFILE_REGION_AMBIGUOUS:"轮廓变化后无法唯一对应，请重新选择区域。",FAILED_SUPPORT:"草图支撑已失效，请重新指定支撑。",SKETCH_SUPPORT_UNAVAILABLE:"草图支撑不可用，请重新选择支撑。",TYPE_MISMATCH:"所选几何类型不符合要求，请重新选择。",AMBIGUOUS:"存在多个对应对象，请明确重新选择。",OUTSIDE_CURRENT_TIP:"原选几何已不存在，请重新选择支撑。",PROFILE_NOT_CLOSED:"轮廓尚未闭合，请补齐或修复草图。",NO_MATERIAL_INTERSECTION:"切削范围没有与目标实体相交，请检查方向、位置和深度。",HOLE_POINT_MISSING:"定位点已丢失，请重新选择草图点。",TERMINATION_TARGET_UNAVAILABLE:"终止平面不可用，请重新选择。",MECHANISM_COMPLETE_INPUT_UNAVAILABLE:"部分接合或支撑需要修复，暂时无法确定自由度。",ANALYSIS_POSE_CHANGED:"当前构型尚无可靠的运动分析结果。",NUMERICAL_NONCONVERGENCE:"尚未找到满足当前要求的结果，请检查参数、支撑和位置。",EXECUTION_FAILURE:"计算未能完成，请重试。",FORBIDDEN:"没有执行此操作的权限。",NOT_FOUND:"所需对象已不存在，请刷新后重新选择。",ASSEMBLY_INTERACTION_UNCONFIRMED:"移动的保存结果尚未确认，可重试确认，或取消恢复。",VALIDATION_FAILED:"当前输入无法完成操作，请检查参数与所选支撑。",VALIDATION_ERROR:"当前输入无法完成操作，请检查参数与所选支撑。",
  };
  const reason=Object.keys(reasons).find(key=>code===key||raw.includes(key));
  const known=[...Object.values(reasons),"尺寸引用需要处理","方向需要三个有限数值，且不能为零向量","中心需要三个有限数值","请选择实体上的平面","请输入长度值、参数别名或表达式","拉伸长度仅支持 mm、cm、m 或 in","拉伸长度必须大于 0","参数别名无效","请输入角度或表达式","角度仅支持 deg 或 rad","角度必须大于 0","圆角半径/倒角长度必须为正有限值，倒角角度必须在 0° 与 180° 之间","数值必须有限","请输入合法数值","请选择上游草图","请先选择实体","当前没有执行此操作的权限。","操作未完成，可重试。","模型已发生变化，请重新预览后确认。","操作未完成，原因尚无法确定。可重试或检查当前输入。"];
  const message=reason?reasons[reason]:known.includes(raw)?raw:retryable?"操作未完成，可重试。":
    ["STALE_VERSION","VERSION_CONFLICT","CAS_CONFLICT","PREVIEW_STALE","PREVIEW_CANDIDATE_STALE_OR_MISMATCHED","MECHANISM_ANALYSIS_STALE"].some(key=>code===key||raw.includes(key))?"模型已发生变化，请重新预览后确认。":"操作未完成，原因尚无法确定。可重试或检查当前输入。";
  return {message,details,code,requestId:value?.requestId??value?.correlationId,diagnosticId:value?.diagnosticId,phase:value?.phase,retryable,severity:retryable?"warning":"error"};
}

export function engineeringProblem(error:unknown):string{return normalizeOperationFailure(error).message;}
