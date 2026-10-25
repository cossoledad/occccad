import {palette} from '../../design/visual-tokens';
export const booleanRoleColors={target:palette.primary,tool:palette.previewReference,retained:palette.previewAdd,excluded:palette.previewRemove};
// Shared roles for exact cell surfaces and command list swatches.
export function booleanRegionStyle(retained:boolean,eligible:boolean,inTarget:boolean){
 return retained?{color:booleanRoleColors.retained,opacity:0.78,role:'retained'}:eligible?{color:booleanRoleColors.excluded,opacity:0.18,role:'excluded'}:inTarget?{color:booleanRoleColors.target,opacity:0.09,role:'target'}:{color:booleanRoleColors.tool,opacity:0.09,role:'tool'};
}
