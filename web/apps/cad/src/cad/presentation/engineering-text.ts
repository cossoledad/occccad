import type {DocumentView,SelectionItem} from '../../types';
import {engineeringInstanceName} from '../assembly/assembly-engineering-state';

// Presentation vocabulary only. Never use these strings as identity or input.
const labels:Record<string,string>={
 READY:'可用',OK:'已满足',CURRENT:'已是最新',UPDATE_AVAILABLE:'有可用更新',UPDATE_BLOCKED:'更新受阻',SURFACE:'曲面',CURVE:'曲线',PARAMETER:'参数',PARAMETER_SET:'参数集',EXTERNAL:'外接触',INTERNAL:'内接触',PART:'零件',PRODUCT:'装配',BODY:'实体',FACE:'面',EDGE:'边',VERTEX:'顶点',POINT:'点',LINE:'直线',CIRCLE:'圆',ARC:'圆弧',ELLIPSE:'椭圆',ELLIPTICAL_ARC:'椭圆弧',SPLINE:'样条',POLYLINE:'折线',PLANE:'平面',AXIS:'轴线',FRAME:'坐标系',CYLINDER:'圆柱面',CONE:'圆锥面',SPHERE:'球面',TORUS:'圆环面',BSPLINE:'样条曲面',BSPLINE_SURFACE:'样条曲面',BSPLINE_CURVE:'样条曲线',BEZIER:'贝塞尔曲面',BEZIER_SURFACE:'贝塞尔曲面',BEZIER_CURVE:'贝塞尔曲线',HYPERBOLA:'双曲线',PARABOLA:'抛物线',SURFACE_OF_EXTRUSION:'拉伸曲面',SURFACE_OF_REVOLUTION:'旋转曲面',CUSTOM:'自定义',DATUM:'基准',
 SKETCH:'草图',PAD:'拉伸',LINEAR_EXTRUDE:'拉伸',REVOLVE:'旋转',LOFT:'放样',HOLE:'孔',SIMPLE:'普通孔',COUNTERBORE:'圆柱沉孔',COUNTERSINK:'锥沉孔',FILLET:'圆角',CHAMFER:'倒角',SHELL:'抽壳',DRAFT:'拔模',BOOLEAN:'布尔运算',PATTERN:'阵列',MIRROR:'镜像',SKETCH_PATTERN:'草图阵列',SOLID_PATTERN:'实体阵列',IMPORT_BODY:'导入实体',IMPORT:'导入实体',
 GROUND:'固定零件',REVOLUTE:'旋转接合',PRISMATIC:'棱形接合',CYLINDRICAL:'圆柱接合',PLANAR:'平面接合',RIGID:'刚性接合',SPHERICAL:'球面接合',FIX:'固定',FIX_TOGETHER:'固联组',COINCIDENT:'重合',CONCENTRIC:'同轴',DISTANCE:'偏移',ANGLE:'角度',CONTACT:'接触',PARALLEL:'平行',PERPENDICULAR:'垂直',TANGENT:'相切',HORIZONTAL:'水平',VERTICAL:'竖直',EQUAL:'相等',MIDPOINT:'中点',SYMMETRY:'对称',MIRROR_RELATION:'镜像关系',
 VERIFIED:'已满足',VALID:'有效',CONNECTED:'有效',RESOLVED:'有效',EXPLICIT_FRAME:'局部坐标支撑',NOT_CONNECTED:'引用已断开',MISSING:'引用已丢失',AMBIGUOUS:'存在多个对应对象',TYPE_MISMATCH:'几何类型不符',OUTSIDE_CURRENT_TIP:'原支撑已不存在',SOURCE_UNAVAILABLE:'来源不可用',UNAVAILABLE:'暂不可用',INVALID:'需要修复',BROKEN:'支撑已失效',FAILED:'计算失败',FAILED_SUPPORT:'支撑已失效',BLOCKED:'受上游失败影响',NOT_UPDATED:'需要重新计算',IMPOSSIBLE:'约束未满足',UNKNOWN:'尚无法确定',NOT_COMPUTED:'尚未分析',AVAILABLE:'分析完成',PARTIAL:'部分结果未知',CONVERGED:'已求解',UNDER_CONSTRAINED:'欠约束',FULLY_CONSTRAINED:'完全约束',OVER_CONSTRAINED:'过约束',INCONSISTENT:'约束冲突',SOLVED:'已求解',CLOSED:'已闭合',OPEN:'未闭合',EMPTY:'尚无轮廓',
 LOCAL:'本地输入',FOLLOW_HEAD:'跟随最新版本',FOLLOW_WORKSPACE_WITH_ACCEPT:'确认后更新',PINNED:'固定版本',DRIVING:'驱动',MEASURED:'测量',ACTIVE:'启用',SUPPRESSED:'停用',NEW_BODY:'新建实体',ADD:'添加材料',REMOVE:'切除材料',INTERSECT:'保留交集',FINITE:'指定长度',TWO_SIDED:'双向',SYMMETRIC:'对称',BLIND:'指定深度',THROUGH_ALL:'贯穿',TO_PLANE:'到平面',SAME:'同向',OPPOSITE:'反向',FREE:'自由角',DIRECTED:'有向角',SPACE:'空间固定',RELATIVE:'相对固定',AXIAL_ROTATION:'旋转',AXIAL_TRANSLATION:'轴向移动',
 SUCCESS:'已完成',SUCCEEDED:'已完成',PENDING:'等待处理',QUEUED:'等待处理',RUNNING:'正在计算',CANCELLED:'已取消',CANCELED:'已取消',CONFLICT:'需要处理冲突',REUSE:'沿用已有关系',OWNER:'所有者',EDITOR:'可编辑',VIEWER:'只读',DRAFT_STATE:'草稿',RELEASED:'已发布',ARCHIVED:'已停用',
};
export function engineeringLabel(value:string|undefined,fallback='待检查'):string{return value?labels[value]??fallback:fallback;}
export function featureName(view:DocumentView,id?:string,fallback='引用特征已丢失'):string{
 const f=view.part?.features.find(f=>f.id===id);return f?.name|| (f?engineeringLabel(f.type,'特征'):fallback);
}
export function objectName(view:DocumentView,id?:string,fallback='引用对象已丢失'):string{
 if(!id)return fallback;
 const instance=view.product?.instances.find(i=>i.id===id);if(instance)return engineeringInstanceName(view,id);
 const f=view.part?.features.find(f=>f.id===id);if(f)return f.name||engineeringLabel(f.type,'特征');
 const object=[...(view.part?.bodies??[]),...(view.part?.datumPlanes??view.datumPlanes??[]),...(view.part?.datumAxes??view.datumAxes??[]),...(view.part?.axisSystems??view.axisSystems??[]),...(view.part?.publications??[]),...(view.product?.publications??[]),...(view.product?.constraints??[]),...(view.product?.kinematics?.mechanisms??[]).flatMap(m=>[m,...m.joints,...m.joints.flatMap(j=>j.constraints??[])])].find(o=>o.id===id);
 return object?.name||fallback;
}
export function selectionName(view:DocumentView,s:SelectionItem):string{
 if(s.kind==='publication')return s.publication?.name??s.productPublication?.name??objectName(view,s.publicationId,'发布接口');
 if(s.kind==='instance')return engineeringInstanceName(view,s.instanceId??s.id);
 if(['face','edge','vertex'].includes(s.kind))return `${objectName(view,s.bodyId,'实体')} / ${engineeringLabel(s.kind.toUpperCase(),'几何元素')}`;
 if(s.kind==='profile-region')return `${featureName(view,s.featureId,'草图')} / 轮廓区域`;
 return objectName(view,s.entityId??s.id,engineeringLabel(s.kind.toUpperCase(),'所选对象'));
}
export function historyAction(type:string):string{
 const actions:Record<string,string>={CREATE_DOCUMENT:'新建文档',CREATE_SKETCH:'新建草图',EDIT_SKETCH:'编辑草图',PAD_SKETCH:'拉伸',CREATE_SOLID_FEATURE:'新建特征',EDIT_FEATURE:'编辑特征',DELETE_FEATURE:'删除特征',DELETE_OBJECTS:'删除对象',SET_FEATURE_SUPPRESSION:'更改特征启用状态',REASSIGN_SKETCH_SUPPORT:'更换草图支撑',MOVE_FEATURE:'调整特征顺序',SAVE_KINEMATICS:'编辑机构',ACTIVATE_MECHANISM:'进入机构',EXIT_MECHANISM:'退出机构',APPLY_MOTION_FRAME:'应用机构位置',INSERT_INSTANCE:'插入零件',MOVE_INSTANCE:'移动零件',ADD_ASSEMBLY_CONSTRAINT:'添加装配约束',EDIT_ASSEMBLY_CONSTRAINT:'编辑装配约束',RECONNECT_ASSEMBLY_CONSTRAINT:'重新指定支撑',LOCALIZE_CONTEXT_INPUT:'解除关联并保留',RESTORE_CONTEXT_INPUT_DEFAULT:'恢复零件默认',DELETE_CONTEXT_BINDING:'恢复零件默认',EDIT_CONTEXT_BINDING:'编辑关联输入',UPDATE_REFERENCES:'更新关联零件',REFRESH_ASSEMBLY_CONSTRAINTS:'重新计算装配',UNDO:'撤销',REDO:'重做',RENAME_OBJECT:'重命名对象',SET_OBJECT_PROPERTIES:'修改属性',SET_PARAMETER:'修改参数',IMPORT_FILE:'导入文件',CREATE_BODY:'新建实体',SET_DEFINITION_VISIBILITY:'更改显示状态',CREATE_DATUM_PLANE:'新建基准面',CREATE_DATUM_AXIS:'新建基准轴'};
 return actions[type]??'修改模型';
}
